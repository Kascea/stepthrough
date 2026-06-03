package orchestrator

import (
	"path/filepath"
	"testing"
)

func TestWorkspaceRootUsesGitRoot(t *testing.T) {
	got := workspaceRoot("../examples/azure-pipelines.yml")
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
