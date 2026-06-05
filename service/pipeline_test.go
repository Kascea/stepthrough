package service

import (
	"testing"

	"github.com/colecarlson/stepthrough/orchestrator"
	"github.com/colecarlson/stepthrough/runner"
)

func newTestService() *PipelineService {
	factory := func(workDir string) orchestrator.Executor {
		return runner.NewEngine(workDir)
	}
	return NewPipelineService(factory)
}

// Regression: GetSession must never return a nil TabOrder.
// A nil Go slice marshals to JSON null, which caused null.length to crash
// the frontend reducer on first launch (no saved session).
func TestGetSessionReturnsNonNilTabOrder(t *testing.T) {
	svc := newTestService()
	sess := svc.GetSession()

	if sess.TabOrder == nil {
		t.Fatal("GetSession returned nil TabOrder; frontend will crash on null.length")
	}
}

func TestGetSessionReturnsNonNilRuns(t *testing.T) {
	svc := newTestService()
	sess := svc.GetSession()

	if sess.Runs == nil {
		t.Fatal("GetSession returned nil Runs map")
	}
}
