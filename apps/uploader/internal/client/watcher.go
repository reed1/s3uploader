package client

import (
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
)

type Watcher struct {
	watcher *fsnotify.Watcher
	queue   *Queue
	cfg     *Config
}

func NewWatcher(queue *Queue, cfg *Config) (*Watcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	return &Watcher{
		watcher: w,
		queue:   queue,
		cfg:     cfg,
	}, nil
}

func (w *Watcher) Start() error {
	for _, watch := range w.cfg.Watches {
		if err := w.addRecursive(watch.LocalPath); err != nil {
			return err
		}
	}

	go w.processEvents()
	return nil
}

func (w *Watcher) addRecursive(root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return w.watcher.Add(path)
		}
		return nil
	})
}

func (w *Watcher) processEvents() {
	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			w.handleEvent(event)
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("watcher error: %v", err)
		}
	}
}

func (w *Watcher) handleEvent(event fsnotify.Event) {
	if event.Op&(fsnotify.Create|fsnotify.Write) == 0 {
		return
	}

	info, err := os.Stat(event.Name)
	if err != nil {
		return
	}

	if info.IsDir() {
		if event.Op&fsnotify.Create != 0 {
			w.watcher.Add(event.Name)
		}
		return
	}

	remotePath := w.getRemotePath(event.Name)
	if remotePath == "" {
		return
	}

	if w.cfg.IsExcluded(remotePath) {
		return
	}

	w.queue.Enqueue(event.Name, remotePath)
}

func (w *Watcher) getRemotePath(localPath string) string {
	for _, watch := range w.cfg.Watches {
		if strings.HasPrefix(localPath, watch.LocalPath) {
			relPath, err := filepath.Rel(watch.LocalPath, localPath)
			if err != nil {
				continue
			}
			return filepath.Join(watch.RemotePrefix, relPath)
		}
	}
	return ""
}

func (w *Watcher) Close() error {
	return w.watcher.Close()
}
