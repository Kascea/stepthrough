package service

import (
	"context"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/colecarlson/stepthrough/orchestrator"
	"github.com/colecarlson/stepthrough/runner"
)

// PipelineService is the Wails-registered adapter over Orchestrator.
// It holds no execution logic — all pipeline state and step running lives in the orchestrator.
type PipelineService struct {
	app  *application.App
	orch *orchestrator.Orchestrator
}

func NewPipelineService(factory orchestrator.ExecutorFactory) *PipelineService {
	svc := &PipelineService{}
	svc.orch = orchestrator.New(factory, svc.emit)
	return svc
}

func (s *PipelineService) SetApp(app *application.App) { s.app = app }

func (s *PipelineService) emit(event string, data any) {
	if s.app != nil {
		s.app.Event.Emit(event, data)
	}
}

func (s *PipelineService) LoadPipeline(file string) orchestrator.PipelineState {
	return s.orch.LoadPipeline(file)
}

func (s *PipelineService) GetState() orchestrator.PipelineState {
	return s.orch.GetState()
}

func (s *PipelineService) SetSafeMode(enabled bool) {
	s.orch.SetSafeMode(enabled)
}

func (s *PipelineService) RunFrom(startIndex int) {
	s.orch.RunFrom(startIndex)
}

func (s *PipelineService) CancelRun() {
	s.orch.CancelRun()
}

func (s *PipelineService) InvalidateFrom(idx int) {
	s.orch.InvalidateFrom(idx)
}

func (s *PipelineService) CheckDockerReady() bool {
	return runner.IsDockerAvailable()
}

func (s *PipelineService) Cleanup() {
	s.orch.Cleanup(context.Background())
}
