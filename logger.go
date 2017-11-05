package dirwatch

import (
	"fmt"
)

//-----------------------------------------------------------------------------

// Logger generic logger
type Logger interface {
	Error(args ...interface{})
	Info(args ...interface{})
	Errorf(format string, args ...interface{})
	Infof(format string, args ...interface{})
}

// LoggerFunc wraps a func(args ...interface{}) in a Logger interface
type LoggerFunc func(args ...interface{})

// Error implements Logger interface
func (lf LoggerFunc) Error(args ...interface{}) { lf(args...) }

// Info implements Logger interface
func (lf LoggerFunc) Info(args ...interface{}) { lf(args...) }

// Errorf implements Logger interface
func (lf LoggerFunc) Errorf(format string, args ...interface{}) { lf(fmt.Sprintf(format, args...)) }

// Infof implements Logger interface
func (lf LoggerFunc) Infof(format string, args ...interface{}) { lf(fmt.Sprintf(format, args...)) }

//-----------------------------------------------------------------------------

// SetLogger replaces logger for this package.
// Must be called before using anything else from this package.
// Can be called only once and the consecutive calls have no effect.
func SetLogger(logger Logger) {
	setLoggerOnce.Do(func() { log = logger })
}

//-----------------------------------------------------------------------------
