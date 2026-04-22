package cli

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type tunnelsDoneMsg struct{}
type tunnelSelectedMsg struct{ index int }
type tunnelAddRequestMsg struct{}

type tunnelMenuModel struct {
	tunnels []TunnelEntry
	states  map[string]*tunnelRuntime // pointer to shared runtimes in main model
	cursor  int
	width   int
	height  int
	msg     string
}

func newTunnelMenuModel(tunnels []TunnelEntry, states map[string]*tunnelRuntime) tunnelMenuModel {
	return tunnelMenuModel{
		tunnels: tunnels,
		states:  states,
	}
}

// itemCount returns total virtual item count: tunnels + "Add Tunnel" + "Back"
func (m tunnelMenuModel) itemCount() int {
	return len(m.tunnels) + 2
}

// addIdx returns virtual index of the "Add Tunnel" item.
func (m tunnelMenuModel) addIdx() int { return len(m.tunnels) }

// backIdx returns virtual index of the "Back" item.
func (m tunnelMenuModel) backIdx() int { return len(m.tunnels) + 1 }

func (m tunnelMenuModel) Update(msg tea.Msg) (tunnelMenuModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			return m, func() tea.Msg { return tunnelsDoneMsg{} }
		case "up", "k":
			m.cursor--
			if m.cursor < 0 {
				m.cursor = m.itemCount() - 1
			}
		case "down", "j":
			m.cursor++
			if m.cursor >= m.itemCount() {
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

func (m tunnelMenuModel) handleSelect() (tunnelMenuModel, tea.Cmd) {
	switch m.cursor {
	case m.addIdx():
		return m, func() tea.Msg { return tunnelAddRequestMsg{} }
	case m.backIdx():
		return m, func() tea.Msg { return tunnelsDoneMsg{} }
	default:
		idx := m.cursor
		return m, func() tea.Msg { return tunnelSelectedMsg{index: idx} }
	}
}

func (m tunnelMenuModel) View() string {
	var b strings.Builder

	b.WriteString(styleTitle.Render("  Outbound Tunnels") + "\n")
	divider := styleDivider.Render(strings.Repeat("─", max(40, m.width-4)))
	b.WriteString(divider + "\n\n")

	if len(m.tunnels) == 0 {
		b.WriteString(styleSubtle.Render("  No tunnels configured yet.") + "\n\n")
	}

	for i, t := range m.tunnels {
		prefix := "   "
		if i == m.cursor {
			prefix = " ▶ "
		}

		rt := m.states[t.Name]
		statusDot, statusLabel := tunnelStatusDisplay(rt)

		// Build the name + description portion
		nameStr := t.Name
		if t.Description != "" {
			nameStr = fmt.Sprintf("%s  %s", t.Name, styleSubtle.Render("("+t.Description+")"))
		}

		badge := fmt.Sprintf(" [%s%s]", statusDot, statusLabel)

		// Show detected URL if running
		urlHint := ""
		if rt != nil && rt.detectedURL != "" {
			short := rt.detectedURL
			if len(short) > 35 {
				short = short[:32] + "..."
			}
			urlHint = "  " + styleSubtle.Render(short)
		}

		var line string
		if i == m.cursor {
			line = styleSelected.Render(prefix+nameStr+badge) + urlHint
		} else {
			line = styleNormal.Render(prefix+nameStr+badge) + urlHint
		}
		b.WriteString(line + "\n")
	}

	// Add Tunnel item
	{
		prefix := "   "
		if m.cursor == m.addIdx() {
			prefix = " ▶ "
		}
		label := prefix + "+ Add Tunnel"
		if m.cursor == m.addIdx() {
			b.WriteString(styleSelected.Render(label) + "\n")
		} else {
			b.WriteString(styleAccent.Render(label) + "\n")
		}
	}

	// Back item
	{
		prefix := "   "
		if m.cursor == m.backIdx() {
			prefix = " ▶ "
		}
		label := prefix + "← Back"
		if m.cursor == m.backIdx() {
			b.WriteString(styleSelected.Render(label) + "\n")
		} else {
			b.WriteString(styleNormal.Render(label) + "\n")
		}
	}

	if m.msg != "" {
		b.WriteString("\n" + styleAccent.Render("  "+m.msg) + "\n")
	}

	b.WriteString("\n" + divider + "\n")
	b.WriteString(styleHelp.Render("  ↑↓ navigate  enter select  esc back") + "\n")

	return b.String()
}

func tunnelStatusDisplay(rt *tunnelRuntime) (dot, label string) {
	if rt == nil || rt.status == tunnelStopped {
		return styleDisabled.Render("○"), styleDisabled.Render(" stopped")
	}
	switch rt.status {
	case tunnelRunning:
		return styleSuccess.Render("●"), styleSuccess.Render(" running")
	case tunnelStarting:
		return styleAccent.Render("◌"), styleAccent.Render(" starting")
	case tunnelStopping:
		return styleDanger.Render("◌"), styleDanger.Render(" stopping")
	}
	return styleDisabled.Render("○"), styleDisabled.Render(" stopped")
}
