package dirwatch

import (
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

//-----------------------------------------------------------------------------

// dirTree will return root itself if it's a file path
func dirTree(root string) <-chan string {
	found := make(chan string)
	go func() {
		defer close(found)
		inf, err := os.Stat(root)
		if err != nil {
			log.Errorf("%+v\n", errors.WithStack(err))
			return
		}
		if !inf.IsDir() {
			found <- root
			return
		}
		err = filepath.Walk(root, func(path string, f os.FileInfo, err error) error {
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
