package logging

import (
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

// New returns a zerolog.Logger configured for CLI tools.
func New(level string) zerolog.Logger {
	zerolog.TimeFieldFormat = time.RFC3339Nano
	logLevel := parseLevel(level)
	output := zerolog.SyncWriter(os.Stdout)
	logger := zerolog.New(output).Level(logLevel).With().Timestamp().Logger()
	return logger
}

func parseLevel(value string) zerolog.Level {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return zerolog.DebugLevel
	case "warn":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	default:
		return zerolog.InfoLevel
	}
}
