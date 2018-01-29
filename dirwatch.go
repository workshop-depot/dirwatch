package dirwatch

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dc0d/retry"
	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"
)

// Watch watches over a directory and it's sub-directories, recursively
type Watch struct {
	root   string
	added  chan string
	notify func(fsnotify.Event)

	stop   chan struct{}
	lastWD string // last working directory
}

// New creates a new *Watch
func New(root string, notify func(fsnotify.Event)) *Watch {
	if notify == nil {
		panic("notify can not be nil")
	}
	var err error
	root, err = filepath.Abs(root)
	if err != nil {
		panic(err)
	}
	res := &Watch{
		root:   root,
		added:  make(chan string),
		notify: notify,
		stop:   make(chan struct{}),
	}
	res.start()
	return res
}

// Stop stops the watcher.
func (dw *Watch) Stop() {
	close(dw.stop)
}

func (dw *Watch) start() {
	started := make(chan struct{})
	go func() {
		close(started)
		retry.Retry(
			dw.agent,
			-1,
			func(e error) { log.Errorf("watcher agent error: %+v", e) },
			time.Second*5)
	}()
	<-started
	// HACK:
	<-time.After(time.Millisecond * 500)
}

func (dw *Watch) stopped() <-chan struct{} { return dw.stop }

func (dw *Watch) agent() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return errors.WithStack(err)
	}
	defer watcher.Close()

	dw.prepAgent()

	underWatch := make(map[string]struct{})

	for {
		select {
		case <-dw.stopped():
			return nil
		case ev := <-watcher.Events:
			dw.onEvent(ev, underWatch)
		case err := <-watcher.Errors:
			log.Errorf("error: %+v\n", errors.WithStack(err))
		case d := <-dw.added:
			dw.onAdd(watcher, d, underWatch)
		}
	}
}

func (dw *Watch) onAdd(
	watcher *fsnotify.Watcher,
	d string,
	underWatch map[string]struct{}) {
	if d == "" {
		return
	}
	d, _ = filepath.Abs(d)
	_, err := os.Stat(d)
	if err != nil && os.IsNotExist(err) {
		return
	}
	_, ok := underWatch[d]
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
	underWatch[d] = struct{}{}
	if err := watcher.Add(d); err != nil {
		log.Errorf("on add error: %+v\n", errors.WithStack(err))
	}
}

func (dw *Watch) onEvent(ev fsnotify.Event, underWatch map[string]struct{}) {
	// callback
	go func() { dw.notify(ev) }()

	name := ev.Name
	info, err := os.Stat(name)
	if err != nil {
		_, ok := underWatch[name]
		if ok && os.IsNotExist(err) {
			// thoughts, following:
			// apparently there is no need to explicitly remove watchs
			// (at least on Ubuntu 16.04, trying to remove, generates an error for
			// non-existant entry, should be investigated more carefully)
			delete(underWatch, name)
		} else if !os.IsNotExist(err) {
			log.Error(err)
		}
		return
	}

	if info.IsDir() {
		dw.lastWD = name
	} else {
		if wd, err := filepath.Abs(filepath.Dir(name)); err == nil {
			dw.lastWD = wd
		}
	}

	if !info.IsDir() {
		return
	}

	go func() {
		select {
		case <-dw.stopped():
			return
		case dw.added <- name:
		}
	}()
}

func (dw *Watch) prepAgent() {
	go func() {
		lastWD := dw.lastWD
		select {
		case <-dw.stopped():
			return
		case dw.added <- lastWD:
		}
	}()

	started := make(chan struct{})
	go func() {
		close(started)
		retry.Retry(
			dw.initTree,
			-1,
			func(e error) { log.Errorf("init tree error: %+v\n", e) },
			time.Second*5)
	}()
	<-started
}

func (dw *Watch) initTree() error {
	dirs := dirTree(dw.root)
	for {
		select {
		case d, ok := <-dirs:
			if !ok {
				return nil
			}
			select {
			case dw.added <- d:
			case <-dw.stopped():
				return nil
			}
		case <-dw.stopped():
			return nil
		}
	}
}
