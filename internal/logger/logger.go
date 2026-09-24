package logger

import (
    "os"
    "github.com/rs/zerolog"
    )

// Log is the package-level zerolog.Logger configured for the application.
// It writes JSON log entries directly to stdout with timestamps.
var Log zerolog.Logger

func init() {
    // Use zerolog with direct stdout writer.
    // Set global log level to info; callers can adjust via zerolog.SetGlobalLevel.
    zerolog.SetGlobalLevel(zerolog.InfoLevel)
    Log = zerolog.New(os.Stdout).With().Timestamp().Logger()
    }

// Get returns the configured logger instance.
func Get() zerolog.Logger { return Log }
