package config

import (
	"github.com/spf13/viper"
)

// Config holds the minimal configuration required for the MVP.
// Values can be provided via config.yaml or overridden by environment variables.
//
//	SERVER_PORT   -> HTTP listening port (default 8080)
//	READINGS_CSV  -> path to the readings CSV (env var READINGS_CSV, default data/readings.csv)
//	EVENTS_CSV    -> path to the events CSV (env var EVENTS_CSV, default data/events.csv)
//	LLM_API_KEY   -> API key for the LLM provider (env var LLM_API_KEY). Optional:
//	                 when unset/empty the deterministic mock provider is used.
//	LLM_BASE_URL  -> base URL of the Ollama endpoint (env var LLM_BASE_URL,
//	                 default https://ollama.com)
//	LLM_MODEL     -> Ollama model in "model:tag" form (env var LLM_MODEL,
//	                 default gpt-oss:20b)
//
// Environment variables override values from config.yaml.
// The function Load() reads config.yaml (if present), applies defaults, binds environment variables,
// and validates required fields.

type Config struct {
	ServerPort  int
	ReadingsCSV string
	EventsCSV   string
	LLMAPIKey   string
	LLMBaseURL  string
	LLMModel    string
}

func Load() (*Config, error) {
	v := viper.New()
	// Configuration file (optional)
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("../")
	v.SetConfigFile("config.yaml")
	// Load file if it exists; ignore error when missing to allow env‑only config
	if err := v.ReadInConfig(); err != nil {
		// If the error is not a "file not found", propagate it
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, err
		}
	}

	// Defaults (can be overridden by config file or env vars)
	v.SetDefault("server.port", 3001)
	v.SetDefault("data.readings_csv", "data/readings.csv")
	v.SetDefault("data.events_csv", "data/events.csv")
	v.SetDefault("llm.api_key", "")
	v.SetDefault("llm.base_url", "https://ollama.com")
	v.SetDefault("llm.model", "gpt-oss:20b")

	// Bind environment variables (env vars take precedence)
	v.AutomaticEnv()
	v.BindEnv("data.readings_csv", "READINGS_CSV")
	v.BindEnv("data.events_csv", "EVENTS_CSV")
	v.BindEnv("llm.api_key", "LLM_API_KEY")
	v.BindEnv("llm.base_url", "LLM_BASE_URL")
	v.BindEnv("llm.model", "LLM_MODEL")

	var cfg Config
	cfg.ServerPort = v.GetInt("server.port")
	cfg.ReadingsCSV = v.GetString("data.readings_csv")
	cfg.EventsCSV = v.GetString("data.events_csv")
	cfg.LLMAPIKey = v.GetString("llm.api_key")
	cfg.LLMBaseURL = v.GetString("llm.base_url")
	cfg.LLMModel = v.GetString("llm.model")

	// Ensure defaults for CSV paths if empty after env loading
	if cfg.ReadingsCSV == "" {
		cfg.ReadingsCSV = "data/readings.csv"
	}
	if cfg.EventsCSV == "" {
		cfg.EventsCSV = "data/events.csv"
	}
	// No other validation; errors are only returned if Viper fails to read the config file

	return &cfg, nil
}
