package logger

import (
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
)

// Logger wraps zerolog with simplified methods
type Logger struct {
	zl     zerolog.Logger
	pinned bool
	pin    zerolog.Level
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

// PinLevel returns a logger that keeps emitting events at or above the given
// level even when the global zerolog level is stricter. Zerolog enforces the
// global level on every leveled event of every logger, so pinned events the
// global level would drop are emitted through zerolog's level-less path with
// the level field stamped by hand; only a global level of Disabled still
// silences them. Fatal and Panic are never rerouted.
func (l *Logger) PinLevel(level string) *Logger {
	lvl, err := zerolog.ParseLevel(level)
	if err != nil {
		lvl = zerolog.InfoLevel
	}
	return &Logger{zl: l.zl, pinned: true, pin: lvl}
}

// NewNoop creates a new Logger that discards all output (for tests)
func NewNoop() *Logger {
	return New(zerolog.New(io.Discard))
}
