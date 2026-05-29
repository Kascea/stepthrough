package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/colecarlson/stepthrough/debugger"
	"github.com/colecarlson/stepthrough/pipeline"
	"github.com/colecarlson/stepthrough/runner"
	"github.com/colecarlson/stepthrough/tui"
)

func main() {
	listFlag := flag.Bool("list", false, "print all stages/jobs/steps and exit")
	jobFlag := flag.String("job", "", "job name to debug (skips interactive selection)")
	stageFlag := flag.String("stage", "", "stage name to filter jobs from")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: stepthrough [flags] <azure-pipelines.yml>")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Runs Azure pipeline steps locally inside a Docker container,")
		fmt.Fprintln(os.Stderr, "letting you step through each step interactively.")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "  Only ubuntu-latest pipelines are supported (requires Docker Desktop).")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Flags:")
		flag.PrintDefaults()
	}
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		flag.Usage()
		os.Exit(1)
	}
	pipelineFile := args[0]

	p, err := pipeline.Parse(pipelineFile)
	if err != nil {
		fatalf("parse pipeline: %v", err)
	}

	if *listFlag {
		printSteps(pipeline.Flatten(p))
		return
	}

	// Collect all jobs.
	var jobs []jobEntry
	for si := range p.Stages {
		for ji := range p.Stages[si].Jobs {
			s := &p.Stages[si]
			j := &p.Stages[si].Jobs[ji]
			if *stageFlag != "" && !strings.EqualFold(s.Stage, *stageFlag) && !strings.EqualFold(s.DisplayName, *stageFlag) {
				continue
			}
			jobs = append(jobs, jobEntry{stage: s, job: j})
		}
	}
	if len(jobs) == 0 {
		fatalf("no jobs found in pipeline")
	}

	// Select job.
	var selected jobEntry
	if *jobFlag != "" {
		for _, je := range jobs {
			name := je.job.Job
			if je.job.DisplayName != "" {
				name = je.job.DisplayName
			}
			if je.job.Deployment != "" {
				name = je.job.Deployment
			}
			if strings.EqualFold(name, *jobFlag) || strings.EqualFold(je.job.Job, *jobFlag) {
				selected = je
				break
			}
		}
		if selected.job == nil {
			fatalf("job %q not found", *jobFlag)
		}
	} else if len(jobs) == 1 {
		selected = jobs[0]
	} else {
		selected = selectJob(jobs)
	}

	// Validate that the job's vmImage is supported.
	vmImage := p.Pool.VMImage
	if selected.job.Pool != nil && selected.job.Pool.VMImage != "" {
		vmImage = selected.job.Pool.VMImage
	}
	if _, ok := runner.ResolveImage(vmImage); !ok {
		fatalf("vmImage %q is not supported for local debugging.\n"+
			"Only ubuntu-latest is currently supported.", vmImage)
	}

	// Build flat step list for the selected job only.
	var steps []pipeline.FlatStep
	si := stageIndex(p, selected.stage)
	ji := jobIndex(selected.stage, selected.job)
	stageName := selected.stage.Stage
	if selected.stage.DisplayName != "" {
		stageName = selected.stage.DisplayName
	}
	jobName := selected.job.Job
	if selected.job.DisplayName != "" {
		jobName = selected.job.DisplayName
	}
	if selected.job.Deployment != "" {
		jobName = selected.job.Deployment
	}
	for ki := range selected.job.Steps {
		steps = append(steps, pipeline.FlatStep{
			StageIndex: si,
			StageName:  stageName,
			JobIndex:   ji,
			JobName:    jobName,
			StepIndex:  ki,
			Step:       &p.Stages[si].Jobs[ji].Steps[ki],
		})
	}
	if len(steps) == 0 {
		fatalf("selected job has no steps")
	}

	// Working directory = directory containing the pipeline file.
	workDir, err := filepath.Abs(filepath.Dir(pipelineFile))
	if err != nil {
		fatalf("resolve workdir: %v", err)
	}

	containerName := fmt.Sprintf("stepthrough-%s", randHex(8))

	eng := runner.NewEngine(workDir)
	session := debugger.New(steps, p.Variables, eng)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)

	// Cleanup container on exit.
	go func() {
		<-ctx.Done()
		eng.Cleanup(context.Background())
	}()

	fmt.Printf("\n  Starting debugger for job: %s (stage: %s)\n", jobName, stageName)
	fmt.Printf("  Container: %s\n", containerName)
	fmt.Printf("  Workspace: %s\n\n", workDir)

	if err := tui.Run(session, eng, p, selected.job, containerName, pipelineFile, ctx, cancel); err != nil {
		fmt.Fprintf(os.Stderr, "tui error: %v\n", err)
	}

	// Final cleanup.
	eng.Cleanup(context.Background())
	cancel()
}

type jobEntry struct {
	stage *pipeline.Stage
	job   *pipeline.Job
}

// selectJob prompts the user to choose a job interactively.
func selectJob(jobs []jobEntry) jobEntry {
	fmt.Println("Multiple jobs found. Select a job to debug:")
	for i, je := range jobs {
		name := je.job.Job
		if je.job.DisplayName != "" {
			name = je.job.DisplayName
		}
		if je.job.Deployment != "" {
			name = "deploy:" + je.job.Deployment
		}
		stageName := je.stage.Stage
		if je.stage.DisplayName != "" {
			stageName = je.stage.DisplayName
		}
		fmt.Printf("  [%d] %s  (stage: %s)\n", i+1, name, stageName)
	}
	fmt.Print("\nEnter number: ")
	var n int
	if _, err := fmt.Scanf("%d", &n); err != nil || n < 1 || n > len(jobs) {
		fatalf("invalid selection")
	}
	return jobs[n-1]
}

func printSteps(steps []pipeline.FlatStep) {
	prevStage, prevJob := "", ""
	for i, fs := range steps {
		if fs.StageName != prevStage {
			prevStage = fs.StageName
			prevJob = ""
			fmt.Printf("\nStage: %s\n", fs.StageName)
		}
		if fs.JobName != prevJob {
			prevJob = fs.JobName
			fmt.Printf("  Job: %s\n", fs.JobName)
		}
		fmt.Printf("    [%d] %-10s %s\n", i+1, "["+string(fs.Step.Type())+"]", fs.Step.Label())
	}
	fmt.Printf("\nTotal: %d steps\n", len(steps))
}

func stageIndex(p *pipeline.Pipeline, s *pipeline.Stage) int {
	for i := range p.Stages {
		if &p.Stages[i] == s {
			return i
		}
	}
	return 0
}

func jobIndex(s *pipeline.Stage, j *pipeline.Job) int {
	for i := range s.Jobs {
		if &s.Jobs[i] == j {
			return i
		}
	}
	return 0
}

func randHex(n int) string {
	const chars = "abcdef0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return string(b)
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}
