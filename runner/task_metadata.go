package runner

import (
	"fmt"
	"strings"
)

type taskInputDefinition struct {
	Name    string
	Aliases []string
	Default string
}

type taskDefinition struct {
	Inputs  []taskInputDefinition
	Adapter taskAdapter
}

func (d taskDefinition) ResolveInputs(raw map[string]interface{}) map[string]string {
	inputs := make(map[string]string, len(d.Inputs))
	keys := make(map[string]string, len(d.Inputs))

	for _, inp := range d.Inputs {
		keys[normalizeInputKey(inp.Name)] = inp.Name
		for _, alias := range inp.Aliases {
			keys[normalizeInputKey(alias)] = inp.Name
		}
		if inp.Default != "" {
			inputs[inp.Name] = expandAzureVariables(inp.Default)
		}
	}

	for key, value := range raw {
		canonical := keys[normalizeInputKey(key)]
		if canonical == "" {
			canonical = key
		}
		inputs[canonical] = expandAzureVariables(strings.TrimSpace(fmt.Sprintf("%v", value)))
	}

	return inputs
}

// localTasks is the complete registry of Azure Pipelines tasks that stepthrough
// can execute locally. Tasks absent from this map fail the pipeline step with exit 1.
var localTasks = map[string]taskDefinition{
	"GoTool": {
		Inputs: []taskInputDefinition{
			{Name: "version", Default: "1.10"},
			{Name: "goPath"},
			{Name: "goBin"},
		},
		Adapter: goToolAdapter{},
	},
	"Go": {
		Inputs: []taskInputDefinition{
			{Name: "command", Default: "build"},
			{Name: "customCommand"},
			{Name: "arguments"},
			{Name: "workingDirectory"},
		},
		Adapter: goAdapter{},
	},
	"UseDotNet": {
		Inputs: []taskInputDefinition{
			{Name: "version", Default: "8.x"},
			{Name: "packageType", Default: "sdk"},
		},
		Adapter: useDotNetAdapter{},
	},
	"UseNode": {
		Inputs:  []taskInputDefinition{{Name: "versionSpec", Default: "6.x"}},
		Adapter: useNodeAdapter{},
	},
	"NodeTool": {
		Inputs:  []taskInputDefinition{{Name: "versionSpec", Default: "6.x"}},
		Adapter: useNodeAdapter{},
	},
	"UsePythonVersion": {
		Inputs:  []taskInputDefinition{{Name: "versionSpec", Default: "3.x"}},
		Adapter: usePythonVersionAdapter{},
	},
	"DotNetCoreCLI": {
		Inputs: []taskInputDefinition{
			{Name: "command"},
			{Name: "projects"},
			{Name: "arguments"},
			{Name: "custom"},
			{Name: "nobuild"},
		},
		Adapter: dotNetCoreCLIAdapter{},
	},
	"NuGetCommand": {
		Inputs:  []taskInputDefinition{{Name: "command"}, {Name: "solution"}},
		Adapter: nuGetCommandAdapter{},
	},
	"CopyFiles": {
		Inputs: []taskInputDefinition{
			{Name: "SourceFolder", Default: "."},
			{Name: "TargetFolder", Default: "_artifacts"},
		},
		Adapter: copyFilesAdapter{},
	},
	"PublishBuildArtifacts": {
		Inputs: []taskInputDefinition{
			{Name: "PathtoPublish", Aliases: []string{"path"}, Default: "$(Build.ArtifactStagingDirectory)"},
			{Name: "ArtifactName", Aliases: []string{"artifact"}, Default: "drop"},
		},
		Adapter: publishArtifactAdapter{},
	},
	"PublishPipelineArtifact": {
		Inputs: []taskInputDefinition{
			{Name: "targetPath", Aliases: []string{"path"}},
			{Name: "artifactName", Aliases: []string{"artifact"}},
		},
		Adapter: publishArtifactAdapter{},
	},
	"PublishTestResults": {
		Inputs: []taskInputDefinition{
			{Name: "testResultsFiles", Default: "**/*.xml"},
			{Name: "testRunTitle"},
			{Name: "searchFolder"},
			{Name: "testResultsFormat"},
		},
		Adapter: publishTestResultsAdapter{},
	},
	"PublishCodeCoverageResults": {
		Inputs:  []taskInputDefinition{{Name: "summaryFileLocation"}, {Name: "codeCoverageTool"}},
		Adapter: publishTestResultsAdapter{},
	},
	"DownloadBuildArtifacts":   {Adapter: downloadArtifactAdapter{}},
	"DownloadPipelineArtifact": {Adapter: downloadArtifactAdapter{}},
	"Docker": {
		Inputs:  []taskInputDefinition{{Name: "command"}, {Name: "containerRegistry"}, {Name: "repository"}, {Name: "Dockerfile"}, {Name: "tags"}},
		Adapter: dockerAdapter{},
	},
	"AzureCLI": {Adapter: azureCLIAdapter{}},
	"Bash": {
		Inputs:  []taskInputDefinition{{Name: "script"}, {Name: "workingDirectory"}},
		Adapter: commandLineAdapter{},
	},
	"CmdLine": {
		Inputs:  []taskInputDefinition{{Name: "script", Default: "echo Write your commands here"}, {Name: "workingDirectory"}},
		Adapter: commandLineAdapter{},
	},
}

func taskDefinitionFor(task string) (taskDefinition, bool) {
	name, _, ok := strings.Cut(task, "@")
	if !ok || name == "" {
		return taskDefinition{}, false
	}
	def, ok := localTasks[name]
	return def, ok
}

func normalizeInputKey(key string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(key), " ", ""))
}
