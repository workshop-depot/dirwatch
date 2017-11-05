package dirwatch

import (
	"os"
	"sync"
)

//-----------------------------------------------------------------------------

var (
	log           Logger
	setLoggerOnce sync.Once

	sep = string([]rune{os.PathSeparator})
)

//-----------------------------------------------------------------------------
