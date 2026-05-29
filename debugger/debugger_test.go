package debugger_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/colecarlson/stepthrough/debugger"
	"github.com/colecarlson/stepthrough/pipeline"
	"github.com/colecarlson/stepthrough/runner"
)

// dryRunExecutor simulates step execution without Docker.
type dryRunExecutor struct {
	failSteps map[string]bool // step labels that should fail
}

func (e *dryRunExecutor) RunStep(_ context.Context, step *pipeline.Step, outputCh chan<- string) *runner.StepResult {
	outputCh <- fmt.Sprintf("[dry-run] %s", step.Label())
	time.Sleep(time.Millisecond) // simulate tiny execution time
	if e.failSteps[step.Label()] {
		return &runner.StepResult{Status: runner.StatusFailed, ExitCode: 1}
	}
	return &runner.StepResult{Status: runner.StatusPassed, ExitCode: 0}
}

const simplePipeline = `
stages:
  - stage: Build
    jobs:
      - job: BuildJob
        steps:
          - script: echo step1
            displayName: Step 1
          - script: echo step2
            displayName: Step 2
          - script: echo step3
            displayName: Step 3
`

const multiStagePipeline = `
stages:
  - stage: A
    jobs:
      - job: AJob
        steps:
          - script: echo a1
            displayName: A1
          - script: echo a2
            displayName: A2
  - stage: B
    dependsOn: A
    jobs:
      - job: BJob
        steps:
          - script: echo b1
            displayName: B1
`

func mustParse(t *testing.T, yaml string) *pipeline.Pipeline {
	t.Helper()
	p, err := pipeline.ParseBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return p
}

func mustFlatten(t *testing.T, yaml string) []pipeline.FlatStep {
	t.Helper()
	return pipeline.Flatten(mustParse(t, yaml))
}

func newSession(t *testing.T, yaml string, failSteps ...string) *debugger.Session {
	t.Helper()
	steps := mustFlatten(t, yaml)
	fail := make(map[string]bool)
	for _, s := range failSteps {
		fail[s] = true
	}
	exec := &dryRunExecutor{failSteps: fail}
	return debugger.New(steps, nil, exec)
}

func drainOutput(ch chan string) {
	for range ch {
	}
}

// runStep is a test helper that drains the output channel.
func runStep(t *testing.T, s *debugger.Session) *debugger.StepResult {
	t.Helper()
	ch := make(chan string, 64)
	go drainOutput(ch)
	result, err := s.Step(context.Background(), ch)
	if err != nil {
		t.Fatalf("step: %v", err)
	}
	return result
}

func runContinue(t *testing.T, s *debugger.Session) []*debugger.StepResult {
	t.Helper()
	ch := make(chan string, 256)
	go drainOutput(ch)
	results, err := s.Continue(context.Background(), ch)
	if err != nil {
		t.Fatalf("continue: %v", err)
	}
	return results
}

// ── Tests ────────────────────────────────────────────────────────────────────

func TestNewSession(t *testing.T) {
	s := newSession(t, simplePipeline)

	if s.Cursor() != 0 {
		t.Errorf("cursor = %d, want 0", s.Cursor())
	}
	if s.IsDone() {
		t.Error("should not be done initially")
	}
	if !s.IsPaused() {
		t.Error("should start paused")
	}
	if len(s.Steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(s.Steps))
	}
}

func TestStep(t *testing.T) {
	s := newSession(t, simplePipeline)

	result := runStep(t, s)
	if result.Status != debugger.StatusPassed {
		t.Errorf("status = %v, want passed", result.Status)
	}
	if s.Cursor() != 1 {
		t.Errorf("cursor = %d, want 1", s.Cursor())
	}
	if !s.IsPaused() {
		t.Error("should be paused after step")
	}
}

func TestStepAll(t *testing.T) {
	s := newSession(t, simplePipeline)

	for i := 0; i < 3; i++ {
		runStep(t, s)
	}
	if !s.IsDone() {
		t.Error("should be done after all steps")
	}
}

func TestStepAfterDone(t *testing.T) {
	s := newSession(t, simplePipeline)
	for i := 0; i < 3; i++ {
		runStep(t, s)
	}

	ch := make(chan string, 64)
	go drainOutput(ch)
	_, err := s.Step(context.Background(), ch)
	if err == nil {
		t.Error("expected error stepping past done")
	}
}

func TestContinueRunsAll(t *testing.T) {
	s := newSession(t, simplePipeline)

	results := runContinue(t, s)
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}
	if !s.IsDone() {
		t.Error("should be done after continue with no breakpoints")
	}
}

func TestBreakpointByStage(t *testing.T) {
	s := newSession(t, multiStagePipeline)
	s.AddBreakpoint(debugger.Breakpoint{StageName: "B"})

	results := runContinue(t, s)
	if len(results) != 2 {
		t.Errorf("expected 2 results before breakpoint, got %d", len(results))
	}
	if s.IsDone() {
		t.Error("should not be done — paused at breakpoint")
	}
	if s.Cursor() != 2 {
		t.Errorf("cursor = %d, want 2 (B1)", s.Cursor())
	}
}

func TestBreakpointByStepName(t *testing.T) {
	s := newSession(t, simplePipeline)
	s.AddBreakpoint(debugger.Breakpoint{StepName: "Step 2"})

	results := runContinue(t, s)
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
	if s.Cursor() != 1 {
		t.Errorf("cursor = %d, want 1", s.Cursor())
	}
}

func TestToggleBreakpoint(t *testing.T) {
	s := newSession(t, simplePipeline)
	bp := debugger.Breakpoint{StepName: "Step 2"}

	added := s.ToggleBreakpoint(bp)
	if !added {
		t.Error("first toggle should add breakpoint")
	}
	if len(s.Breakpoints()) != 1 {
		t.Errorf("expected 1 breakpoint, got %d", len(s.Breakpoints()))
	}

	removed := s.ToggleBreakpoint(bp)
	if removed {
		t.Error("second toggle should remove breakpoint, returned true (added)")
	}
	if len(s.Breakpoints()) != 0 {
		t.Errorf("expected 0 breakpoints, got %d", len(s.Breakpoints()))
	}
}

func TestHasBreakpoint(t *testing.T) {
	s := newSession(t, simplePipeline)
	fs := s.Steps[1]
	s.AddBreakpoint(debugger.Breakpoint{StepName: "Step 2"})

	if !s.HasBreakpoint(fs) {
		t.Error("expected HasBreakpoint=true for Step 2")
	}
	if s.HasBreakpoint(s.Steps[0]) {
		t.Error("expected HasBreakpoint=false for Step 1")
	}
}

func TestRemoveBreakpoint(t *testing.T) {
	s := newSession(t, simplePipeline)
	bp := debugger.Breakpoint{StepName: "Step 2"}
	s.AddBreakpoint(bp)

	ok := s.RemoveBreakpoint(bp)
	if !ok {
		t.Error("expected RemoveBreakpoint=true")
	}
	if len(s.Breakpoints()) != 0 {
		t.Error("expected 0 breakpoints after removal")
	}
}

func TestSkipCurrent(t *testing.T) {
	s := newSession(t, simplePipeline)

	if err := s.SkipCurrent(); err != nil {
		t.Fatalf("skip: %v", err)
	}
	if s.Cursor() != 1 {
		t.Errorf("cursor = %d, want 1", s.Cursor())
	}
	if s.Results[0].Status != debugger.StatusSkipped {
		t.Errorf("step[0] = %v, want skipped", s.Results[0].Status)
	}
}

func TestReset(t *testing.T) {
	s := newSession(t, simplePipeline)
	runContinue(t, s)
	if !s.IsDone() {
		t.Fatal("should be done before reset")
	}

	s.Reset()
	if s.IsDone() {
		t.Error("should not be done after reset")
	}
	if s.Cursor() != 0 {
		t.Errorf("cursor = %d, want 0", s.Cursor())
	}
	if !s.IsPaused() {
		t.Error("should be paused after reset")
	}
	for i, r := range s.Results {
		if r.Status != debugger.StatusPending {
			t.Errorf("result[%d] = %v, want pending", i, r.Status)
		}
	}
}

func TestSetVariable(t *testing.T) {
	s := newSession(t, simplePipeline)
	s.SetVariable("key", "value")
	vars := s.Variables()
	if vars["key"] != "value" {
		t.Errorf("variable key = %q, want 'value'", vars["key"])
	}
}

func TestVariablesFromPipeline(t *testing.T) {
	p := mustParse(t, `
variables:
  buildConfig: Release
  version: "2.0"
stages:
  - stage: S
    jobs:
      - job: J
        steps:
          - script: echo hi
`)
	steps := pipeline.Flatten(p)
	s := debugger.New(steps, p.Variables, &dryRunExecutor{})
	vars := s.Variables()
	if vars["buildConfig"] != "Release" {
		t.Errorf("buildConfig = %q, want 'Release'", vars["buildConfig"])
	}
	if vars["version"] != "2.0" {
		t.Errorf("version = %q, want '2.0'", vars["version"])
	}
}

func TestFailedStepPausesContinue(t *testing.T) {
	s := newSession(t, simplePipeline, "Step 2")

	ch := make(chan string, 256)
	go drainOutput(ch)
	results, err := s.Continue(context.Background(), ch)
	if err != nil {
		t.Fatalf("continue: %v", err)
	}
	// Should run Step 1 (pass) and Step 2 (fail), then pause.
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
	if results[1].Status != debugger.StatusFailed {
		t.Errorf("step 2 status = %v, want failed", results[1].Status)
	}
	if s.IsDone() {
		t.Error("should not be done after step failure")
	}
	if !s.IsPaused() {
		t.Error("should be paused after step failure")
	}
}

func TestCurrentStep(t *testing.T) {
	s := newSession(t, simplePipeline)

	fs := s.CurrentStep()
	if fs == nil {
		t.Fatal("CurrentStep should not be nil at start")
	}
	if fs.Step.Label() != "Step 1" {
		t.Errorf("CurrentStep = %q, want 'Step 1'", fs.Step.Label())
	}

	runStep(t, s)
	fs = s.CurrentStep()
	if fs.Step.Label() != "Step 2" {
		t.Errorf("CurrentStep = %q, want 'Step 2'", fs.Step.Label())
	}
}

func TestCurrentStepWhenDone(t *testing.T) {
	s := newSession(t, simplePipeline)
	runContinue(t, s)

	if s.CurrentStep() != nil {
		t.Error("CurrentStep should be nil when done")
	}
}

func TestContinueAfterDone(t *testing.T) {
	s := newSession(t, simplePipeline)
	runContinue(t, s)

	ch := make(chan string, 64)
	go drainOutput(ch)
	_, err := s.Continue(context.Background(), ch)
	if err == nil {
		t.Error("expected error continuing past done")
	}
}
