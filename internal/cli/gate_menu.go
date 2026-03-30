package cli

import (
	"fmt"
	"strconv"
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

// Static action items — rendered below the unlimited toggle and max-uses input.
var gateMenuItems = []gateMenuItem{
	{label: "Copy Key", id: "copy"},
	{label: "Refresh Key", id: "refresh"},
	{label: "Set Custom Key", id: "custom"},
	{label: "View Past Keys", id: "history"},
	{label: "← Back", id: "back"},
}

type gateMenuModel struct {
	g    *gate.Gate
	repo gate.GateRepository

	cursor   int
	width    int
	height   int
	msg      string
	msgIsErr bool

	// Custom key input mode
	inputMode bool
	input     textinput.Model

	// Unlimited toggle and max-uses input
	unlimited      bool
	maxUsesInput   textinput.Model
	editingMaxUses bool

	// History sub-screen
	viewingHistory bool
	history        gateHistoryModel
}

func newGateMenuModel(g *gate.Gate) gateMenuModel {
	ti := textinput.New()
	ti.Placeholder = "custom key..."
	ti.CharLimit = 256
	ti.Width = 66

	mui := textinput.New()
	mui.Placeholder = "e.g. 10"
	mui.CharLimit = 9
	mui.Width = 20

	return gateMenuModel{
		g:            g,
		repo:         g.Repo(),
		input:        ti,
		maxUsesInput: mui,
		unlimited:    g.MaxUses() == 0,
	}
}

type gateDoneMsg struct{} // signals return to main menu

// effectiveItems returns the full navigable virtual item list:
// [0] unlimited toggle
// [1] max_uses input (only when !unlimited)
// [2..] action items
func (m gateMenuModel) effectiveItems() []string {
	items := []string{"unlimited_toggle"}
	if !m.unlimited {
		items = append(items, "max_uses_input")
	}
	for _, item := range gateMenuItems {
		items = append(items, item.id)
	}
	return items
}

func (m gateMenuModel) Update(msg tea.Msg) (gateMenuModel, tea.Cmd) {
	// Delegate to history sub-screen when active.
	if m.viewingHistory {
		var cmd tea.Cmd
		m.history, cmd = m.history.Update(msg)
		return m, cmd
	}

	if m.inputMode {
		return m.updateInput(msg)
	}

	if m.editingMaxUses {
		return m.updateMaxUsesInput(msg)
	}

	switch msg := msg.(type) {
	case gateHistoryDoneMsg:
		m.viewingHistory = false
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			return m, func() tea.Msg { return gateDoneMsg{} }
		case "up", "k":
			m.cursor--
			if m.cursor < 0 {
				m.cursor = len(m.effectiveItems()) - 1
			}
		case "down", "j":
			m.cursor++
			if m.cursor >= len(m.effectiveItems()) {
				m.cursor = 0
			}
		case "r":
			if err := m.g.RefreshFromDB(); err != nil {
				m.msg = "Refresh failed: " + err.Error()
				m.msgIsErr = true
			} else {
				m.msg = "Usage count refreshed."
				m.msgIsErr = false
			}
			return m, nil
		case " ":
			items := m.effectiveItems()
			if m.cursor < len(items) && items[m.cursor] == "unlimited_toggle" {
				return m.handleSelect()
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
	items := m.effectiveItems()
	if m.cursor >= len(items) {
		return m, nil
	}
	id := items[m.cursor]

	switch id {
	case "unlimited_toggle":
		newUnlimited := !m.unlimited
		maxUses := 0
		if !newUnlimited {
			maxUses = 10
			m.maxUsesInput.SetValue("10")
		}
		if err := m.g.SetMaxUses(maxUses); err != nil {
			m.msg = "Failed to update limit: " + err.Error()
			m.msgIsErr = true
			return m, nil
		}
		m.unlimited = newUnlimited
		m.msg = ""
		// Recalculate cursor so it stays valid after the list length changes.
		if m.cursor >= len(m.effectiveItems()) {
			m.cursor = len(m.effectiveItems()) - 1
		}

	case "max_uses_input":
		m.editingMaxUses = true
		m.maxUsesInput.SetValue(strconv.Itoa(m.g.MaxUses()))
		m.maxUsesInput.Focus()

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

	case "history":
		if m.repo == nil {
			m.msg = "No DB connection available."
			m.msgIsErr = true
			return m, nil
		}
		m.history = newGateHistoryModel(m.repo, m.width, m.height)
		m.viewingHistory = true
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
			maxUses := 0
			if !m.unlimited {
				maxUses = parseMaxUses(m.maxUsesInput.Value())
			}
			if err := m.g.SetKey(val, maxUses); err != nil {
				m.msg = "Failed to set key: " + err.Error()
				m.msgIsErr = true
				return m, nil
			}
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

func (m gateMenuModel) updateMaxUsesInput(msg tea.Msg) (gateMenuModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.editingMaxUses = false
			m.maxUsesInput.Blur()
			return m, nil
		case "enter":
			n := parseMaxUses(m.maxUsesInput.Value())
			if err := m.g.SetMaxUses(n); err != nil {
				m.msg = "Failed to update max uses: " + err.Error()
				m.msgIsErr = true
			} else {
				m.msg = fmt.Sprintf("Max uses set to %d.", n)
				m.msgIsErr = false
			}
			m.editingMaxUses = false
			m.maxUsesInput.Blur()
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.maxUsesInput, cmd = m.maxUsesInput.Update(msg)
	return m, cmd
}

func parseMaxUses(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

func (m gateMenuModel) View() string {
	// Delegate to history sub-screen.
	if m.viewingHistory {
		return m.history.View()
	}

	var b strings.Builder

	title := styleTitle.Render("  Gate Key")
	b.WriteString(title + "\n")

	divider := styleDivider.Render(strings.Repeat("─", max(50, m.width-4)))
	b.WriteString(divider + "\n\n")

	// Current key display
	key := m.g.Key()
	uses := m.g.UseCount()
	maxUsesStr := "∞"
	if m.g.MaxUses() > 0 {
		maxUsesStr = strconv.Itoa(m.g.MaxUses())
	}
	keyLine := fmt.Sprintf("  %s  (uses: %d / %s)", key, uses, maxUsesStr)
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

	items := m.effectiveItems()

	// Unlimited toggle (always index 0)
	{
		prefix := "   "
		if m.cursor == 0 {
			prefix = " ▶ "
		}
		checkmark := styleDisabled.Render("[ ]")
		if m.unlimited {
			checkmark = styleAccent.Render("[✓]")
		}
		label := styleNormal.Render(" Unlimited")
		if m.cursor == 0 {
			label = styleSelected.Render(" Unlimited")
		}
		b.WriteString(styleNormal.Render(prefix) + checkmark + label + "\n")
	}

	// Max-uses input (index 1 when !unlimited)
	if !m.unlimited {
		prefix := "   "
		if m.cursor == 1 {
			prefix = " ▶ "
		}
		if m.editingMaxUses {
			b.WriteString(styleNormal.Render(prefix+"Max uses: ") + m.maxUsesInput.View() + "\n")
		} else {
			val := strconv.Itoa(m.g.MaxUses())
			line := prefix + "Max uses: " + val
			if m.cursor == 1 {
				b.WriteString(styleSelected.Render(line) + "\n")
			} else {
				b.WriteString(styleNormal.Render(line) + "\n")
			}
		}
	}

	b.WriteString("\n")

	// Action items
	actionOffset := 1
	if !m.unlimited {
		actionOffset = 2
	}
	for i, item := range gateMenuItems {
		virtualIdx := actionOffset + i
		prefix := "   "
		if m.cursor == virtualIdx {
			prefix = " ▶ "
		}
		var line string
		if m.cursor == virtualIdx {
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

	_ = items // used for cursor bounds only
	b.WriteString("\n" + styleHelp.Render("  ↑↓ navigate  enter select  space toggle  r refresh count  esc back") + "\n")

	return b.String()
}
