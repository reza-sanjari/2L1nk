package cli

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type tunnelAddDoneMsg struct{ entry TunnelEntry }
type tunnelAddCancelMsg struct{}

type addStep int

const (
	addStepPreset addStep = iota // pick preset or custom
	addStepForm                  // fill in fields
)

// formField tracks which field of the form is active.
type formField int

const (
	fieldName formField = iota
	fieldCommand
	fieldDescription
	fieldPort
	fieldAutoStart
	fieldConfirm
	fieldCancel
	fieldCount // sentinel
)

type tunnelAddModel struct {
	step      addStep
	presetIdx int // cursor on preset list
	width     int
	height    int

	// form state
	activeField formField
	nameInput   textinput.Model
	cmdInput    textinput.Model
	descInput   textinput.Model
	portInput   textinput.Model
	autoStart   bool

	errMsg string
}

func newTunnelAddModel() tunnelAddModel {
	name := textinput.New()
	name.Placeholder = "e.g. Cloudflare"
	name.CharLimit = 64
	name.Width = 40

	cmd := textinput.New()
	cmd.Placeholder = "e.g. cloudflared tunnel --url http://localhost:{PORT}"
	cmd.CharLimit = 256
	cmd.Width = 60

	desc := textinput.New()
	desc.Placeholder = "optional description"
	desc.CharLimit = 128
	desc.Width = 40

	port := textinput.New()
	port.Placeholder = "e.g. 3847"
	port.CharLimit = 6
	port.Width = 10

	m := tunnelAddModel{
		step:        addStepPreset,
		nameInput:   name,
		cmdInput:    cmd,
		descInput:   desc,
		portInput:   port,
		activeField: fieldName,
	}
	return m
}

// presetCount returns number of preset items + 1 for "Custom".
func (m tunnelAddModel) presetCount() int { return len(tunnelPresets) + 1 }

// customPresetIdx is the index of the "Custom" option in the preset list.
func (m tunnelAddModel) customPresetIdx() int { return len(tunnelPresets) }

func (m tunnelAddModel) Update(msg tea.Msg) (tunnelAddModel, tea.Cmd) {
	switch m.step {
	case addStepPreset:
		return m.updatePreset(msg)
	case addStepForm:
		return m.updateForm(msg)
	}
	return m, nil
}

func (m tunnelAddModel) updatePreset(msg tea.Msg) (tunnelAddModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			return m, func() tea.Msg { return tunnelAddCancelMsg{} }
		case "up", "k":
			m.presetIdx--
			if m.presetIdx < 0 {
				m.presetIdx = m.presetCount() - 1
			}
		case "down", "j":
			m.presetIdx++
			if m.presetIdx >= m.presetCount() {
				m.presetIdx = 0
			}
		case "enter":
			return m.selectPreset()
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}
	return m, nil
}

func (m tunnelAddModel) selectPreset() (tunnelAddModel, tea.Cmd) {
	m.step = addStepForm
	m.activeField = fieldName

	if m.presetIdx < len(tunnelPresets) {
		p := tunnelPresets[m.presetIdx]
		m.nameInput.SetValue(p.Name)
		m.cmdInput.SetValue(p.Command)
		m.descInput.SetValue(p.Description)
		if p.Port > 0 {
			m.portInput.SetValue(strconv.Itoa(p.Port))
		}
		m.autoStart = p.AutoStart
	} else {
		// Custom — start blank
		m.nameInput.SetValue("")
		m.cmdInput.SetValue("")
		m.descInput.SetValue("")
		m.portInput.SetValue("")
		m.autoStart = false
	}

	m.focusField(m.activeField)
	return m, nil
}

func (m *tunnelAddModel) focusField(f formField) {
	m.nameInput.Blur()
	m.cmdInput.Blur()
	m.descInput.Blur()
	m.portInput.Blur()
	switch f {
	case fieldName:
		m.nameInput.Focus()
	case fieldCommand:
		m.cmdInput.Focus()
	case fieldDescription:
		m.descInput.Focus()
	case fieldPort:
		m.portInput.Focus()
	}
}

func (m tunnelAddModel) updateForm(msg tea.Msg) (tunnelAddModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return tunnelAddCancelMsg{} }
		case "tab", "down":
			m.activeField = (m.activeField + 1) % fieldCount
			m.focusField(m.activeField)
			return m, nil
		case "shift+tab", "up":
			m.activeField = (m.activeField - 1 + fieldCount) % fieldCount
			m.focusField(m.activeField)
			return m, nil
		case "enter":
			switch m.activeField {
			case fieldAutoStart:
				m.autoStart = !m.autoStart
				return m, nil
			case fieldConfirm:
				return m.confirm()
			case fieldCancel:
				return m, func() tea.Msg { return tunnelAddCancelMsg{} }
			default:
				// move to next field on enter (unless on last input field)
				m.activeField = (m.activeField + 1) % fieldCount
				m.focusField(m.activeField)
				return m, nil
			}
		case " ":
			if m.activeField == fieldAutoStart {
				m.autoStart = !m.autoStart
				return m, nil
			}
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}

	// Forward to active input
	var cmd tea.Cmd
	switch m.activeField {
	case fieldName:
		m.nameInput, cmd = m.nameInput.Update(msg)
	case fieldCommand:
		m.cmdInput, cmd = m.cmdInput.Update(msg)
	case fieldDescription:
		m.descInput, cmd = m.descInput.Update(msg)
	case fieldPort:
		m.portInput, cmd = m.portInput.Update(msg)
	}
	return m, cmd
}

func (m tunnelAddModel) confirm() (tunnelAddModel, tea.Cmd) {
	name := strings.TrimSpace(m.nameInput.Value())
	cmd := strings.TrimSpace(m.cmdInput.Value())

	if name == "" {
		m.errMsg = "Name is required."
		m.activeField = fieldName
		m.focusField(m.activeField)
		return m, nil
	}
	if cmd == "" {
		m.errMsg = "Command is required."
		m.activeField = fieldCommand
		m.focusField(m.activeField)
		return m, nil
	}

	port := 0
	if ps := strings.TrimSpace(m.portInput.Value()); ps != "" {
		n, err := strconv.Atoi(ps)
		if err != nil || n <= 0 || n > 65535 {
			m.errMsg = "Port must be a number between 1 and 65535."
			m.activeField = fieldPort
			m.focusField(m.activeField)
			return m, nil
		}
		port = n
	}

	entry := TunnelEntry{
		Name:        name,
		Command:     cmd,
		Description: strings.TrimSpace(m.descInput.Value()),
		Port:        port,
		AutoStart:   m.autoStart,
	}
	return m, func() tea.Msg { return tunnelAddDoneMsg{entry: entry} }
}

func (m tunnelAddModel) View() string {
	var b strings.Builder

	b.WriteString(styleTitle.Render("  Add Tunnel") + "\n")
	divider := styleDivider.Render(strings.Repeat("─", max(40, m.width-4)))
	b.WriteString(divider + "\n\n")

	switch m.step {
	case addStepPreset:
		b.WriteString(styleSubtle.Render("  Select a preset or choose Custom:") + "\n\n")
		for i, p := range tunnelPresets {
			prefix := "   "
			if i == m.presetIdx {
				prefix = " ▶ "
			}
			if i == m.presetIdx {
				b.WriteString(styleSelected.Render(prefix+p.Name) + "  " + styleSubtle.Render("("+p.Description+")") + "\n")
			} else {
				b.WriteString(styleNormal.Render(prefix+p.Name) + "  " + styleSubtle.Render("("+p.Description+")") + "\n")
			}
		}
		// Custom option
		{
			prefix := "   "
			if m.presetIdx == m.customPresetIdx() {
				prefix = " ▶ "
			}
			label := prefix + "Custom"
			if m.presetIdx == m.customPresetIdx() {
				b.WriteString(styleSelected.Render(label) + "\n")
			} else {
				b.WriteString(styleNormal.Render(label) + "\n")
			}
		}
		b.WriteString("\n" + styleHelp.Render("  ↑↓ navigate  enter select  esc cancel") + "\n")

	case addStepForm:
		b.WriteString(m.renderFormField(fieldName, "Name", m.nameInput.View()))
		b.WriteString(m.renderFormField(fieldCommand, "Command", m.cmdInput.View()))
		b.WriteString(m.renderFormField(fieldDescription, "Description", m.descInput.View()))
		b.WriteString(m.renderFormField(fieldPort, "Port", m.portInput.View()))

		// AutoStart toggle
		{
			check := styleDisabled.Render("[ ]")
			if m.autoStart {
				check = styleAccent.Render("[✓]")
			}
			label := " Auto-start with server"
			var line string
			if m.activeField == fieldAutoStart {
				line = styleSelected.Render(" ▶ ") + check + styleSelected.Render(label)
			} else {
				line = styleNormal.Render("   ") + check + styleNormal.Render(label)
			}
			b.WriteString(line + "\n")
		}

		b.WriteString("\n")

		if m.errMsg != "" {
			b.WriteString(styleDanger.Render("  ✗ "+m.errMsg) + "\n\n")
		}

		// Confirm / Cancel buttons
		confirmStyle := styleNormal
		if m.activeField == fieldConfirm {
			confirmStyle = styleSelected
		}
		cancelStyle := styleNormal
		if m.activeField == fieldCancel {
			cancelStyle = styleDanger
		}

		prefix := func(f formField) string {
			if m.activeField == f {
				return " ▶ "
			}
			return "   "
		}
		b.WriteString(confirmStyle.Render(prefix(fieldConfirm)+"✓ Add Tunnel") + "\n")
		b.WriteString(cancelStyle.Render(prefix(fieldCancel)+"✗ Cancel") + "\n")

		b.WriteString("\n" + styleHelp.Render("  tab/↑↓ navigate  enter confirm  space toggle  esc cancel") + "\n")
	}

	return b.String()
}

func (m tunnelAddModel) renderFormField(f formField, label, inputView string) string {
	prefix := "   "
	if m.activeField == f {
		prefix = " ▶ "
	}
	labelStr := styleSubtle.Render(label + ":")
	if m.activeField == f {
		return styleSelected.Render(prefix) + labelStr + " " + inputView + "\n"
	}
	return styleNormal.Render(prefix) + labelStr + " " + inputView + "\n"
}
