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
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	require := require.New(t)
	// prepare sample home directory to watch over
	rootDirectory, err := ioutil.TempDir(os.TempDir(), "dirwatch-example")
	require.NoError(err)
	os.RemoveAll(rootDirectory)
	os.Mkdir(rootDirectory, 0777)

	// our notification callback (I feel it's simpler to
	// have a callback instead of passing a channel in an API)
	var events = make(chan Event, 100)
	notify := func(ev Event) {
		events <- ev
	}

	// create the watcher
	watcher := New(Notify(notify))
	defer watcher.Stop()

	watcher.Add(rootDirectory, true)
	<-time.After(time.Millisecond * 50)

	// creating a directory inside the root/home
	dir2 := filepath.Join(rootDirectory, "lab2")
	os.Remove(dir2)
	err = os.Mkdir(dir2, 0777)
	require.NoError(err)

	<-time.After(time.Millisecond * 50)

	ok := false
	select {
	case ev := <-events:
		if strings.Contains(ev.Name, "dirwatch-example") &&
			strings.Contains(ev.Name, "lab2") && ev.Op == fsnotify.Create {
			ok = true
		}
	case <-time.After(time.Second * 10):
	}
	require.True(ok)

	fp := filepath.Join(dir2, "sample.txt")
	ioutil.WriteFile(fp, []byte("DATA"), 0777)
	<-time.After(time.Millisecond * 50)

	ok = false
	select {
	case ev := <-events:
		if strings.Contains(ev.Name, "sample.txt") {
			ok = true
		}
	case <-time.After(time.Second * 10):
	}
	require.True(ok)

	dir3 := filepath.Join(dir2, "lab3")
	err = os.Mkdir(dir3, 0777)
	if err != nil {
		panic(err)
	}
	<-time.After(time.Millisecond * 50)

	fp = filepath.Join(dir3, "sample3.txt")
	ioutil.WriteFile(fp, []byte("DATA"), 0777)
	<-time.After(time.Millisecond * 50)

	count := 0
T1:
	for {
		select {
		case ev := <-events:
			if strings.Contains(ev.Name, "lab3") {
				count++
			}
			if strings.Contains(ev.Name, "sample3.txt") {
				count++
			}
		case <-time.After(time.Millisecond * 100):
			break T1
		}
	}
	require.Condition(func() bool { return count >= 2 })
}

func TestRemove(t *testing.T) {
	require := require.New(t)

	// prepare sample home directory to watch over
	rootDirectory, err := ioutil.TempDir(os.TempDir(), "dirwatch-example")
	require.NoError(err)
	os.RemoveAll(rootDirectory)
	os.Mkdir(rootDirectory, 0777)

	// our notification callback (I feel it's simpler to
	// have a callback instead of passing a channel in an API)
	var events = make(chan Event, 100)
	notify := func(ev Event) {
		events <- ev
	}

	// create the watcher
	watcher := New(Notify(notify))
	defer watcher.Stop()

	watcher.Add(rootDirectory, true)
	<-time.After(time.Millisecond * 50)

	// creating a directory inside the root/home
	dir2 := filepath.Join(rootDirectory, "lab2")
	os.Remove(dir2)

	err = os.Mkdir(dir2, 0777)
	require.NoError(err)
	<-time.After(time.Millisecond * 50)
	err = os.Remove(dir2)
	require.NoError(err)
	<-time.After(time.Millisecond * 50)
	err = os.Mkdir(dir2, 0777)
	require.NoError(err)
	<-time.After(time.Millisecond * 50)

	actions := 0
T3:
	for {
		select {
		case ev := <-events:
			if ev.Op == fsnotify.Create ||
				ev.Op == fsnotify.Remove {
				actions++
			}
		case <-time.After(time.Millisecond * 60):
			break T3
		}
	}
	require.Condition(func() bool { return actions > 2 })
}

func prep() string {
	rootDirectory := filepath.Join(os.TempDir(), "dirwatch-example-exclude")
	if err := os.RemoveAll(rootDirectory); err != nil {
		panic(err)
	}
	os.Mkdir(rootDirectory, 0777)
	return rootDirectory
}

func ExampleWatcher_simple() {
	dir := prep()

	notify := func(ev Event) {
		fmt.Println(filepath.Base(ev.Name))
	}

	// create the watcher
	watcher := New(Notify(notify))
	defer watcher.Stop()
	watcher.Add(dir, true)

	ioutil.WriteFile(filepath.Join(dir, "text.txt"), nil, 0777)
	<-time.After(time.Millisecond * 300)

	// Output:
	// text.txt
}

func ExampleWatcher_recursive() {
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
	watcher := New(Notify(notify), Exclude("/*/*/node_modules"))
	defer watcher.Stop()
	watcher.Add(rootDirectory, true)
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

	count := 0
	for v := range events {
		if v.Name == "ALLDONE" {
			break
		}
		if strings.Contains(v.Name, "LVL2.txt") {
			count++
			continue
		}
	}
	fmt.Println(count)

	// Output:
	// 4
}

func ExampleWatcher_simpleExclude() {
	// prepare sample home directory to watch over
	// rootDirectory, err := ioutil.TempDir(os.TempDir(), "dirwatch-example-")
	rootDirectory := filepath.Join(os.TempDir(), "dirwatch-example-exclude")
	os.RemoveAll(rootDirectory)
	os.Mkdir(rootDirectory, 0777)

	// our notification callback (I feel it's simpler to
	// have a callback instead of passing a channel in an API)
	var events = make(chan Event, 100)
	notify := func(ev Event) {
		events <- ev
	}

	// create the watcher
	watcher := New(Notify(notify), Exclude("/*/*/node_modules"))
	defer watcher.Stop()
	watcher.Add(rootDirectory, true)
	<-time.After(time.Millisecond * 500)

	go func() {
		defer func() {
			events <- Event{Name: "ALLDONE"}
		}()
		os.MkdirAll(filepath.Join(rootDirectory, "node_modules"), 0777)
		os.MkdirAll(filepath.Join(rootDirectory, "lab1"), 0777)
		os.MkdirAll(filepath.Join(rootDirectory, "lab2"), 0777)
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

	count := 0
	for v := range events {
		if v.Name == "ALLDONE" {
			break
		}
		if strings.Contains(v.Name, "LVL2.txt") {
			count++
			continue
		}
	}
	fmt.Println(count)

	// Output:
	// 4
}
