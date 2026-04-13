package cli

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type menuItem struct {
	label string
	id    string
}

var menuItems = []menuItem{
	{label: "Run Server", id: "run"},
	{label: "Stop Server", id: "stop"},
	{label: "Gate Key", id: "gate"},
	{label: "View Logs", id: "logs"},
	{label: "Outbound Tunnels", id: "tunnels"},
	{label: "Links", id: "links"},
	{label: "Reset Database", id: "reset"},
	{label: "Options", id: "options"},
	{label: "☢  Nuke", id: "nuke"},
}

type menuModel struct {
	cursor      int
	width       int
	height      int
	serverState serverState
	port        int
	noLogs      bool
	flashMsg    string
}

func newMenuModel() menuModel {
	return menuModel{cursor: 0}
}

func (m menuModel) Update(msg tea.Msg) (menuModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			m.cursor--
			if m.cursor < 0 {
				m.cursor = len(menuItems) - 1
			}
		case "down", "j":
			m.cursor++
			if m.cursor >= len(menuItems) {
				m.cursor = 0
			}
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case flashMsg:
		m.flashMsg = string(msg)
	}
	return m, nil
}

func (m menuModel) selectedItem() string {
	return menuItems[m.cursor].id
}

func (m menuModel) isDisabled(id string) bool {
	switch id {
	case "run":
		return m.serverState == stateRunning
	case "stop":
		return m.serverState == stateStopped
	case "logs":
		return m.noLogs
	}
	return false
}

func (m menuModel) View() string {
	var b strings.Builder

	b.WriteString(styleTitle.Render("  2L1nk Control Panel") + "\n")
	divider := styleDivider.Render(strings.Repeat("─", max(40, m.width-4)))
	b.WriteString(divider + "\n\n")

	for i, item := range menuItems {
		prefix := "   "
		if i == m.cursor {
			prefix = " ▶ "
		}

		label := item.label
		switch item.id {
		case "gate", "options", "tunnels", "links":
			label += "  →"
		}

		var line string
		switch {
		case m.isDisabled(item.id):
			line = styleDisabled.Render(prefix + label)
		case item.id == "nuke" && i != m.cursor:
			// Nuke is always danger-colored even when not selected
			line = styleDanger.Render(prefix + label)
		case i == m.cursor:
			line = styleSelected.Render(prefix + label)
		default:
			line = styleNormal.Render(prefix + label)
		}
		b.WriteString(line + "\n")
	}

	b.WriteString("\n" + divider + "\n")
	b.WriteString(m.renderStatus() + "\n")

	if m.flashMsg != "" {
		b.WriteString("\n" + styleAccent.Render("  "+m.flashMsg) + "\n")
	}

	b.WriteString("\n" + styleHelp.Render("  ↑↓ navigate  enter select  q quit") + "\n")

	return b.String()
}

func (m menuModel) renderStatus() string {
	switch m.serverState {
	case stateRunning:
		dot := styleSuccess.Render("●")
		port := styleAccent.Render(fmt.Sprintf(":%d", m.port))
		return styleStatusBar.Render("  Status: ") + dot + styleStatusBar.Render(" Running  ") + port
	case stateStopping:
		dot := styleDanger.Render("◌")
		return styleStatusBar.Render("  Status: ") + dot + styleStatusBar.Render(" Stopping...")
	default:
		dot := styleDisabled.Render("○")
		return styleStatusBar.Render("  Status: ") + dot + styleStatusBar.Render(" Stopped")
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

type flashMsg string

func cmdFlash(msg string) tea.Cmd {
	return func() tea.Msg { return flashMsg(msg) }
}
