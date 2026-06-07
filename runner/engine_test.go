package runner

import (
	"context"
	"strings"
	"testing"

	"github.com/kascea/stepthrough/pipeline"
)

// TestChanLineWriterForwardsLines guards against regressions where step output
// is silently dropped. This was the symptom of the missing -i flag on docker exec:
// chanLineWriter received no bytes because stdin was never forwarded.
func TestChanLineWriterForwardsLines(t *testing.T) {
	ch := make(chan string, 8)
	w := &chanLineWriter{ch: ch}

	input := "line one\nline two\nline three\n"
	n, err := w.Write([]byte(input))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != len(input) {
		t.Fatalf("Write returned %d, want %d", n, len(input))
	}
	close(ch)

	var got []string
	for line := range ch {
		got = append(got, line)
	}
	want := []string{"line one", "line two", "line three"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

// TestBootstrapScript_SudoConditional guards against unconditional sudo usage.
// The agent image runs as a non-root user with sudo, but older images (and any
// root-based container) do not have sudo installed — the bootstrap must handle both.
func TestBootstrapScript_SudoConditional(t *testing.T) {
	if !strings.Contains(bootstrapScript, "command -v sudo") {
		t.Error("bootstrapScript must check for sudo before using it; unconditional sudo fails on root containers without sudo installed")
	}
}

func TestEngineSetup_DockerUnavailable(t *testing.T) {
	old := dockerChecker
	dockerChecker = func() bool { return false }
	defer func() { dockerChecker = old }()

	e := NewEngine("/tmp")
	p := &pipeline.Pipeline{Pool: pipeline.Pool{VMImage: "ubuntu-latest"}}
	out := make(chan string, 16)
	err := e.Setup(context.Background(), p, &pipeline.Job{}, "test-container", out)
	if err == nil {
		t.Fatal("expected error when Docker is unavailable")
	}
	if !strings.Contains(err.Error(), "Docker is not running") {
		t.Errorf("error = %q, want to contain 'Docker is not running'", err.Error())
	}
}

func TestEngineSetup_UnsupportedVMImage(t *testing.T) {
	e := NewEngine("/tmp")
	p := &pipeline.Pipeline{Pool: pipeline.Pool{VMImage: "windows-latest"}}
	out := make(chan string, 16)
	err := e.Setup(context.Background(), p, &pipeline.Job{}, "test-container", out)
	if err == nil {
		t.Fatal("expected error for unsupported vmImage")
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Errorf("error = %q, want to contain 'not supported'", err.Error())
	}
}
