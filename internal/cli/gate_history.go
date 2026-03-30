package cli

import (
	"fmt"
	"strings"
	"time"

	"2L1nk/internal/gate"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

type gateHistoryDoneMsg struct{}

type gateHistoryModel struct {
	repo    gate.GateRepository
	records []gate.GateTokenRecord
	vp      viewport.Model
	width   int
	height  int
}

func newGateHistoryModel(repo gate.GateRepository, width, height int) gateHistoryModel {
	vp := viewport.New(width, height-6)
	vp.Style = styleApp

	m := gateHistoryModel{
		repo:   repo,
		vp:     vp,
		width:  width,
		height: height,
	}
	m.loadAndRender()
	return m
}

func (m *gateHistoryModel) loadAndRender() {
	records, err := m.repo.GetAllTokens()
	if err != nil {
		m.vp.SetContent(styleDanger.Render("  Error loading history: " + err.Error()))
		return
	}
	m.records = records
	m.vp.SetContent(m.renderTable())
	m.vp.GotoTop()
}

func (m *gateHistoryModel) renderTable() string {
	if len(m.records) == 0 {
		return styleSubtle.Render("  No gate tokens recorded yet.")
	}

	header := fmt.Sprintf("  %-12s  %-9s  %-9s  %-8s  %s",
		"Token", "Max Uses", "Use Count", "Status", "Created At")
	divider := strings.Repeat("─", max(70, m.width-6))

	var sb strings.Builder
	sb.WriteString(styleSubtle.Render(header) + "\n")
	sb.WriteString(styleDivider.Render("  "+divider) + "\n")

	for _, rec := range m.records {
		truncToken := rec.Token
		if len(truncToken) > 10 {
			truncToken = truncToken[:8] + "…"
		}

		maxUsesStr := "∞"
		if rec.MaxUses > 0 {
			maxUsesStr = fmt.Sprintf("%d", rec.MaxUses)
		}

		statusStr := "retired"
		statusStyle := styleSubtle
		if rec.IsActive {
			statusStr = "active"
			statusStyle = styleSuccess
		}

		created := time.Unix(rec.CreatedAt, 0).Format("2006-01-02 15:04")

		row := fmt.Sprintf("  %-12s  %-9s  %-9d  ", truncToken, maxUsesStr, rec.UseCount)
		sb.WriteString(styleNormal.Render(row))
		sb.WriteString(statusStyle.Render(fmt.Sprintf("%-8s", statusStr)))
		sb.WriteString(styleSubtle.Render("  " + created))
		sb.WriteString("\n")
	}
	return sb.String()
}

func (m gateHistoryModel) Update(msg tea.Msg) (gateHistoryModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc":
			return m, func() tea.Msg { return gateHistoryDoneMsg{} }
		case "r":
			m.loadAndRender()
			return m, nil
		}
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		return m, cmd

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.vp.Width = msg.Width
		m.vp.Height = msg.Height - 6
		m.loadAndRender()
		return m, nil
	}

	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
}

func (m gateHistoryModel) View() string {
	title := styleTitle.Render("  Gate Key History")
	divider := styleDivider.Render(strings.Repeat("─", max(50, m.width-4)))
	help := styleHelp.Render("  ↑↓/pgup/pgdn scroll  r refresh  q/esc back")
	return title + "\n" + divider + "\n" + m.vp.View() + "\n" + help + "\n"
}
