package runner

import (
	"fmt"
	"strings"

	"github.com/colecarlson/stepthrough/pipeline"
)

type taskContext struct {
	Step       *pipeline.Step
	Definition taskDefinition
	Inputs     map[string]string
}

type taskAdapter interface {
	Resolve(ctx taskContext) (string, bool)
}

var taskAdapters = map[string]taskAdapter{
	"GoTool":                   goToolAdapter{},
	"UseDotNet":                useDotNetAdapter{},
	"UseNode":                  useNodeAdapter{},
	"NodeTool":                 useNodeAdapter{},
	"UsePythonVersion":         usePythonVersionAdapter{},
	"DotNetCoreCLI":            dotNetCoreCLIAdapter{},
	"NuGetCommand":             nuGetCommandAdapter{},
	"CopyFiles":                copyFilesAdapter{},
	"PublishBuildArtifacts":    publishArtifactAdapter{},
	"PublishPipelineArtifact":  publishArtifactAdapter{},
	"PublishTestResults":       publishTestResultsAdapter{},
	"DownloadBuildArtifacts":   downloadArtifactAdapter{},
	"DownloadPipelineArtifact": downloadArtifactAdapter{},
	"Docker":                   dockerAdapter{},
	"AzureCLI":                 azureCLIAdapter{},
	"Bash":                     commandLineAdapter{},
	"CmdLine":                  commandLineAdapter{},
}

type goToolAdapter struct{}

func (goToolAdapter) Resolve(ctx taskContext) (string, bool) {
	version := input(ctx, "version")
	return fmt.Sprintf(`
version_spec=%s
if [ -z "$version_spec" ]; then
  echo "[task] GoTool could not resolve version"
  exit 1
fi
version_spec="${version_spec#go}"
if [[ "$version_spec" == *.x ]]; then
  prefix="go${version_spec%%.x}."
  go_version=$(curl -fsSL 'https://go.dev/dl/?mode=json&include=all' | tr -d '\n' | sed 's/},{/}\n{/g' | sed -n "s/.*\"version\":\"\(${prefix}[0-9][^\"]*\)\".*/\1/p" | head -1)
else
  go_version="go$version_spec"
fi
if [ -z "$go_version" ]; then
  echo "[task] GoTool could not find Go version for '$version_spec'"
  exit 1
fi
echo "Installing $go_version..."
curl -fsSL "https://go.dev/dl/${go_version}.linux-amd64.tar.gz" -o /tmp/go.tar.gz
rm -rf /usr/local/go
tar -xzf /tmp/go.tar.gz -C /usr/local
ln -sf /usr/local/go/bin/go /usr/local/bin/go
ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt
go version
`, shellValue(version)), true
}

type useDotNetAdapter struct{}

func (useDotNetAdapter) Resolve(ctx taskContext) (string, bool) {
	channel := strings.SplitN(input(ctx, "version"), ".", 2)[0]
	return fmt.Sprintf(`
curl -sSL https://dot.net/v1/dotnet-install.sh -o /tmp/dotnet-install.sh
bash /tmp/dotnet-install.sh --channel %s --install-dir /usr/local/bin --no-path
ln -sf /usr/local/bin/dotnet /usr/local/bin/dotnet
export PATH="$PATH:/usr/local/bin"
dotnet --version
`, channel), true
}

type useNodeAdapter struct{}

func (useNodeAdapter) Resolve(ctx taskContext) (string, bool) {
	major := strings.SplitN(input(ctx, "versionSpec"), ".", 2)[0]
	return fmt.Sprintf(`
curl -fsSL https://deb.nodesource.com/setup_%s.x | bash -
apt-get install -y nodejs
node --version && npm --version
`, major), true
}

type usePythonVersionAdapter struct{}

func (usePythonVersionAdapter) Resolve(ctx taskContext) (string, bool) {
	major := strings.SplitN(input(ctx, "versionSpec"), ".", 2)[0]
	return fmt.Sprintf(`
apt-get install -y -qq python%s python%s-pip
python%s --version
`, major, major, major), true
}

type dotNetCoreCLIAdapter struct{}

func (dotNetCoreCLIAdapter) Resolve(ctx taskContext) (string, bool) {
	return fmt.Sprintf(`dotnet %s %s %s`, input(ctx, "command"), input(ctx, "projects"), input(ctx, "arguments")), true
}

type nuGetCommandAdapter struct{}

func (nuGetCommandAdapter) Resolve(ctx taskContext) (string, bool) {
	return fmt.Sprintf(`nuget %s %s`, input(ctx, "command"), input(ctx, "solution")), true
}

type copyFilesAdapter struct{}

func (copyFilesAdapter) Resolve(ctx taskContext) (string, bool) {
	return fmt.Sprintf(`mkdir -p %q && cp -r %s/. %q`, input(ctx, "TargetFolder"), input(ctx, "SourceFolder"), input(ctx, "TargetFolder")), true
}

type publishArtifactAdapter struct{}

func (publishArtifactAdapter) Resolve(ctx taskContext) (string, bool) {
	return fmt.Sprintf(`echo "[publish] artifact '%s' from %s - skipped (no artifact server locally)"`, input(ctx, "ArtifactName"), input(ctx, "PathtoPublish")), true
}

type publishTestResultsAdapter struct{}

func (publishTestResultsAdapter) Resolve(ctx taskContext) (string, bool) {
	return fmt.Sprintf(`echo "[publish] test results from %s - skipped (no test reporting server locally)"`, input(ctx, "testResultsFiles")), true
}

type downloadArtifactAdapter struct{}

func (downloadArtifactAdapter) Resolve(taskContext) (string, bool) {
	return `echo "[download] artifact download skipped (no artifact server locally)"`, true
}

type dockerAdapter struct{}

func (dockerAdapter) Resolve(ctx taskContext) (string, bool) {
	return fmt.Sprintf(`echo "[docker] docker %s - Docker-in-Docker not supported locally"`, input(ctx, "command")), true
}

type azureCLIAdapter struct{}

func (azureCLIAdapter) Resolve(taskContext) (string, bool) {
	return `echo "[task] AzureCLI - Azure CLI tasks require authentication, skipping"`, true
}

type commandLineAdapter struct{}

func (commandLineAdapter) Resolve(ctx taskContext) (string, bool) {
	if script := input(ctx, "script"); script != "" {
		return script, true
	}
	return fmt.Sprintf(`echo "[task] %s - no script provided"`, ctx.Step.Task), true
}

func input(ctx taskContext, name string) string {
	return ctx.Inputs[name]
}
