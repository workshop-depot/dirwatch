# dirwatch
For watching for changes inside a directory and all sub-directories, recursively. Uses the package [fsnotify](https://github.com/fsnotify/fsnotify).

### Documentation:

[![GoDoc](https://godoc.org/github.com/dc0d/dirwatch?status.svg)](https://godoc.org/github.com/dc0d/dirwatch)

## Sample Usage

```go
notify := func(ev Event) {
	// processing the event ev
}

// create the watcher which excludes
// any folder along the added paths
// that matches provided pattern(s).
watcher := New(Notify(notify), Exclude("/*/*/node_modules"))
defer watcher.Stop()
watcher.Add(dir1, true)
watcher.Add(dir2, false)
watcher.Add(dir3, true)
```

### Environment:
* Ubuntu 18.04
* Go 1.10.3

### TODO:
* more tests