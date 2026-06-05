package orchestrator

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/colecarlson/stepthrough/pipeline"
)

// Executor runs pipeline steps in an isolated environment (e.g. a Docker container).
// runner.Engine satisfies this interface via Go's structural typing.
type Executor interface {
	Setup(ctx context.Context, p *pipeline.Pipeline, job *pipeline.Job, name string, out chan<- string) error
	RunStep(ctx context.Context, step *pipeline.Step, out chan<- string) *pipeline.StepResult
	Ready() bool
	Cleanup(ctx context.Context)
}

// ExecutorFactory creates an Executor for the given working directory.
// Lazy creation allows the working directory to be resolved from the pipeline file path at run time.
type ExecutorFactory func(workDir string) Executor

// EventSink receives pipeline lifecycle events for forwarding to a UI layer.
type EventSink func(event string, data any)

// StepState is the frontend view of a single pipeline step.
type StepState struct {
	Index           int                 `json:"index"`
	StageName       string              `json:"stageName"`
	JobName         string              `json:"jobName"`
	Label           string              `json:"label"`
	Type            string              `json:"type"`
	Status          pipeline.StepStatus `json:"status"`
	ExitCode        int                 `json:"exitCode"`
	DurationMs      int64               `json:"durationMs"`
	ResolvedEnv     map[string]string   `json:"resolvedEnv"`
	IsDeploymentJob bool                `json:"isDeploymentJob"`
}

// PipelineState is the full frontend-visible state of the pipeline.
type PipelineState struct {
	File      string            `json:"file"`
	Valid     bool              `json:"valid"`
	Error     string            `json:"error"`
	Steps     []StepState       `json:"steps"`
	Variables map[string]string `json:"variables"`
	SafeMode  bool              `json:"safeMode"`
	Running   bool              `json:"running"`
}

// LogLine is emitted per log line during step execution.
type LogLine struct {
	StepIndex int    `json:"stepIndex"`
	Line      string `json:"line"`
}

// Orchestrator manages pipeline loading, caching, and step execution.
// It has no dependency on any UI framework and can be driven headlessly.
type Orchestrator struct {
	mu              sync.Mutex
	factory         ExecutorFactory
	activeExecutors map[string]Executor
	sink            EventSink
	state           PipelineState
	pipeline        *pipeline.Pipeline
	cancelRun       context.CancelFunc
	runID           int
}

// New returns an Orchestrator wired to the given executor factory and event sink.
func New(factory ExecutorFactory, sink EventSink) *Orchestrator {
	return &Orchestrator{
		factory:         factory,
		sink:            sink,
		state:           PipelineState{SafeMode: true},
		activeExecutors: make(map[string]Executor),
	}
}

// LoadPipeline parses the YAML file and returns the new state.
func (o *Orchestrator) LoadPipeline(file string) PipelineState {
	o.mu.Lock()
	defer o.mu.Unlock()

	p, err := pipeline.Parse(file)
	if err != nil {
		o.state.File = file
		o.state.Valid = false
		o.state.Error = err.Error()
		o.state.Steps = nil
		return o.state
	}

	o.pipeline = p
	o.state.File = file
	o.state.Valid = true
	o.state.Error = ""
	o.state.Variables = p.Variables
	if o.state.Variables == nil {
		o.state.Variables = map[string]string{}
	}
	o.state.Steps = buildStepStates(p)
	return o.state
}

// GetState returns the current pipeline state.
func (o *Orchestrator) GetState() PipelineState {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.state
}

// SetSafeMode toggles safe mode.
func (o *Orchestrator) SetSafeMode(enabled bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.state.SafeMode = enabled
}

// RunFrom starts execution from the given step index, skipping cached steps.
func (o *Orchestrator) RunFrom(startIndex int) {
	o.mu.Lock()
	var staleExecutors []Executor
	if o.state.Running && o.cancelRun != nil {
		o.cancelRun()
		staleExecutors = o.drainActiveExecutorsLocked()
	}
	if !o.state.Valid || o.pipeline == nil {
		o.mu.Unlock()
		return
	}
	p := o.pipeline
	steps := o.state.Steps
	file := o.state.File
	o.state.Running = true
	o.runID++
	runID := o.runID
	ctx, cancel := context.WithCancel(context.Background())
	o.cancelRun = cancel
	o.mu.Unlock()

	for _, exec := range staleExecutors {
		exec.Cleanup(context.Background())
	}

	go o.runSteps(ctx, p, steps, startIndex, file, runID)
}

// CancelRun cancels any running execution.
func (o *Orchestrator) CancelRun() {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.cancelRun != nil {
		o.cancelRun()
	}
}

// InvalidateFrom marks all steps at or after idx as pending and clears their cache entries.
func (o *Orchestrator) InvalidateFrom(idx int) {
	o.mu.Lock()
	defer o.mu.Unlock()
	for i := idx; i < len(o.state.Steps); i++ {
		o.state.Steps[i].Status = pipeline.StepStatusPending
	}
}

// Cleanup cancels any running execution and tears down active job executors.
func (o *Orchestrator) Cleanup(ctx context.Context) {
	o.mu.Lock()
	if o.cancelRun != nil {
		o.cancelRun()
	}
	executors := o.drainActiveExecutorsLocked()
	o.mu.Unlock()

	for _, exec := range executors {
		exec.Cleanup(ctx)
	}
}

func (o *Orchestrator) runSteps(ctx context.Context, p *pipeline.Pipeline, steps []StepState, startIndex int, file string, runID int) {
	defer func() {
		o.mu.Lock()
		current := o.runID == runID
		if current {
			o.state.Running = false
		}
		o.mu.Unlock()
		if current {
			o.sink("pipeline:done", o.GetState())
		}
	}()

	workDir := workspaceRoot(file)

	flat := pipeline.Flatten(p)
	var currentKey string
	var currentExec Executor
	cleanupCurrent := func() {
		if currentExec == nil {
			return
		}
		if o.unregisterExecutor(currentKey) {
			currentExec.Cleanup(context.Background())
		}
		currentExec = nil
		currentKey = ""
	}
	defer cleanupCurrent()

	for i := startIndex; i < len(flat) && i < len(steps); i++ {
		select {
		case <-ctx.Done():
			return
		default:
		}

		fs := flat[i]
		key := jobRuntimeKey(fs)
		if currentExec != nil && currentKey != key {
			cleanupCurrent()
		}

		// Deployment jobs are never executed locally — display only.
		if fs.IsDeploymentJob {
			logCh := make(chan string, 1)
			go func(idx int) {
				for line := range logCh {
					o.sink("step:log", LogLine{StepIndex: idx, Line: line})
				}
			}(i)
			logCh <- "[stepthrough] deployment jobs are not executed locally — skipping"
			close(logCh)
			o.sink("step:done", i)
			continue
		}

		if currentExec == nil {
			job := &p.Stages[fs.StageIndex].Jobs[fs.JobIndex]
			exec := o.factory(workDir)
			currentKey = key
			currentExec = exec
			o.registerExecutor(key, exec)

			setupCh := make(chan string, 100)
			go func() {
				for line := range setupCh {
					o.sink("setup:log", line)
				}
			}()
			setupCh <- fmt.Sprintf("[stepthrough] setting up job %s", fs.JobName)
			err := exec.Setup(ctx, p, job, containerNameForJob(fs), setupCh)
			close(setupCh)
			if err != nil {
				o.sink("pipeline:setup-error", err.Error())
				return
			}
		}

		o.updateStep(i, func(st *StepState) { st.Status = pipeline.StepStatusRunning })
		o.sink("step:started", i)

		logCh := make(chan string, 200)
		go func(idx int) {
			for line := range logCh {
				o.sink("step:log", LogLine{StepIndex: idx, Line: line})
			}
		}(i)

		result := currentExec.RunStep(ctx, fs.Step, logCh)
		close(logCh)

		status := pipeline.StepStatusPassed
		if result.Status == pipeline.StepStatusFailed {
			status = pipeline.StepStatusFailed
		} else if result.Status == pipeline.StepStatusSkipped {
			status = pipeline.StepStatusSkipped
		}

		o.updateStep(i, func(st *StepState) {
			st.Status = status
			st.ExitCode = result.ExitCode
			st.DurationMs = result.Duration.Milliseconds()
		})
		o.sink("step:done", i)

		if status != pipeline.StepStatusPassed && status != pipeline.StepStatusSkipped && !fs.Step.ContinueOnError {
			return
		}
	}
}

func (o *Orchestrator) registerExecutor(key string, exec Executor) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.activeExecutors[key] = exec
}

func (o *Orchestrator) unregisterExecutor(key string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	if _, ok := o.activeExecutors[key]; !ok {
		return false
	}
	delete(o.activeExecutors, key)
	return true
}

func (o *Orchestrator) drainActiveExecutorsLocked() []Executor {
	executors := make([]Executor, 0, len(o.activeExecutors))
	for key, exec := range o.activeExecutors {
		executors = append(executors, exec)
		delete(o.activeExecutors, key)
	}
	return executors
}

func (o *Orchestrator) updateStep(idx int, fn func(*StepState)) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if idx < len(o.state.Steps) {
		fn(&o.state.Steps[idx])
	}
}

func buildStepStates(p *pipeline.Pipeline) []StepState {
	flat := pipeline.Flatten(p)
	states := make([]StepState, len(flat))
	for i, fs := range flat {
		status := pipeline.StepStatusPending
		if fs.IsDeploymentJob {
			status = pipeline.StepStatusDeployment
		}
		states[i] = StepState{
			Index:           i,
			StageName:       fs.StageName,
			JobName:         fs.JobName,
			Label:           fs.Step.Label(),
			Type:            string(fs.Step.Type()),
			Status:          status,
			IsDeploymentJob: fs.IsDeploymentJob,
		}
	}
	return states
}

func jobRuntimeKey(fs pipeline.FlatStep) string {
	return fmt.Sprintf("stage-%d-job-%d", fs.StageIndex, fs.JobIndex)
}

func containerNameForJob(fs pipeline.FlatStep) string {
	return fmt.Sprintf("stepthrough-run-s%d-j%d", fs.StageIndex, fs.JobIndex)
}

func workspaceRoot(filePath string) string {
	dir := filepath.Dir(filePath)
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}

	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err == nil {
		root := strings.TrimSpace(string(out))
		if root != "" {
			return root
		}
	}

	return dir
}
