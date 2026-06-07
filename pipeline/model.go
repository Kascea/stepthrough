package pipeline

import (
	"time"

	"gopkg.in/yaml.v3"
)

// Variables is a pipeline variable map that handles both YAML forms:
//   - object form: key: value
//   - sequence form: [{name: key, value: val}, ...]
type Variables map[string]string

func (v *Variables) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.MappingNode:
		var m map[string]string
		if err := value.Decode(&m); err != nil {
			return err
		}
		*v = m
	case yaml.SequenceNode:
		*v = make(Variables)
		for _, item := range value.Content {
			var entry struct {
				Name  string `yaml:"name"`
				Value string `yaml:"value"`
			}
			if err := item.Decode(&entry); err != nil {
				return err
			}
			if entry.Name != "" {
				(*v)[entry.Name] = entry.Value
			}
		}
	}
	return nil
}

// Pipeline is the top-level representation of an azure-pipelines.yml file.
type Pipeline struct {
	Name      string    `yaml:"name"`
	Trigger   Trigger   `yaml:"trigger"`
	PR        PRTrigger `yaml:"pr"`
	Pool      Pool      `yaml:"pool"`
	Variables Variables `yaml:"variables"`
	Stages    []Stage   `yaml:"stages"`
	// Flat jobs at root level (no stages)
	Jobs []Job `yaml:"jobs"`
	// Flat steps at root level (no stages/jobs)
	Steps []Step `yaml:"steps"`
}

type Trigger struct {
	Branches BranchFilter `yaml:"branches"`
	Paths    PathFilter   `yaml:"paths"`
	Raw      string       `yaml:"-"`
}

func (t *Trigger) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		t.Raw = value.Value
	case yaml.SequenceNode:
		for _, n := range value.Content {
			t.Branches.Include = append(t.Branches.Include, n.Value)
		}
	case yaml.MappingNode:
		type triggerAlias Trigger
		var alias triggerAlias
		if err := value.Decode(&alias); err != nil {
			return err
		}
		*t = Trigger(alias)
	}
	return nil
}

type PRTrigger struct {
	Branches BranchFilter `yaml:"branches"`
	Drafts   bool         `yaml:"drafts"`
}

func (t *PRTrigger) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		// "none" or similar — no-op
	case yaml.SequenceNode:
		for _, n := range value.Content {
			t.Branches.Include = append(t.Branches.Include, n.Value)
		}
	case yaml.MappingNode:
		type prAlias PRTrigger
		var alias prAlias
		if err := value.Decode(&alias); err != nil {
			return err
		}
		*t = PRTrigger(alias)
	}
	return nil
}

type BranchFilter struct {
	Include []string `yaml:"include"`
	Exclude []string `yaml:"exclude"`
}

type PathFilter struct {
	Include []string `yaml:"include"`
	Exclude []string `yaml:"exclude"`
}

type Pool struct {
	VMImage string `yaml:"vmImage"`
	Name    string `yaml:"name"`
}

type Stage struct {
	Stage       string            `yaml:"stage"`
	DisplayName string            `yaml:"displayName"`
	DependsOn   []string          `yaml:"-"` // parsed manually (string or []string)
	Condition   string            `yaml:"condition"`
	Variables   Variables `yaml:"variables"`
	Pool        *Pool     `yaml:"pool"`
	Jobs        []Job     `yaml:"jobs"`
}

type Job struct {
	Job              string            `yaml:"job"`
	DisplayName      string            `yaml:"displayName"`
	DependsOn        []string          `yaml:"-"` // parsed manually
	Condition        string            `yaml:"condition"`
	Pool             *Pool     `yaml:"pool"`
	Variables        Variables `yaml:"variables"`
	TimeoutInMinutes int       `yaml:"timeoutInMinutes"`
	Steps            []Step            `yaml:"steps"`
	// Deployment job fields
	Deployment  string    `yaml:"deployment"`
	Environment string    `yaml:"environment"`
	Strategy    *Strategy `yaml:"strategy"`
}

// Strategy covers the runOnce/rolling/canary deployment strategies.
// For local debugging we only extract steps from whichever strategy is set.
type Strategy struct {
	RunOnce *LifecycleHooks `yaml:"runOnce"`
	Rolling *LifecycleHooks `yaml:"rolling"`
	Canary  *LifecycleHooks `yaml:"canary"`
}

// LifecycleHooks holds the step groups for each deployment phase.
type LifecycleHooks struct {
	PreDeploy        *PhaseSteps `yaml:"preDeploy"`
	Deploy           *PhaseSteps `yaml:"deploy"`
	RouteTraffic     *PhaseSteps `yaml:"routeTraffic"`
	PostRouteTraffic *PhaseSteps `yaml:"postRouteTraffic"`
}

// PhaseSteps groups the steps within one lifecycle phase.
type PhaseSteps struct {
	Steps []Step `yaml:"steps"`
}

// AllSteps returns steps from all defined lifecycle phases in declaration order.
func (h *LifecycleHooks) AllSteps() []Step {
	var out []Step
	for _, phase := range []*PhaseSteps{h.PreDeploy, h.Deploy, h.RouteTraffic, h.PostRouteTraffic} {
		if phase != nil {
			out = append(out, phase.Steps...)
		}
	}
	return out
}

// Step represents a single pipeline step, which may be a script, bash,
// powershell, checkout, task, or template reference.
type Step struct {
	// Common to all step types
	DisplayName      string            `yaml:"displayName"`
	Name             string            `yaml:"name"`
	Condition        string            `yaml:"condition"`
	ContinueOnError  bool              `yaml:"continueOnError"`
	Enabled          bool              `yaml:"-"` // defaults to true
	Env              map[string]string `yaml:"env"`
	TimeoutInMinutes int               `yaml:"timeoutInMinutes"`

	// script / bash / pwsh / powershell steps
	Script     string `yaml:"script"`
	Bash       string `yaml:"bash"`
	Pwsh       string `yaml:"pwsh"`
	Powershell string `yaml:"powershell"`

	// task step
	Task   string                 `yaml:"task"`
	Inputs map[string]interface{} `yaml:"inputs"`

	// checkout step
	Checkout string `yaml:"checkout"`
	Clean    bool   `yaml:"clean"`

	// template step
	Template   string                 `yaml:"template"`
	Parameters map[string]interface{} `yaml:"parameters"`
}

// StepType categorises what kind of step this is.
type StepType string

const (
	StepTypeScript     StepType = "script"
	StepTypeBash       StepType = "bash"
	StepTypePwsh       StepType = "pwsh"
	StepTypePowerShell StepType = "powershell"
	StepTypeTask       StepType = "task"
	StepTypeCheckout   StepType = "checkout"
	StepTypeTemplate   StepType = "template"
	StepTypeUnknown    StepType = "unknown"
)

// Type returns the resolved step type.
func (s *Step) Type() StepType {
	switch {
	case s.Script != "":
		return StepTypeScript
	case s.Bash != "":
		return StepTypeBash
	case s.Pwsh != "":
		return StepTypePwsh
	case s.Powershell != "":
		return StepTypePowerShell
	case s.Task != "":
		return StepTypeTask
	case s.Checkout != "":
		return StepTypeCheckout
	case s.Template != "":
		return StepTypeTemplate
	default:
		return StepTypeUnknown
	}
}

// Label returns a human-readable label for the step.
func (s *Step) Label() string {
	if s.DisplayName != "" {
		return s.DisplayName
	}
	switch s.Type() {
	case StepTypeScript:
		if len(s.Script) > 60 {
			return "script: " + s.Script[:57] + "..."
		}
		return "script: " + s.Script
	case StepTypeBash:
		if len(s.Bash) > 60 {
			return "bash: " + s.Bash[:57] + "..."
		}
		return "bash: " + s.Bash
	case StepTypePwsh, StepTypePowerShell:
		return "pwsh: " + s.Pwsh + s.Powershell
	case StepTypeTask:
		return "task: " + s.Task
	case StepTypeCheckout:
		return "checkout: " + s.Checkout
	case StepTypeTemplate:
		return "template: " + s.Template
	default:
		return "(unknown step)"
	}
}

// StepStatus is the execution outcome of a single pipeline step.
type StepStatus string

const (
	StepStatusPending    StepStatus = "pending"
	StepStatusRunning    StepStatus = "running"
	StepStatusPassed     StepStatus = "passed"
	StepStatusFailed     StepStatus = "failed"
	StepStatusSkipped    StepStatus = "skipped"
	StepStatusCached     StepStatus = "cached"
	StepStatusDeployment StepStatus = "deployment" // step belongs to a deployment job; never executed locally
)

// StepResult is the outcome of running a single step.
type StepResult struct {
	Status   StepStatus
	ExitCode int
	Duration time.Duration
	Err      error
}

// FlatStep is a fully-resolved step with its parent stage and job context.
type FlatStep struct {
	StageIndex      int
	StageName       string
	JobIndex        int
	JobName         string
	StepIndex       int
	Step            *Step
	IsDeploymentJob bool // true when the parent job is a deployment job (job.Deployment != "")
}
