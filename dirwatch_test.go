package dirwatch

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/stretchr/testify/assert"
)

var rootDirectory string
var mainWatch *Watcher
var events = make(chan Event, 1000)

func notify(ev Event) {
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
	mainWatch = New(Notify(notify), Paths(rootDirectory))

	os.Exit(m.Run())
}

func TestCreateDir(t *testing.T) {
	assert := assert.New(t)

	_errlog = t.Log

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

func TestCreateDirFile(t *testing.T) {
	assert := assert.New(t)

	var wg sync.WaitGroup
	_errlog = t.Log

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 1; i <= 100; i++ {
			name := fmt.Sprintf("%06d-dir", i)
			d := filepath.Join(rootDirectory, name)
			os.Remove(d)
			assert.NoError(os.Mkdir(d, 0777))
		}

		<-time.After(time.Millisecond * 150)

		for i := 1; i <= 100; i++ {
			name := fmt.Sprintf("%06d-dir", i)
			d := filepath.Join(rootDirectory, name)

			fp := fmt.Sprintf("%06d-file", i)
			fp = filepath.Join(d, fp)
			f, err := os.Create(fp)
			assert.NoError(err)
			assert.NoError(f.Close())
		}
	}()

	wg.Add(1)
	go func() {
		dirs := 0
		files := 0
		defer wg.Done()
		for i := 1; i <= 200; i++ {
			select {
			case ev := <-events:
				if strings.Contains(ev.Name, "-dir") {
					dirs++
				}
				if strings.Contains(ev.Name, "-file") {
					files++
				}
				assert.Equal(ev.Op, fsnotify.Create)
			case <-time.After(time.Second * 3):
				assert.Fail("noevents")
				return
			}
		}
		assert.Equal(200, dirs)
		assert.Equal(100, files)
	}()

	wg.Wait()
}

func TestAddWatchFile(t *testing.T) {
	assert := assert.New(t)
	var wg sync.WaitGroup
	_errlog = t.Log

	fp := filepath.Join(os.TempDir(), fmt.Sprintf("test03-%v", time.Now().UnixNano()))
	f, err := os.Create(fp)
	assert.NoError(err)
	assert.NoError(f.Close())

	<-time.After(time.Millisecond * 100)
	mainWatch.AddSingle(fp)
	<-time.After(time.Millisecond * 150)

	// test this without -race (?)
	// _, ok := mainWatch.paths[fp]
	// assert.True(ok)

	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case ev := <-events:
			assert.Contains(ev.Name, "test03")
			assert.Equal(ev.Op, fsnotify.Write)
		case <-time.After(time.Second * 3):
			assert.Fail("noevents")
			return
		}
	}()

	assert.NoError(ioutil.WriteFile(fp, []byte("DATA"), 0777))

	wg.Wait()
}

func ExampleNew() {
	// prepare sample home directory to watch over
	rootDirectory, err := ioutil.TempDir(os.TempDir(), "dirwatch-example")
	if err != nil {
		panic(err)
	}
	os.RemoveAll(rootDirectory)
	os.Mkdir(rootDirectory, 0777)

	// our notification callback (I feel it's simpler to
	// have a callback instead of passing a channel in an API)
	var events = make(chan Event, 100)
	notify := func(ev Event) {
		events <- ev
	}

	// create the watcher
	watcher := New(Notify(notify), Paths(rootDirectory))
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

func ExampleExclude() {
	// prepare sample home directory to watch over
	// rootDirectory, err := ioutil.TempDir(os.TempDir(), "dirwatch-example-")
	rootDirectory := filepath.Join(os.TempDir(), "dirwatch-example-exclude")
	os.RemoveAll(rootDirectory)
	os.Mkdir(rootDirectory, 0777)

	os.MkdirAll(filepath.Join(rootDirectory, "node_modules"), 0777)
	os.MkdirAll(filepath.Join(rootDirectory, "lab1"), 0777)
	os.MkdirAll(filepath.Join(rootDirectory, "lab2"), 0777)

	// our notification callback (I feel it's simpler to
	// have a callback instead of passing a channel in an API)
	var events = make(chan Event, 100)
	notify := func(ev Event) {
		events <- ev
	}

	// create the watcher
	watcher := New(Notify(notify), Paths(rootDirectory), Exclude("node_modules"))
	defer watcher.Stop()
	<-time.After(time.Millisecond * 500)

	go func() {
		defer func() {
			events <- Event{Name: "ALLDONE"}
		}()
		<-time.After(time.Millisecond * 10)
		if err := ioutil.WriteFile(filepath.Join(rootDirectory, "node_modules", "LVL2.txt"), []byte("TEST"), 0777); err != nil {
			panic(err)
		}
		<-time.After(time.Millisecond * 10)
		if err := ioutil.WriteFile(filepath.Join(rootDirectory, "lab1", "LVL2.txt"), []byte("TEST"), 0777); err != nil {
			panic(err)
		}
		<-time.After(time.Millisecond * 10)
		if err := ioutil.WriteFile(filepath.Join(rootDirectory, "lab2", "LVL2.txt"), []byte("TEST"), 0777); err != nil {
			panic(err)
		}
		<-time.After(time.Millisecond * 10)
	}()

	for v := range events {
		if v.Name == "ALLDONE" {
			break
		}
		fmt.Println(v.Name)
		// fmt.Println(filepath.Base(v.Name))
	}

	// select {
	// case ev := <-events:
	// 	if strings.Contains(ev.Name, "dirwatch-example") &&
	// 		strings.Contains(ev.Name, "lab2") && ev.Op == fsnotify.Create {
	// 		fmt.Println("OK")
	// 	}
	// case <-time.After(time.Second * 3):
	// }

	// Output:
	// OK
}
