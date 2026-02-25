package logger

import (
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Init configures the global zerolog logger.
// level: trace|debug|info|warn|error|fatal|panic|disabled (default "info")
// format: "console" for human-readable, anything else for JSON (default)
func Init(level, format string) {
	zerolog.TimeFieldFormat = time.RFC3339

	lvl, err := zerolog.ParseLevel(strings.ToLower(strings.TrimSpace(level)))
	if err != nil || level == "" {
		lvl = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(lvl)

	if strings.ToLower(strings.TrimSpace(format)) == "console" {
		log.Logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}).
			With().Timestamp().Logger()
	} else {
		log.Logger = zerolog.New(os.Stderr).With().Timestamp().Logger()
	}
}
