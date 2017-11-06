package dirwatch

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/stretchr/testify/assert"
)

var rootDirectory string
var mainWatch *Watch
var events = make(chan fsnotify.Event, 100)

func notify(ev fsnotify.Event) {
	events <- ev
}

func TestMain(m *testing.M) {
	var err error
	rootDirectory, err = ioutil.TempDir("", "dirwatch-")
	if err != nil {
		os.Exit(1)
	}
	os.RemoveAll(rootDirectory)
	// needs error checking
	os.Mkdir(rootDirectory, 0777)

	<-time.After(time.Millisecond * 100)

	// needs error checking
	mainWatch, _ = New(rootDirectory, notify)

	os.Exit(m.Run())
}

func Test01(t *testing.T) {
	assert := assert.New(t)

	SetLogger(LoggerFunc(t.Log))

	dir1 := filepath.Join(rootDirectory, "lab1")
	os.Remove(dir1)

	err := os.Mkdir(dir1, 0777)
	if !assert.NoError(err) {
		return
	}

	<-time.After(time.Millisecond * 100)

	testFile := filepath.Join(dir1, "testfile")
	f, err := os.Create(testFile)
	if !assert.NoError(err) {
		return
	}
	defer f.Close()

	fileCount := 0
	for i := 0; i < 2; i++ {
		select {
		case ev := <-events:
			assert.Contains(ev.Name, "dirwatch-")
			assert.Contains(ev.Name, "lab1")
			if strings.Contains(ev.Name, "testfile") {
				fileCount++
			}
			assert.Equal(ev.Op, fsnotify.Create)
		case <-time.After(time.Second * 3):
			assert.Fail("noevents")
		}
	}
	assert.NotEqual(0, fileCount)
}

func ExampleNew() {
	// prepare sample home directory to watch over
	rootDirectory, err := ioutil.TempDir("", "dirwatch-example")
	if err != nil {
		panic(err)
	}
	os.RemoveAll(rootDirectory)
	os.Mkdir(rootDirectory, 0777)

	// our notification callback (I feel it's simpler to
	// have a callback instead of passing a channel in an API)
	var events = make(chan fsnotify.Event, 100)
	notify := func(ev fsnotify.Event) {
		events <- ev
	}

	// create the watcher
	watcher, _ := New(rootDirectory, notify)
	defer watcher.Stop()

	// creating a directory inside the root/home
	dir2 := filepath.Join(rootDirectory, "lab2")
	os.Remove(dir2)
	err = os.Mkdir(dir2, 0777)
	if err != nil {
		panic(err)
	}

	<-time.After(time.Millisecond * 100)

	select {
	case ev := <-events:
		if strings.Contains(ev.Name, "dirwatch-example") &&
			strings.Contains(ev.Name, "lab2") && ev.Op == fsnotify.Create {
			fmt.Println("OK")
		}
	case <-time.After(time.Second * 3):
	}

	// Output:
	// OK
}
