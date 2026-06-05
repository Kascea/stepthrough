package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/colecarlson/stepthrough/orchestrator"
)

// MaxLogsPerStep caps how many log lines we persist per step.
const MaxLogsPerStep = 500

// SavedRun stores the last run results for a single pipeline.
type SavedRun struct {
	Steps []orchestrator.StepState `json:"steps"`
	Logs  map[string][]string      `json:"logs"` // step index (string key) → lines
	RanAt time.Time                `json:"ranAt"`
}

// Data is the full persisted session written to disk.
type Data struct {
	Version    int                 `json:"version"`
	TabOrder   []string            `json:"tabOrder"`
	ActiveFile string              `json:"activeFile"`
	Runs       map[string]SavedRun `json:"runs"` // absolute file path → last run
}

// Load reads the session from disk. Always returns a valid *Data.
func Load() *Data {
	p, err := dataPath()
	if err != nil {
		return empty()
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return empty()
	}
	var d Data
	if err := json.Unmarshal(b, &d); err != nil {
		return empty()
	}
	if d.Runs == nil {
		d.Runs = make(map[string]SavedRun)
	}
	return &d
}

// Save writes the session to disk, silently ignoring errors.
func Save(d *Data) {
	p, err := dataPath()
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(p), 0755)
	b, _ := json.MarshalIndent(d, "", "  ")
	_ = os.WriteFile(p, b, 0644)
}

func empty() *Data {
	return &Data{Version: 1, Runs: make(map[string]SavedRun)}
}

func dataPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "stepthrough", "session.json"), nil
}
