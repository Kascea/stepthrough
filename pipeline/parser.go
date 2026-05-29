package pipeline

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Parse reads and parses an azure-pipelines.yml file.
func Parse(path string) (*Pipeline, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read pipeline file: %w", err)
	}
	return ParseBytes(data)
}

// ParseBytes parses an azure-pipelines.yml from raw bytes.
func ParseBytes(data []byte) (*Pipeline, error) {
	// First pass: unmarshal into a raw node so we can handle polymorphic fields.
	var raw yaml.Node
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("yaml parse: %w", err)
	}

	// Decode the top-level map manually to handle edge cases.
	p := &Pipeline{}
	if err := raw.Decode(p); err != nil {
		return nil, fmt.Errorf("decode pipeline: %w", err)
	}

	// Post-process: resolve dependsOn (string or []string) for stages and jobs.
	if err := resolveDependsOn(&raw, p); err != nil {
		return nil, err
	}

	// Normalise: if no stages but there are jobs, wrap in a synthetic stage.
	if len(p.Stages) == 0 && len(p.Jobs) > 0 {
		p.Stages = []Stage{{
			Stage: "__root__",
			Jobs:  p.Jobs,
		}}
		p.Jobs = nil
	}

	// If no stages and no jobs but there are steps, wrap in a synthetic stage+job.
	if len(p.Stages) == 0 && len(p.Steps) > 0 {
		p.Stages = []Stage{{
			Stage: "__root__",
			Jobs: []Job{{
				Job:   "__root__",
				Steps: p.Steps,
			}},
		}}
		p.Steps = nil
	}

	// Ensure every step has Enabled defaulted to true.
	for si := range p.Stages {
		for ji := range p.Stages[si].Jobs {
			for ki := range p.Stages[si].Jobs[ji].Steps {
				p.Stages[si].Jobs[ji].Steps[ki].Enabled = true
			}
		}
	}

	return p, nil
}

// Flatten converts the nested pipeline structure into an ordered flat list of steps.
func Flatten(p *Pipeline) []FlatStep {
	var flat []FlatStep
	for si, stage := range p.Stages {
		stageName := stage.Stage
		if stage.DisplayName != "" {
			stageName = stage.DisplayName
		}
		for ji, job := range stage.Jobs {
			jobName := job.Job
			if job.DisplayName != "" {
				jobName = job.DisplayName
			}
			if job.Deployment != "" {
				jobName = job.Deployment
			}
			for ki := range job.Steps {
				flat = append(flat, FlatStep{
					StageIndex: si,
					StageName:  stageName,
					JobIndex:   ji,
					JobName:    jobName,
					StepIndex:  ki,
					Step:       &p.Stages[si].Jobs[ji].Steps[ki],
				})
			}
		}
	}
	return flat
}

// resolveDependsOn walks the raw YAML node to handle dependsOn being either
// a bare string or a sequence.
func resolveDependsOn(root *yaml.Node, p *Pipeline) error {
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return nil
	}
	topMap := root.Content[0]
	if topMap.Kind != yaml.MappingNode {
		return nil
	}

	for i := 0; i+1 < len(topMap.Content); i += 2 {
		key := topMap.Content[i].Value
		val := topMap.Content[i+1]

		if key == "stages" && val.Kind == yaml.SequenceNode {
			for si, stageNode := range val.Content {
				deps, err := extractDependsOn(stageNode)
				if err != nil {
					return fmt.Errorf("stage[%d] dependsOn: %w", si, err)
				}
				if si < len(p.Stages) {
					p.Stages[si].DependsOn = deps
				}
				// also resolve job dependsOn
				if err := resolveJobDependsOn(stageNode, p, si); err != nil {
					return err
				}
			}
		}

		if key == "jobs" && val.Kind == yaml.SequenceNode {
			for ji, jobNode := range val.Content {
				deps, err := extractDependsOn(jobNode)
				if err != nil {
					return fmt.Errorf("job[%d] dependsOn: %w", ji, err)
				}
				// root-level jobs end up in p.Stages[0] after normalisation,
				// but at this point they're still in p.Jobs.
				if ji < len(p.Jobs) {
					p.Jobs[ji].DependsOn = deps
				}
			}
		}
	}
	return nil
}

func resolveJobDependsOn(stageNode *yaml.Node, p *Pipeline, si int) error {
	if stageNode.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(stageNode.Content); i += 2 {
		if stageNode.Content[i].Value == "jobs" {
			jobsNode := stageNode.Content[i+1]
			if jobsNode.Kind != yaml.SequenceNode {
				return nil
			}
			for ji, jobNode := range jobsNode.Content {
				deps, err := extractDependsOn(jobNode)
				if err != nil {
					return fmt.Errorf("stage[%d].job[%d] dependsOn: %w", si, ji, err)
				}
				if si < len(p.Stages) && ji < len(p.Stages[si].Jobs) {
					p.Stages[si].Jobs[ji].DependsOn = deps
				}
			}
		}
	}
	return nil
}

func extractDependsOn(node *yaml.Node) ([]string, error) {
	if node.Kind != yaml.MappingNode {
		return nil, nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value != "dependsOn" {
			continue
		}
		val := node.Content[i+1]
		switch val.Kind {
		case yaml.ScalarNode:
			return []string{val.Value}, nil
		case yaml.SequenceNode:
			var deps []string
			for _, child := range val.Content {
				deps = append(deps, child.Value)
			}
			return deps, nil
		}
	}
	return nil, nil
}
