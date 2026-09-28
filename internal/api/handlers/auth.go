package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// Demo login flow (POST /api/auth/login).
//
// This project has no database by design, so the credential store for the demo
// login is the committed CSV data/users.csv (one row, with the password stored
// as a SHA-256 hex digest). The endpoint's only job is to verify those
// credentials and ISSUE an HS256 JWT with the standard library.
//
// Scope decision (deliberate): NO route validates the issued token. The backend
// only hands out the token; keeping the frontend guard is a UX affordance, not a
// security boundary. See docs/architecture.md §9.
const (
	// defaultUsersCSV is the committed credential store, resolved relative to
	// the process working directory (the repository root in development).
	defaultUsersCSV = "data/users.csv"
	// envUsersCSV optionally overrides the credential store path.
	envUsersCSV = "USERS_CSV"
	// envJWTSecret carries the HS256 signing secret.
	envJWTSecret = "JWT_SECRET"
	// defaultJWTSecret is the documented development fallback used when
	// JWT_SECRET is unset, so the demo runs with no configuration at all. It is
	// deliberately a public, non-secret value: a real deployment must set
	// JWT_SECRET. The secret is never logged and never echoed in a response.
	defaultJWTSecret = "bia-demo-only-jwt-secret-do-not-use-in-production"
	// jwtTTL is the lifetime of an issued token: 8 hours.
	jwtTTL = 8 * time.Hour
	// missingCredentialsMessage is the user-visible body of a 400: it is exactly
	// the same for a missing username and a missing password.
	missingCredentialsMessage = "usuario y contraseña son obligatorios"
)

// loginRequest is the POST /api/auth/login request body.
type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// loginUser is the user object of a successful login.
type loginUser struct {
	Username   string `json:"username"`
	Name       string `json:"name"`
	Authorized bool   `json:"authorized"`
}

// loginResponse is the 200 body. It is a struct, not a map, so the JSON key
// order is the documented one: token, expires_at, user.
type loginResponse struct {
	Token     string    `json:"token"`
	ExpiresAt string    `json:"expires_at"`
	User      loginUser `json:"user"`
}

// demoUser is one row of the credential store.
type demoUser struct {
	username       string
	passwordSHA256 string
	name           string
	authorized     bool
}

// userColumns are the headers the credential CSV must carry.
var userColumns = []string{"username", "password_sha256", "name", "authorized"}

// usersCSVPath resolves the credential store path, preferring USERS_CSV.
func usersCSVPath() string {
	if path := os.Getenv(envUsersCSV); path != "" {
		return path
	}
	return defaultUsersCSV
}

// jwtSecret resolves the HS256 signing secret, preferring JWT_SECRET. The
// resolved value is never logged, never returned and never named in an error.
func jwtSecret() []byte {
	if secret := os.Getenv(envJWTSecret); secret != "" {
		return []byte(secret)
	}
	return []byte(defaultJWTSecret)
}

// loadDemoUsers reads the credential CSV, skipping its header row.
//
// The file is read on every request on purpose: it holds a single row, so the
// read is negligible, and it keeps the demo credential editable without a
// restart. A missing or malformed store is a server-side fault, so it is
// reported as a descriptive error instead of a credential failure.
func loadDemoUsers(path string) ([]demoUser, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("no se pudo abrir el almacén de credenciales %s: %w", path, err)
	}
	defer file.Close()

	records, err := csv.NewReader(file).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("almacén de credenciales malformado %s: %w", path, err)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("almacén de credenciales vacío %s", path)
	}
	// The first record is the header. Columns are mapped by name so the reader
	// does not silently depend on their order.
	index, err := columnIndex(records[0])
	if err != nil {
		return nil, fmt.Errorf("encabezado inválido en %s: %w", path, err)
	}

	users := make([]demoUser, 0, len(records)-1)
	for rowNumber, record := range records[1:] {
		// Tolerate a trailing blank line.
		if len(record) == 1 && strings.TrimSpace(record[0]) == "" {
			continue
		}
		if len(record) < len(records[0]) {
			return nil, fmt.Errorf("fila %d de %s: faltan columnas", rowNumber+2, path)
		}
		authorized, err := strconv.ParseBool(strings.TrimSpace(record[index["authorized"]]))
		if err != nil {
			return nil, fmt.Errorf("fila %d de %s: authorized debe ser true o false", rowNumber+2, path)
		}
		users = append(users, demoUser{
			username:       strings.TrimSpace(record[index["username"]]),
			passwordSHA256: strings.ToLower(strings.TrimSpace(record[index["password_sha256"]])),
			name:           strings.TrimSpace(record[index["name"]]),
			authorized:     authorized,
		})
	}
	if len(users) == 0 {
		return nil, fmt.Errorf("almacén de credenciales sin usuarios %s", path)
	}
	return users, nil
}

// columnIndex maps the lowercased header names to their column position and
// fails when a required column is missing.
func columnIndex(header []string) (map[string]int, error) {
	index := make(map[string]int, len(header))
	for i, name := range header {
		index[strings.TrimSpace(strings.ToLower(name))] = i
	}
	for _, required := range userColumns {
		if _, ok := index[required]; !ok {
			return nil, fmt.Errorf("falta la columna %q", required)
		}
	}
	return index, nil
}

// sha256Hex is the lowercase hex SHA-256 digest of value, the exact format the
// credential CSV stores.
func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// jwtHeader is the fixed compact-JWS header.
type jwtHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

// jwtClaims is the token payload. The time claims are Unix seconds.
type jwtClaims struct {
	Subject    string `json:"sub"`
	Name       string `json:"name"`
	Authorized bool   `json:"authorized"`
	IssuedAt   int64  `json:"iat"`
	ExpiresAt  int64  `json:"exp"`
}

// signJWT builds a compact JWS over HS256 with the standard library only:
//
//	base64url(header) "." base64url(claims) "." base64url(HMAC-SHA256(input))
//
// using the raw (unpadded) URL-safe base64 alphabet.
func signJWT(claims jwtClaims, secret []byte) (string, error) {
	headerJSON, err := json.Marshal(jwtHeader{Alg: "HS256", Typ: "JWT"})
	if err != nil {
		return "", err
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	encoding := base64.RawURLEncoding
	signingInput := encoding.EncodeToString(headerJSON) + "." + encoding.EncodeToString(claimsJSON)
	mac := hmac.New(sha256.New, secret)
	if _, err := mac.Write([]byte(signingInput)); err != nil {
		return "", err
	}
	return signingInput + "." + encoding.EncodeToString(mac.Sum(nil)), nil
}

// AuthLogin implements POST /api/auth/login for the demo login flow.
//
// A successful login is 200 with a fresh HS256 JWT. Bad credentials are always
// 401 with one single body, an unauthorized user is 403, a missing field is 400
// and any other method is 405.
func AuthLogin(w http.ResponseWriter, r *http.Request) {
	// The mux registers route patterns and never enforces a method, so the
	// handler does it. OPTIONS never reaches this point: corsWrapper answers it.
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "método no permitido")
		return
	}

	var request loginRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		// A body that is not valid JSON cannot carry both fields either.
		writeJSONError(w, http.StatusBadRequest, missingCredentialsMessage)
		return
	}
	// The password is never trimmed: only the username and the CSV's own fields
	// tolerate surrounding whitespace.
	username := strings.TrimSpace(request.Username)
	if username == "" || request.Password == "" {
		writeJSONError(w, http.StatusBadRequest, missingCredentialsMessage)
		return
	}

	users, err := loadDemoUsers(usersCSVPath())
	if err != nil {
		// A broken credential store is a server-side fault, never a 401.
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// The supplied password is hashed BEFORE the lookup, so an unknown username
	// and a wrong password cost the same and the endpoint does not enumerate
	// users by body or by timing.
	digest := sha256Hex(request.Password)
	var user *demoUser
	for i := range users {
		if users[i].username == username {
			user = &users[i]
			break
		}
	}
	// Constant-time comparison of the two hex digests.
	if user == nil || subtle.ConstantTimeCompare([]byte(user.passwordSHA256), []byte(digest)) != 1 {
		// One single body for "unknown user" and "wrong password": the response
		// must not reveal which of the two failed.
		writeJSONError(w, http.StatusUnauthorized, "usuario o contraseña incorrectos")
		return
	}
	if !user.authorized {
		writeJSONError(w, http.StatusForbidden, "el usuario no está autorizado")
		return
	}

	issuedAt := time.Now().UTC()
	expiresAt := issuedAt.Add(jwtTTL)
	token, err := signJWT(jwtClaims{
		Subject:    user.username,
		Name:       user.name,
		Authorized: user.authorized,
		IssuedAt:   issuedAt.Unix(),
		ExpiresAt:  expiresAt.Unix(),
	}, jwtSecret())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "no se pudo emitir el token")
		return
	}

	writeJSON(w, http.StatusOK, loginResponse{
		Token:     token,
		ExpiresAt: expiresAt.Format(time.RFC3339),
		User: loginUser{
			Username:   user.username,
			Name:       user.name,
			Authorized: user.authorized,
		},
	})
}
