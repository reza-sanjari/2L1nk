package cli

import (
	"strings"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	qrcode "github.com/skip2/go-qrcode"
)

type linkDetailDoneMsg struct{}

type linkDetailModel struct {
	label  string
	url    string
	cursor int
	showQR bool
	qrStr  string // generated once on first show
	msg    string
	msgErr bool
	width  int
	height int
}

func newLinkDetailModel(label, url string) linkDetailModel {
	return linkDetailModel{label: label, url: url}
}

func (m linkDetailModel) itemLabels() []string {
	qrLabel := "Show QR Code"
	if m.showQR {
		qrLabel = "Hide QR Code"
	}
	return []string{"Copy Link", qrLabel, "← Back"}
}

func (m linkDetailModel) Update(msg tea.Msg) (linkDetailModel, tea.Cmd) {
	labels := m.itemLabels()
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			return m, func() tea.Msg { return linkDetailDoneMsg{} }
		case "up", "k":
			m.cursor--
			if m.cursor < 0 {
				m.cursor = len(labels) - 1
			}
		case "down", "j":
			m.cursor++
			if m.cursor >= len(labels) {
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

func (m linkDetailModel) handleSelect() (linkDetailModel, tea.Cmd) {
	switch m.cursor {
	case 0: // Copy Link
		if err := clipboard.WriteAll(m.url); err != nil {
			m.msg = "Clipboard unavailable: " + err.Error()
			m.msgErr = true
		} else {
			m.msg = "Copied!"
			m.msgErr = false
		}
	case 1: // Toggle QR Code
		if m.showQR {
			m.showQR = false
		} else {
			if m.qrStr == "" {
				qr, err := qrcode.New(m.url, qrcode.Medium)
				if err != nil {
					m.msg = "QR generation failed: " + err.Error()
					m.msgErr = true
					return m, nil
				}
				m.qrStr = qr.ToSmallString(false)
			}
			m.showQR = true
			m.msg = ""
			m.msgErr = false
		}
	case 2: // Back
		return m, func() tea.Msg { return linkDetailDoneMsg{} }
	}
	return m, nil
}

func (m linkDetailModel) View() string {
	var b strings.Builder

	b.WriteString(styleTitle.Render("  "+m.label) + "\n")
	divider := styleDivider.Render(strings.Repeat("─", max(40, m.width-4)))
	b.WriteString(divider + "\n\n")

	b.WriteString(styleSubtle.Render("  URL: ") + styleAccent.Render(m.url) + "\n\n")

	labels := m.itemLabels()
	for i, label := range labels {
		prefix := "   "
		if i == m.cursor {
			prefix = " ▶ "
		}
		var line string
		if i == m.cursor {
			line = styleSelected.Render(prefix + label)
		} else {
			line = styleNormal.Render(prefix + label)
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

	if m.showQR && m.qrStr != "" {
		b.WriteString("\n" + divider + "\n\n")
		b.WriteString(m.qrStr)
	}

	b.WriteString("\n" + styleHelp.Render("  ↑↓ navigate  enter select  esc back") + "\n")

	return b.String()
}
