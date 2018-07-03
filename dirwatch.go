package dirwatch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dc0d/retry"
	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"
)

//-----------------------------------------------------------------------------

// Event represents a single file system notification.
type Event = fsnotify.Event

//-----------------------------------------------------------------------------

// Watcher watches over a directory and it's sub-directories (passed to New), recursively.
// Also watches files, if the path is explicitly provided.
// If a path does no longer exists, it will be removed.
type Watcher struct {
	paths  map[string]struct{}
	add    chan string
	notify func(Event)

	ctx    context.Context
	cancel context.CancelFunc
}

// New creates a new *Watcher
func New(notify func(Event), pathList ...string) *Watcher {
	if notify == nil {
		panic("notify can not be nil")
	}
	res := &Watcher{
		paths:  make(map[string]struct{}),
		add:    make(chan string),
		notify: notify,
	}
	res.ctx, res.cancel = context.WithCancel(context.Background())
	res.AddSingle(pathList...)
	res.start()
	return res
}

// Stop stops the watcher. Safe to be called mutiple times.
func (dw *Watcher) Stop() {
	dw.cancel()
}

// AddSingle adds individual paths that won't be watched recursively. For a
// dir-path to be watched recursively, it should be passed to New.
func (dw *Watcher) AddSingle(pathList ...string) {
	go func() {
		for _, v := range pathList {
			v, err := filepath.Abs(v)
			if err != nil {
				lerror(err)
				continue
			}
			select {
			case dw.add <- v:
			case <-dw.stopped():
				return
			}
		}
	}()
}

func (dw *Watcher) stopped() <-chan struct{} { return dw.ctx.Done() }

func (dw *Watcher) start() {
	started := make(chan struct{})
	go func() {
		close(started)
		retry.Retry(
			dw.agent,
			-1,
			func(e error) { lerrorf("watcher agent error: %+v", e) },
			time.Second*5)
	}()
	<-started
	// HACK:
	<-time.After(time.Millisecond * 500)
}

func (dw *Watcher) agent() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return errors.WithStack(err)
	}
	defer watcher.Close()

	dw.prepAgent()

	for {
		select {
		case <-dw.stopped():
			return nil
		case ev := <-watcher.Events:
			dw.onEvent(ev)
		case err := <-watcher.Errors:
			lerrorf("error: %+v\n", errors.WithStack(err))
		case d := <-dw.add:
			dw.onAdd(watcher, d)
		}
	}
}

func (dw *Watcher) onAdd(
	watcher *fsnotify.Watcher,
	d string) {

	if d == "" {
		return
	}
	var err error
	d, err = filepath.Abs(d)
	if err != nil {
		lerror(err)
		return
	}
	_, err = os.Stat(d)
	if err != nil {
		if os.IsNotExist(err) {
			delete(dw.paths, d)
			return
		}
		lerror(err)
		return
	}
	_, ok := dw.paths[d]
	if ok {
		return
	}
	var (
		sep = string([]rune{os.PathSeparator})
	)
	parts := strings.Split(d, sep)
	for _, p := range parts {
		p := strings.ToLower(p)
		if p == ".git" {
			return
		}
	}
	if err := watcher.Add(d); err != nil {
		lerrorf("on add error: %+v\n", errors.WithStack(err))
	}
	dw.paths[d] = struct{}{}
}

func (dw *Watcher) onEvent(ev Event) {
	// callback
	go retry.Try(func() error { dw.notify(ev); return nil })

	name := ev.Name
	isdir, err := isDir(name)
	if err != nil {
		if os.IsNotExist(err) {
			delete(dw.paths, name)
		} else {
			lerror(err)
		}
		return
	}

	if !isdir {
		return
	}

	go func() {
		select {
		case <-dw.stopped():
			return
		case dw.add <- name:
		}
	}()
}

func (dw *Watcher) prepAgent() {
	started := make(chan struct{})
	paths := make(map[string]struct{})
	for k, v := range dw.paths {
		paths[k] = v
	}
	go func() {
		close(started)
		retry.Retry(
			func() error { return initTree(paths, dw.add, dw.stopped()) },
			-1,
			func(e error) { lerrorf("init tree error: %+v\n", e) },
			time.Second*5)
	}()
	<-started
}

func initTree(
	current map[string]struct{},
	add chan<- string,
	stop <-chan struct{}) error {
	paths := make(chan string)
	var wg sync.WaitGroup
	for k := range current {
		v, err := filepath.Abs(k)
		if err != nil {
			lerror(err)
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			var d <-chan string
			retry.Try(func() error {
				d = dirTree(v)
				return nil
			})
			for item := range d {
				paths <- item
			}
		}()
	}
	go func() {
		defer close(paths)
		wg.Wait()
	}()
	for {
		select {
		case d, ok := <-paths:
			if !ok {
				return nil
			}
			select {
			case add <- d:
			case <-stop:
				return nil
			}
		case <-stop:
			return nil
		}
	}
}

//-----------------------------------------------------------------------------

func dirTree(root string) <-chan string {
	found := make(chan string)
	go func() {
		defer close(found)
		ok, err := isDir(root)
		if err != nil {
			lerror(err)
			return
		}
		if !ok {
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
			lerrorf("%+v", errors.WithStack(err))
		}
	}()
	return found
}

func isDir(path string) (bool, error) {
	inf, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return inf.IsDir(), nil
}

//-----------------------------------------------------------------------------
