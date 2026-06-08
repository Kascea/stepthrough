package runner_test

import (
	"strings"
	"testing"

	"github.com/kascea/stepthrough/pipeline"
	"github.com/kascea/stepthrough/runner"
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
	if img != runner.DefaultImage {
		t.Errorf("image = %q, want %q", img, runner.DefaultImage)
	}
}

func TestResolveImage_UbuntuExact(t *testing.T) {
	img, ok := runner.ResolveImage("ubuntu-22.04")
	if !ok {
		t.Fatal("ubuntu-22.04 should be supported")
	}
	if img != runner.DefaultImage {
		t.Errorf("image = %q, want %q", img, runner.DefaultImage)
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

func TestResolveStep_TaskGoTool(t *testing.T) {
	s := step(t, "- task: GoTool@0\n  inputs:\n    version: \"1.25.4\"")
	script, ok := runner.ResolveStep(s)
	if !ok {
		t.Fatal("GoTool@0 should be supported")
	}
	if !strings.Contains(script, "go.dev/dl") {
		t.Errorf("GoTool script should download from go.dev, got: %q", script)
	}
	if !strings.Contains(script, "go version") {
		t.Errorf("GoTool script should verify Go installation, got: %q", script)
	}
}

func TestResolveStep_TaskGo(t *testing.T) {
	s := step(t, "- task: Go@0\n  inputs:\n    command: build\n    arguments: '-o ./out ./...'")
	script, ok := runner.ResolveStep(s)
	if !ok {
		t.Fatal("Go@0 should be supported")
	}
	if !strings.Contains(script, "go build") {
		t.Errorf("Go@0 build script should invoke 'go build', got: %q", script)
	}
	if !strings.Contains(script, "./out") {
		t.Errorf("Go@0 build script should include arguments, got: %q", script)
	}
}

func TestResolveStep_TaskGoCustomCommand(t *testing.T) {
	s := step(t, "- task: Go@0\n  inputs:\n    command: custom\n    customCommand: vet\n    arguments: './...'")
	script, ok := runner.ResolveStep(s)
	if !ok {
		t.Fatal("Go@0 custom command should be supported")
	}
	if !strings.Contains(script, "go vet") {
		t.Errorf("Go@0 custom script should run 'go vet', got: %q", script)
	}
}

func TestResolveStep_WorkingDirectoryScript(t *testing.T) {
	s := step(t, "- script: go test ./...\n  workingDirectory: '/workspace/backend'")
	script, ok := runner.ResolveStep(s)
	if !ok {
		t.Fatal("script with workingDirectory should be supported")
	}
	if !strings.HasPrefix(script, "cd ") {
		t.Errorf("script should start with cd, got: %q", script)
	}
	if !strings.Contains(script, "/workspace/backend") {
		t.Errorf("script should cd to workingDirectory, got: %q", script)
	}
	if !strings.Contains(script, "go test ./...") {
		t.Errorf("script body should follow the cd, got: %q", script)
	}
}

func TestResolveStep_WorkingDirectoryBash(t *testing.T) {
	s := step(t, "- bash: go test ./...\n  workingDirectory: '/workspace/backend'")
	script, ok := runner.ResolveStep(s)
	if !ok {
		t.Fatal("bash with workingDirectory should be supported")
	}
	if !strings.Contains(script, "cd ") || !strings.Contains(script, "/workspace/backend") {
		t.Errorf("bash should cd to workingDirectory, got: %q", script)
	}
}

func TestResolveStep_WorkingDirectoryTask(t *testing.T) {
	s := step(t, "- task: Go@0\n  inputs:\n    command: test\n    arguments: './...'\n  workingDirectory: '/workspace/backend'")
	script, ok := runner.ResolveStep(s)
	if !ok {
		t.Fatal("task with workingDirectory should be supported")
	}
	if !strings.Contains(script, "cd ") || !strings.Contains(script, "/workspace/backend") {
		t.Errorf("task should cd to workingDirectory, got: %q", script)
	}
}

func TestResolveStep_WorkingDirectoryExpandsAzureVariable(t *testing.T) {
	s := step(t, "- script: go test ./...\n  workingDirectory: '$(System.DefaultWorkingDirectory)/backend'")
	script, ok := runner.ResolveStep(s)
	if !ok {
		t.Fatal("script with Azure variable workingDirectory should be supported")
	}
	if strings.Contains(script, "$(System.DefaultWorkingDirectory)") {
		t.Errorf("workingDirectory should expand Azure variable syntax, got: %q", script)
	}
	if !strings.Contains(script, "${SYSTEM_DEFAULTWORKINGDIRECTORY}") {
		t.Errorf("workingDirectory should expand to shell env ref, got: %q", script)
	}
}

func TestResolveStep_NoWorkingDirectoryNoCD(t *testing.T) {
	s := step(t, "- script: echo hello")
	script, ok := runner.ResolveStep(s)
	if !ok {
		t.Fatal("script without workingDirectory should be supported")
	}
	if strings.HasPrefix(script, "cd ") {
		t.Errorf("script without workingDirectory should not prepend cd, got: %q", script)
	}
}

func TestResolveStep_TaskGoToolVariableSpec(t *testing.T) {
	s := step(t, "- task: GoTool@0\n  inputs:\n    version: \"$(goVersion)\"")
	script, ok := runner.ResolveStep(s)
	if !ok {
		t.Fatal("GoTool@0 should be supported")
	}
	if strings.Contains(script, "$(goVersion)") {
		t.Errorf("GoTool script should not pass raw Azure variable syntax to bash, got: %q", script)
	}
	if !strings.Contains(script, "version_spec=\"${GOVERSION}\"") {
		t.Errorf("GoTool script should use normalized metadata input, got: %q", script)
	}
}

func TestResolveStep_TaskDotNetCoreCLI(t *testing.T) {
	s := step(t, "- task: DotNetCoreCLI@2\n  inputs:\n    command: build\n    projects: \"**/*.csproj\"")
	script, ok := runner.ResolveStep(s)
	if !ok {
		t.Fatal("DotNetCoreCLI@2 should be supported")
	}
	if !strings.Contains(script, "dotnet") {
		t.Errorf("DotNetCoreCLI script should invoke dotnet, got: %q", script)
	}
}

func TestResolveStep_TaskDotNetCoreCLI_TestMultiProject(t *testing.T) {
	// MSBuild's VSTest target only accepts one project; dotnet test with a glob
	// and nobuild:true must loop over each project individually.
	yml := "- task: DotNetCoreCLI@2\n  inputs:\n    command: test\n    projects: 'test/**/*.csproj'\n    nobuild: 'true'\n    arguments: '--logger trx'"
	s := step(t, yml)
	script, ok := runner.ResolveStep(s)
	if !ok {
		t.Fatal("DotNetCoreCLI@2 test command should be supported")
	}
	if !strings.Contains(script, "for proj in") {
		t.Errorf("test command with glob projects must loop per-project, got: %q", script)
	}
	if !strings.Contains(script, "--no-build") {
		t.Errorf("nobuild:true must emit --no-build flag, got: %q", script)
	}
}

func TestResolveStep_TaskDotNetCoreCLI_ToolInstallAlreadyInstalled(t *testing.T) {
	// `dotnet tool install` exits 1 when already installed; locally we re-run
	// against the same container so the adapter must fall back to `tool update`.
	yml := "- task: DotNetCoreCLI@2\n  inputs:\n    command: custom\n    custom: tool\n    arguments: 'install --tool-path . dotnet-reportgenerator-globaltool'"
	s := step(t, yml)
	script, ok := runner.ResolveStep(s)
	if !ok {
		t.Fatal("DotNetCoreCLI@2 custom tool install should be supported")
	}
	if !strings.Contains(script, "dotnet tool install") {
		t.Errorf("script should attempt install first, got: %q", script)
	}
	if !strings.Contains(script, "dotnet tool update") {
		t.Errorf("script should fall back to update when already installed, got: %q", script)
	}
}

func TestResolveStep_UnknownTaskFails(t *testing.T) {
	s := step(t, "- task: UseGoVersion@0\n  inputs:\n    versionSpec: \"1.25.x\"")
	script, ok := runner.ResolveStep(s)
	if !ok {
		t.Fatal("unknown task should still produce a script (not be silently skipped)")
	}
	if !strings.Contains(script, "not supported locally") {
		t.Errorf("script should name the problem, got: %q", script)
	}
	if !strings.Contains(script, "exit 1") {
		t.Errorf("script should exit 1 so the step fails, got: %q", script)
	}
}

func TestResolveStep_TaskGoToolDefaultVersionFromMetadata(t *testing.T) {
	s := step(t, "- task: GoTool@0")
	script, ok := runner.ResolveStep(s)
	if !ok {
		t.Fatal("GoTool@0 should be supported")
	}
	if !strings.Contains(script, "version_spec=\"1.10\"") {
		t.Errorf("GoTool should use metadata default version, got: %q", script)
	}
}

func TestResolveStep_TaskCmdLineExpandsAzureVariableSyntax(t *testing.T) {
	s := step(t, "- task: CmdLine@2\n  inputs:\n    script: echo $(Build.SourcesDirectory)")
	script, ok := runner.ResolveStep(s)
	if !ok {
		t.Fatal("CmdLine@2 should be supported")
	}
	if strings.Contains(script, "$(Build.SourcesDirectory)") {
		t.Fatalf("script still contains Azure variable command substitution: %q", script)
	}
	if !strings.Contains(script, "${BUILD_SOURCESDIRECTORY}") {
		t.Fatalf("script = %q, want Azure system variable converted to shell env ref", script)
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

func TestResolveStep_TaskUnknownFails(t *testing.T) {
	s := step(t, "- task: SomeObscureTask@99")
	script, ok := runner.ResolveStep(s)
	if !ok {
		t.Fatal("unknown task should produce a script, not be silently skipped")
	}
	if !strings.Contains(script, "SomeObscureTask@99") {
		t.Errorf("unknown task script should name the task, got: %q", script)
	}
	if !strings.Contains(script, "exit 1") {
		t.Errorf("unknown task should exit 1, got: %q", script)
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

func TestResolveStep_ScriptExpandsAzureVariableSyntax(t *testing.T) {
	s := step(t, "- script: echo $(goVersion)")
	script, ok := runner.ResolveStep(s)
	if !ok {
		t.Fatal("script step should be supported")
	}
	if strings.Contains(script, "$(goVersion)") {
		t.Fatalf("script still contains Azure variable command substitution: %q", script)
	}
	if !strings.Contains(script, "${GOVERSION}") {
		t.Fatalf("script = %q, want Azure variable converted to shell env ref", script)
	}
}

func TestResolveStep_ScriptExpandsDottedAzureVariableSyntax(t *testing.T) {
	s := step(t, "- script: echo $(Build.SourcesDirectory)")
	script, ok := runner.ResolveStep(s)
	if !ok {
		t.Fatal("script step should be supported")
	}
	if !strings.Contains(script, "${BUILD_SOURCESDIRECTORY}") {
		t.Fatalf("script = %q, want dotted Azure variable converted to env ref", script)
	}
}

func TestResolveStep_ScriptPreservesBashCommandSubstitution(t *testing.T) {
	s := step(t, "- script: files=$(find . -name '*.go')")
	script, ok := runner.ResolveStep(s)
	if !ok {
		t.Fatal("script step should be supported")
	}
	if !strings.Contains(script, "$(find . -name '*.go')") {
		t.Fatalf("script = %q, want bash command substitution preserved", script)
	}
}
