package dirwatch

import (
	internalog "log"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

//-----------------------------------------------------------------------------

func init() {
	if log == nil {
		log = LoggerFunc(internalog.Print)
	}
}

//-----------------------------------------------------------------------------

func dirTree(root string) <-chan string {
	found := make(chan string)
	go func() {
		defer close(found)
		err := filepath.Walk(root, func(path string, f os.FileInfo, err error) error {
			if !f.IsDir() {
				return nil
			}
			found <- path
			return nil
		})
		if err != nil {
			log.Errorf("%+v\n", errors.WithStack(err))
		}
	}()
	return found
}

//-----------------------------------------------------------------------------
