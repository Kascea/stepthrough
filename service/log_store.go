package service

import (
	"strconv"
	"sync"

	"github.com/kascea/stepthrough/orchestrator"
	"github.com/kascea/stepthrough/session"
)

// logStore captures and bounds log lines per pipeline file and step index.
// All methods are safe for concurrent use.
type logStore struct {
	mu   sync.Mutex
	logs map[string]map[string][]string // file → step-index-str → lines
}

func newLogStore() *logStore {
	return &logStore{logs: make(map[string]map[string][]string)}
}

// capture appends a log line for the given file and step, up to MaxLogsPerStep.
func (l *logStore) capture(file string, ll orchestrator.LogLine) {
	key := strconv.Itoa(ll.StepIndex)
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.logs[file] == nil {
		l.logs[file] = make(map[string][]string)
	}
	lines := l.logs[file][key]
	if len(lines) < session.MaxLogsPerStep {
		l.logs[file][key] = append(lines, ll.Line)
	}
}

// clear resets log lines for file, called before a pipeline run.
func (l *logStore) clear(file string) {
	l.mu.Lock()
	l.logs[file] = make(map[string][]string)
	l.mu.Unlock()
}

// remove deletes all log lines for file.
func (l *logStore) remove(file string) {
	l.mu.Lock()
	delete(l.logs, file)
	l.mu.Unlock()
}

// rename moves log lines from oldFile to newFile.
func (l *logStore) rename(oldFile, newFile string) {
	l.mu.Lock()
	if v, ok := l.logs[oldFile]; ok {
		l.logs[newFile] = v
		delete(l.logs, oldFile)
	}
	l.mu.Unlock()
}

// snapshot returns a deep copy of all log lines, for session persistence.
func (l *logStore) snapshot() map[string]map[string][]string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make(map[string]map[string][]string, len(l.logs))
	for file, steps := range l.logs {
		cp := make(map[string][]string, len(steps))
		for k, v := range steps {
			cp[k] = v
		}
		out[file] = cp
	}
	return out
}
