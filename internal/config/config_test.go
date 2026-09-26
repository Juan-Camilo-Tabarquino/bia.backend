package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_NoConfigFileAndMissingEnv(t *testing.T) {
	// Create a temporary empty directory and switch to it.
	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current working directory: %v", err)
	}
	defer func() {
		_ = os.Chdir(origDir)
	}()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory to temp dir: %v", err)
	}

	// Ensure the required environment variable is unset.
	_ = os.Unsetenv("LLM_API_KEY")

	if _, err := Load(); err == nil {
		t.Fatalf("expected error when config file is absent and LLM_API_KEY is not set, got nil")
	}
}

// chdirToTempDir switches the process working directory to a fresh temporary
// directory and restores the original one on cleanup. Load() resolves the
// relative "config.yaml" against the working directory, so any test that
// exercises file-based configuration must control it explicitly.
//
// The temporary directory is created before the restore cleanup is registered,
// so the LIFO cleanup order restores the working directory before that
// directory is removed (removing the current directory fails on Windows).
func chdirToTempDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current working directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(origDir); err != nil {
			t.Errorf("failed to restore working directory: %v", err)
		}
	})

	if err := os.Chdir(dir); err != nil {
		t.Fatalf("failed to change directory to temp dir: %v", err)
	}
	return dir
}

// writeConfigYAML writes a config.yaml file into dir.
func writeConfigYAML(t *testing.T, dir, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(contents), 0o600); err != nil {
		t.Fatalf("failed to write config.yaml: %v", err)
	}
}

// TestLoad_ReadsCSVPathsFromConfigFile proves that the values written to
// config.yaml are actually honoured by Load and are not shadowed by the
// hardcoded defaults.
func TestLoad_ReadsCSVPathsFromConfigFile(t *testing.T) {
	dir := chdirToTempDir(t)
	writeConfigYAML(t, dir, `server:
  port: 3001
data:
  readings_csv: "yaml-readings-distinctive.csv"
  events_csv: "yaml-events-distinctive.csv"
`)
	// Neutralise any ambient override so the YAML value is the only source.
	t.Setenv("READINGS_CSV", "")
	t.Setenv("EVENTS_CSV", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}
	if got, want := cfg.ReadingsCSV, "yaml-readings-distinctive.csv"; got != want {
		t.Errorf("ReadingsCSV = %q, want %q (YAML value was ignored)", got, want)
	}
	if got, want := cfg.EventsCSV, "yaml-events-distinctive.csv"; got != want {
		t.Errorf("EventsCSV = %q, want %q (YAML value was ignored)", got, want)
	}
}

// TestLoad_EnvOverridesCSVPathsFromConfigFile proves that READINGS_CSV and
// EVENTS_CSV still take precedence over the values in config.yaml.
func TestLoad_EnvOverridesCSVPathsFromConfigFile(t *testing.T) {
	dir := chdirToTempDir(t)
	writeConfigYAML(t, dir, `server:
  port: 3001
data:
  readings_csv: "yaml-readings-distinctive.csv"
  events_csv: "yaml-events-distinctive.csv"
`)
	t.Setenv("READINGS_CSV", "env-readings-override.csv")
	t.Setenv("EVENTS_CSV", "env-events-override.csv")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}
	if got, want := cfg.ReadingsCSV, "env-readings-override.csv"; got != want {
		t.Errorf("ReadingsCSV = %q, want %q (READINGS_CSV did not override YAML)", got, want)
	}
	if got, want := cfg.EventsCSV, "env-events-override.csv"; got != want {
		t.Errorf("EventsCSV = %q, want %q (EVENTS_CSV did not override YAML)", got, want)
	}
}

// TestLoad_DefaultsWhenNoYAMLAndNoEnv proves the fallback defaults are applied
// when neither config.yaml nor the environment supplies a CSV path.
//
// Load() cannot succeed without a config.yaml on disk (it uses
// SetConfigFile("config.yaml"), so a missing file surfaces as a path error),
// so this test writes a config file that exists but intentionally omits the
// data keys.
func TestLoad_DefaultsWhenNoYAMLAndNoEnv(t *testing.T) {
	dir := chdirToTempDir(t)
	writeConfigYAML(t, dir, "server:\n  port: 3001\n")
	// Empty values make Viper treat the env vars as unset (AllowEmptyEnv is
	// false by default), isolating the defaults from ambient READINGS_CSV /
	// EVENTS_CSV values.
	t.Setenv("READINGS_CSV", "")
	t.Setenv("EVENTS_CSV", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}
	if got, want := cfg.ReadingsCSV, "data/readings.csv"; got != want {
		t.Errorf("ReadingsCSV = %q, want default %q", got, want)
	}
	if got, want := cfg.EventsCSV, "data/events.csv"; got != want {
		t.Errorf("EventsCSV = %q, want default %q", got, want)
	}
}
