package service

import (
	"sync"

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
	orch   *orchestrator.Orchestrator // nil for session-seeded tabs before RestoreTab
	status TabStatus
}

// tabManager owns the set of active pipeline tabs, their orchestrators, and ordering.
// All methods are safe for concurrent use.
type tabManager struct {
	mu         sync.Mutex
	entries    map[string]*tabEntry
	order      []string
	activeFile string
}

func newTabManager() *tabManager {
	return &tabManager{
		entries: make(map[string]*tabEntry),
	}
}

// seedOrder sets the tab order and active file from a saved session, creating
// placeholder entries for each file. Only takes effect on the first call.
// Orchestrators are added later via addOrch or ensure.
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

// addOrch attaches an orchestrator to an existing (or new) entry without
// touching tab order or activeFile. Used by RestoreTab where order is already seeded.
func (m *tabManager) addOrch(file string, make func() *orchestrator.Orchestrator) *orchestrator.Orchestrator {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[file]
	if ok && e.orch != nil {
		return e.orch
	}
	orch := make()
	if ok {
		e.orch = orch
	} else {
		m.entries[file] = &tabEntry{orch: orch, status: TabStatusEmpty}
	}
	return orch
}

// ensure registers a tab for file if one doesn't already exist, appending it to
// the order. Use for new tabs opened by the user.
// Returns (true, newOrch) if created, (false, existingOrch) otherwise.
func (m *tabManager) ensure(file string, make func() *orchestrator.Orchestrator) (bool, *orchestrator.Orchestrator) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e, ok := m.entries[file]; ok && e.orch != nil {
		return false, e.orch
	}
	orch := make()
	m.entries[file] = &tabEntry{orch: orch, status: TabStatusEmpty}
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

// remove deletes the tab for file and returns its orchestrator for cleanup.
func (m *tabManager) remove(file string) (*orchestrator.Orchestrator, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[file]
	if !ok {
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

// relocate moves a tab from oldFile to newFile, replacing its orchestrator.
// Returns the old orchestrator for cleanup, or (nil, false) if oldFile not found.
func (m *tabManager) relocate(oldFile, newFile string, make func() *orchestrator.Orchestrator) (*orchestrator.Orchestrator, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[oldFile]
	if !ok {
		return nil, false
	}
	oldOrch := e.orch
	delete(m.entries, oldFile)
	m.entries[newFile] = &tabEntry{orch: make(), status: TabStatusEmpty}
	for i, f := range m.order {
		if f == oldFile {
			m.order[i] = newFile
			break
		}
	}
	if m.activeFile == oldFile {
		m.activeFile = newFile
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
