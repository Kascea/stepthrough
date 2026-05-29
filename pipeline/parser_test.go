package pipeline_test

import (
	"testing"

	"github.com/colecarlson/stepthrough/pipeline"
)

const minimalPipeline = `
pool:
  vmImage: ubuntu-latest
steps:
  - script: echo hello
    displayName: Say hello
  - bash: echo world
    displayName: Say world
`

const stagesPipeline = `
variables:
  buildConfig: Release

stages:
  - stage: Build
    displayName: Build Stage
    jobs:
      - job: BuildJob
        steps:
          - script: dotnet build
            displayName: Build
          - task: PublishBuildArtifacts@1
            displayName: Publish
  - stage: Test
    dependsOn: Build
    jobs:
      - job: TestJob
        steps:
          - script: dotnet test
            displayName: Run tests
`

const dependsOnStringPipeline = `
stages:
  - stage: A
    jobs:
      - job: A1
        steps:
          - script: echo A
  - stage: B
    dependsOn: A
    jobs:
      - job: B1
        steps:
          - script: echo B
`

const dependsOnListPipeline = `
stages:
  - stage: A
    jobs:
      - job: A1
        steps:
          - script: echo A
  - stage: B
    jobs:
      - job: B1
        steps:
          - script: echo B
  - stage: C
    dependsOn:
      - A
      - B
    jobs:
      - job: C1
        steps:
          - script: echo C
`

const continueOnErrorPipeline = `
stages:
  - stage: S
    jobs:
      - job: J
        steps:
          - script: exit 1
            displayName: Failing step
            continueOnError: true
`

func TestParseMinimal(t *testing.T) {
	p, err := pipeline.ParseBytes([]byte(minimalPipeline))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.Stages) != 1 {
		t.Fatalf("expected 1 synthetic stage, got %d", len(p.Stages))
	}
	if len(p.Stages[0].Jobs) != 1 {
		t.Fatalf("expected 1 synthetic job, got %d", len(p.Stages[0].Jobs))
	}
	if len(p.Stages[0].Jobs[0].Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(p.Stages[0].Jobs[0].Steps))
	}
}

func TestParseStages(t *testing.T) {
	p, err := pipeline.ParseBytes([]byte(stagesPipeline))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.Stages) != 2 {
		t.Fatalf("expected 2 stages, got %d", len(p.Stages))
	}
	if p.Stages[0].Stage != "Build" {
		t.Errorf("expected stage[0] = Build, got %q", p.Stages[0].Stage)
	}
	if p.Stages[0].DisplayName != "Build Stage" {
		t.Errorf("expected displayName = 'Build Stage', got %q", p.Stages[0].DisplayName)
	}
	if p.Variables["buildConfig"] != "Release" {
		t.Errorf("expected variable buildConfig=Release, got %q", p.Variables["buildConfig"])
	}
	if len(p.Stages[1].DependsOn) != 1 || p.Stages[1].DependsOn[0] != "Build" {
		t.Errorf("expected Test dependsOn=[Build], got %v", p.Stages[1].DependsOn)
	}
}

func TestDependsOnString(t *testing.T) {
	p, err := pipeline.ParseBytes([]byte(dependsOnStringPipeline))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.Stages[1].DependsOn) != 1 || p.Stages[1].DependsOn[0] != "A" {
		t.Errorf("expected B dependsOn=[A], got %v", p.Stages[1].DependsOn)
	}
}

func TestDependsOnList(t *testing.T) {
	p, err := pipeline.ParseBytes([]byte(dependsOnListPipeline))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	deps := p.Stages[2].DependsOn
	if len(deps) != 2 {
		t.Fatalf("expected C dependsOn=[A,B], got %v", deps)
	}
	if deps[0] != "A" || deps[1] != "B" {
		t.Errorf("wrong dependsOn values: %v", deps)
	}
}

func TestContinueOnError(t *testing.T) {
	p, err := pipeline.ParseBytes([]byte(continueOnErrorPipeline))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	step := p.Stages[0].Jobs[0].Steps[0]
	if !step.ContinueOnError {
		t.Error("expected continueOnError=true")
	}
}

func TestFlatten(t *testing.T) {
	p, err := pipeline.ParseBytes([]byte(stagesPipeline))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	flat := pipeline.Flatten(p)
	// Build has 2 steps, Test has 1 step.
	if len(flat) != 3 {
		t.Fatalf("expected 3 flat steps, got %d", len(flat))
	}
	if flat[0].StageName != "Build Stage" {
		t.Errorf("flat[0] stage = %q, want 'Build Stage'", flat[0].StageName)
	}
	if flat[2].StageName != "Test" {
		t.Errorf("flat[2] stage = %q, want 'Test'", flat[2].StageName)
	}
}

func TestStepLabel(t *testing.T) {
	p, err := pipeline.ParseBytes([]byte(minimalPipeline))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	steps := p.Stages[0].Jobs[0].Steps
	if steps[0].Label() != "Say hello" {
		t.Errorf("expected 'Say hello', got %q", steps[0].Label())
	}
}

func TestStepType(t *testing.T) {
	p, err := pipeline.ParseBytes([]byte(minimalPipeline))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	steps := p.Stages[0].Jobs[0].Steps
	if steps[0].Type() != pipeline.StepTypeScript {
		t.Errorf("expected StepTypeScript, got %v", steps[0].Type())
	}
	if steps[1].Type() != pipeline.StepTypeBash {
		t.Errorf("expected StepTypeBash, got %v", steps[1].Type())
	}
}

func TestParseFile(t *testing.T) {
	p, err := pipeline.Parse("../examples/azure-pipelines.yml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.Stages) != 4 {
		t.Fatalf("expected 4 stages, got %d", len(p.Stages))
	}
	if p.Name != "ExamplePipeline" {
		t.Errorf("expected name=ExamplePipeline, got %q", p.Name)
	}
}

func TestParseInvalidYAML(t *testing.T) {
	_, err := pipeline.ParseBytes([]byte(":: invalid yaml ::"))
	if err == nil {
		t.Error("expected error for invalid YAML, got nil")
	}
}
