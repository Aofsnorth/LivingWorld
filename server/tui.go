package server

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"livingworld/internal/harness"
	"livingworld/internal/infrastructure/logging"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// logStore is a thread-safe ring buffer of rendered log lines. It is installed
// as the standard logger's output (via logging.SetOutput) so every log line —
// including direct log.Printf calls from the protocol layers — is captured for
// the TUI instead of corrupting the alt-screen.
type logStore struct {
	mu    sync.Mutex
	lines []string
	dirty bool
}

const logStoreCap = 2000

func (s *logStore) Write(p []byte) (int, error) {
	s.mu.Lock()
	for _, ln := range strings.Split(strings.TrimRight(string(p), "\n"), "\n") {
		s.lines = append(s.lines, ln)
	}
	if len(s.lines) > logStoreCap {
		s.lines = s.lines[len(s.lines)-logStoreCap:]
	}
	s.dirty = true
	s.mu.Unlock()
	return len(p), nil
}

// snapshot returns the joined log content and whether it changed since the last
// snapshot.
func (s *logStore) snapshot() (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.dirty
	s.dirty = false
	return strings.Join(s.lines, "\n"), d
}

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("219"))
	dimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	statStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("117"))
	okStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("78"))
)

type tickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(300*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

type tuiModel struct {
	srv   *Server
	store *logStore
	con   *console
	vp    viewport.Model
	input textinput.Model
	start time.Time
	width int
	ready bool
}

func (m tuiModel) Init() tea.Cmd { return tea.Batch(tickCmd(), textinput.Blink) }

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		vpH := msg.Height - 4 // 3-line header + 1-line input
		if vpH < 3 {
			vpH = 3
		}
		if !m.ready {
			m.vp = viewport.New(msg.Width, vpH)
			m.ready = true
		} else {
			m.vp.Width, m.vp.Height = msg.Width, vpH
		}
		m.input.Width = msg.Width - 4
		content, _ := m.store.snapshot()
		m.vp.SetContent(content)
		m.vp.GotoBottom()
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "enter":
			line := strings.TrimSpace(m.input.Value())
			m.input.Reset()
			if line == "" {
				return m, nil
			}
			switch strings.ToLower(line) {
			case "stop", "exit", "quit":
				return m, tea.Quit
			default:
				fmt.Fprintf(m.store, "❯ %s\n", line)
				m.con.dispatch(line)
			}
			return m, nil
		}
	case tickMsg:
		if m.ready {
			atBottom := m.vp.AtBottom()
			if content, dirty := m.store.snapshot(); dirty {
				m.vp.SetContent(content)
				if atBottom {
					m.vp.GotoBottom()
				}
			}
		}
		return m, tickCmd()
	}
	var cmds []tea.Cmd
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	cmds = append(cmds, cmd)
	m.vp, cmd = m.vp.Update(msg)
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

func (m tuiModel) View() string {
	if !m.ready {
		return "starting LivingWorld…"
	}
	return m.headerView() + "\n" + m.vp.View() + "\n" + m.input.View()
}

func (m tuiModel) headerView() string {
	up := time.Since(m.start).Truncate(time.Second)
	title := titleStyle.Render("⬢ LivingWorld") + dimStyle.Render("  dual-protocol (Java + Bedrock)")
	stats := statStyle.Render(fmt.Sprintf(
		"Java %s   Bedrock %s   ", m.srv.cfg.Address(), m.srv.cfg.BedrockAddress())) +
		okStyle.Render(fmt.Sprintf("%d online", m.srv.PlayerCount())) +
		dimStyle.Render(fmt.Sprintf("   uptime %s   (type 'help', 'stop' to quit)", up))
	rule := dimStyle.Render(strings.Repeat("─", max(0, m.width)))
	return title + "\n" + stats + "\n" + rule
}

// RunTUI starts the server and runs an interactive terminal UI (live status
// header + scrolling log panel + command input) until the user quits with
// "stop"/Ctrl-C, then shuts down gracefully via the application harness.
//
// The TUI owns the terminal and stdin, so the harness is constructed with
// noop signal handling (Ctrl-C is handled by bubbletea) and without the
// console component; the TUI's "stop" command and Ctrl-C both drive shutdown
// through Harness.Stop. Use Run for a plain (non-TUI) console, e.g. when
// stdout is not a terminal.
func (s *Server) RunTUI() error {
	store := &logStore{}
	logging.SetOutput(store)

	h := s.NewHarness(harness.WithNoopSignals())
	if err := h.Start(context.Background()); err != nil {
		logging.SetOutput(os.Stderr)
		return err
	}

	ti := textinput.New()
	ti.Placeholder = "command… (help, list, say <text>, stop)"
	ti.Prompt = "❯ "
	ti.Focus()

	m := tuiModel{
		srv:   s,
		store: store,
		con:   newConsole(s, store, func() { _ = h.Stop() }),
		input: ti,
		start: time.Now(),
	}
	_, err := tea.NewProgram(m, tea.WithAltScreen()).Run()

	logging.SetOutput(os.Stderr) // restore plain logging for shutdown
	// Ensure teardown even if the TUI exited without an explicit "stop";
	// Harness.Stop is idempotent.
	_ = h.Stop()
	return err
}
