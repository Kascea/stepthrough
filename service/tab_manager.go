package service

import (
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/colecarlson/stepthrough/orchestrator"
)

// TabStatus represents the lifecycle state of a pipeline tab.
type TabStatus int

const (
	TabStatusEmpty   TabStatus = iota // registered, no pipeline loaded yet
	TabStatusLoaded                   // pipeline parsed; idle or post-run
	TabStatusRunning                  // pipeline execution in progress
	TabStatusMissing                  // source file not found on disk
	TabStatusError                    // parse or setup error
)

type tabEntry struct {
	orch    *orchestrator.Orchestrator
	status  TabStatus
	watcher *fsnotify.Watcher
}

// tabManager owns the set of active pipeline tabs, their orchestrators, file watchers,
// and ordering. All methods are safe for concurrent use.
type tabManager struct {
	mu           sync.Mutex
	entries      map[string]*tabEntry
	order        []string
	activeFile   string
	onFileChange func(file string)
	onWatchError func(err string)
}

func newTabManager(onFileChange func(string), onWatchError func(string)) *tabManager {
	return &tabManager{
		entries:      make(map[string]*tabEntry),
		onFileChange: onFileChange,
		onWatchError: onWatchError,
	}
}

// startWatch creates an fsnotify watcher for file and starts the debounce goroutine.
// Returns nil if the file cannot be watched (e.g. does not exist yet).
// Safe to call with m.mu held — the goroutine runs independently.
func (m *tabManager) startWatch(file string) *fsnotify.Watcher {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		if m.onWatchError != nil {
			m.onWatchError(err.Error())
		}
		return nil
	}
	if err := w.Add(file); err != nil {
		w.Close()
		return nil
	}
	go m.watchLoop(file, w)
	return w
}

func (m *tabManager) watchLoop(file string, w *fsnotify.Watcher) {
	var debounce *time.Timer
	for {
		select {
		case event, ok := <-w.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				if debounce != nil {
					debounce.Stop()
				}
				debounce = time.AfterFunc(150*time.Millisecond, func() {
					if m.onFileChange != nil {
						m.onFileChange(file)
					}
				})
			}
		case err, ok := <-w.Errors:
			if !ok {
				return
			}
			if m.onWatchError != nil {
				m.onWatchError(err.Error())
			}
		}
	}
}

// seedOrder sets the tab order and active file from a saved session, creating
// placeholder entries for each file. Only takes effect on the first call.
// Orchestrators and watchers are added later via addOrch.
func (m *tabManager) seedOrder(order []string, activeFile string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.order) == 0 && len(order) > 0 {
		m.order = make([]string, len(order))
		copy(m.order, order)
		m.activeFile = activeFile
		for _, file := range order {
			if _, ok := m.entries[file]; !ok {
				m.entries[file] = &tabEntry{status: TabStatusEmpty}
			}
		}
	}
}

// addOrch attaches an orchestrator to an existing (or new) entry and starts watching
// the file. Used by RestoreTab where order is already seeded.
func (m *tabManager) addOrch(file string, make func() *orchestrator.Orchestrator) *orchestrator.Orchestrator {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[file]
	if ok && e.orch != nil {
		return e.orch
	}
	orch := make()
	w := m.startWatch(file) // nil if file is missing — caller handles the missing case
	if ok {
		e.orch = orch
		if e.watcher == nil {
			e.watcher = w
		}
	} else {
		m.entries[file] = &tabEntry{orch: orch, status: TabStatusEmpty, watcher: w}
	}
	return orch
}

// ensure registers a tab for file if one doesn't already exist, appending it to
// the order and starting the file watcher. Use for new tabs opened by the user.
// Returns (true, newOrch) if created, (false, existingOrch) otherwise.
func (m *tabManager) ensure(file string, make func() *orchestrator.Orchestrator) (bool, *orchestrator.Orchestrator) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e, ok := m.entries[file]; ok && e.orch != nil {
		return false, e.orch
	}
	orch := make()
	w := m.startWatch(file)
	m.entries[file] = &tabEntry{orch: orch, status: TabStatusEmpty, watcher: w}
	m.order = append(m.order, file)
	m.activeFile = file
	return true, orch
}

// get returns the orchestrator for file.
func (m *tabManager) get(file string) (*orchestrator.Orchestrator, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e, ok := m.entries[file]; ok && e.orch != nil {
		return e.orch, true
	}
	return nil, false
}

// setStatus updates the lifecycle status for a tab.
func (m *tabManager) setStatus(file string, status TabStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e, ok := m.entries[file]; ok {
		e.status = status
	}
}

// getStatus returns the current lifecycle status for a tab.
func (m *tabManager) getStatus(file string) (TabStatus, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e, ok := m.entries[file]; ok {
		return e.status, true
	}
	return TabStatusEmpty, false
}

// remove deletes the tab, closes its watcher, and returns its orchestrator for cleanup.
func (m *tabManager) remove(file string) (*orchestrator.Orchestrator, bool) {
	m.mu.Lock()
	e, ok := m.entries[file]
	if !ok {
		m.mu.Unlock()
		return nil, false
	}
	delete(m.entries, file)
	for i, f := range m.order {
		if f == file {
			m.order = append(m.order[:i], m.order[i+1:]...)
			break
		}
	}
	if m.activeFile == file {
		if len(m.order) > 0 {
			m.activeFile = m.order[0]
		} else {
			m.activeFile = ""
		}
	}
	w := e.watcher
	m.mu.Unlock()
	if w != nil {
		w.Close()
	}
	if e.orch != nil {
		return e.orch, true
	}
	return nil, true
}

// setActive records the currently focused tab.
func (m *tabManager) setActive(file string) {
	m.mu.Lock()
	m.activeFile = file
	m.mu.Unlock()
}

// relocate moves a tab from oldFile to newFile, replacing its orchestrator and watcher.
// Returns the old orchestrator for cleanup.
func (m *tabManager) relocate(oldFile, newFile string, make func() *orchestrator.Orchestrator) (*orchestrator.Orchestrator, bool) {
	m.mu.Lock()
	e, ok := m.entries[oldFile]
	if !ok {
		m.mu.Unlock()
		return nil, false
	}
	oldOrch := e.orch
	oldWatcher := e.watcher
	newWatcher := m.startWatch(newFile)
	delete(m.entries, oldFile)
	m.entries[newFile] = &tabEntry{orch: make(), status: TabStatusEmpty, watcher: newWatcher}
	for i, f := range m.order {
		if f == oldFile {
			m.order[i] = newFile
			break
		}
	}
	if m.activeFile == oldFile {
		m.activeFile = newFile
	}
	m.mu.Unlock()
	if oldWatcher != nil {
		oldWatcher.Close()
	}
	return oldOrch, true
}

// snapshot returns copies of the current state for session persistence.
func (m *tabManager) snapshot() (order []string, activeFile string, orchs map[string]*orchestrator.Orchestrator) {
	m.mu.Lock()
	defer m.mu.Unlock()
	order = make([]string, len(m.order))
	copy(order, m.order)
	activeFile = m.activeFile
	orchs = make(map[string]*orchestrator.Orchestrator, len(m.entries))
	for k, e := range m.entries {
		if e.orch != nil {
			orchs[k] = e.orch
		}
	}
	return
}

// all returns all orchestrators as a slice, for bulk cleanup.
func (m *tabManager) all() []*orchestrator.Orchestrator {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*orchestrator.Orchestrator, 0, len(m.entries))
	for _, e := range m.entries {
		if e.orch != nil {
			result = append(result, e.orch)
		}
	}
	return result
}

// shutdown closes all file watchers.
func (m *tabManager) shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.entries {
		if e.watcher != nil {
			e.watcher.Close()
			e.watcher = nil
		}
	}
}
