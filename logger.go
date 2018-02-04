package dirwatch

import (
	"fmt"
	internalog "log"
	"sync"
)

//-----------------------------------------------------------------------------

func lerrorf(format string, args ...interface{}) {
	_errlog(fmt.Sprintf(format, args...))
}

func lerror(args ...interface{}) {
	_errlog(args...)
}

//-----------------------------------------------------------------------------

var _errlog func(args ...interface{})

func init() {
	internalog.SetPrefix("dirwatch: ")
	internalog.SetFlags(internalog.Ltime | internalog.Lshortfile)
	_errlog = internalog.Println
}

//-----------------------------------------------------------------------------

var setLoggerOnce sync.Once

// SetLogger replaces logger for this package.
// Must be called before using anything else from this package.
// Can be called only once and the consecutive calls have no effect.
func SetLogger(errlog func(args ...interface{})) {
	setLoggerOnce.Do(func() {
		if errlog == nil {
			panic("logger can not be nil")
		}
		_errlog = errlog
	})
}

//-----------------------------------------------------------------------------
