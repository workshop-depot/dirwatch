package dirwatch

import (
	"context"
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
	ctx    context.Context
	cancel context.CancelFunc
	lastWD string // last working directory
	notify func(fsnotify.Event)
}

// New creates a new *Watch
func New(root string, notify func(fsnotify.Event), ctx ...context.Context) (*Watch, error) {
	if notify == nil {
		return nil, errors.New("notify can not be nil")
	}
	res := &Watch{
		root:   root,
		added:  make(chan string),
		notify: notify,
	}
	var vctx context.Context
	if len(ctx) > 0 {
		vctx = ctx[0]
	}
	if vctx == nil {
		vctx = context.Background()
	}
	res.ctx, res.cancel = context.WithCancel(vctx)
	res.start()
	return res, nil
}

func (dw *Watch) start() {
	go retry.Retry(
		dw.agent,
		-1,
		func(e error) { log.Errorf("watcher agent error: %+v", e) },
		time.Second*5)
}

// Stop stops the watcher. Watcher would also stop when the parent context
// be canceled (if provided).
func (dw *Watch) Stop() {
	dw.cancel()
}

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
		case <-dw.ctx.Done():
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

func (dw *Watch) prepAgent() {
	go func() {
		lastWD := dw.lastWD
		select {
		case <-dw.ctx.Done():
			return
		case dw.added <- lastWD:
		}
	}()

	go retry.Retry(
		dw.initTree,
		-1,
		func(e error) { log.Errorf("init tree error: %+v\n", e) },
		time.Second*5)
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
			case <-dw.ctx.Done():
				return nil
			}
		case <-dw.ctx.Done():
			return nil
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
	_, err := os.Stat(d)
	if err != nil && os.IsNotExist(err) {
		return
	}
	_, ok := underWatch[d]
	if ok {
		return
	}
	underWatch[d] = struct{}{}
	parts := strings.Split(d, sep)
	for _, p := range parts {
		p := strings.ToLower(p)
		if p == ".git" {
			return
		}
	}
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
			// thoughts:
			// we need a proper virtual tree of directories
			// to remove a node and it's children from the watcher,
			// currently this is about testing an idea (of generics + type aliases)

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
		case <-dw.ctx.Done():
			return
		case dw.added <- name:
		}
	}()
}
