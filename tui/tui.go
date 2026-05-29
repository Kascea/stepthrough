// Package tui provides a two-panel interactive terminal UI for debugging Azure pipelines.
// Left panel: step list with status.  Right panel: live-streaming step output.
package tui

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/colecarlson/stepthrough/debugger"
	"github.com/colecarlson/stepthrough/pipeline"
	"github.com/colecarlson/stepthrough/runner"
)

// ─── Styles ─────────────────────────────────────────────────────────────────

var (
	styleBold    = lipgloss.NewStyle().Bold(true)
	styleTitle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	styleDim     = lipgloss.NewStyle().Faint(true)
	styleSuccess = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	styleFailure = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	styleWarning = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	styleRunning = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)
	stylePending = lipgloss.NewStyle().Faint(true)
	styleSkipped = lipgloss.NewStyle().Foreground(lipgloss.Color("13"))
	styleCursor  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	styleBreakpt = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	styleStage   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	styleJob     = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	styleDivider = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	styleOutput  = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	styleHelp    = lipgloss.NewStyle().Faint(true)
	styleStatus  = lipgloss.NewStyle().Bold(true)
	stylePanelBg = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

// ─── Messages ───────────────────────────────────────────────────────────────

type outputLineMsg string
type outputDoneMsg struct{}
type stepDoneMsg struct{ result *debugger.StepResult }
type continueDoneMsg struct{ results []*debugger.StepResult }
type setupDoneMsg struct{ err error }
type errMsg struct{ err error }
type shellDoneMsg struct{ err error }

// ─── Model ───────────────────────────────────────────────────────────────────

type setupPhase int

const (
	phaseSetup  setupPhase = iota // pulling image + starting container
	phaseReady                    // ready to step
)

// Model is the bubbletea model for the TUI.
type Model struct {
	session      *debugger.Session
	engine       *runner.Engine
	pipelineFile string
	jobName      string

	phase  setupPhase
	ctx    context.Context
	cancel context.CancelFunc

	// UI dimensions
	width  int
	height int

	// Step list navigation (independent of execution cursor — for setting breakpoints).
	listCursor int
	listOffset int

	// Streaming output
	outputCh      chan string
	outputLines   []string
	outputOffset  int // scroll offset in output pane
	currentLabel  string

	statusMsg string
	err       error
}

// New creates a Model. Call Run() to start the program.
func New(
	session *debugger.Session,
	engine *runner.Engine,
	pipelineFile, jobName string,
	ctx context.Context,
	cancel context.CancelFunc,
) Model {
	return Model{
		session:      session,
		engine:       engine,
		pipelineFile: pipelineFile,
		jobName:      jobName,
		phase:        phaseSetup,
		ctx:          ctx,
		cancel:       cancel,
		outputCh:     make(chan string, 256),
	}
}

func (m Model) Init() tea.Cmd {
	// Start container setup asynchronously.
	return m.cmdSetup()
}

// ─── Update ──────────────────────────────────────────────────────────────────

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		return m.handleKey(msg)

	// ── Setup phase messages ──

	case outputLineMsg:
		m.outputLines = append(m.outputLines, string(msg))
		m.scrollOutputToBottom()
		return m, cmdListenOutput(m.outputCh)

	case outputDoneMsg:
		// output channel flushed — nothing more to schedule.

	case setupDoneMsg:
		if msg.err != nil {
			m.err = msg.err
			m.statusMsg = "setup failed: " + msg.err.Error()
			return m, nil
		}
		m.phase = phaseReady
		m.statusMsg = "ready — press s to step, c to continue"

	// ── Execution messages ──

	case stepDoneMsg:
		if msg.result != nil {
			m.currentLabel = ""
			m.statusMsg = fmt.Sprintf("step %s in %s",
				msg.result.Status,
				msg.result.Duration.Round(1_000_000),
			)
			if msg.result.Err != nil {
				m.statusMsg += " — " + msg.result.Err.Error()
			}
		}
		if m.session.IsDone() {
			m.statusMsg = "pipeline complete"
		}
		m.listCursor = m.session.Cursor()
		m.scrollListToCursor()

	case continueDoneMsg:
		m.currentLabel = ""
		if m.session.IsDone() {
			m.statusMsg = "pipeline complete"
		} else {
			m.statusMsg = fmt.Sprintf("paused at step %d — %s",
				m.session.Cursor()+1,
				m.session.Steps[m.session.Cursor()].Step.Label(),
			)
		}
		m.listCursor = m.session.Cursor()
		m.scrollListToCursor()

	case errMsg:
		m.err = msg.err
		m.statusMsg = "error: " + msg.err.Error()

	case shellDoneMsg:
		m.statusMsg = "shell session ended"
		if msg.err != nil {
			m.statusMsg += " (" + msg.err.Error() + ")"
		}
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Always allow quit.
	if msg.String() == "q" || msg.String() == "ctrl+c" {
		m.cancel()
		return m, tea.Quit
	}

	// During setup, only output listening is active.
	if m.phase == phaseSetup {
		return m, nil
	}

	switch msg.String() {

	// ── Step list navigation ──
	case "up", "k":
		if m.listCursor > 0 {
			m.listCursor--
			m.scrollListToListCursor()
		}
	case "down", "j":
		if m.listCursor < len(m.session.Steps)-1 {
			m.listCursor++
			m.scrollListToListCursor()
		}

	// ── Output pane scroll ──
	case "pgup", "u":
		if m.outputOffset > 0 {
			m.outputOffset -= m.outputPaneHeight() / 2
			if m.outputOffset < 0 {
				m.outputOffset = 0
			}
		}
	case "pgdn", "d":
		max := len(m.outputLines) - m.outputPaneHeight()
		if max < 0 {
			max = 0
		}
		if m.outputOffset < max {
			m.outputOffset += m.outputPaneHeight() / 2
			if m.outputOffset > max {
				m.outputOffset = max
			}
		}

	// ── Debugger controls ──
	case "s", " ":
		if !m.session.IsDone() && m.session.IsPaused() {
			fs := m.session.Steps[m.session.Cursor()]
			m.currentLabel = fs.Step.Label()
			m.outputLines = nil
			m.outputOffset = 0
			m.statusMsg = "running: " + m.currentLabel
			return m, tea.Batch(
				m.cmdStep(),
				cmdListenOutput(m.outputCh),
			)
		}

	case "c":
		if !m.session.IsDone() && m.session.IsPaused() {
			m.outputLines = nil
			m.outputOffset = 0
			m.statusMsg = "running…"
			return m, tea.Batch(
				m.cmdContinue(),
				cmdListenOutput(m.outputCh),
			)
		}

	case "x":
		if !m.session.IsDone() && m.session.IsPaused() {
			m.session.SkipCurrent() //nolint
			m.statusMsg = "step skipped"
			m.listCursor = m.session.Cursor()
			m.scrollListToCursor()
		}

	case "b":
		if m.listCursor < len(m.session.Steps) {
			fs := m.session.Steps[m.listCursor]
			bp := debugger.Breakpoint{
				StageName: fs.StageName,
				JobName:   fs.JobName,
				StepName:  fs.Step.Label(),
			}
			added := m.session.ToggleBreakpoint(bp)
			if added {
				m.statusMsg = fmt.Sprintf("breakpoint set at: %s", fs.Step.Label())
			} else {
				m.statusMsg = fmt.Sprintf("breakpoint removed from: %s", fs.Step.Label())
			}
		}

	case "i":
		// Drop into interactive shell — suspend TUI, restore terminal.
		if m.engine != nil && m.engine.Ready() {
			shellCmd := m.engine.ShellCmd()
			return m, tea.ExecProcess(shellCmd, func(err error) tea.Msg {
				return shellDoneMsg{err}
			})
		}
		m.statusMsg = "container not ready"

	case "r":
		m.session.Reset()
		m.outputLines = nil
		m.outputOffset = 0
		m.listCursor = 0
		m.statusMsg = "reset to start"
	}

	return m, nil
}

// ─── Commands ─────────────────────────────────────────────────────────────────

func (m Model) cmdSetup() tea.Cmd {
	return func() tea.Msg {
		// This is a long-running operation so we also need to start listening for output.
		// We do it by running setup in a goroutine and returning a Batch.
		return nil
	}
}

func (m *Model) Init2() tea.Cmd {
	return tea.Batch(
		cmdListenOutput(m.outputCh),
		m.cmdSetupAsync(),
	)
}

func (m Model) cmdSetupAsync() tea.Cmd {
	return func() tea.Msg {
		err := m.engine.Setup(m.ctx, nil, nil, "", m.outputCh) // called with real args from Run()
		return setupDoneMsg{err}
	}
}

func (m Model) cmdStep() tea.Cmd {
	return func() tea.Msg {
		result, err := m.session.Step(m.ctx, m.outputCh)
		close(m.outputCh)
		if err != nil {
			return errMsg{err}
		}
		return stepDoneMsg{result}
	}
}

func (m Model) cmdContinue() tea.Cmd {
	return func() tea.Msg {
		results, err := m.session.Continue(m.ctx, m.outputCh)
		close(m.outputCh)
		if err != nil {
			return errMsg{err}
		}
		return continueDoneMsg{results}
	}
}

func cmdListenOutput(ch chan string) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-ch
		if !ok {
			return outputDoneMsg{}
		}
		return outputLineMsg(line)
	}
}

// ─── View ─────────────────────────────────────────────────────────────────────

func (m Model) View() string {
	if m.width == 0 {
		return "loading…"
	}

	var sb strings.Builder

	// Title bar (1 line)
	container := ""
	if m.engine != nil && m.engine.Ready() {
		container = "  " + styleDim.Render("container: "+m.engine.ContainerName())
	}
	title := styleTitle.Render(fmt.Sprintf("  Azure Pipeline Debugger  —  %s  job: %s", m.pipelineFile, m.jobName))
	sb.WriteString(title + container + "\n")

	// Horizontal rule
	sb.WriteString(styleDivider.Render(strings.Repeat("─", m.width)) + "\n")

	// Two-panel body
	sb.WriteString(m.renderPanels())

	// Horizontal rule
	sb.WriteString(styleDivider.Render(strings.Repeat("─", m.width)) + "\n")

	// Status bar
	sb.WriteString(m.renderStatus() + "\n")

	// Help bar
	sb.WriteString(m.renderHelp())

	return sb.String()
}

func (m Model) renderPanels() string {
	leftW := m.leftPaneWidth()
	rightW := m.width - leftW - 1 // 1 for divider

	leftLines := m.renderLeft(leftW)
	rightLines := m.renderRight(rightW)

	bodyHeight := m.bodyHeight()

	// Pad/trim both to bodyHeight.
	leftLines = padLines(leftLines, bodyHeight)
	rightLines = padLines(rightLines, bodyHeight)

	var sb strings.Builder
	for i := 0; i < bodyHeight; i++ {
		left := leftLines[i]
		right := rightLines[i]
		sb.WriteString(left + styleDivider.Render("│") + right + "\n")
	}
	return sb.String()
}

func (m Model) renderLeft(width int) []string {
	steps := m.session.Steps
	results := m.session.Results
	cursor := m.session.Cursor()
	done := m.session.IsDone()

	var lines []string

	lastStage := ""
	lastJob := ""

	visible := m.bodyHeight()
	end := m.listOffset + visible
	if end > len(steps) {
		end = len(steps)
	}

	for i := m.listOffset; i < end; i++ {
		fs := steps[i]
		result := results[i]

		if fs.StageName != lastStage {
			lastStage = fs.StageName
			lastJob = ""
			lines = append(lines, styleStage.Render(truncate("  "+fs.StageName, width)))
		}
		if fs.JobName != lastJob {
			lastJob = fs.JobName
			lines = append(lines, styleJob.Render(truncate("    "+fs.JobName, width)))
		}

		execCursor := !done && i == cursor
		listCursor := i == m.listCursor
		hasBreakpt := m.session.HasBreakpoint(fs)

		var prefix string
		switch {
		case execCursor && listCursor:
			prefix = styleCursor.Render("▶ ")
		case execCursor:
			prefix = styleCursor.Render("▶ ")
		case listCursor:
			prefix = styleBold.Render("› ")
		default:
			prefix = "  "
		}

		bp := "  "
		if hasBreakpt {
			bp = styleBreakpt.Render("● ")
		}

		icon := statusIcon(result.Status)
		label := truncate(fs.Step.Label(), width-8)
		line := "      " + prefix + bp + icon + " " + label
		lines = append(lines, truncate(line, width))
	}

	if len(steps) == 0 {
		lines = append(lines, styleDim.Render("  (no steps)"))
	}

	return lines
}

func (m Model) renderRight(width int) []string {
	header := ""
	if m.currentLabel != "" {
		header = styleRunning.Render("▶ "+m.currentLabel) + "\n" + strings.Repeat("─", width)
	} else if m.session.Cursor() > 0 || m.session.IsDone() {
		prev := m.session.Cursor()
		if m.session.IsDone() {
			prev = len(m.session.Steps)
		}
		if prev > 0 {
			header = styleDim.Render(m.session.Steps[prev-1].Step.Label())
		}
	}

	headerLines := strings.Split(header, "\n")
	outputH := m.outputPaneHeight()

	// Visible output lines with scroll offset.
	allOut := m.outputLines
	start := m.outputOffset
	if start > len(allOut) {
		start = len(allOut)
	}
	end := start + outputH
	if end > len(allOut) {
		end = len(allOut)
	}
	visible := allOut[start:end]

	var lines []string
	for _, l := range headerLines {
		lines = append(lines, truncate(" "+l, width))
	}
	for _, l := range visible {
		lines = append(lines, styleOutput.Render(truncate(" "+l, width)))
	}
	return lines
}

func (m Model) renderStatus() string {
	state := "PAUSED"
	stateStyle := styleWarning
	if m.session.IsDone() {
		state = "  DONE"
		stateStyle = styleSuccess
	} else if !m.session.IsPaused() {
		state = "RUNNING"
		stateStyle = styleRunning
	}
	if m.phase == phaseSetup {
		state = " SETUP"
		stateStyle = styleWarning
	}
	if m.err != nil {
		state = " ERROR"
		stateStyle = styleFailure
	}

	total := len(m.session.Steps)
	passed, failed, skipped := 0, 0, 0
	for _, r := range m.session.Results {
		switch r.Status {
		case debugger.StatusPassed:
			passed++
		case debugger.StatusFailed:
			failed++
		case debugger.StatusSkipped:
			skipped++
		}
	}

	cur := m.session.Cursor()
	if m.session.IsDone() {
		cur = total
	}

	left := stateStyle.Render(fmt.Sprintf(" %-8s", state))
	mid := fmt.Sprintf("  step %d/%d  %s %d  %s %d  %s %d",
		cur, total,
		styleSuccess.Render("✓"), passed,
		styleFailure.Render("✗"), failed,
		styleSkipped.Render("⊘"), skipped,
	)
	right := ""
	if m.statusMsg != "" {
		right = "  " + styleDim.Render(m.statusMsg)
	}
	return left + mid + right
}

func (m Model) renderHelp() string {
	keys := "  s:step  c:continue  x:skip  b:breakpoint  i:shell  r:reset  j/k:navigate  d/u:scroll  q:quit"
	return styleHelp.Render(keys)
}

// ─── Layout helpers ───────────────────────────────────────────────────────────

func (m Model) leftPaneWidth() int {
	w := m.width * 2 / 5
	if w < 30 {
		w = 30
	}
	if w > 60 {
		w = 60
	}
	return w
}

func (m Model) bodyHeight() int {
	reserved := 5 // title + divider + divider + status + help
	h := m.height - reserved
	if h < 1 {
		return 1
	}
	return h
}

func (m Model) outputPaneHeight() int {
	h := m.bodyHeight() - 2 // minus header lines
	if h < 1 {
		return 1
	}
	return h
}

func (m *Model) scrollListToCursor() {
	visible := m.bodyHeight()
	if m.listCursor < m.listOffset {
		m.listOffset = m.listCursor
	} else if m.listCursor >= m.listOffset+visible {
		m.listOffset = m.listCursor - visible + 1
	}
}

func (m *Model) scrollListToListCursor() {
	visible := m.bodyHeight()
	if m.listCursor < m.listOffset {
		m.listOffset = m.listCursor
	} else if m.listCursor >= m.listOffset+visible {
		m.listOffset = m.listCursor - visible + 1
	}
}

func (m *Model) scrollOutputToBottom() {
	h := m.outputPaneHeight()
	max := len(m.outputLines) - h
	if max < 0 {
		max = 0
	}
	m.outputOffset = max
}

// ─── Utilities ───────────────────────────────────────────────────────────────

func statusIcon(s debugger.Status) string {
	switch s {
	case debugger.StatusRunning:
		return styleRunning.Render("▶")
	case debugger.StatusPassed:
		return styleSuccess.Render("✓")
	case debugger.StatusFailed:
		return styleFailure.Render("✗")
	case debugger.StatusSkipped:
		return styleSkipped.Render("⊘")
	default:
		return stylePending.Render("·")
	}
}

func padLines(lines []string, height int) []string {
	for len(lines) < height {
		lines = append(lines, "")
	}
	return lines[:height]
}

func truncate(s string, maxLen int) string {
	// Strip ANSI codes length is not easy, so we truncate rune-count as an approximation.
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-1]) + "…"
}

// ─── Compile-time checks ─────────────────────────────────────────────────────

var _ tea.Model = Model{}
var _ *exec.Cmd = (*exec.Cmd)(nil)
var _ pipeline.FlatStep   // ensure import used

// ─── Run ─────────────────────────────────────────────────────────────────────

// setupCmd is the real setup command used by Run().
type setupCmd struct {
	engine  *runner.Engine
	p       *pipeline.Pipeline
	job     *pipeline.Job
	name    string
	outputCh chan string
	ctx     context.Context
}

func (sc setupCmd) run() tea.Msg {
	err := sc.engine.Setup(sc.ctx, sc.p, sc.job, sc.name, sc.outputCh)
	close(sc.outputCh)
	return setupDoneMsg{err}
}

// Run starts the interactive TUI for the given session and engine.
func Run(
	session *debugger.Session,
	engine *runner.Engine,
	p *pipeline.Pipeline,
	job *pipeline.Job,
	containerName string,
	pipelineFile string,
	ctx context.Context,
	cancel context.CancelFunc,
) error {
	outputCh := make(chan string, 512)

	m := Model{
		session:      session,
		engine:       engine,
		pipelineFile: pipelineFile,
		jobName:      job.Job,
		phase:        phaseSetup,
		ctx:          ctx,
		cancel:       cancel,
		outputCh:     outputCh,
		statusMsg:    "pulling image and starting container…",
	}
	if job.DisplayName != "" {
		m.jobName = job.DisplayName
	}
	if job.Deployment != "" {
		m.jobName = job.Deployment
	}

	sc := setupCmd{
		engine:   engine,
		p:        p,
		job:      job,
		name:     containerName,
		outputCh: outputCh,
		ctx:      ctx,
	}

	prog := tea.NewProgram(m, tea.WithAltScreen())

	// Override Init to run setup + listen concurrently.
	type initModel struct{ Model }
	_ = initModel{}

	prog2 := tea.NewProgram(initWrapper{m, sc}, tea.WithAltScreen())
	_, err := prog2.Run()
	_ = prog
	return err
}

// initWrapper wraps Model to provide a custom Init that starts setup + output listener.
type initWrapper struct {
	Model
	sc setupCmd
}

func (w initWrapper) Init() tea.Cmd {
	return tea.Batch(
		func() tea.Msg { return w.sc.run() },
		cmdListenOutput(w.sc.outputCh),
	)
}

func (w initWrapper) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// After setup is done, switch to a fresh outputCh for steps.
	if _, ok := msg.(setupDoneMsg); ok {
		updated, cmd := w.Model.Update(msg)
		m := updated.(Model)
		m.outputCh = make(chan string, 512) // fresh channel for step execution
		return initWrapper{m, w.sc}, cmd
	}
	updated, cmd := w.Model.Update(msg)
	m := updated.(Model)
	return initWrapper{m, w.sc}, cmd
}

func (w initWrapper) View() string {
	return w.Model.View()
}
