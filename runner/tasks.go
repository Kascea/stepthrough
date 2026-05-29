package runner

import (
	"fmt"
	"strings"

	"github.com/colecarlson/stepthrough/pipeline"
)

// ResolveStep converts a pipeline step into a bash script suitable for running
// inside the container. Returns (script, true) or ("", false) if unsupported.
func ResolveStep(step *pipeline.Step) (script string, supported bool) {
	switch step.Type() {
	case pipeline.StepTypeScript:
		return step.Script, true
	case pipeline.StepTypeBash:
		return step.Bash, true
	case pipeline.StepTypePwsh, pipeline.StepTypePowerShell:
		// pwsh is not installed in ubuntu:22.04 by default — install then run.
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
	case pipeline.StepTypeCheckout:
		if step.Checkout == "none" {
			return `echo "[checkout] skipped (checkout: none)"`, true
		}
		// Workspace is already mounted; just confirm.
		return `echo "[checkout] workspace already mounted at /workspace"`, true
	case pipeline.StepTypeTask:
		return resolveTask(step)
	case pipeline.StepTypeTemplate:
		return fmt.Sprintf(`echo "[template] %s — templates are not expanded locally"`, step.Template), true
	default:
		return "", false
	}
}

// resolveTask maps common Azure task names to bash scripts.
func resolveTask(step *pipeline.Step) (string, bool) {
	base := taskBase(step.Task)
	inputs := step.Inputs

	switch base {
	case "UseDotNet":
		version := strInput(inputs, "version", "8.x")
		channel := strings.SplitN(version, ".", 2)[0]
		return fmt.Sprintf(`
curl -sSL https://dot.net/v1/dotnet-install.sh -o /tmp/dotnet-install.sh
bash /tmp/dotnet-install.sh --channel %s --install-dir /usr/local/bin --no-path
ln -sf /usr/local/bin/dotnet /usr/local/bin/dotnet
export PATH="$PATH:/usr/local/bin"
dotnet --version
`, channel), true

	case "UseNode", "NodeTool":
		version := strInput(inputs, "versionSpec", strInput(inputs, "version", "18"))
		major := strings.SplitN(version, ".", 2)[0]
		return fmt.Sprintf(`
curl -fsSL https://deb.nodesource.com/setup_%s.x | bash -
apt-get install -y nodejs
node --version && npm --version
`, major), true

	case "UsePythonVersion":
		version := strInput(inputs, "versionSpec", "3.x")
		major := strings.SplitN(version, ".", 2)[0]
		return fmt.Sprintf(`
apt-get install -y -qq python%s python%s-pip
python%s --version
`, major, major, major), true

	case "DotNetCoreCLI":
		command := strInput(inputs, "command", "build")
		projects := strInput(inputs, "projects", "**/*.csproj")
		args := strInput(inputs, "arguments", "")
		return fmt.Sprintf(`dotnet %s %s %s`, command, projects, args), true

	case "NuGetCommand":
		command := strInput(inputs, "command", "restore")
		solution := strInput(inputs, "solution", "**/*.sln")
		return fmt.Sprintf(`nuget %s %s`, command, solution), true

	case "CopyFiles":
		src := strInput(inputs, "SourceFolder", ".")
		dst := strInput(inputs, "TargetFolder", "_artifacts")
		return fmt.Sprintf(`mkdir -p %q && cp -r %s/. %q`, dst, src, dst), true

	case "PublishBuildArtifacts", "PublishPipelineArtifact":
		path := strInput(inputs, "PathtoPublish", strInput(inputs, "path", "."))
		name := strInput(inputs, "ArtifactName", strInput(inputs, "artifact", "drop"))
		return fmt.Sprintf(`echo "[publish] artifact '%s' from %s — skipped (no artifact server locally)"`, name, path), true

	case "PublishTestResults":
		return fmt.Sprintf(`echo "[publish] test results from %s — skipped (no test reporting server locally)"`,
			strInput(inputs, "testResultsFiles", "**/*.xml")), true

	case "DownloadBuildArtifacts", "DownloadPipelineArtifact":
		return `echo "[download] artifact download skipped (no artifact server locally)"`, true

	case "Docker":
		command := strInput(inputs, "command", "build")
		return fmt.Sprintf(`echo "[docker] docker %s — Docker-in-Docker not supported locally"`, command), true

	case "AzureCLI":
		return `echo "[task] AzureCLI — Azure CLI tasks require authentication, skipping"`, true

	case "Bash", "CmdLine":
		script := strInput(inputs, "script", strInput(inputs, "Script", ""))
		if script != "" {
			return script, true
		}
		return fmt.Sprintf(`echo "[task] %s — no script provided"`, step.Task), true

	default:
		// Unknown task — log and continue.
		return fmt.Sprintf(`echo "[task] %s — not supported locally, skipping"`, step.Task), true
	}
}

// taskBase strips the version suffix: "UseDotNet@2" → "UseDotNet".
func taskBase(task string) string {
	if i := strings.IndexByte(task, '@'); i >= 0 {
		return task[:i]
	}
	return task
}

func strInput(inputs map[string]interface{}, key, defaultVal string) string {
	if inputs == nil {
		return defaultVal
	}
	if v, ok := inputs[key]; ok {
		return fmt.Sprintf("%v", v)
	}
	return defaultVal
}
