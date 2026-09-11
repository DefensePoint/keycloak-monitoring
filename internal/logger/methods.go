package logger

import "github.com/rs/zerolog"

func (l *Logger) event(lvl zerolog.Level) *zerolog.Event {
	if l.pinned && lvl >= l.pin && lvl < zerolog.GlobalLevel() {
		return l.zl.Log().Str(zerolog.LevelFieldName, lvl.String())
	}
	return l.zl.WithLevel(lvl)
}

func (l *Logger) emit(lvl zerolog.Level, msg string, fields []Field) {
	e := l.event(lvl)
	for _, f := range fields {
		f.apply(e)
	}
	e.Msg(msg)
}

// Trace logs a trace level message
func (l *Logger) Trace(msg string, fields ...Field) {
	l.emit(zerolog.TraceLevel, msg, fields)
}

// Debug logs a debug level message
func (l *Logger) Debug(msg string, fields ...Field) {
	l.emit(zerolog.DebugLevel, msg, fields)
}

// Info logs an info level message
func (l *Logger) Info(msg string, fields ...Field) {
	l.emit(zerolog.InfoLevel, msg, fields)
}

// Warn logs a warning level message
func (l *Logger) Warn(msg string, fields ...Field) {
	l.emit(zerolog.WarnLevel, msg, fields)
}

// Error logs an error level message
func (l *Logger) Error(msg string, fields ...Field) {
	l.emit(zerolog.ErrorLevel, msg, fields)
}

// Fatal logs a fatal level message and exits
func (l *Logger) Fatal(msg string, fields ...Field) {
	e := l.zl.Fatal()
	for _, f := range fields {
		f.apply(e)
	}
	e.Msg(msg)
}

// Panic logs a panic level message and panics
func (l *Logger) Panic(msg string, fields ...Field) {
	e := l.zl.Panic()
	for _, f := range fields {
		f.apply(e)
	}
	e.Msg(msg)
}
