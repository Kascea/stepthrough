package service

import (
	"context"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/colecarlson/stepthrough/orchestrator"
	"github.com/colecarlson/stepthrough/runner"
	"github.com/colecarlson/stepthrough/session"
)

// PipelineFileEvent wraps any pipeline event with the file it originated from.
type PipelineFileEvent struct {
	File string `json:"file"`
	Data any    `json:"data"`
}

// SessionData is returned to the frontend on startup to restore tabs.
type SessionData struct {
	TabOrder   []string                   `json:"tabOrder"`
	ActiveFile string                     `json:"activeFile"`
	Runs       map[string]session.SavedRun `json:"runs"`
}

// PipelineService manages multiple pipeline tabs and their orchestrators.
type PipelineService struct {
	app     *application.App
	factory orchestrator.ExecutorFactory

	mu            sync.Mutex
	orchestrators map[string]*orchestrator.Orchestrator
	tabOrder      []string
	activeFile    string
	savedLogs     map[string]map[string][]string // file → step-index-str → log lines

	pendingFile string // set by main.go for CLI arg mode
}

func NewPipelineService(factory orchestrator.ExecutorFactory) *PipelineService {
	return &PipelineService{
		factory:       factory,
		orchestrators: make(map[string]*orchestrator.Orchestrator),
		savedLogs:     make(map[string]map[string][]string),
	}
}

func (s *PipelineService) SetApp(app *application.App) { s.app = app }

// SetPendingFile stores a CLI-provided pipeline file to open on startup.
func (s *PipelineService) SetPendingFile(file string) {
	s.mu.Lock()
	s.pendingFile = file
	s.mu.Unlock()
}

func (s *PipelineService) emit(event string, data any) {
	if s.app != nil {
		s.app.Event.Emit(event, data)
	}
}

func (s *PipelineService) fileEmit(file, event string, data any) {
	s.emit(event, PipelineFileEvent{File: file, Data: data})
}

func (s *PipelineService) orchestratorSink(file string) orchestrator.EventSink {
	return func(event string, data any) {
		if event == "step:log" {
			if ll, ok := data.(orchestrator.LogLine); ok {
				s.captureLog(file, ll)
			}
		}
		s.fileEmit(file, event, data)
		if event == "pipeline:done" {
			go s.persistSession()
		}
	}
}

func (s *PipelineService) captureLog(file string, ll orchestrator.LogLine) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.savedLogs[file] == nil {
		s.savedLogs[file] = make(map[string][]string)
	}
	key := strconv.Itoa(ll.StepIndex)
	lines := s.savedLogs[file][key]
	if len(lines) < session.MaxLogsPerStep {
		s.savedLogs[file][key] = append(lines, ll.Line)
	}
}

func (s *PipelineService) persistSession() {
	s.mu.Lock()
	tabOrder := make([]string, len(s.tabOrder))
	copy(tabOrder, s.tabOrder)
	activeFile := s.activeFile
	orchSnap := make(map[string]*orchestrator.Orchestrator, len(s.orchestrators))
	for k, v := range s.orchestrators {
		orchSnap[k] = v
	}
	logsSnap := make(map[string]map[string][]string, len(s.savedLogs))
	for k, v := range s.savedLogs {
		cp := make(map[string][]string, len(v))
		for sk, sv := range v {
			cp[sk] = sv
		}
		logsSnap[k] = cp
	}
	s.mu.Unlock()

	d := session.Load()
	d.TabOrder = tabOrder
	d.ActiveFile = activeFile

	// Remove runs for closed tabs.
	for file := range d.Runs {
		if _, open := orchSnap[file]; !open {
			delete(d.Runs, file)
		}
	}

	for file, orch := range orchSnap {
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
// Also initialises the service's tabOrder from the session so persistSession stays correct.
func (s *PipelineService) GetSession() SessionData {
	d := session.Load()

	s.mu.Lock()
	if len(s.tabOrder) == 0 && len(d.TabOrder) > 0 {
		s.tabOrder = make([]string, len(d.TabOrder))
		copy(s.tabOrder, d.TabOrder)
		s.activeFile = d.ActiveFile
	}
	s.mu.Unlock()

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
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.orchestrators[file]; exists {
		return false
	}
	orch := orchestrator.New(s.factory, s.orchestratorSink(file))
	s.orchestrators[file] = orch
	s.tabOrder = append(s.tabOrder, file)
	s.activeFile = file
	return true
}

// ReloadFile loads the pipeline YAML and, on success, runs from step 0.
// Called by WatcherService for initial loads and hot-reloads.
func (s *PipelineService) ReloadFile(file string) {
	s.mu.Lock()
	orch, ok := s.orchestrators[file]
	s.mu.Unlock()
	if !ok {
		return
	}

	state := orch.LoadPipeline(file)
	if !state.Valid {
		s.fileEmit(file, "pipeline:error", state.Error)
		return
	}

	s.mu.Lock()
	s.savedLogs[file] = make(map[string][]string)
	s.mu.Unlock()

	s.fileEmit(file, "pipeline:loaded", state)
	orch.RunFrom(0)
}

// RestoreTab creates an orchestrator for a session-restored file without running it.
// Emits pipeline:loaded if the file is valid, or pipeline:missing if absent.
func (s *PipelineService) RestoreTab(file string) {
	s.mu.Lock()
	if _, exists := s.orchestrators[file]; !exists {
		orch := orchestrator.New(s.factory, s.orchestratorSink(file))
		s.orchestrators[file] = orch
	}
	s.mu.Unlock()

	s.mu.Lock()
	orch := s.orchestrators[file]
	s.mu.Unlock()

	if _, err := os.Stat(file); os.IsNotExist(err) {
		s.fileEmit(file, "pipeline:missing", nil)
		return
	}

	state := orch.LoadPipeline(file)
	if !state.Valid {
		s.fileEmit(file, "pipeline:error", state.Error)
		return
	}
	s.fileEmit(file, "pipeline:loaded", state)
}

// SetActiveTab records which tab is currently focused.
func (s *PipelineService) SetActiveTab(file string) {
	s.mu.Lock()
	s.activeFile = file
	s.mu.Unlock()
	go s.persistSession()
}

// RemoveTab closes a pipeline tab and cleans up its orchestrator.
func (s *PipelineService) RemoveTab(file string) {
	s.mu.Lock()
	orch, ok := s.orchestrators[file]
	if !ok {
		s.mu.Unlock()
		return
	}
	delete(s.orchestrators, file)
	delete(s.savedLogs, file)
	for i, f := range s.tabOrder {
		if f == file {
			s.tabOrder = append(s.tabOrder[:i], s.tabOrder[i+1:]...)
			break
		}
	}
	if s.activeFile == file {
		if len(s.tabOrder) > 0 {
			s.activeFile = s.tabOrder[0]
		} else {
			s.activeFile = ""
		}
	}
	s.mu.Unlock()

	go func() {
		orch.Cleanup(context.Background())
		s.persistSession()
	}()
}

// RunPipeline starts running a specific pipeline from the given step index.
func (s *PipelineService) RunPipeline(file string, startIndex int) {
	s.mu.Lock()
	orch, ok := s.orchestrators[file]
	s.mu.Unlock()
	if !ok {
		return
	}
	s.mu.Lock()
	s.savedLogs[file] = make(map[string][]string)
	s.mu.Unlock()
	orch.RunFrom(startIndex)
}

// CancelPipeline cancels a running pipeline.
func (s *PipelineService) CancelPipeline(file string) {
	s.mu.Lock()
	orch, ok := s.orchestrators[file]
	s.mu.Unlock()
	if ok {
		orch.CancelRun()
	}
}

// GetPipelineState returns the current state of a specific pipeline.
func (s *PipelineService) GetPipelineState(file string) orchestrator.PipelineState {
	s.mu.Lock()
	orch, ok := s.orchestrators[file]
	s.mu.Unlock()
	if !ok {
		return orchestrator.PipelineState{File: file}
	}
	return orch.GetState()
}

// InvalidatePipelineFrom marks steps at or after idx as pending.
func (s *PipelineService) InvalidatePipelineFrom(file string, idx int) {
	s.mu.Lock()
	orch, ok := s.orchestrators[file]
	s.mu.Unlock()
	if ok {
		orch.InvalidateFrom(idx)
	}
}

// RelocatePipeline updates a tab's file path when the user locates a moved file.
func (s *PipelineService) RelocatePipeline(oldFile, newFile string) {
	s.mu.Lock()
	oldOrch, ok := s.orchestrators[oldFile]
	if !ok {
		s.mu.Unlock()
		return
	}
	delete(s.orchestrators, oldFile)
	newOrch := orchestrator.New(s.factory, s.orchestratorSink(newFile))
	s.orchestrators[newFile] = newOrch
	for i, f := range s.tabOrder {
		if f == oldFile {
			s.tabOrder[i] = newFile
			break
		}
	}
	if s.activeFile == oldFile {
		s.activeFile = newFile
	}
	if logs, ok := s.savedLogs[oldFile]; ok {
		s.savedLogs[newFile] = logs
		delete(s.savedLogs, oldFile)
	}
	s.mu.Unlock()

	go oldOrch.Cleanup(context.Background())
	s.fileEmit(newFile, "pipeline:tab:relocated", map[string]string{"from": oldFile, "to": newFile})
}

// CheckDockerReady returns true if Docker is available.
func (s *PipelineService) CheckDockerReady() bool {
	return runner.IsDockerAvailable()
}

// Cleanup shuts down all orchestrators.
func (s *PipelineService) Cleanup() {
	s.mu.Lock()
	orchs := make([]*orchestrator.Orchestrator, 0, len(s.orchestrators))
	for _, orch := range s.orchestrators {
		orchs = append(orchs, orch)
	}
	s.mu.Unlock()
	for _, orch := range orchs {
		orch.Cleanup(context.Background())
	}
}
