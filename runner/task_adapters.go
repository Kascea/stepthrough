package runner

import (
	"fmt"
	"strings"

	"github.com/kascea/stepthrough/pipeline"
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
if command -v go >/dev/null 2>&1; then
  installed=$(go version | awk '{print $3}')
  if [ "$installed" = "$go_version" ]; then
    echo "[stepthrough] Go $go_version already installed (agent image)"
    go version
    exit 0
  fi
  echo "[stepthrough] WARNING: requested $go_version but agent image has $installed — installing. Use $installed for faster builds."
fi
echo "Installing $go_version..."
ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
curl -fsSL "https://go.dev/dl/${go_version}.linux-${ARCH}.tar.gz" -o /tmp/go.tar.gz
rm -rf /usr/local/go
tar -xzf /tmp/go.tar.gz -C /usr/local
ln -sf /usr/local/go/bin/go /usr/local/bin/go
ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt
go version
`, shellValue(version)), true
}

type useDotNetAdapter struct{}

func (useDotNetAdapter) Resolve(ctx taskContext) (string, bool) {
	version := input(ctx, "version")
	major := strings.SplitN(version, ".", 2)[0]
	// dotnet-install.sh requires a channel like "10.0", not just "10".
	channel := major + ".0"
	return fmt.Sprintf(`
major=%s
channel=%s
if command -v dotnet >/dev/null 2>&1; then
  installed=$(dotnet --version | cut -d. -f1)
  if [ "$installed" = "$major" ]; then
    echo "[stepthrough] .NET $major already installed (agent image)"
    dotnet --version
    exit 0
  fi
  echo "[stepthrough] WARNING: requested .NET $major but agent image has $installed — installing. Use .NET $installed for faster builds."
fi
curl -sSL https://dot.net/v1/dotnet-install.sh -o /tmp/dotnet-install.sh
bash /tmp/dotnet-install.sh --channel $channel --install-dir /usr/local/bin --no-path
export PATH="$PATH:/usr/local/bin"
dotnet --version
`, major, channel), true
}

type useNodeAdapter struct{}

func (useNodeAdapter) Resolve(ctx taskContext) (string, bool) {
	major := strings.SplitN(input(ctx, "versionSpec"), ".", 2)[0]
	return fmt.Sprintf(`
major=%s
if command -v node >/dev/null 2>&1; then
  installed=$(node --version | sed 's/v//' | cut -d. -f1)
  if [ "$installed" = "$major" ]; then
    echo "[stepthrough] Node $major already installed (agent image)"
    node --version && npm --version
    exit 0
  fi
  echo "[stepthrough] WARNING: requested Node $major but agent image has $installed — installing. Use Node $installed for faster builds."
fi
curl -fsSL https://deb.nodesource.com/setup_${major}.x | bash -
apt-get install -y nodejs
node --version && npm --version
`, major), true
}

type usePythonVersionAdapter struct{}

func (usePythonVersionAdapter) Resolve(ctx taskContext) (string, bool) {
	major := strings.SplitN(input(ctx, "versionSpec"), ".", 2)[0]
	return fmt.Sprintf(`
major=%s
if command -v python${major} >/dev/null 2>&1; then
  echo "[stepthrough] Python $major already installed (agent image)"
  python${major} --version
  exit 0
fi
echo "[stepthrough] WARNING: Python $major not pre-installed in agent image — installing. Use a pre-installed version for faster builds."
apt-get install -y -qq python${major} python${major}-pip
python${major} --version
`, major), true
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
