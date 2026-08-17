package logger

import (
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
)

// Logger wraps zerolog with simplified methods
type Logger struct {
	zl zerolog.Logger
}

// New creates a new Logger from a zerolog.Logger
func New(zl zerolog.Logger) *Logger {
	return &Logger{zl: zl}
}

// NewDefault creates a new Logger with default console output
func NewDefault() *Logger {
	return New(zerolog.New(os.Stdout).With().Timestamp().Logger())
}

// NewJSON creates a new Logger with JSON output at the specified level
func NewJSON(level string) *Logger {
	lvl, err := zerolog.ParseLevel(level)
	if err != nil {
		lvl = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(lvl)

	return New(zerolog.New(os.Stdout).With().Timestamp().Logger())
}

// NewConsole creates a new Logger with console output at the specified level
func NewConsole(level string, pretty bool) *Logger {
	lvl, err := zerolog.ParseLevel(level)
	if err != nil {
		lvl = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(lvl)

	var writer io.Writer = os.Stdout
	if pretty {
		writer = zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		}
	}

	return New(zerolog.New(writer).With().Timestamp().Logger())
}

// NewNoop creates a new Logger that discards all output (for tests)
func NewNoop() *Logger {
	return New(zerolog.New(io.Discard))
}
