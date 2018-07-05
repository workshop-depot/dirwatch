package dirwatch

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/dc0d/retry"
	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"
)

//-----------------------------------------------------------------------------

// Event represents a single file system notification.
type Event struct {
	Name string
	Op   fsnotify.Op
}

//-----------------------------------------------------------------------------

type options struct {
	notify  func(Event)
	exclude []string
	logger  func(args ...interface{})
}

// Option modifies the options.
type Option func(*options)

// Notify sets the notify callback.
func Notify(notify func(Event)) Option {
	return func(opt *options) {
		opt.notify = notify
	}
}

// Exclude sets patterns to exclude from watch.
func Exclude(exclude ...string) Option {
	return func(opt *options) {
		opt.exclude = exclude
	}
}

// Logger sets the logger for the watcher.
func Logger(logger func(args ...interface{})) Option {
	return func(opt *options) {
		opt.logger = logger
	}
}

//-----------------------------------------------------------------------------

// Watcher watches over a directory and it's sub-directories, recursively.
type Watcher struct {
	notify  func(Event)
	exclude []string
	logger  func(args ...interface{})

	paths  map[string]bool
	add    chan fspath
	ctx    context.Context
	cancel context.CancelFunc
}

type fspath struct {
	path      string
	recursive *bool
}

// New creates a new *Watcher. Excluded patterns are based on
// filepath.Match function patterns.
func New(opt ...Option) *Watcher {
	o := &options{}
	for _, v := range opt {
		v(o)
	}
	if o.notify == nil {
		panic("notify can not be nil")
	}
	if o.logger == nil {
		o.logger = log.Println
	}

	res := &Watcher{
		add:     make(chan fspath),
		paths:   make(map[string]bool),
		notify:  o.notify,
		exclude: o.exclude,
		logger:  o.logger,
	}
	res.ctx, res.cancel = context.WithCancel(context.Background())

	res.start()
	return res
}

// Stop stops the watcher. Safe to be called mutiple times.
func (dw *Watcher) Stop() {
	dw.cancel()
}

// Add adds a path to be watched.
func (dw *Watcher) Add(path string, recursive bool) {
	started := make(chan struct{})
	go func() {
		close(started)
		v, err := filepath.Abs(path)
		if err != nil {
			dw.logger(err)
			return
		}
		select {
		case dw.add <- fspath{path: v, recursive: &recursive}:
		case <-dw.stopped():
			return
		}
	}()
	<-started
}

//-----------------------------------------------------------------------------

func (dw *Watcher) stopped() <-chan struct{} { return dw.ctx.Done() }

func (dw *Watcher) start() {
	started := make(chan struct{})
	go func() {
		close(started)
		retry.Retry(
			dw.agent,
			-1,
			func(err error) {
				e := err.(interface{ CausedBy() interface{} })
				fmt.Printf(">>> %+v\n", e.CausedBy())
			},
			time.Second)
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

	for {
		select {
		case <-dw.stopped():
			return nil
		case ev := <-watcher.Events:
			dw.onEvent(Event(ev))
		case err := <-watcher.Errors:
			dw.logger(fmt.Sprintf("error: %+v\n", errors.WithStack(err)))
		case d := <-dw.add:
			dw.onAdd(watcher, d)
		}
	}
}

func (dw *Watcher) onAdd(
	watcher *fsnotify.Watcher,
	fsp fspath) {
	if fsp.path == "" {
		return
	}
	var err error
	fsp.path, err = filepath.Abs(fsp.path)
	if err != nil {
		dw.logger(err)
		return
	}
	_, err = os.Stat(fsp.path)
	if err != nil {
		if os.IsNotExist(err) {
			delete(dw.paths, fsp.path)
			return
		}
		dw.logger(err)
		return
	}
	_, ok := dw.paths[fsp.path]
	if ok {
		return
	}
	if dw.excludePath(fsp.path) {
		return
	}
	if err := watcher.Add(fsp.path); err != nil {
		dw.logger(fmt.Sprintf("on add error: %+v\n", errors.WithStack(err)))
	}
	recursive, _ := dw.paths[fsp.path]
	if fsp.recursive != nil {
		recursive = *fsp.recursive
	}
	dw.paths[fsp.path] = recursive
	isd, _ := isDir(fsp.path)
	if recursive && isd {
		go func() {
			tree := dw.dirTree(fsp.path)
			for v := range tree {
				dw.add <- fspath{path: v}
			}
		}()
	}
}

func (dw *Watcher) onEvent(ev Event) {
	if dw.excludePath(ev.Name) {
		return
	}
	// callback
	go retry.Try(func() error { dw.notify(ev); return nil })

	name := ev.Name
	isdir, err := isDir(name)
	if err != nil {
		if os.IsNotExist(err) {
			delete(dw.paths, name)
		} else {
			dw.logger(err)
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
		case dw.add <- fspath{path: name}:
		}
	}()
}

func (dw *Watcher) excludePath(p string) bool {
	for _, ptrn := range dw.exclude {
		matched, err := filepath.Match(ptrn, p)
		if err != nil {
			dw.logger(err)
			continue
		}
		if matched {
			return true
		}
	}
	return false
}

func (dw *Watcher) dirTree(queryRoot string) <-chan string {
	found := make(chan string)
	go func() {
		defer close(found)
		err := filepath.Walk(queryRoot, func(path string, f os.FileInfo, err error) error {
			if !f.IsDir() {
				return nil
			}
			if filepath.Clean(path) == filepath.Clean(queryRoot) {
				return nil
			}
			found <- path
			return nil
		})
		if err != nil {
			dw.logger(fmt.Sprintf("%+v", errors.WithStack(err)))
		}
	}()
	return found
}

func isDir(path string) (ok bool, err error) {
	var inf os.FileInfo
	inf, err = os.Stat(path)
	if inf != nil {
		ok = inf.IsDir()
	}
	return
}

//-----------------------------------------------------------------------------
