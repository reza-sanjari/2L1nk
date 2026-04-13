package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

type tunnelLogDoneMsg struct{ name string }
type tunnelURLDetectedMsg struct {
	name string
	url  string
}

type tunnelLogModel struct {
	vp          viewport.Model
	tunnelName  string
	logPath     string
	urlPattern  string // domain suffix to identify the tunnel URL in logs
	width       int
	height      int
	atBottom    bool
	lineCount   int
	lastURL     string // most recently scanned URL, for deduplication
}

func newTunnelLogModel(name, logPath, urlPattern string, width, height int) tunnelLogModel {
	vp := viewport.New(width, height-4)
	vp.Style = styleApp

	m := tunnelLogModel{
		vp:         vp,
		tunnelName: name,
		logPath:    logPath,
		urlPattern: urlPattern,
		width:      width,
		height:     height,
		atBottom:   true,
	}
	m.refresh()
	return m
}

func (m *tunnelLogModel) refresh() {
	data, err := os.ReadFile(m.logPath)
	if err != nil {
		m.vp.SetContent(styleSubtle.Render("  No log file found. Start the tunnel to generate output."))
		m.lineCount = 0
		return
	}

	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")

	// Keep last 500 lines to avoid unbounded growth
	if len(lines) > 500 {
		lines = lines[len(lines)-500:]
	}

	m.lineCount = len(lines)
	m.vp.SetContent(strings.Join(lines, "\n"))

	if m.atBottom {
		m.vp.GotoBottom()
	}
}

// scanURLs scans log lines for the tunnel's public URL.
// If urlPattern is set, only URLs containing that pattern are considered.
// Returns the URL if a new one is found since last scan.
func (m *tunnelLogModel) scanURLs() (string, bool) {
	data, err := os.ReadFile(m.logPath)
	if err != nil {
		return "", false
	}

	lines := strings.Split(string(data), "\n")
	found := ""
	for _, line := range lines {
		matches := tunnelURLRegex.FindAllString(line, -1)
		for _, u := range matches {
			if m.urlPattern == "" || strings.Contains(u, m.urlPattern) {
				found = u
				break
			}
		}
		if found != "" {
			break
		}
	}

	if found != "" && found != m.lastURL {
		m.lastURL = found
		return found, true
	}
	return "", false
}

func (m tunnelLogModel) Update(msg tea.Msg) (tunnelLogModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc":
			name := m.tunnelName
			return m, func() tea.Msg { return tunnelLogDoneMsg{name: name} }
		case "G":
			m.vp.GotoBottom()
			m.atBottom = true
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
		// Check for new URL
		name := m.tunnelName
		if url, found := m.scanURLs(); found {
			return m, tea.Batch(
				tickCmd(),
				func() tea.Msg { return tunnelURLDetectedMsg{name: name, url: url} },
			)
		}
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

func (m tunnelLogModel) View() string {
	header := styleTitle.Render(fmt.Sprintf("  %s Logs  (%d lines)", m.tunnelName, m.lineCount))
	if m.lastURL != "" {
		short := m.lastURL
		if len(short) > 40 {
			short = short[:37] + "..."
		}
		header += "  " + styleAccent.Render("URL: "+short)
	}
	divider := styleDivider.Render(strings.Repeat("─", max(50, m.width-4)))

	scrollHint := ""
	if !m.atBottom {
		scrollHint = styleAccent.Render("  ↓ scroll  G jump to bottom") + "\n"
	}

	help := styleHelp.Render("  ↑↓/pgup/pgdn scroll  G bottom  q back")

	return header + "\n" + divider + "\n" + m.vp.View() + "\n" + scrollHint + help + "\n"
}
