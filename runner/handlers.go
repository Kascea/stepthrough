package runner

import (
	"fmt"

	"github.com/colecarlson/stepthrough/pipeline"
)

// ResolveStep selects a local handler for an Azure Pipeline step and returns the
// script that handler wants executed. This mirrors the Azure agent shape: choose
// a handler first, then let the step host execute the handler output.
func ResolveStep(step *pipeline.Step) (script string, supported bool) {
	for _, handler := range stepHandlers {
		if handler.Supports(step) {
			return handler.Resolve(step)
		}
	}
	return "", false
}

type stepHandler interface {
	Supports(step *pipeline.Step) bool
	Resolve(step *pipeline.Step) (string, bool)
}

var stepHandlers = []stepHandler{
	scriptHandler{},
	bashHandler{},
	pwshHandler{},
	checkoutHandler{},
	taskHandler{},
	templateHandler{},
}

type scriptHandler struct{}

func (scriptHandler) Supports(step *pipeline.Step) bool {
	return step.Type() == pipeline.StepTypeScript
}
func (scriptHandler) Resolve(step *pipeline.Step) (string, bool) {
	return expandAzureVariables(step.Script), true
}

type bashHandler struct{}

func (bashHandler) Supports(step *pipeline.Step) bool { return step.Type() == pipeline.StepTypeBash }
func (bashHandler) Resolve(step *pipeline.Step) (string, bool) {
	return expandAzureVariables(step.Bash), true
}

type pwshHandler struct{}

func (pwshHandler) Supports(step *pipeline.Step) bool {
	return step.Type() == pipeline.StepTypePwsh || step.Type() == pipeline.StepTypePowerShell
}

func (pwshHandler) Resolve(step *pipeline.Step) (string, bool) {
	src := step.Pwsh
	if src == "" {
		src = step.Powershell
	}
	return fmt.Sprintf(`
if ! command -v pwsh >/dev/null 2>&1; then
  apt-get install -y -qq wget && \
  wget -q https://packages.microsoft.com/config/ubuntu/22.04/packages-microsoft-prod.deb && \
  dpkg -i packages-microsoft-prod.deb && \
  apt-get update -qq && apt-get install -y -qq powershell
fi
pwsh -Command %q
`, src), true
}

type checkoutHandler struct{}

func (checkoutHandler) Supports(step *pipeline.Step) bool {
	return step.Type() == pipeline.StepTypeCheckout
}
func (checkoutHandler) Resolve(step *pipeline.Step) (string, bool) {
	if step.Checkout == "none" {
		return `echo "[checkout] skipped (checkout: none)"`, true
	}
	return `echo "[checkout] workspace already mounted at /workspace"`, true
}

type templateHandler struct{}

func (templateHandler) Supports(step *pipeline.Step) bool {
	return step.Type() == pipeline.StepTypeTemplate
}
func (templateHandler) Resolve(step *pipeline.Step) (string, bool) {
	return fmt.Sprintf(`echo "[template] %s - templates are not expanded locally"`, step.Template), true
}

type taskHandler struct{}

func (taskHandler) Supports(step *pipeline.Step) bool { return step.Type() == pipeline.StepTypeTask }
func (taskHandler) Resolve(step *pipeline.Step) (string, bool) {
	definition, ok := taskDefinitionFor(step.Task)
	if !ok {
		return fmt.Sprintf(`echo "[task] %s - task definition not found in azure-pipelines-tasks, skipping"`, step.Task), true
	}
	if definition.Adapter == nil {
		return fmt.Sprintf(`echo "[task] %s - task definition loaded, but no local execution adapter exists yet"`, step.Task), true
	}
	return definition.Adapter.Resolve(taskContext{
		Step:       step,
		Definition: definition,
		Inputs:     definition.ResolveInputs(step.Inputs),
	})
}
