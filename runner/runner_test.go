package runner_test

import (
	"strings"
	"testing"

	"github.com/colecarlson/stepthrough/pipeline"
	"github.com/colecarlson/stepthrough/runner"
)

// step parses a single step from a YAML fragment starting with "- ".
// All lines are indented under a "steps:" key so multi-line inputs work correctly.
func step(t *testing.T, stepYAML string) *pipeline.Step {
	t.Helper()
	var sb strings.Builder
	sb.WriteString("steps:\n")
	for _, line := range strings.Split(stepYAML, "\n") {
		if line != "" {
			sb.WriteString("  " + line + "\n")
		}
	}
	p, err := pipeline.ParseBytes([]byte(sb.String()))
	if err != nil {
		t.Fatalf("parse step YAML: %v\nYAML:\n%s", err, sb.String())
	}
	if len(p.Stages[0].Jobs[0].Steps) == 0 {
		t.Fatal("no steps parsed")
	}
	return &p.Stages[0].Jobs[0].Steps[0]
}

// ── ResolveImage tests ────────────────────────────────────────────────────────

func TestResolveImage_UbuntuLatest(t *testing.T) {
	img, ok := runner.ResolveImage("ubuntu-latest")
	if !ok {
		t.Fatal("ubuntu-latest should be supported")
	}
	if img != "ubuntu:22.04" {
		t.Errorf("image = %q, want 'ubuntu:22.04'", img)
	}
}

func TestResolveImage_UbuntuExact(t *testing.T) {
	img, ok := runner.ResolveImage("ubuntu-22.04")
	if !ok {
		t.Fatal("ubuntu-22.04 should be supported")
	}
	if img != "ubuntu:22.04" {
		t.Errorf("image = %q, want 'ubuntu:22.04'", img)
	}
}

func TestResolveImage_Empty(t *testing.T) {
	img, ok := runner.ResolveImage("")
	if !ok {
		t.Fatal("empty vmImage should fall back to default (supported)")
	}
	if img != runner.DefaultImage {
		t.Errorf("image = %q, want DefaultImage %q", img, runner.DefaultImage)
	}
}

func TestResolveImage_WindowsUnsupported(t *testing.T) {
	_, ok := runner.ResolveImage("windows-latest")
	if ok {
		t.Error("windows-latest should not be supported")
	}
}

func TestResolveImage_MacOSUnsupported(t *testing.T) {
	_, ok := runner.ResolveImage("macos-latest")
	if ok {
		t.Error("macos-latest should not be supported")
	}
}

func TestResolveImage_UnknownPassthrough(t *testing.T) {
	img, ok := runner.ResolveImage("my-custom-image:1.0")
	if !ok {
		t.Fatal("unknown image should pass through as supported")
	}
	if img != "my-custom-image:1.0" {
		t.Errorf("image = %q, want 'my-custom-image:1.0'", img)
	}
}

// ── ResolveStep tests ─────────────────────────────────────────────────────────

func TestResolveStep_Script(t *testing.T) {
	s := step(t, "- script: echo hello")
	script, ok := runner.ResolveStep(s)
	if !ok {
		t.Fatal("script step should be supported")
	}
	if script != "echo hello" {
		t.Errorf("script = %q, want 'echo hello'", script)
	}
}

func TestResolveStep_Bash(t *testing.T) {
	s := step(t, "- bash: echo hello")
	script, ok := runner.ResolveStep(s)
	if !ok {
		t.Fatal("bash step should be supported")
	}
	if script != "echo hello" {
		t.Errorf("script = %q, want 'echo hello'", script)
	}
}

func TestResolveStep_Pwsh(t *testing.T) {
	s := step(t, `- pwsh: Write-Host "hi"`)
	script, ok := runner.ResolveStep(s)
	if !ok {
		t.Fatal("pwsh step should be supported")
	}
	if !strings.Contains(script, "pwsh") {
		t.Errorf("pwsh script should install/call pwsh, got: %q", script)
	}
}

func TestResolveStep_CheckoutSelf(t *testing.T) {
	s := step(t, "- checkout: self")
	script, ok := runner.ResolveStep(s)
	if !ok {
		t.Fatal("checkout: self should be supported")
	}
	if !strings.Contains(script, "workspace") {
		t.Errorf("checkout script should mention workspace mount, got: %q", script)
	}
}

func TestResolveStep_CheckoutNone(t *testing.T) {
	s := step(t, "- checkout: none")
	script, ok := runner.ResolveStep(s)
	if !ok {
		t.Fatal("checkout: none should be supported")
	}
	if !strings.Contains(script, "skipped") {
		t.Errorf("checkout:none script should say skipped, got: %q", script)
	}
}

func TestResolveStep_Template(t *testing.T) {
	s := step(t, "- template: path/to/tmpl.yml")
	script, ok := runner.ResolveStep(s)
	if !ok {
		t.Fatal("template step should be supported (as a no-op message)")
	}
	if !strings.Contains(script, "path/to/tmpl.yml") {
		t.Errorf("template script should name the template, got: %q", script)
	}
}

func TestResolveStep_TaskUseDotNet(t *testing.T) {
	s := step(t, "- task: UseDotNet@2\n  inputs:\n    version: \"8.x\"")
	script, ok := runner.ResolveStep(s)
	if !ok {
		t.Fatal("UseDotNet@2 should be supported")
	}
	if !strings.Contains(script, "dotnet-install") {
		t.Errorf("UseDotNet script should install dotnet, got: %q", script)
	}
	if !strings.Contains(script, "8") {
		t.Errorf("UseDotNet script should include channel '8', got: %q", script)
	}
}

func TestResolveStep_TaskUseNode(t *testing.T) {
	s := step(t, "- task: NodeTool@0\n  inputs:\n    versionSpec: \"18.x\"")
	script, ok := runner.ResolveStep(s)
	if !ok {
		t.Fatal("NodeTool should be supported")
	}
	if !strings.Contains(script, "nodesource") || !strings.Contains(script, "18") {
		t.Errorf("UseNode script should set up node 18, got: %q", script)
	}
}

func TestResolveStep_TaskPublishArtifacts(t *testing.T) {
	s := step(t, "- task: PublishBuildArtifacts@1\n  inputs:\n    artifactName: drop")
	script, ok := runner.ResolveStep(s)
	if !ok {
		t.Fatal("PublishBuildArtifacts should be supported (as skip message)")
	}
	if !strings.Contains(script, "skipped") {
		t.Errorf("publish script should say skipped locally, got: %q", script)
	}
}

func TestResolveStep_TaskPublishTestResults(t *testing.T) {
	s := step(t, "- task: PublishTestResults@2")
	script, ok := runner.ResolveStep(s)
	if !ok {
		t.Fatal("PublishTestResults should be supported (as skip message)")
	}
	if !strings.Contains(script, "skipped") {
		t.Errorf("publish test results script should say skipped, got: %q", script)
	}
}

func TestResolveStep_TaskCopyFiles(t *testing.T) {
	s := step(t, "- task: CopyFiles@2\n  inputs:\n    SourceFolder: src\n    TargetFolder: out")
	script, ok := runner.ResolveStep(s)
	if !ok {
		t.Fatal("CopyFiles should be supported")
	}
	if !strings.Contains(script, "cp") {
		t.Errorf("CopyFiles script should use cp, got: %q", script)
	}
}

func TestResolveStep_TaskUnknownProducesEchoMessage(t *testing.T) {
	s := step(t, "- task: SomeObscureTask@99")
	script, ok := runner.ResolveStep(s)
	if !ok {
		t.Fatal("unknown task should still produce a supported echo message")
	}
	if !strings.Contains(script, "SomeObscureTask@99") {
		t.Errorf("unknown task script should name the task, got: %q", script)
	}
	if !strings.Contains(script, "not supported") {
		t.Errorf("unknown task script should say not supported, got: %q", script)
	}
}

func TestResolveStep_ScriptMultiline(t *testing.T) {
	s := step(t, "- script: |\n    echo line1\n    echo line2")
	script, ok := runner.ResolveStep(s)
	if !ok {
		t.Fatal("multiline script should be supported")
	}
	if !strings.Contains(script, "line1") || !strings.Contains(script, "line2") {
		t.Errorf("multiline script not preserved: %q", script)
	}
}
