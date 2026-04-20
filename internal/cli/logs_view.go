package cli

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

type tickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

type logsDoneMsg struct{}

var logLevels = []string{"ALL", "DEBUG", "INFO", "WARN", "ERROR"}

type logsModel struct {
	vp        viewport.Model
	logPath   string
	width     int
	height    int
	atBottom  bool
	lineCount int
	filterIdx int
}

func newLogsModel(logPath string, width, height int) logsModel {
	vp := viewport.New(width, height-4)
	vp.Style = styleApp

	m := logsModel{
		vp:       vp,
		logPath:  logPath,
		width:    width,
		height:   height,
		atBottom: true,
	}
	m.refresh()
	return m
}

func (m *logsModel) refresh() {
	data, err := os.ReadFile(m.logPath)
	if err != nil {
		m.vp.SetContent(styleSubtle.Render("  No log file found. Start the server to generate logs."))
		m.lineCount = 0
		return
	}

	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")

	// Keep last 500 lines to avoid unbounded growth in the viewport
	if len(lines) > 500 {
		lines = lines[len(lines)-500:]
	}

	if level := logLevels[m.filterIdx]; level != "ALL" {
		filtered := lines[:0]
		for _, l := range lines {
			if strings.Contains(l, level) {
				filtered = append(filtered, l)
			}
		}
		lines = filtered
	}

	m.lineCount = len(lines)
	m.vp.SetContent(strings.Join(lines, "\n"))

	if m.atBottom {
		m.vp.GotoBottom()
	}
}

func (m logsModel) Update(msg tea.Msg) (logsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc":
			return m, func() tea.Msg { return logsDoneMsg{} }
		case "G":
			m.vp.GotoBottom()
			m.atBottom = true
			return m, nil
		case "f", "tab":
			m.filterIdx = (m.filterIdx + 1) % len(logLevels)
			m.refresh()
			return m, nil
		}
		prevOffset := m.vp.YOffset
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		if m.vp.YOffset < prevOffset {
			m.atBottom = false
		}
		if m.vp.AtBottom() {
			m.atBottom = true
		}
		return m, cmd

	case tickMsg:
		m.refresh()
		return m, tickCmd()

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.vp.Width = msg.Width
		m.vp.Height = msg.Height - 4
		m.refresh()
		return m, nil
	}

	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
}

func (m logsModel) View() string {
	level := logLevels[m.filterIdx]
	filterLabel := styleSubtle.Render("filter:") + " " + styleAccent.Render(level)
	header := styleTitle.Render(fmt.Sprintf("  Server Logs  (%d lines)", m.lineCount)) + "  " + filterLabel
	divider := styleDivider.Render(strings.Repeat("─", max(50, m.width-4)))

	scrollHint := ""
	if !m.atBottom {
		scrollHint = styleAccent.Render("  ↓ scroll  G jump to bottom") + "\n"
	}

	help := styleHelp.Render("  ↑↓/pgup/pgdn scroll  G bottom  f/tab filter  q back")

	return header + "\n" + divider + "\n" + m.vp.View() + "\n" + scrollHint + help + "\n"
}
