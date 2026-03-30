package cli

import (
	"fmt"
	"strings"

	"2L1nk/internal/gate"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type gateMenuItem struct {
	label string
	id    string
}

var gateMenuItems = []gateMenuItem{
	{label: "Copy Key", id: "copy"},
	{label: "Refresh Key", id: "refresh"},
	{label: "Set Custom Key", id: "custom"},
	{label: "← Back", id: "back"},
}

type gateMenuModel struct {
	g        *gate.Gate
	cursor   int
	width    int
	height   int
	msg      string // status/error message
	msgIsErr bool

	inputMode bool
	input     textinput.Model
}

func newGateMenuModel(g *gate.Gate) gateMenuModel {
	ti := textinput.New()
	ti.Placeholder = "custom key..."
	ti.CharLimit = 256
	ti.Width = 66

	return gateMenuModel{
		g:     g,
		input: ti,
	}
}

type gateDoneMsg struct{} // signals return to main menu

func (m gateMenuModel) Update(msg tea.Msg) (gateMenuModel, tea.Cmd) {
	if m.inputMode {
		return m.updateInput(msg)
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			return m, func() tea.Msg { return gateDoneMsg{} }
		case "up", "k":
			m.cursor--
			if m.cursor < 0 {
				m.cursor = len(gateMenuItems) - 1
			}
		case "down", "j":
			m.cursor++
			if m.cursor >= len(gateMenuItems) {
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

func (m gateMenuModel) handleSelect() (gateMenuModel, tea.Cmd) {
	id := gateMenuItems[m.cursor].id
	switch id {
	case "copy":
		key := m.g.Key()
		if err := clipboard.WriteAll(key); err != nil {
			m.msg = "Clipboard unavailable: " + err.Error()
			m.msgIsErr = true
		} else {
			m.msg = "Key copied to clipboard!"
			m.msgIsErr = false
		}
	case "refresh":
		if err := m.g.Rotate(); err != nil {
			m.msg = "Failed to rotate key: " + err.Error()
			m.msgIsErr = true
		} else {
			m.msg = "Key refreshed."
			m.msgIsErr = false
		}
	case "custom":
		m.inputMode = true
		m.input.SetValue("")
		m.input.Focus()
		m.msg = ""
	case "back":
		return m, func() tea.Msg { return gateDoneMsg{} }
	}
	return m, nil
}

func (m gateMenuModel) updateInput(msg tea.Msg) (gateMenuModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.inputMode = false
			m.input.Blur()
			m.msg = ""
			return m, nil
		case "enter":
			val := strings.TrimSpace(m.input.Value())
			if len(val) == 0 {
				m.msg = "Key cannot be empty."
				m.msgIsErr = true
				return m, nil
			}
			m.g.SetKey(val)
			m.inputMode = false
			m.input.Blur()
			m.msg = "Custom key set."
			m.msgIsErr = false
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m gateMenuModel) View() string {
	var b strings.Builder

	title := styleTitle.Render("  Gate Key")
	b.WriteString(title + "\n")

	divider := styleDivider.Render(strings.Repeat("─", max(50, m.width-4)))
	b.WriteString(divider + "\n\n")

	// Current key display
	key := m.g.Key()
	uses := m.g.UseCount()
	keyLine := fmt.Sprintf("  %s  (uses: %d / unlimited)", key, uses)
	b.WriteString(styleSubtle.Render("  Current key:") + "\n")
	b.WriteString(styleKeyBadge.Render(keyLine) + "\n\n")

	b.WriteString(divider + "\n\n")

	// Custom input mode
	if m.inputMode {
		b.WriteString(styleAccent.Render("  Enter custom key:") + "\n")
		b.WriteString("  " + m.input.View() + "\n")
		b.WriteString("\n" + styleHelp.Render("  enter confirm  esc cancel") + "\n")
		return b.String()
	}

	for i, item := range gateMenuItems {
		prefix := "   "
		if i == m.cursor {
			prefix = " ▶ "
		}
		var line string
		if i == m.cursor {
			line = styleSelected.Render(prefix + item.label)
		} else {
			line = styleNormal.Render(prefix + item.label)
		}
		b.WriteString(line + "\n")
	}

	b.WriteString("\n")

	if m.msg != "" {
		if m.msgIsErr {
			b.WriteString(styleDanger.Render("  ✗ "+m.msg) + "\n")
		} else {
			b.WriteString(styleSuccess.Render("  ✓ "+m.msg) + "\n")
		}
	}

	b.WriteString("\n" + styleHelp.Render("  ↑↓ navigate  enter select  esc back") + "\n")

	return b.String()
}
