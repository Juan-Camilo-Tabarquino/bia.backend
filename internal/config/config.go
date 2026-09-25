package config

import (	"github.com/spf13/viper"
)

// Config holds the minimal configuration required for the MVP.
// Values can be provided via config.yaml or overridden by environment variables.
//
//   SERVER_PORT   -> HTTP listening port (default 8080)
//   READINGS_CSV  -> path to the readings CSV (env var READINGS_CSV, default assets/readings.csv)
//   EVENTS_CSV    -> path to the events CSV (env var EVENTS_CSV, default assets/events.csv)
//   LLM_API_KEY   -> API key for the LLM provider (env var LLM_API_KEY). Optional:
//                    when unset/empty the deterministic mock provider is used.
//
// Environment variables override values from config.yaml.
// The function Load() reads config.yaml (if present), applies defaults, binds environment variables,
// and validates required fields.

type Config struct {
    ServerPort   int
    ReadingsCSV  string
    EventsCSV    string
    LLMAPIKey    string
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
    v.SetDefault("data.readings_csv", "assets/readings.csv")
    v.SetDefault("data.events_csv", "assets/events.csv")
    v.SetDefault("llm.api_key", "")

    // Bind environment variables (env vars take precedence)
    v.AutomaticEnv()
    v.BindEnv("data.readings_csv", "READINGS_CSV")
    v.BindEnv("data.events_csv", "EVENTS_CSV")
    v.BindEnv("llm.api_key", "LLM_API_KEY")

    var cfg Config
    cfg.ServerPort = v.GetInt("server.port")
    cfg.ReadingsCSV = v.GetString("data.readings_csv")
    cfg.EventsCSV = v.GetString("data.events_csv")
    cfg.LLMAPIKey = v.GetString("llm.api_key")

    // Ensure defaults for CSV paths if empty after env loading
    if cfg.ReadingsCSV == "" {
        cfg.ReadingsCSV = "assets/readings.csv"
    }
    if cfg.EventsCSV == "" {
        cfg.EventsCSV = "assets/events.csv"
    }
    // No other validation; errors are only returned if Viper fails to read the config file

    return &cfg, nil
}
