package logger

import (
	"time"

	"github.com/rs/zerolog"
)

// With returns a logger with additional context fields
func (l *Logger) With() *Context {
	return &Context{event: l.zl.With()}
}

// WithComponent returns a logger with a component field
func (l *Logger) WithComponent(component string) *Logger {
	return &Logger{zl: l.zl.With().Str("component", component).Logger(), pinned: l.pinned, pin: l.pin}
}

// Context allows building a logger with context fields
type Context struct {
	event zerolog.Context
}

// Logger returns the final logger with all context fields
func (c *Context) Logger() *Logger {
	return &Logger{zl: c.event.Logger()}
}

// Str adds a string field to the context
func (c *Context) Str(key, val string) *Context {
	c.event = c.event.Str(key, val)
	return c
}

// Int adds an int field to the context
func (c *Context) Int(key string, val int) *Context {
	c.event = c.event.Int(key, val)
	return c
}

// Bool adds a bool field to the context
func (c *Context) Bool(key string, val bool) *Context {
	c.event = c.event.Bool(key, val)
	return c
}

// Dur adds a duration field to the context
func (c *Context) Dur(key string, val time.Duration) *Context {
	c.event = c.event.Dur(key, val)
	return c
}
