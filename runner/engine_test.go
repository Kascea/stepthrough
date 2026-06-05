package runner

import (
	"context"
	"strings"
	"testing"

	"github.com/colecarlson/stepthrough/pipeline"
)

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
