package cli

import (
	"strings"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
)

type tunnelDetailDoneMsg struct{}
type tunnelDetailLogsMsg struct{ name string }
type tunnelDetailStartMsg struct{ name string }
type tunnelDetailStopMsg struct{ name string }
type tunnelDetailDeleteMsg struct{ name string }

type detailItem struct {
	label string
	id    string
}

type tunnelDetailModel struct {
	entry   TunnelEntry
	runtime *tunnelRuntime // may be nil if stopped
	cursor  int
	width   int
	height  int
	msg     string
	msgErr  bool
}

func newTunnelDetailModel(entry TunnelEntry, rt *tunnelRuntime) tunnelDetailModel {
	return tunnelDetailModel{
		entry:   entry,
		runtime: rt,
	}
}

func (m tunnelDetailModel) items() []detailItem {
	items := []detailItem{}
	isRunning := m.runtime != nil && m.runtime.status == tunnelRunning
	isStarting := m.runtime != nil && m.runtime.status == tunnelStarting

	if isRunning || isStarting {
		items = append(items, detailItem{label: "Stop", id: "stop"})
	} else {
		items = append(items, detailItem{label: "Start", id: "start"})
	}

	if m.runtime != nil && m.runtime.detectedURL != "" {
		items = append(items, detailItem{label: "Copy URL", id: "copy_url"})
	}

	items = append(items,
		detailItem{label: "View Logs", id: "logs"},
		detailItem{label: "Delete Tunnel", id: "delete"},
		detailItem{label: "← Back", id: "back"},
	)
	return items
}

func (m tunnelDetailModel) Update(msg tea.Msg) (tunnelDetailModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		items := m.items()
		switch msg.String() {
		case "esc", "q":
			return m, func() tea.Msg { return tunnelDetailDoneMsg{} }
		case "up", "k":
			m.cursor--
			if m.cursor < 0 {
				m.cursor = len(items) - 1
			}
		case "down", "j":
			m.cursor++
			if m.cursor >= len(items) {
				m.cursor = 0
			}
		case "enter":
			return m.handleSelect()
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}
	return m, nil
}

func (m tunnelDetailModel) handleSelect() (tunnelDetailModel, tea.Cmd) {
	items := m.items()
	if m.cursor >= len(items) {
		return m, nil
	}
	id := items[m.cursor].id
	name := m.entry.Name

	switch id {
	case "start":
		return m, func() tea.Msg { return tunnelDetailStartMsg{name: name} }
	case "stop":
		return m, func() tea.Msg { return tunnelDetailStopMsg{name: name} }
	case "copy_url":
		url := ""
		if m.runtime != nil {
			url = m.runtime.detectedURL
		}
		if url == "" {
			m.msg = "No URL detected yet."
			m.msgErr = false
			return m, nil
		}
		if err := clipboard.WriteAll(url); err != nil {
			m.msg = "Clipboard unavailable: " + err.Error()
			m.msgErr = true
		} else {
			m.msg = "URL copied!"
			m.msgErr = false
		}
		return m, nil
	case "logs":
		return m, func() tea.Msg { return tunnelDetailLogsMsg{name: name} }
	case "delete":
		return m, func() tea.Msg { return tunnelDetailDeleteMsg{name: name} }
	case "back":
		return m, func() tea.Msg { return tunnelDetailDoneMsg{} }
	}
	return m, nil
}

func (m tunnelDetailModel) View() string {
	var b strings.Builder

	title := styleTitle.Render("  " + m.entry.Name)
	b.WriteString(title + "\n")
	divider := styleDivider.Render(strings.Repeat("─", max(40, m.width-4)))
	b.WriteString(divider + "\n\n")

	// Status line
	dot, label := tunnelStatusDisplay(m.runtime)
	b.WriteString(styleSubtle.Render("  Status: ") + dot + label + "\n")

	// Detected URL
	if m.runtime != nil && m.runtime.detectedURL != "" {
		b.WriteString(styleSubtle.Render("  URL:    ") + styleAccent.Render(m.runtime.detectedURL) + "\n")
	} else {
		b.WriteString(styleSubtle.Render("  URL:    ") + styleDisabled.Render("—") + "\n")
	}

	// Command preview
	if m.entry.Port > 0 {
		b.WriteString(styleSubtle.Render("  Port:   ") + styleNormal.Render(resolveCommand("{PORT}", m.entry.Port)) + "\n")
	}
	if m.entry.Description != "" {
		b.WriteString(styleSubtle.Render("  Info:   ") + styleNormal.Render(m.entry.Description) + "\n")
	}

	b.WriteString("\n" + divider + "\n\n")

	// Action items
	items := m.items()
	for i, item := range items {
		prefix := "   "
		if i == m.cursor {
			prefix = " ▶ "
		}
		var line string
		switch {
		case item.id == "delete" && i != m.cursor:
			line = styleDanger.Render(prefix + item.label)
		case i == m.cursor:
			line = styleSelected.Render(prefix + item.label)
		default:
			line = styleNormal.Render(prefix + item.label)
		}
		b.WriteString(line + "\n")
	}

	if m.msg != "" {
		b.WriteString("\n")
		if m.msgErr {
			b.WriteString(styleDanger.Render("  ✗ "+m.msg) + "\n")
		} else {
			b.WriteString(styleSuccess.Render("  ✓ "+m.msg) + "\n")
		}
	}

	b.WriteString("\n" + styleHelp.Render("  ↑↓ navigate  enter select  esc back") + "\n")

	return b.String()
}
