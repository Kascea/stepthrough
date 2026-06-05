package runner

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var azurePipelinesTasksBaseURL = "https://raw.githubusercontent.com/microsoft/azure-pipelines-tasks/master/Tasks"

type taskReference struct {
	Name  string
	Major string
}

type taskDefinition struct {
	Name    string
	Major   string
	Inputs  []taskInputDefinition
	Adapter taskAdapter
}

type taskInputDefinition struct {
	Name    string
	Aliases []string
	Default string
}

type azureTaskJSON struct {
	Name   string `json:"name"`
	Inputs []struct {
		Name         string   `json:"name"`
		Aliases      []string `json:"aliases"`
		DefaultValue string   `json:"defaultValue"`
	} `json:"inputs"`
}

func (d taskDefinition) ResolveInputs(raw map[string]interface{}) map[string]string {
	inputs := make(map[string]string, len(d.Inputs))
	keys := make(map[string]string, len(d.Inputs))

	for _, input := range d.Inputs {
		keys[normalizeInputKey(input.Name)] = input.Name
		for _, alias := range input.Aliases {
			keys[normalizeInputKey(alias)] = input.Name
		}
		if input.Default != "" {
			inputs[input.Name] = expandAzureVariables(input.Default)
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

func taskDefinitionFor(task string) (taskDefinition, bool) {
	ref, ok := parseTaskReference(task)
	if !ok {
		return taskDefinition{}, false
	}

	definition, err := loadTaskDefinition(ref)
	if err != nil {
		return taskDefinition{}, false
	}
	return definition, true
}

func loadTaskDefinition(ref taskReference) (taskDefinition, error) {
	data, err := loadCachedTaskJSON(ref)
	if err != nil {
		return taskDefinition{}, err
	}
	if len(data) == 0 {
		data, err = downloadTaskJSON(ref)
		if err != nil {
			return taskDefinition{}, err
		}
		_ = cacheTaskJSON(ref, data)
	}

	var parsed azureTaskJSON
	if err := json.Unmarshal(data, &parsed); err != nil {
		return taskDefinition{}, err
	}

	definition := taskDefinition{
		Name:    parsed.Name,
		Major:   ref.Major,
		Adapter: taskAdapters[parsed.Name],
	}
	for _, input := range parsed.Inputs {
		definition.Inputs = append(definition.Inputs, taskInputDefinition{
			Name:    input.Name,
			Aliases: input.Aliases,
			Default: input.DefaultValue,
		})
	}
	return definition, nil
}

func parseTaskReference(task string) (taskReference, bool) {
	name, version, ok := strings.Cut(task, "@")
	if !ok || name == "" || version == "" {
		return taskReference{}, false
	}
	major := strings.SplitN(version, ".", 2)[0]
	if major == "" {
		return taskReference{}, false
	}
	return taskReference{Name: name, Major: major}, true
}

func loadCachedTaskJSON(ref taskReference) ([]byte, error) {
	path, err := taskCachePath(ref)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	return data, err
}

func cacheTaskJSON(ref taskReference, data []byte) error {
	path, err := taskCachePath(ref)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func taskCachePath(ref taskReference) (string, error) {
	dir, err := taskCacheRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ref.Name+"V"+ref.Major, "task.json"), nil
}

func taskCacheRoot() (string, error) {
	if dir := os.Getenv("STEPTHROUGH_TASK_CACHE"); dir != "" {
		return dir, nil
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "stepthrough", "azure-pipelines-tasks"), nil
}

func downloadTaskJSON(ref taskReference) ([]byte, error) {
	baseURL := azurePipelinesTasksBaseURL
	if override := os.Getenv("STEPTHROUGH_TASKS_BASE_URL"); override != "" {
		baseURL = override
	}
	url := strings.TrimRight(baseURL, "/") + "/" + ref.Name + "V" + ref.Major + "/task.json"
	client := http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("fetch task metadata %s: %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func taskBase(task string) string {
	name, _, _ := strings.Cut(task, "@")
	return name
}

func normalizeInputKey(key string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(key), " ", ""))
}
