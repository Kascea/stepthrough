// Package debugger implements the step-through controller for an Azure pipeline.
// Execution is delegated to an Executor, which may be Docker-backed (runner.Engine)
// or an in-process dry-run (for testing and previewing steps without Docker).
package debugger

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/colecarlson/stepthrough/pipeline"
	"github.com/colecarlson/stepthrough/runner"
)

// Status is the execution state of a single step.
type Status string

const (
	StatusPending Status = "pending"
	StatusRunning Status = "running"
	StatusPassed  Status = "passed"
	StatusFailed  Status = "failed"
	StatusSkipped Status = "skipped"
)

// StepResult holds the outcome of executing a step.
type StepResult struct {
	FlatStep pipeline.FlatStep
	Status   Status
	Output   []string
	ExitCode int
	Duration time.Duration
	Err      error
}

// Executor is the interface for running a single step.
// The runner.Engine satisfies this interface for real Docker execution;
// DryRunExecutor satisfies it for tests and previewing.
type Executor interface {
	RunStep(ctx context.Context, step *pipeline.Step, outputCh chan<- string) *runner.StepResult
}

// Breakpoint identifies a location in the pipeline where execution should pause.
type Breakpoint struct {
	StageName string // empty = match any stage
	JobName   string // empty = match any job
	StepName  string // matches DisplayName or Name; empty = match any step in job/stage
}

// Matches returns true if this breakpoint matches the given flat step.
func (b Breakpoint) Matches(fs pipeline.FlatStep) bool {
	if b.StageName != "" && !strings.EqualFold(b.StageName, fs.StageName) {
		return false
	}
	if b.JobName != "" && !strings.EqualFold(b.JobName, fs.JobName) {
		return false
	}
	if b.StepName != "" {
		label := fs.Step.Label()
		name := fs.Step.Name
		if !strings.EqualFold(b.StepName, label) && !strings.EqualFold(b.StepName, name) {
			return false
		}
	}
	return true
}

// Session drives interactive step-through execution.
type Session struct {
	Steps    []pipeline.FlatStep
	Results  []StepResult

	executor    Executor
	cursor      int
	breakpoints []Breakpoint
	variables   map[string]string
	paused      bool
	done        bool
}

// New creates a Session for the given pipeline job.
// The executor handles actual step execution.
func New(steps []pipeline.FlatStep, variables map[string]string, exec Executor) *Session {
	results := make([]StepResult, len(steps))
	for i, fs := range steps {
		results[i] = StepResult{FlatStep: fs, Status: StatusPending}
	}
	vars := make(map[string]string, len(variables))
	for k, v := range variables {
		vars[k] = v
	}
	return &Session{
		Steps:     steps,
		Results:   results,
		executor:  exec,
		variables: vars,
		paused:    true,
	}
}

// Cursor returns the index of the current step (the next step to execute).
func (s *Session) Cursor() int { return s.cursor }

// IsDone returns true when all steps have been processed.
func (s *Session) IsDone() bool { return s.done }

// IsPaused returns true when execution is waiting for user input.
func (s *Session) IsPaused() bool { return s.paused }

// CurrentStep returns the FlatStep the cursor is pointing at, or nil if done.
func (s *Session) CurrentStep() *pipeline.FlatStep {
	if s.done || s.cursor >= len(s.Steps) {
		return nil
	}
	fs := s.Steps[s.cursor]
	return &fs
}

// Variables returns a snapshot of the current variable map.
func (s *Session) Variables() map[string]string {
	snap := make(map[string]string, len(s.variables))
	for k, v := range s.variables {
		snap[k] = v
	}
	return snap
}

// SetVariable sets a runtime variable (visible to subsequent steps via the env).
func (s *Session) SetVariable(key, value string) {
	s.variables[key] = value
}

// AddBreakpoint registers a new breakpoint.
func (s *Session) AddBreakpoint(b Breakpoint) {
	s.breakpoints = append(s.breakpoints, b)
}

// RemoveBreakpoint removes the first breakpoint equal to b. Returns true if found.
func (s *Session) RemoveBreakpoint(b Breakpoint) bool {
	for i, existing := range s.breakpoints {
		if existing == b {
			s.breakpoints = append(s.breakpoints[:i], s.breakpoints[i+1:]...)
			return true
		}
	}
	return false
}

// ToggleBreakpoint adds b if not present, removes it if present.
func (s *Session) ToggleBreakpoint(b Breakpoint) bool {
	if s.RemoveBreakpoint(b) {
		return false // removed
	}
	s.AddBreakpoint(b)
	return true // added
}

// HasBreakpoint returns true if the given flat step has a registered breakpoint.
func (s *Session) HasBreakpoint(fs pipeline.FlatStep) bool {
	for _, bp := range s.breakpoints {
		if bp.Matches(fs) {
			return true
		}
	}
	return false
}

// Breakpoints returns a copy of the registered breakpoints.
func (s *Session) Breakpoints() []Breakpoint {
	cp := make([]Breakpoint, len(s.breakpoints))
	copy(cp, s.breakpoints)
	return cp
}

// Step executes the current step, streams output to outputCh, then pauses.
// The returned *StepResult is the same pointer stored in s.Results[cursor-1].
func (s *Session) Step(ctx context.Context, outputCh chan<- string) (*StepResult, error) {
	if s.done {
		return nil, fmt.Errorf("pipeline execution already complete")
	}
	result, err := s.runCurrent(ctx, outputCh)
	if err != nil {
		return nil, err
	}
	s.advance()
	s.paused = true
	return result, nil
}

// Continue runs steps until a breakpoint is hit, a step fails, or the pipeline ends.
// Output is streamed to outputCh. Returns results for each step executed.
func (s *Session) Continue(ctx context.Context, outputCh chan<- string) ([]*StepResult, error) {
	if s.done {
		return nil, fmt.Errorf("pipeline execution already complete")
	}
	s.paused = false
	var results []*StepResult

	for !s.done && !s.paused {
		result, err := s.runCurrent(ctx, outputCh)
		if err != nil {
			return results, err
		}
		results = append(results, result)
		s.advance()

		if result.Status == StatusFailed {
			s.paused = true
			break
		}
		if !s.done && s.atBreakpoint() {
			s.paused = true
		}
	}
	return results, nil
}

// SkipCurrent marks the current step as skipped without executing it.
func (s *Session) SkipCurrent() error {
	if s.done {
		return fmt.Errorf("pipeline execution already complete")
	}
	s.Results[s.cursor].Status = StatusSkipped
	s.advance()
	s.paused = true
	return nil
}

// Reset resets execution to the beginning.
func (s *Session) Reset() {
	s.cursor = 0
	s.done = false
	s.paused = true
	for i := range s.Results {
		s.Results[i] = StepResult{FlatStep: s.Steps[i], Status: StatusPending}
	}
}

// --- internal ---------------------------------------------------------------

func (s *Session) runCurrent(ctx context.Context, outputCh chan<- string) (*StepResult, error) {
	if s.cursor >= len(s.Steps) {
		return nil, fmt.Errorf("cursor %d out of range (total %d)", s.cursor, len(s.Steps))
	}

	s.Results[s.cursor].Status = StatusRunning

	runnerResult := s.executor.RunStep(ctx, s.Steps[s.cursor].Step, outputCh)

	// Map runner status to debugger status.
	var status Status
	switch runnerResult.Status {
	case runner.StatusPassed:
		status = StatusPassed
	case runner.StatusFailed:
		status = StatusFailed
	case runner.StatusSkipped:
		status = StatusSkipped
	default:
		status = StatusFailed
	}

	s.Results[s.cursor].Status = status
	s.Results[s.cursor].ExitCode = runnerResult.ExitCode
	s.Results[s.cursor].Duration = runnerResult.Duration
	s.Results[s.cursor].Err = runnerResult.Err

	return &s.Results[s.cursor], nil
}

func (s *Session) advance() {
	s.cursor++
	if s.cursor >= len(s.Steps) {
		s.done = true
		s.paused = false
	}
}

func (s *Session) atBreakpoint() bool {
	if s.cursor >= len(s.Steps) {
		return false
	}
	return s.HasBreakpoint(s.Steps[s.cursor])
}
