package logger

// Trace logs a trace level message
func (l *Logger) Trace(msg string, fields ...Field) {
	e := l.zl.Trace()
	for _, f := range fields {
		f.apply(e)
	}
	e.Msg(msg)
}

// Debug logs a debug level message
func (l *Logger) Debug(msg string, fields ...Field) {
	e := l.zl.Debug()
	for _, f := range fields {
		f.apply(e)
	}
	e.Msg(msg)
}

// Info logs an info level message
func (l *Logger) Info(msg string, fields ...Field) {
	e := l.zl.Info()
	for _, f := range fields {
		f.apply(e)
	}
	e.Msg(msg)
}

// Warn logs a warning level message
func (l *Logger) Warn(msg string, fields ...Field) {
	e := l.zl.Warn()
	for _, f := range fields {
		f.apply(e)
	}
	e.Msg(msg)
}

// Error logs an error level message
func (l *Logger) Error(msg string, fields ...Field) {
	e := l.zl.Error()
	for _, f := range fields {
		f.apply(e)
	}
	e.Msg(msg)
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
