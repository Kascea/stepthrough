package service

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/kascea/stepthrough/orchestrator"
	"github.com/kascea/stepthrough/runner"
	"github.com/kascea/stepthrough/session"
)

// PipelineFileEvent wraps any pipeline event with the file it originated from.
type PipelineFileEvent struct {
	File string `json:"file"`
	Data any    `json:"data"`
}

// SessionData is returned to the frontend on startup to restore tabs.
type SessionData struct {
	TabOrder   []string                    `json:"tabOrder"`
	ActiveFile string                      `json:"activeFile"`
	Runs       map[string]session.SavedRun `json:"runs"`
}

// PipelineService is a thin façade over tabManager and logStore.
// It wires orchestrator event sinks, handles session persistence, and exposes
// all Wails RPC methods. Structural logic lives in the sub-modules.
type PipelineService struct {
	emitFn  func(event string, data any)
	factory orchestrator.ExecutorFactory
	tabs    *tabManager
	logs    *logStore

	mu          sync.Mutex
	pendingFile string // set by main.go for CLI arg mode
}

func NewPipelineService(factory orchestrator.ExecutorFactory) *PipelineService {
	svc := &PipelineService{
		factory: factory,
		logs:    newLogStore(),
	}
	svc.tabs = newTabManager(
		func(file string) { svc.ReloadFile(file) },
		func(err string) { svc.emit("watcher:error", err) },
	)
	return svc
}

func (s *PipelineService) SetEmitter(fn func(event string, data any)) { s.emitFn = fn }

// SetPendingFile stores a CLI-provided pipeline file to open on startup.
func (s *PipelineService) SetPendingFile(file string) {
	s.mu.Lock()
	s.pendingFile = file
	s.mu.Unlock()
}

func (s *PipelineService) emit(event string, data any) {
	if s.emitFn != nil {
		s.emitFn(event, data)
	}
}

func (s *PipelineService) fileEmit(file, event string, data any) {
	s.emit(event, PipelineFileEvent{File: file, Data: data})
}

func (s *PipelineService) orchestratorSink(file string) orchestrator.EventSink {
	return func(event string, data any) {
		if event == "step:log" {
			if ll, ok := data.(orchestrator.LogLine); ok {
				s.logs.capture(file, ll)
			}
		}
		s.fileEmit(file, event, data)
		switch event {
		case "pipeline:done":
			s.tabs.setStatus(file, TabStatusLoaded)
			go s.persistSession()
		case "pipeline:setup-error":
			s.tabs.setStatus(file, TabStatusError)
		}
	}
}

func (s *PipelineService) persistSession() {
	order, activeFile, orchs := s.tabs.snapshot()
	logsSnap := s.logs.snapshot()

	d := session.Load()
	d.TabOrder = order
	d.ActiveFile = activeFile

	// Only remove runs for files no longer in the tab order — not merely
	// for files without an orchestrator yet (e.g. restored but not yet run).
	inOrder := make(map[string]bool, len(order))
	for _, f := range order {
		inOrder[f] = true
	}
	for file := range d.Runs {
		if !inOrder[file] {
			delete(d.Runs, file)
		}
	}

	for file, orch := range orchs {
		state := orch.GetState()
		run := d.Runs[file]
		run.Steps = state.Steps
		run.Logs = logsSnap[file]
		run.RanAt = time.Now()
		d.Runs[file] = run
	}

	session.Save(d)
}

// GetSession returns saved session data so the frontend can restore tabs on startup.
// Seeds the tab order from the session so persistSession doesn't overwrite it with
// an empty slice before RestoreTab has had a chance to create orchestrators.
func (s *PipelineService) GetSession() SessionData {
	d := session.Load()
	s.tabs.seedOrder(d.TabOrder, d.ActiveFile)

	tabOrder := d.TabOrder
	if tabOrder == nil {
		tabOrder = []string{}
	}
	return SessionData{
		TabOrder:   tabOrder,
		ActiveFile: d.ActiveFile,
		Runs:       d.Runs,
	}
}

// EnsureTab registers an orchestrator for file if one doesn't already exist.
// Returns true if a new tab was created.
func (s *PipelineService) EnsureTab(file string) bool {
	created, _ := s.tabs.ensure(file, func() *orchestrator.Orchestrator {
		return orchestrator.New(s.factory, s.orchestratorSink(file))
	})
	return created
}

// ReloadFile loads the pipeline YAML and, on success, runs from step 0.
// Called by WatcherService for initial loads and hot-reloads.
func (s *PipelineService) ReloadFile(file string) {
	orch, ok := s.tabs.get(file)
	if !ok {
		return
	}

	state := orch.LoadPipeline(file)
	if !state.Valid {
		s.tabs.setStatus(file, TabStatusError)
		s.fileEmit(file, "pipeline:error", state.Error)
		return
	}

	s.logs.clear(file)
	s.fileEmit(file, "pipeline:loaded", state)
	s.tabs.setStatus(file, TabStatusRunning)
	orch.RunFrom(0)
}

// RestoreTab creates an orchestrator for a session-restored file without running it.
// Emits pipeline:loaded if the file is valid, or pipeline:missing if absent.
func (s *PipelineService) RestoreTab(file string) {
	orch := s.tabs.addOrch(file, func() *orchestrator.Orchestrator {
		return orchestrator.New(s.factory, s.orchestratorSink(file))
	})

	if _, err := os.Stat(file); os.IsNotExist(err) {
		s.tabs.setStatus(file, TabStatusMissing)
		s.fileEmit(file, "pipeline:missing", nil)
		return
	}

	state := orch.LoadPipeline(file)
	if !state.Valid {
		s.tabs.setStatus(file, TabStatusError)
		s.fileEmit(file, "pipeline:error", state.Error)
		return
	}
	s.tabs.setStatus(file, TabStatusLoaded)
	s.fileEmit(file, "pipeline:loaded", state)
}

// SetActiveTab records which tab is currently focused.
func (s *PipelineService) SetActiveTab(file string) {
	s.tabs.setActive(file)
	go s.persistSession()
}

// RemoveTab closes a pipeline tab and cleans up its orchestrator.
// The tab's file watcher is closed by tabManager.remove.
func (s *PipelineService) RemoveTab(file string) {
	orch, ok := s.tabs.remove(file)
	if !ok {
		return
	}
	s.logs.remove(file)
	go func() {
		if orch != nil {
			orch.Cleanup(context.Background())
		}
		s.persistSession()
	}()
}

// AddTab opens a new pipeline tab for file, starts watching it, and triggers an
// initial load+run. If the tab already exists this is a no-op.
func (s *PipelineService) AddTab(file string) {
	abs, err := filepath.Abs(file)
	if err != nil {
		return
	}
	created, _ := s.tabs.ensure(abs, func() *orchestrator.Orchestrator {
		return orchestrator.New(s.factory, s.orchestratorSink(abs))
	})
	if created {
		s.emit("pipeline:tab:added", abs)
		s.ReloadFile(abs)
	}
}

// RunPipeline starts running a specific pipeline from the given step index.
func (s *PipelineService) RunPipeline(file string, startIndex int) {
	orch, ok := s.tabs.get(file)
	if !ok {
		return
	}
	s.logs.clear(file)
	s.tabs.setStatus(file, TabStatusRunning)
	orch.RunFrom(startIndex)
}

// CancelPipeline cancels a running pipeline.
func (s *PipelineService) CancelPipeline(file string) {
	if orch, ok := s.tabs.get(file); ok {
		orch.CancelRun()
	}
}

// GetPipelineState returns the current state of a specific pipeline.
func (s *PipelineService) GetPipelineState(file string) orchestrator.PipelineState {
	orch, ok := s.tabs.get(file)
	if !ok {
		return orchestrator.PipelineState{File: file}
	}
	return orch.GetState()
}

// InvalidatePipelineFrom marks steps at or after idx as pending.
func (s *PipelineService) InvalidatePipelineFrom(file string, idx int) {
	if orch, ok := s.tabs.get(file); ok {
		orch.InvalidateFrom(idx)
	}
}

// RelocatePipeline updates a tab's file path when the user locates a moved file.
func (s *PipelineService) RelocatePipeline(oldFile, newFile string) {
	oldOrch, ok := s.tabs.relocate(oldFile, newFile, func() *orchestrator.Orchestrator {
		return orchestrator.New(s.factory, s.orchestratorSink(newFile))
	})
	if !ok {
		return
	}
	s.logs.rename(oldFile, newFile)
	go oldOrch.Cleanup(context.Background())
	s.fileEmit(newFile, "pipeline:tab:relocated", map[string]string{"from": oldFile, "to": newFile})
}

// CheckDockerReady returns true if Docker is available.
func (s *PipelineService) CheckDockerReady() bool {
	return runner.IsDockerAvailable()
}

// Cleanup shuts down all orchestrators.
func (s *PipelineService) Cleanup() {
	for _, orch := range s.tabs.all() {
		orch.Cleanup(context.Background())
	}
}
