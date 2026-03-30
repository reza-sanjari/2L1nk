package cli

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type optionItem struct {
	label       string
	id          string
	description string
}

var optionItems = []optionItem{
	{
		label:       "No Logs",
		id:          "no_logs",
		description: "Disables log file creation. View Logs will be grayed out.",
	},
	{
		label:       "Temp Server",
		id:          "temp_server",
		description: "Securely wipes database + log file when the server is stopped.",
	},
}

type optionsDoneMsg struct{ opts Options }

type optionsModel struct {
	cursor   int
	opts     Options
	optsPath string
	width    int
	height   int
}

func newOptionsModel(opts Options, optsPath string) optionsModel {
	return optionsModel{
		opts:     opts,
		optsPath: optsPath,
	}
}

func (m optionsModel) isEnabled(id string) bool {
	switch id {
	case "no_logs":
		return m.opts.NoLogs
	case "temp_server":
		return m.opts.TempServer
	}
	return false
}

func (m optionsModel) toggle(id string) Options {
	o := m.opts
	switch id {
	case "no_logs":
		o.NoLogs = !o.NoLogs
	case "temp_server":
		o.TempServer = !o.TempServer
	}
	return o
}

func (m optionsModel) Update(msg tea.Msg) (optionsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "b", "q":
			_ = saveOptions(m.optsPath, m.opts)
			return m, func() tea.Msg { return optionsDoneMsg{opts: m.opts} }
		case "up", "k":
			m.cursor--
			if m.cursor < 0 {
				m.cursor = len(optionItems) - 1
			}
		case "down", "j":
			m.cursor++
			if m.cursor >= len(optionItems) {
				m.cursor = 0
			}
		case "enter", " ":
			id := optionItems[m.cursor].id
			m.opts = m.toggle(id)
			_ = saveOptions(m.optsPath, m.opts)
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}
	return m, nil
}

func (m optionsModel) View() string {
	var b strings.Builder

	b.WriteString(styleTitle.Render("  Options") + "\n")
	divider := styleDivider.Render(strings.Repeat("─", max(40, m.width-4)))
	b.WriteString(divider + "\n\n")

	for i, item := range optionItems {
		prefix := "   "
		if i == m.cursor {
			prefix = " ▶ "
		}

		var checkbox string
		if m.isEnabled(item.id) {
			checkbox = styleAccent.Render("[✓]")
		} else {
			checkbox = styleDisabled.Render("[ ]")
		}

		var labelStyle string
		if i == m.cursor {
			labelStyle = styleSelected.Render(" " + item.label)
		} else {
			labelStyle = styleNormal.Render(" " + item.label)
		}

		b.WriteString(styleNormal.Render(prefix) + checkbox + labelStyle + "\n")
	}

	b.WriteString("\n" + divider + "\n\n")

	// Description of currently selected item
	if m.cursor < len(optionItems) {
		b.WriteString(styleSubtle.Render("  "+optionItems[m.cursor].description) + "\n")
	}

	b.WriteString("\n" + styleHelp.Render("  ↑↓ navigate  space/enter toggle  esc back") + "\n")

	return b.String()
}
