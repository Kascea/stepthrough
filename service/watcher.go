package service

import (
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/colecarlson/stepthrough/orchestrator"
)

// WatcherService watches multiple pipeline YAML files for changes.
type WatcherService struct {
	pipeline *PipelineService
	sysEmit  orchestrator.EventSink

	mu       sync.Mutex
	watchers map[string]*fsnotify.Watcher
}

func NewWatcherService(pipeline *PipelineService, sysEmit orchestrator.EventSink) *WatcherService {
	return &WatcherService{
		pipeline: pipeline,
		sysEmit:  sysEmit,
		watchers: make(map[string]*fsnotify.Watcher),
	}
}

// AddWatch starts watching the given file and triggers an initial load+run.
// If the file is already being watched, this is a no-op.
func (w *WatcherService) AddWatch(file string) error {
	abs, err := filepath.Abs(file)
	if err != nil {
		return err
	}

	w.mu.Lock()
	if _, exists := w.watchers[abs]; exists {
		w.mu.Unlock()
		return nil
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		w.mu.Unlock()
		return err
	}
	if err := watcher.Add(abs); err != nil {
		watcher.Close()
		w.mu.Unlock()
		return err
	}
	w.watchers[abs] = watcher
	w.mu.Unlock()

	isNew := w.pipeline.EnsureTab(abs)
	if isNew {
		w.pipeline.emit("pipeline:tab:added", abs)
		w.pipeline.ReloadFile(abs) // new file: load + auto-run
	}
	// Session-restored tabs (isNew=false) are loaded by RestoreTab; just watch for future changes.

	go w.watch(abs, watcher)
	return nil
}

// RemoveWatch stops watching the given file.
func (w *WatcherService) RemoveWatch(file string) {
	abs, _ := filepath.Abs(file)
	w.mu.Lock()
	watcher, ok := w.watchers[abs]
	if ok {
		delete(w.watchers, abs)
	}
	w.mu.Unlock()
	if watcher != nil {
		watcher.Close()
	}
}

func (w *WatcherService) watch(file string, watcher *fsnotify.Watcher) {
	var debounce *time.Timer
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				if debounce != nil {
					debounce.Stop()
				}
				debounce = time.AfterFunc(150*time.Millisecond, func() {
					w.pipeline.ReloadFile(file)
				})
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			w.sysEmit("watcher:error", err.Error())
		}
	}
}
