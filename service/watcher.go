package service

import (
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/colecarlson/stepthrough/orchestrator"
)

// WatcherService watches a pipeline YAML file and triggers hot-refresh on save.
// It has no dependency on the Wails application or any UI framework.
type WatcherService struct {
	pipeline *PipelineService
	emit     orchestrator.EventSink
	file     string
}

func NewWatcherService(pipeline *PipelineService, emit orchestrator.EventSink) *WatcherService {
	return &WatcherService{pipeline: pipeline, emit: emit}
}

// Watch starts watching the given YAML file and triggers hot-refresh on save.
// It also performs an initial load immediately.
func (w *WatcherService) Watch(file string) error {
	abs, err := filepath.Abs(file)
	if err != nil {
		return err
	}
	w.file = abs

	w.reload()

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	if err := watcher.Add(abs); err != nil {
		return err
	}

	// Debounce: wait 150ms after last event before reloading.
	var debounce *time.Timer
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				if debounce != nil {
					debounce.Stop()
				}
				debounce = time.AfterFunc(150*time.Millisecond, w.reload)
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			w.emit("watcher:error", err.Error())
		}
	}
}

// GetWatchedFile returns the currently watched file path.
func (w *WatcherService) GetWatchedFile() string {
	return w.file
}

func (w *WatcherService) reload() {
	state := w.pipeline.LoadPipeline(w.file)

	if !state.Valid {
		w.emit("pipeline:error", state.Error)
		return
	}

	w.emit("pipeline:loaded", state)
	w.pipeline.RunFrom(0)
}
