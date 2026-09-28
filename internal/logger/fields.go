package logger

import (
	"time"

	"github.com/rs/zerolog"
)

// Field represents a structured log field
type Field interface {
	apply(*zerolog.Event)
}

type stringField struct {
	key string
	val string
}

func (f stringField) apply(e *zerolog.Event) {
	e.Str(f.key, f.val)
}

// Str creates a string field
func Str(key, val string) Field {
	return stringField{key: key, val: val}
}

type intField struct {
	key string
	val int
}

func (f intField) apply(e *zerolog.Event) {
	e.Int(f.key, f.val)
}

// Int creates an int field
func Int(key string, val int) Field {
	return intField{key: key, val: val}
}

type uintField struct {
	key string
	val uint
}

func (f uintField) apply(e *zerolog.Event) {
	e.Uint(f.key, f.val)
}

// Uint creates a uint field
func Uint(key string, val uint) Field {
	return uintField{key: key, val: val}
}

type boolField struct {
	key string
	val bool
}

func (f boolField) apply(e *zerolog.Event) {
	e.Bool(f.key, f.val)
}

// Bool creates a bool field
func Bool(key string, val bool) Field {
	return boolField{key: key, val: val}
}

type errorField struct {
	err error
}

func (f errorField) apply(e *zerolog.Event) {
	e.Err(f.err)
}

// Err creates an error field
func Err(err error) Field {
	return errorField{err: err}
}

type timeField struct {
	key string
	val time.Time
}

func (f timeField) apply(e *zerolog.Event) {
	e.Time(f.key, f.val)
}

// Time creates a time field
func Time(key string, val time.Time) Field {
	return timeField{key: key, val: val}
}

type durField struct {
	key string
	val time.Duration
}

func (f durField) apply(e *zerolog.Event) {
	e.Dur(f.key, f.val)
}

// Dur creates a duration field
func Dur(key string, val time.Duration) Field {
	return durField{key: key, val: val}
}

type stringsField struct {
	key string
	val []string
}

func (f stringsField) apply(e *zerolog.Event) {
	e.Strs(f.key, f.val)
}

// Strs creates a string slice field
func Strs(key string, val []string) Field {
	return stringsField{key: key, val: val}
}

type float64Field struct {
	key string
	val float64
}

func (f float64Field) apply(e *zerolog.Event) {
	e.Float64(f.key, f.val)
}

// Float64 creates a float64 field
func Float64(key string, val float64) Field {
	return float64Field{key: key, val: val}
}

type int64Field struct {
	key string
	val int64
}

func (f int64Field) apply(e *zerolog.Event) {
	e.Int64(f.key, f.val)
}

// Int64 creates an int64 field
func Int64(key string, val int64) Field {
	return int64Field{key: key, val: val}
}

type anyField struct {
	key string
	val interface{}
}

func (f anyField) apply(e *zerolog.Event) {
	e.Interface(f.key, f.val)
}

// Any creates an interface{} field for arbitrary values
func Any(key string, val interface{}) Field {
	return anyField{key: key, val: val}
}
