package runner

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/colecarlson/stepthrough/pipeline"
)

// StepStatus mirrors debugger.Status to avoid a circular dependency.
type StepStatus string

const (
	StatusPending StepStatus = "pending"
	StatusRunning StepStatus = "running"
	StatusPassed  StepStatus = "passed"
	StatusFailed  StepStatus = "failed"
	StatusSkipped StepStatus = "skipped"
)

// StepResult is the outcome of running a single step.
type StepResult struct {
	Status   StepStatus
	ExitCode int
	Output   []string
	Duration time.Duration
	Err      error
}

// Engine executes pipeline steps inside a Docker container.
// One Engine instance manages exactly one container (one job).
type Engine struct {
	container *Container
	workDir   string
}

// NewEngine returns an Engine configured for the given pipeline and working directory.
func NewEngine(workDir string) *Engine {
	return &Engine{workDir: workDir}
}

// Setup pulls the Docker image and starts a container for the job.
// Output (docker pull progress) is sent to outputCh.
func (e *Engine) Setup(ctx context.Context, p *pipeline.Pipeline, job *pipeline.Job, containerName string, outputCh chan<- string) error {
	// Resolve the vmImage for this job (job pool overrides pipeline pool).
	vmImage := p.Pool.VMImage
	if job.Pool != nil && job.Pool.VMImage != "" {
		vmImage = job.Pool.VMImage
	}

	image, ok := ResolveImage(vmImage)
	if !ok {
		return fmt.Errorf("vmImage %q is not supported for local debugging (only ubuntu-latest is supported)", vmImage)
	}

	outputCh <- fmt.Sprintf("[stepthrough] pulling image %s…", image)
	if err := Pull(ctx, image, &chanLineWriter{ch: outputCh}); err != nil {
		return fmt.Errorf("pull image %s: %w", image, err)
	}

	outputCh <- fmt.Sprintf("[stepthrough] starting container %s…", containerName)
	c, err := Start(ctx, containerName, image, e.workDir, p.Variables)
	if err != nil {
		return fmt.Errorf("start container: %w", err)
	}
	e.container = c
	outputCh <- fmt.Sprintf("[stepthrough] container ready  (image: %s)", image)
	return nil
}

// RunStep executes a single pipeline step, streaming output to outputCh.
func (e *Engine) RunStep(ctx context.Context, step *pipeline.Step, outputCh chan<- string) *StepResult {
	result := &StepResult{Status: StatusRunning}
	start := time.Now()

	script, ok := ResolveStep(step)
	if !ok {
		result.Status = StatusSkipped
		result.Duration = time.Since(start)
		outputCh <- fmt.Sprintf("[stepthrough] step type %q has no local equivalent — skipped", step.Type())
		return result
	}

	exitCode, err := e.container.ExecScript(ctx, script, step.Env, outputCh)
	result.Duration = time.Since(start)
	result.ExitCode = exitCode
	result.Err = err

	if err != nil {
		result.Status = StatusFailed
		return result
	}
	if exitCode != 0 && !step.ContinueOnError {
		result.Status = StatusFailed
		return result
	}

	result.Status = StatusPassed
	return result
}

// ShellCmd returns an *exec.Cmd for an interactive bash session in the container.
// Use with tea.ExecProcess so the TUI suspends while the shell is active.
func (e *Engine) ShellCmd() *exec.Cmd {
	args := e.container.ShellArgs()
	cmd := exec.Command(args[0], args[1:]...)
	return cmd
}

// ContainerName returns the running container's name.
func (e *Engine) ContainerName() string {
	if e.container == nil {
		return ""
	}
	return e.container.Name()
}

// ContainerImage returns the running container's image.
func (e *Engine) ContainerImage() string {
	if e.container == nil {
		return ""
	}
	return e.container.Image()
}

// Ready returns true once a container has been started.
func (e *Engine) Ready() bool { return e.container != nil }

// Cleanup stops and removes the container.
func (e *Engine) Cleanup(ctx context.Context) {
	if e.container != nil {
		e.container.Stop(ctx)
		e.container = nil
	}
}
