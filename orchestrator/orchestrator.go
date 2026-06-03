package orchestrator

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
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
	mu        sync.Mutex
	factory   ExecutorFactory
	executor  Executor
	sink      EventSink
	state     PipelineState
	pipeline  *pipeline.Pipeline
	cancelRun context.CancelFunc
	cache     map[string]bool
}

// New returns an Orchestrator wired to the given executor factory and event sink.
func New(factory ExecutorFactory, sink EventSink) *Orchestrator {
	return &Orchestrator{
		factory: factory,
		sink:    sink,
		state:   PipelineState{SafeMode: true},
		cache:   make(map[string]bool),
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
	if o.state.Running && o.cancelRun != nil {
		o.cancelRun()
	}
	if !o.state.Valid || o.pipeline == nil {
		o.mu.Unlock()
		return
	}
	p := o.pipeline
	steps := o.state.Steps
	o.state.Running = true
	ctx, cancel := context.WithCancel(context.Background())
	o.cancelRun = cancel
	o.mu.Unlock()

	go o.runSteps(ctx, p, steps, startIndex)
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
	flat := pipeline.Flatten(o.pipeline)
	for i := idx; i < len(o.state.Steps) && i < len(flat); i++ {
		hash := stepHash(flat[i].Step, o.state.Variables)
		delete(o.cache, hash)
		o.state.Steps[i].Status = pipeline.StepStatusPending
	}
}

// Cleanup cancels any running execution and tears down the executor.
func (o *Orchestrator) Cleanup(ctx context.Context) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.cancelRun != nil {
		o.cancelRun()
	}
	if o.executor != nil {
		o.executor.Cleanup(ctx)
		o.executor = nil
	}
}

func (o *Orchestrator) runSteps(ctx context.Context, p *pipeline.Pipeline, steps []StepState, startIndex int) {
	defer func() {
		o.mu.Lock()
		o.state.Running = false
		o.mu.Unlock()
		o.sink("pipeline:done", o.GetState())
	}()

	workDir := workspaceRoot(o.state.File)

	o.mu.Lock()
	if o.executor == nil {
		o.executor = o.factory(workDir)
	}
	exec := o.executor
	o.mu.Unlock()

	if !exec.Ready() {
		setupCh := make(chan string, 100)
		go func() {
			for line := range setupCh {
				o.sink("setup:log", line)
			}
		}()
		err := exec.Setup(ctx, p, findFirstJob(p), "stepthrough-run", setupCh)
		close(setupCh)
		if err != nil {
			o.sink("pipeline:error", err.Error())
			return
		}
	}

	flat := pipeline.Flatten(p)

	for i := startIndex; i < len(flat) && i < len(steps); i++ {
		select {
		case <-ctx.Done():
			return
		default:
		}

		fs := flat[i]

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

		hash := stepHash(fs.Step, o.state.Variables)

		if o.cache[hash] {
			o.updateStep(i, func(st *StepState) { st.Status = pipeline.StepStatusCached })
			o.sink("step:cached", i)
			continue
		}

		o.updateStep(i, func(st *StepState) { st.Status = pipeline.StepStatusRunning })
		o.sink("step:started", i)

		logCh := make(chan string, 200)
		go func(idx int) {
			for line := range logCh {
				o.sink("step:log", LogLine{StepIndex: idx, Line: line})
			}
		}(i)

		result := exec.RunStep(ctx, fs.Step, logCh)
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

		if status == pipeline.StepStatusPassed || status == pipeline.StepStatusSkipped {
			o.cache[hash] = true
		} else if !fs.Step.ContinueOnError {
			return
		}
	}
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

func findFirstJob(p *pipeline.Pipeline) *pipeline.Job {
	for si := range p.Stages {
		if len(p.Stages[si].Jobs) > 0 {
			return &p.Stages[si].Jobs[0]
		}
	}
	return nil
}

func stepHash(step *pipeline.Step, vars map[string]string) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%s|%s|%s|%v", step.Task, step.Script, step.Bash, step.Pwsh, step.Inputs)
	stepText := fmt.Sprintf("%v %s %s %s %v", step.Inputs, step.Script, step.Bash, step.Pwsh, step.Env)
	varRef := regexp.MustCompile(`\$\((\w+)\)|\$\{?(\w+)\}?`)
	for _, m := range varRef.FindAllStringSubmatch(stepText, -1) {
		name := m[1]
		if name == "" {
			name = m[2]
		}
		if v, ok := vars[name]; ok {
			fmt.Fprintf(h, "|%s=%s", name, v)
		}
	}
	return fmt.Sprintf("%x", h.Sum(nil))
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
