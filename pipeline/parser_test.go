package pipeline_test

import (
	"strings"
	"testing"

	"github.com/kascea/stepthrough/pipeline"
)

// fixture loads a YAML file from the examples/fixtures directory.
func fixture(t *testing.T, name string) *pipeline.Pipeline {
	t.Helper()
	p, err := pipeline.Parse("../examples/fixtures/" + name)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return p
}

// ── Structure tests ───────────────────────────────────────────────────────────

func TestParseMinimal_SyntheticStageAndJob(t *testing.T) {
	p := fixture(t, "minimal.yml")

	if len(p.Stages) != 1 {
		t.Fatalf("expected 1 synthetic stage, got %d", len(p.Stages))
	}
	if p.Stages[0].Stage != "__root__" {
		t.Errorf("synthetic stage name = %q, want '__root__'", p.Stages[0].Stage)
	}
	if len(p.Stages[0].Jobs) != 1 {
		t.Fatalf("expected 1 synthetic job, got %d", len(p.Stages[0].Jobs))
	}
	if len(p.Stages[0].Jobs[0].Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(p.Stages[0].Jobs[0].Steps))
	}
}

func TestParseMinimal_PoolVMImage(t *testing.T) {
	p := fixture(t, "minimal.yml")
	if p.Pool.VMImage != "ubuntu-latest" {
		t.Errorf("pool.vmImage = %q, want 'ubuntu-latest'", p.Pool.VMImage)
	}
}

func TestParseStages_Count(t *testing.T) {
	p := fixture(t, "stages.yml")
	if len(p.Stages) != 2 {
		t.Fatalf("expected 2 stages, got %d", len(p.Stages))
	}
}

func TestParseStages_Names(t *testing.T) {
	p := fixture(t, "stages.yml")
	if p.Stages[0].Stage != "Build" {
		t.Errorf("stage[0].stage = %q, want 'Build'", p.Stages[0].Stage)
	}
	if p.Stages[0].DisplayName != "Build Stage" {
		t.Errorf("stage[0].displayName = %q, want 'Build Stage'", p.Stages[0].DisplayName)
	}
	if p.Stages[1].Stage != "Test" {
		t.Errorf("stage[1].stage = %q, want 'Test'", p.Stages[1].Stage)
	}
}

func TestParseStages_PipelineVariables(t *testing.T) {
	p := fixture(t, "stages.yml")
	if p.Variables["buildConfig"] != "Release" {
		t.Errorf("variables.buildConfig = %q, want 'Release'", p.Variables["buildConfig"])
	}
}

func TestParseStages_JobStepCounts(t *testing.T) {
	p := fixture(t, "stages.yml")
	if len(p.Stages[0].Jobs[0].Steps) != 2 {
		t.Errorf("Build job steps = %d, want 2", len(p.Stages[0].Jobs[0].Steps))
	}
	if len(p.Stages[1].Jobs[0].Steps) != 1 {
		t.Errorf("Test job steps = %d, want 1", len(p.Stages[1].Jobs[0].Steps))
	}
}

// ── dependsOn tests ───────────────────────────────────────────────────────────

func TestDependsOnString(t *testing.T) {
	p := fixture(t, "depends-on-string.yml")
	deps := p.Stages[1].DependsOn
	if len(deps) != 1 || deps[0] != "A" {
		t.Errorf("stage B dependsOn = %v, want [A]", deps)
	}
}

func TestDependsOnList(t *testing.T) {
	p := fixture(t, "depends-on-list.yml")
	deps := p.Stages[2].DependsOn
	if len(deps) != 2 {
		t.Fatalf("stage C dependsOn = %v, want [A B]", deps)
	}
	if deps[0] != "A" || deps[1] != "B" {
		t.Errorf("stage C dependsOn = %v, want [A B]", deps)
	}
}

func TestDependsOnNone(t *testing.T) {
	p := fixture(t, "stages.yml")
	if len(p.Stages[0].DependsOn) != 0 {
		t.Errorf("first stage should have no dependsOn, got %v", p.Stages[0].DependsOn)
	}
}

func TestJobDependsOn(t *testing.T) {
	p := fixture(t, "multi-job.yml")
	jobB := p.Stages[0].Jobs[1]
	if len(jobB.DependsOn) != 1 || jobB.DependsOn[0] != "JobA" {
		t.Errorf("JobB dependsOn = %v, want [JobA]", jobB.DependsOn)
	}
}

// ── Step type tests ───────────────────────────────────────────────────────────

func TestStepTypeScript(t *testing.T) {
	p := fixture(t, "step-types.yml")
	step := p.Stages[0].Jobs[0].Steps[0]
	if step.Type() != pipeline.StepTypeScript {
		t.Errorf("step[0].Type() = %v, want script", step.Type())
	}
	if step.Script != "echo hello from script" {
		t.Errorf("step[0].Script = %q", step.Script)
	}
}

func TestStepTypeBash(t *testing.T) {
	p := fixture(t, "step-types.yml")
	step := p.Stages[0].Jobs[0].Steps[1]
	if step.Type() != pipeline.StepTypeBash {
		t.Errorf("step[1].Type() = %v, want bash", step.Type())
	}
	if step.Bash != "echo hello from bash" {
		t.Errorf("step[1].Bash = %q", step.Bash)
	}
}

func TestStepTypePwsh(t *testing.T) {
	p := fixture(t, "step-types.yml")
	step := p.Stages[0].Jobs[0].Steps[2]
	if step.Type() != pipeline.StepTypePwsh {
		t.Errorf("step[2].Type() = %v, want pwsh", step.Type())
	}
	if step.Pwsh != `Write-Host "hello from pwsh"` {
		t.Errorf("step[2].Pwsh = %q", step.Pwsh)
	}
}

func TestStepTypeCheckout(t *testing.T) {
	p := fixture(t, "step-types.yml")
	step := p.Stages[0].Jobs[0].Steps[3]
	if step.Type() != pipeline.StepTypeCheckout {
		t.Errorf("step[3].Type() = %v, want checkout", step.Type())
	}
	if step.Checkout != "self" {
		t.Errorf("step[3].Checkout = %q, want 'self'", step.Checkout)
	}
	if !step.Clean {
		t.Error("step[3].Clean should be true")
	}
}

func TestStepTypeTask(t *testing.T) {
	p := fixture(t, "step-types.yml")
	step := p.Stages[0].Jobs[0].Steps[4]
	if step.Type() != pipeline.StepTypeTask {
		t.Errorf("step[4].Type() = %v, want task", step.Type())
	}
	if step.Task != "UseDotNet@2" {
		t.Errorf("step[4].Task = %q, want 'UseDotNet@2'", step.Task)
	}
	if step.Inputs["version"] != "8.x" {
		t.Errorf("step[4].Inputs[version] = %v, want '8.x'", step.Inputs["version"])
	}
}

func TestStepTypeTemplate(t *testing.T) {
	p := fixture(t, "step-types.yml")
	step := p.Stages[0].Jobs[0].Steps[5]
	if step.Type() != pipeline.StepTypeTemplate {
		t.Errorf("step[5].Type() = %v, want template", step.Type())
	}
	if step.Template != "templates/build.yml" {
		t.Errorf("step[5].Template = %q, want 'templates/build.yml'", step.Template)
	}
	if step.Parameters["config"] != "Release" {
		t.Errorf("step[5].Parameters[config] = %v, want 'Release'", step.Parameters["config"])
	}
}

func TestStepEnvVars(t *testing.T) {
	p := fixture(t, "step-types.yml")
	step := p.Stages[0].Jobs[0].Steps[6]
	if step.Env["MY_VAR"] != "hello" {
		t.Errorf("step env MY_VAR = %q, want 'hello'", step.Env["MY_VAR"])
	}
	if step.Env["ANOTHER"] != "world" {
		t.Errorf("step env ANOTHER = %q, want 'world'", step.Env["ANOTHER"])
	}
}

func TestStepTimeout(t *testing.T) {
	p := fixture(t, "step-types.yml")
	step := p.Stages[0].Jobs[0].Steps[7]
	if step.TimeoutInMinutes != 10 {
		t.Errorf("step timeoutInMinutes = %d, want 10", step.TimeoutInMinutes)
	}
}

func TestStepName(t *testing.T) {
	p := fixture(t, "step-types.yml")
	step := p.Stages[0].Jobs[0].Steps[8]
	if step.Name != "myNamedStep" {
		t.Errorf("step name = %q, want 'myNamedStep'", step.Name)
	}
}

// ── Step label tests ──────────────────────────────────────────────────────────

func TestLabelUsesDisplayName(t *testing.T) {
	p := fixture(t, "step-types.yml")
	step := p.Stages[0].Jobs[0].Steps[0]
	if step.Label() != "Script step" {
		t.Errorf("Label() = %q, want 'Script step'", step.Label())
	}
}

func TestLabelFallsBackToScript(t *testing.T) {
	// minimal.yml steps all have displayName; use ParseBytes for a step without one.
	raw := []byte(`steps:
  - script: echo hello
`)
	parsed, _ := pipeline.ParseBytes(raw)
	step := parsed.Stages[0].Jobs[0].Steps[0]
	if step.Label() != "script: echo hello" {
		t.Errorf("Label() = %q, want 'script: echo hello'", step.Label())
	}
}

func TestLabelTruncatesLongScript(t *testing.T) {
	long := "echo " + strings.Repeat("x", 70)
	raw := []byte("steps:\n  - script: " + long + "\n")
	parsed, err := pipeline.ParseBytes(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	step := parsed.Stages[0].Jobs[0].Steps[0]
	label := step.Label()
	if len([]rune(label)) > 70 {
		t.Errorf("Label() not truncated: len=%d", len(label))
	}
	if label[len(label)-3:] != "..." {
		t.Errorf("Label() not truncated with '...': %q", label)
	}
}

func TestLabelTask(t *testing.T) {
	raw := []byte(`steps:
  - task: SomeTask@1
`)
	parsed, _ := pipeline.ParseBytes(raw)
	step := parsed.Stages[0].Jobs[0].Steps[0]
	if step.Label() != "task: SomeTask@1" {
		t.Errorf("Label() = %q, want 'task: SomeTask@1'", step.Label())
	}
}

func TestLabelCheckout(t *testing.T) {
	raw := []byte(`steps:
  - checkout: self
`)
	parsed, _ := pipeline.ParseBytes(raw)
	step := parsed.Stages[0].Jobs[0].Steps[0]
	if step.Label() != "checkout: self" {
		t.Errorf("Label() = %q, want 'checkout: self'", step.Label())
	}
}

func TestLabelTemplate(t *testing.T) {
	raw := []byte(`steps:
  - template: path/to/template.yml
`)
	parsed, _ := pipeline.ParseBytes(raw)
	step := parsed.Stages[0].Jobs[0].Steps[0]
	if step.Label() != "template: path/to/template.yml" {
		t.Errorf("Label() = %q, want 'template: path/to/template.yml'", step.Label())
	}
}

// ── continueOnError tests ─────────────────────────────────────────────────────

func TestContinueOnError_True(t *testing.T) {
	p := fixture(t, "continue-on-error.yml")
	step := p.Stages[0].Jobs[0].Steps[0]
	if !step.ContinueOnError {
		t.Error("step[0].continueOnError should be true")
	}
}

func TestContinueOnError_DefaultFalse(t *testing.T) {
	p := fixture(t, "continue-on-error.yml")
	step := p.Stages[0].Jobs[0].Steps[1]
	if step.ContinueOnError {
		t.Error("step[1].continueOnError should default to false")
	}
}

// ── Deployment job / strategy tests ──────────────────────────────────────────

func TestDeploymentJob_Name(t *testing.T) {
	p := fixture(t, "deployment-job.yml")
	job := p.Stages[0].Jobs[0]
	if job.Deployment != "DeployProd" {
		t.Errorf("job.Deployment = %q, want 'DeployProd'", job.Deployment)
	}
	if job.Environment != "production" {
		t.Errorf("job.Environment = %q, want 'production'", job.Environment)
	}
}

func TestDeploymentJob_StepsExtractedFromStrategy(t *testing.T) {
	p := fixture(t, "deployment-job.yml")
	job := p.Stages[0].Jobs[0]
	if len(job.Steps) != 2 {
		t.Fatalf("expected 2 steps from strategy, got %d", len(job.Steps))
	}
	if job.Steps[0].DisplayName != "Deploy app" {
		t.Errorf("steps[0].displayName = %q, want 'Deploy app'", job.Steps[0].DisplayName)
	}
	if job.Steps[1].DisplayName != "Smoke test" {
		t.Errorf("steps[1].displayName = %q, want 'Smoke test'", job.Steps[1].DisplayName)
	}
}

func TestDeploymentJob_InMainExample(t *testing.T) {
	p, err := pipeline.Parse("../examples/azure-pipelines.yml")
	if err != nil {
		t.Fatalf("parse main example: %v", err)
	}
	deployStage := p.Stages[4]
	job := deployStage.Jobs[0]
	if job.Deployment != "DeployStaging" {
		t.Errorf("deploy job.Deployment = %q, want 'DeployStaging'", job.Deployment)
	}
	if len(job.Steps) == 0 {
		t.Error("deploy job should have steps extracted from strategy")
	}
}

// ── Variables tests ───────────────────────────────────────────────────────────

func TestVariables_PipelineLevel(t *testing.T) {
	p := fixture(t, "variables.yml")
	if p.Variables["pipelineVar"] != "pipeline-level" {
		t.Errorf("pipelineVar = %q, want 'pipeline-level'", p.Variables["pipelineVar"])
	}
}

func TestVariables_StageLevel(t *testing.T) {
	p := fixture(t, "variables.yml")
	if p.Stages[0].Variables["stageVar"] != "stage-level" {
		t.Errorf("stageVar = %q, want 'stage-level'", p.Stages[0].Variables["stageVar"])
	}
}

func TestVariables_JobLevel(t *testing.T) {
	p := fixture(t, "variables.yml")
	if p.Stages[0].Jobs[0].Variables["jobVar"] != "job-level" {
		t.Errorf("jobVar = %q, want 'job-level'", p.Stages[0].Jobs[0].Variables["jobVar"])
	}
}

// ── Flatten tests ─────────────────────────────────────────────────────────────

func TestFlatten_Count(t *testing.T) {
	p := fixture(t, "stages.yml")
	flat := pipeline.Flatten(p)
	// Build: 2 steps, Test: 1 step
	if len(flat) != 3 {
		t.Fatalf("expected 3 flat steps, got %d", len(flat))
	}
}

func TestFlatten_StageNames(t *testing.T) {
	p := fixture(t, "stages.yml")
	flat := pipeline.Flatten(p)
	if flat[0].StageName != "Build Stage" {
		t.Errorf("flat[0].StageName = %q, want 'Build Stage'", flat[0].StageName)
	}
	if flat[2].StageName != "Test" {
		t.Errorf("flat[2].StageName = %q, want 'Test'", flat[2].StageName)
	}
}

func TestFlatten_StepIndices(t *testing.T) {
	flat := pipeline.Flatten(fixture(t, "stages.yml"))
	if flat[0].StepIndex != 0 || flat[1].StepIndex != 1 {
		t.Errorf("Build job step indices: [%d, %d], want [0, 1]", flat[0].StepIndex, flat[1].StepIndex)
	}
	if flat[2].StepIndex != 0 {
		t.Errorf("Test job step index: %d, want 0", flat[2].StepIndex)
	}
}

func TestFlatten_PreservesStepPointers(t *testing.T) {
	p := fixture(t, "stages.yml")
	flat := pipeline.Flatten(p)
	// Verify the Step pointer actually points into the pipeline's step slice
	if flat[0].Step != &p.Stages[0].Jobs[0].Steps[0] {
		t.Error("flat[0].Step should point to pipeline.Stages[0].Jobs[0].Steps[0]")
	}
}

func TestFlatten_MultiJob(t *testing.T) {
	p := fixture(t, "multi-job.yml")
	flat := pipeline.Flatten(p)
	if len(flat) != 3 {
		t.Fatalf("expected 3 flat steps, got %d", len(flat))
	}
	if flat[0].JobName != "Job A" {
		t.Errorf("flat[0].JobName = %q, want 'Job A'", flat[0].JobName)
	}
	if flat[2].JobName != "Job B" {
		t.Errorf("flat[2].JobName = %q, want 'Job B'", flat[2].JobName)
	}
}

func TestFlatten_DeploymentJob(t *testing.T) {
	p := fixture(t, "deployment-job.yml")
	flat := pipeline.Flatten(p)
	if len(flat) != 2 {
		t.Fatalf("expected 2 flat steps from deployment job, got %d", len(flat))
	}
	if flat[0].JobName != "DeployProd" {
		t.Errorf("flat[0].JobName = %q, want 'DeployProd'", flat[0].JobName)
	}
}

// ── Main example file test ────────────────────────────────────────────────────

func TestParseMainExample(t *testing.T) {
	p, err := pipeline.Parse("../examples/azure-pipelines.yml")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if p.Name != "stepthrough-ci" {
		t.Errorf("name = %q, want 'stepthrough-ci'", p.Name)
	}
	if len(p.Stages) != 5 {
		t.Fatalf("expected 5 stages, got %d", len(p.Stages))
	}
	// Validate trigger
	if len(p.Trigger.Branches.Include) != 1 {
		t.Errorf("trigger branches = %v, want [main]", p.Trigger.Branches.Include)
	}
	// All stages should have jobs
	for _, stage := range p.Stages {
		if len(stage.Jobs) == 0 {
			t.Errorf("stage %q has no jobs", stage.Stage)
		}
	}
}

func TestParseMainExample_TotalStepCount(t *testing.T) {
	p, err := pipeline.Parse("../examples/azure-pipelines.yml")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	flat := pipeline.Flatten(p)
	if len(flat) == 0 {
		t.Error("expected steps, got 0")
	}
	// Validate every step has a non-empty label
	for i, fs := range flat {
		if fs.Step.Label() == "" || fs.Step.Label() == "(unknown step)" {
			t.Errorf("flat[%d] has empty/unknown label", i)
		}
	}
}

// ── Error handling tests ──────────────────────────────────────────────────────

func TestParseInvalidYAML(t *testing.T) {
	_, err := pipeline.ParseBytes([]byte(":: invalid yaml ::"))
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestParseEmptyFile(t *testing.T) {
	_, err := pipeline.ParseBytes([]byte(""))
	if err != nil {
		t.Errorf("empty file should not error, got: %v", err)
	}
}

func TestParseFileNotFound(t *testing.T) {
	_, err := pipeline.Parse("../examples/fixtures/does-not-exist.yml")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

// ── Enabled default test ──────────────────────────────────────────────────────

func TestStepEnabledDefaultsTrue(t *testing.T) {
	p := fixture(t, "stages.yml")
	for _, stage := range p.Stages {
		for _, job := range stage.Jobs {
			for i, step := range job.Steps {
				if !step.Enabled {
					t.Errorf("stage %q job %q step[%d] Enabled should default to true", stage.Stage, job.Job, i)
				}
			}
		}
	}
}
