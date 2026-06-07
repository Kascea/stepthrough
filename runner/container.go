package runner

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

const workspacePath = "/workspace"

// Container is a running Docker container that persists across steps in a job.
// All steps in a job share the same container so they share filesystem state.
type Container struct {
	name    string // unique container name (e.g. "stepthrough-abc123")
	image   string
	workDir string // host directory mounted at /workspace
}

// Pull pulls the Docker image, writing progress to w.
func Pull(ctx context.Context, image string, w io.Writer) error {
	cmd := exec.CommandContext(ctx, "docker", "pull", image)
	cmd.Stdout = w
	cmd.Stderr = w
	return cmd.Run()
}

// Start creates a detached container with the host workspace mounted.
// pipelineVars are set as environment variables alongside standard Azure env vars.
func Start(ctx context.Context, name, image, hostWorkDir string, pipelineVars map[string]string) (*Container, error) {
	c := &Container{name: name, image: image, workDir: hostWorkDir}

	args := []string{
		"run", "-d",
		"--name", name,
		"-v", hostWorkDir + ":" + workspacePath,
		"-w", workspacePath,
	}

	// Standard Azure Pipelines environment variables.
	azureEnv := map[string]string{
		"BUILD_SOURCESDIRECTORY":         workspacePath,
		"SYSTEM_DEFAULTWORKINGDIRECTORY": workspacePath,
		"BUILD_ARTIFACTSTAGINGDIRECTORY": workspacePath + "/_artifacts",
		"AGENT_BUILDDIRECTORY":           workspacePath,
		"BUILD_REPOSITORY_LOCALPATH":     workspacePath,
		"CI":                             "true",
		"TF_BUILD":                       "True",
		"AGENT_OS":                       "Linux",
	}
	for k, v := range pipelineVars {
		// Azure pipeline variables use BUILD_BUILDCONFIGURATION style naming,
		// but the raw YAML key is camelCase — expose both forms.
		azureEnv[k] = v
		// Also expose as uppercased with underscores for shell convenience.
		upper := azureEnvName(k)
		azureEnv[upper] = v
	}
	for k, v := range azureEnv {
		args = append(args, "-e", k+"="+v)
	}

	args = append(args, image, "tail", "-f", "/dev/null")

	out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("docker run: %s: %w", strings.TrimSpace(string(out)), err)
	}

	// Bootstrap: ensure base tools are present (no-op on agent image).
	// Mark /workspace safe so go build's VCS stamping doesn't fail when the
	// directory is owned by a different UID than the container user.
	_, _ = c.execScript(ctx, `
if command -v apt-get >/dev/null 2>&1; then
  apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq git curl ca-certificates >/dev/null 2>&1
fi
git config --global --add safe.directory '*'
mkdir -p /workspace/_artifacts
`, nil, io.Discard)

	return c, nil
}

// ExecScript runs a bash script inside the container, streaming each output line to outputCh.
// Returns the command exit code.
func (c *Container) ExecScript(ctx context.Context, script string, env map[string]string, outputCh chan<- string) (int, error) {
	return c.execScript(ctx, script, env, &chanLineWriter{ch: outputCh})
}

// execScript is the internal implementation that writes to an io.Writer.
func (c *Container) execScript(ctx context.Context, script string, env map[string]string, w io.Writer) (int, error) {
	// -i is required: without it Docker does not forward stdin to the container
	// and "cat > $tmp" reads empty stdin, producing a no-op script with no output.
	args := []string{"exec", "-i"}
	for k, v := range env {
		args = append(args, "-e", k+"="+v)
	}
	// Azure script tasks write a temp file and run bash against that file. Feed the
	// script over stdin to avoid shell-quoting the user's script through docker exec.
	args = append(args, "-w", workspacePath, c.name,
		"bash", "--noprofile", "--norc", "-c",
		`tmp=$(mktemp /tmp/stepthrough-script.XXXXXX); cat > "$tmp"; bash --noprofile --norc "$tmp"; code=$?; rm -f "$tmp"; exit "$code"`,
	)

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdin = strings.NewReader("set -eo pipefail\n" + script)

	// Merge stdout+stderr into a single pipe so output is in order.
	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		return -1, fmt.Errorf("docker exec start: %w", err)
	}

	scanDone := make(chan struct{})
	go func() {
		defer close(scanDone)
		scanner := bufio.NewScanner(pr)
		scanner.Buffer(make([]byte, 1<<20), 1<<20)
		for scanner.Scan() {
			fmt.Fprintln(w, scanner.Text())
		}
	}()

	waitErr := cmd.Wait()
	pw.Close()
	<-scanDone

	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			return exitErr.ExitCode(), nil
		}
		return -1, waitErr
	}
	return 0, nil
}

// ShellArgs returns the docker exec args for an interactive shell session.
// The caller should use tea.ExecProcess to properly suspend/resume the TUI.
func (c *Container) ShellArgs() []string {
	return []string{"docker", "exec", "-it", c.name, "/bin/bash"}
}

// Stop stops and removes the container.
func (c *Container) Stop(ctx context.Context) {
	exec.CommandContext(ctx, "docker", "stop", "-t", "5", c.name).Run() //nolint
	exec.CommandContext(ctx, "docker", "rm", "-f", c.name).Run()        //nolint
}

// Name returns the container name.
func (c *Container) Name() string { return c.name }

// Image returns the Docker image name.
func (c *Container) Image() string { return c.image }

// chanLineWriter buffers writes and sends complete lines to a channel.
type chanLineWriter struct {
	ch  chan<- string
	buf strings.Builder
}

func (w *chanLineWriter) Write(p []byte) (int, error) {
	s := string(p)
	for {
		i := strings.IndexByte(s, '\n')
		if i < 0 {
			w.buf.WriteString(s)
			break
		}
		w.buf.WriteString(s[:i])
		line := w.buf.String()
		w.buf.Reset()
		select {
		case w.ch <- line:
		default:
		}
		s = s[i+1:]
	}
	return len(p), nil
}
