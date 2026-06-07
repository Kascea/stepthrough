package orchestrator

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kascea/stepthrough/pipeline"
)

func TestWorkspaceRootUsesGitRoot(t *testing.T) {
	got := workspaceRoot("../.azure/pipelines/ci.yml")
	want, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("abs repo root: %v", err)
	}

	if got != want {
		t.Fatalf("workspaceRoot() = %q, want %q", got, want)
	}
}

func TestWorkspaceRootFallsBackToPipelineDirectory(t *testing.T) {
	dir := t.TempDir()
	got := workspaceRoot(filepath.Join(dir, "azure-pipelines.yml"))

	if got != dir {
		t.Fatalf("workspaceRoot() = %q, want %q", got, dir)
	}
}

func TestRunFromCreatesExecutorPerJob(t *testing.T) {
	file := writePipeline(t, `
pool:
  vmImage: ubuntu-latest
stages:
  - stage: Build
    jobs:
      - job: A
        displayName: Job A
        steps:
          - script: echo one
            displayName: One
          - script: echo two
            displayName: Two
      - job: B
        displayName: Job B
        steps:
          - script: echo three
            displayName: Three
`)

	factory := &recordingFactory{}
	_, done := eventRecorder()
	orch := New(factory.newExecutor, done.sink)
	state := orch.LoadPipeline(file)
	if !state.Valid {
		t.Fatalf("pipeline should be valid: %s", state.Error)
	}

	orch.RunFrom(0)
	done.wait(t)

	execs := factory.executors()
	if len(execs) != 2 {
		t.Fatalf("created executors = %d, want 2", len(execs))
	}

	assertExecutor(t, execs[0], "Job A", "stepthrough-run-s0-j0", 2, 1)
	assertExecutor(t, execs[1], "Job B", "stepthrough-run-s0-j1", 1, 1)
}

func TestRunFromSkipsDeploymentJobWithoutExecutor(t *testing.T) {
	file := writePipeline(t, `
pool:
  vmImage: ubuntu-latest
stages:
  - stage: Deploy
    jobs:
      - deployment: DeployProd
        environment: prod
        strategy:
          runOnce:
            deploy:
              steps:
                - script: echo deploy
                  displayName: Deploy
`)

	factory := &recordingFactory{}
	_, done := eventRecorder()
	orch := New(factory.newExecutor, done.sink)
	state := orch.LoadPipeline(file)
	if !state.Valid {
		t.Fatalf("pipeline should be valid: %s", state.Error)
	}

	orch.RunFrom(0)
	done.wait(t)

	if got := len(factory.executors()); got != 0 {
		t.Fatalf("created executors = %d, want 0 for deployment-only job", got)
	}
}

func TestRunFromDoesNotImplicitlyCacheRepeatedStepsAcrossJobs(t *testing.T) {
	file := writePipeline(t, `
pool:
  vmImage: ubuntu-latest
stages:
  - stage: Build
    jobs:
      - job: A
        steps:
          - script: echo shared
            displayName: Shared A
      - job: B
        steps:
          - script: echo shared
            displayName: Shared B
`)

	factory := &recordingFactory{}
	_, done := eventRecorder()
	orch := New(factory.newExecutor, done.sink)
	state := orch.LoadPipeline(file)
	if !state.Valid {
		t.Fatalf("pipeline should be valid: %s", state.Error)
	}

	orch.RunFrom(0)
	done.wait(t)

	execs := factory.executors()
	if len(execs) != 2 {
		t.Fatalf("created executors = %d, want 2 because jobs are isolated", len(execs))
	}
	assertExecutor(t, execs[0], "A", "stepthrough-run-s0-j0", 1, 1)
	assertExecutor(t, execs[1], "B", "stepthrough-run-s0-j1", 1, 1)
}

func TestCleanupCancelsAndCleansActiveJobExecutorOnce(t *testing.T) {
	file := writePipeline(t, `
pool:
  vmImage: ubuntu-latest
steps:
  - script: echo blocking
    displayName: Blocking
`)

	factory := &recordingFactory{blockRun: true}
	_, done := eventRecorder()
	orch := New(factory.newExecutor, done.sink)
	state := orch.LoadPipeline(file)
	if !state.Valid {
		t.Fatalf("pipeline should be valid: %s", state.Error)
	}

	orch.RunFrom(0)
	exec := factory.waitForExecutor(t)
	exec.waitForRun(t)

	orch.Cleanup(context.Background())
	done.wait(t)

	exec.mu.Lock()
	cleanupCalls := exec.cleanupCalls
	exec.mu.Unlock()
	if cleanupCalls != 1 {
		t.Fatalf("cleanup calls = %d, want 1", cleanupCalls)
	}
}

func TestRunFromEmitsSetupErrorEventOnSetupFailure(t *testing.T) {
	file := writePipeline(t, `
pool:
  vmImage: ubuntu-latest
steps:
  - script: echo hello
    displayName: Hello
`)

	factory := &recordingFactory{setupErr: errors.New("Docker is not running — start Docker Desktop and try again")}

	var mu sync.Mutex
	var emitted []string
	doneCh := make(chan struct{})
	var once sync.Once
	sink := func(name string, _ any) {
		mu.Lock()
		emitted = append(emitted, name)
		mu.Unlock()
		if name == "pipeline:done" {
			once.Do(func() { close(doneCh) })
		}
	}

	orch := New(factory.newExecutor, sink)
	state := orch.LoadPipeline(file)
	if !state.Valid {
		t.Fatalf("pipeline should be valid: %s", state.Error)
	}

	orch.RunFrom(0)

	select {
	case <-doneCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for pipeline:done")
	}

	mu.Lock()
	events := append([]string(nil), emitted...)
	mu.Unlock()

	var sawSetupError, sawLegacyError bool
	for _, e := range events {
		if e == "pipeline:setup-error" {
			sawSetupError = true
		}
		if e == "pipeline:error" {
			sawLegacyError = true
		}
	}
	if !sawSetupError {
		t.Errorf("events = %v, want 'pipeline:setup-error' to be emitted", events)
	}
	if sawLegacyError {
		t.Errorf("setup failure emitted legacy 'pipeline:error'; use 'pipeline:setup-error' instead")
	}

	// No steps should run when setup fails.
	execs := factory.executors()
	if len(execs) == 0 {
		t.Fatal("expected an executor to have been created")
	}
	execs[0].mu.Lock()
	runs := execs[0].runCalls
	execs[0].mu.Unlock()
	if runs != 0 {
		t.Errorf("RunStep called %d times, want 0 when setup fails", runs)
	}
}

func TestRunFromEmitsPipelineLoadedWithPendingStepsBeforeExecution(t *testing.T) {
	file := writePipeline(t, `
pool:
  vmImage: ubuntu-latest
steps:
  - script: echo one
    displayName: One
  - script: echo two
    displayName: Two
`)

	factory := &recordingFactory{}

	type namedEvent struct {
		name string
		data any
	}
	// doneChs[0] signals first run done; doneChs[1] signals second run done.
	// Both channels are pre-allocated so the sink never reassigns shared state,
	// avoiding the race on sync.Once that arises from closing a channel and
	// immediately replacing the Once before the goroutine has fully returned.
	doneChs := [2]chan struct{}{make(chan struct{}), make(chan struct{})}
	var mu sync.Mutex
	var captured []namedEvent
	doneCount := 0
	sink := func(name string, data any) {
		mu.Lock()
		captured = append(captured, namedEvent{name, data})
		if name == "pipeline:done" && doneCount < 2 {
			close(doneChs[doneCount])
			doneCount++
		}
		mu.Unlock()
	}

	orch := New(factory.newExecutor, sink)
	orch.LoadPipeline(file)

	// First run — steps complete as passed.
	orch.RunFrom(0)
	select {
	case <-doneChs[0]:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first pipeline:done")
	}

	// Reset captured events for second run.
	mu.Lock()
	captured = captured[:0]
	mu.Unlock()

	orch.RunFrom(0)
	select {
	case <-doneChs[1]:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for second pipeline:done")
	}

	mu.Lock()
	events := append([]namedEvent(nil), captured...)
	mu.Unlock()

	// The very first event on rerun must be pipeline:loaded with all steps pending.
	if len(events) == 0 {
		t.Fatal("no events emitted on rerun")
	}
	first := events[0]
	if first.name != "pipeline:loaded" {
		t.Fatalf("first event on rerun = %q, want \"pipeline:loaded\"", first.name)
	}
	state, ok := first.data.(PipelineState)
	if !ok {
		t.Fatalf("pipeline:loaded data is %T, want PipelineState", first.data)
	}
	for _, s := range state.Steps {
		if s.Status != pipeline.StepStatusPending {
			t.Errorf("step %d status = %q on rerun pipeline:loaded, want %q", s.Index, s.Status, pipeline.StepStatusPending)
		}
	}

	// pipeline:loaded must precede any step:started events.
	for i, e := range events[1:] {
		if e.name == "step:started" {
			break
		}
		if e.name == "pipeline:loaded" {
			t.Errorf("unexpected second pipeline:loaded at position %d before step:started", i+1)
		}
	}
}

func TestStepLogEventsArriveBeforeStepDone(t *testing.T) {
	file := writePipeline(t, `
pool:
  vmImage: ubuntu-latest
steps:
  - script: echo hello
    displayName: Hello
`)

	factory := &recordingFactory{}

	type namedEvent struct {
		name string
		data any
	}
	var mu sync.Mutex
	var captured []namedEvent
	doneCh := make(chan struct{})
	var once sync.Once
	sink := func(name string, data any) {
		mu.Lock()
		captured = append(captured, namedEvent{name, data})
		mu.Unlock()
		if name == "pipeline:done" {
			once.Do(func() { close(doneCh) })
		}
	}

	orch := New(factory.newExecutor, sink)
	orch.LoadPipeline(file)
	orch.RunFrom(0)

	select {
	case <-doneCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for pipeline:done")
	}

	mu.Lock()
	events := append([]namedEvent(nil), captured...)
	mu.Unlock()

	// Verify: every step:done is preceded by at least one step:log for that step.
	logsSeenForStep := map[int]bool{}
	for _, e := range events {
		if e.name == "step:log" {
			ll, ok := e.data.(LogLine)
			if ok {
				logsSeenForStep[ll.StepIndex] = true
			}
		}
		if e.name == "step:done" {
			state, ok := e.data.(PipelineState)
			if !ok {
				continue
			}
			// Find the step that just completed.
			for _, s := range state.Steps {
				if s.Status == pipeline.StepStatusPassed || s.Status == pipeline.StepStatusFailed {
					if !logsSeenForStep[s.Index] {
						t.Errorf("step:done received for step %d before any step:log events", s.Index)
					}
				}
			}
		}
	}
}

func TestSetupLogEventsArriveBeforeStepStarted(t *testing.T) {
	file := writePipeline(t, `
pool:
  vmImage: ubuntu-latest
steps:
  - script: echo hello
    displayName: Hello
`)

	factory := &recordingFactory{}

	var mu sync.Mutex
	var events []string
	doneCh := make(chan struct{})
	var once sync.Once
	sink := func(name string, _ any) {
		mu.Lock()
		events = append(events, name)
		mu.Unlock()
		if name == "pipeline:done" {
			once.Do(func() { close(doneCh) })
		}
	}

	orch := New(factory.newExecutor, sink)
	orch.LoadPipeline(file)
	orch.RunFrom(0)

	select {
	case <-doneCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for pipeline:done")
	}

	mu.Lock()
	evts := append([]string(nil), events...)
	mu.Unlock()

	// Verify setup:log precedes step:started.
	setupLogIdx := -1
	stepStartedIdx := -1
	for i, e := range evts {
		if e == "setup:log" && setupLogIdx == -1 {
			setupLogIdx = i
		}
		if e == "step:started" && stepStartedIdx == -1 {
			stepStartedIdx = i
		}
	}
	if setupLogIdx == -1 {
		t.Fatal("no setup:log event emitted")
	}
	if stepStartedIdx == -1 {
		t.Fatal("no step:started event emitted")
	}
	if setupLogIdx >= stepStartedIdx {
		t.Errorf("setup:log at position %d should precede step:started at position %d", setupLogIdx, stepStartedIdx)
	}
}

func writePipeline(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	file := filepath.Join(dir, "azure-pipelines.yml")
	if err := os.WriteFile(file, []byte(contents), 0o600); err != nil {
		t.Fatalf("write pipeline: %v", err)
	}
	return file
}

type eventLog struct {
	sink func(string, any)
	done chan struct{}
	once sync.Once
}

func eventRecorder() ([]string, *eventLog) {
	var mu sync.Mutex
	var events []string
	log := &eventLog{done: make(chan struct{})}
	log.sink = func(name string, _ any) {
		mu.Lock()
		events = append(events, name)
		mu.Unlock()
		if name == "pipeline:done" {
			log.once.Do(func() { close(log.done) })
		}
	}
	return events, log
}

func (l *eventLog) wait(t *testing.T) {
	t.Helper()
	select {
	case <-l.done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for pipeline:done")
	}
}

type recordingFactory struct {
	mu        sync.Mutex
	created   []*recordingExecutor
	createdCh chan *recordingExecutor
	blockRun  bool
	setupErr  error
}

func (f *recordingFactory) newExecutor(string) Executor {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.createdCh == nil {
		f.createdCh = make(chan *recordingExecutor, 8)
	}
	exec := &recordingExecutor{
		blockRun: f.blockRun,
		setupErr: f.setupErr,
		runCh:    make(chan struct{}),
	}
	f.created = append(f.created, exec)
	f.createdCh <- exec
	return exec
}

func (f *recordingFactory) executors() []*recordingExecutor {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*recordingExecutor, len(f.created))
	copy(out, f.created)
	return out
}

func (f *recordingFactory) waitForExecutor(t *testing.T) *recordingExecutor {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		f.mu.Lock()
		createdCh := f.createdCh
		f.mu.Unlock()
		if createdCh != nil {
			select {
			case exec := <-createdCh:
				return exec
			case <-deadline:
				t.Fatal("timed out waiting for executor")
			}
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for executor")
		case <-time.After(5 * time.Millisecond):
		}
	}
}

type recordingExecutor struct {
	mu            sync.Mutex
	setupJob      string
	containerName string
	runCalls      int
	cleanupCalls  int
	ready         bool
	blockRun      bool
	setupErr      error
	runCh         chan struct{}
	runOnce       sync.Once
}

func (e *recordingExecutor) Setup(_ context.Context, _ *pipeline.Pipeline, job *pipeline.Job, name string, out chan<- string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.setupErr != nil {
		return e.setupErr
	}
	e.setupJob = job.DisplayName
	if e.setupJob == "" {
		e.setupJob = job.Job
	}
	e.containerName = name
	e.ready = true
	out <- "setup " + e.setupJob
	return nil
}

func (e *recordingExecutor) RunStep(ctx context.Context, _ *pipeline.Step, out chan<- string) *pipeline.StepResult {
	e.mu.Lock()
	e.runCalls++
	e.mu.Unlock()
	e.runOnce.Do(func() { close(e.runCh) })
	out <- "run"
	if e.blockRun {
		<-ctx.Done()
		return &pipeline.StepResult{Status: pipeline.StepStatusFailed}
	}
	return &pipeline.StepResult{Status: pipeline.StepStatusPassed}
}

func (e *recordingExecutor) Ready() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.ready
}

func (e *recordingExecutor) Cleanup(context.Context) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cleanupCalls++
}

func (e *recordingExecutor) waitForRun(t *testing.T) {
	t.Helper()
	select {
	case <-e.runCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for RunStep")
	}
}

func assertExecutor(t *testing.T, exec *recordingExecutor, wantJob, wantContainer string, wantRuns, wantCleanup int) {
	t.Helper()
	exec.mu.Lock()
	defer exec.mu.Unlock()
	if exec.setupJob != wantJob {
		t.Fatalf("setup job = %q, want %q", exec.setupJob, wantJob)
	}
	if exec.containerName != wantContainer {
		t.Fatalf("container name = %q, want %q", exec.containerName, wantContainer)
	}
	if exec.runCalls != wantRuns {
		t.Fatalf("run calls = %d, want %d", exec.runCalls, wantRuns)
	}
	if exec.cleanupCalls != wantCleanup {
		t.Fatalf("cleanup calls = %d, want %d", exec.cleanupCalls, wantCleanup)
	}
}
