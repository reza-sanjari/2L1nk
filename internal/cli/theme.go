package cli

import "github.com/charmbracelet/lipgloss"

// Palette matches web/css/mainsite.css
const (
	colorAccent     = lipgloss.Color("#bc13fe")
	colorAccentDark = lipgloss.Color("#4b0082")
	colorBgDark     = lipgloss.Color("#1a0525")
	colorBgMid      = lipgloss.Color("#310a5d")
	colorText       = lipgloss.Color("#ffffff")
	colorSubtle     = lipgloss.Color("#888888")
	colorSuccess    = lipgloss.Color("#4ade80")
	colorDanger     = lipgloss.Color("#ff6464")
	colorMuted      = lipgloss.Color("#555555")
)

var (
	styleApp = lipgloss.NewStyle().
			Background(colorBgDark).
			Foreground(colorText)

	styleBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorAccentDark).
			Background(colorBgDark).
			Foreground(colorText).
			Padding(0, 1)

	styleTitle = lipgloss.NewStyle().
			Foreground(colorAccent).
			Bold(true).
			Padding(0, 1)

	styleDivider = lipgloss.NewStyle().
			Foreground(colorAccentDark)

	styleSelected = lipgloss.NewStyle().
			Foreground(colorAccent).
			Bold(true)

	styleNormal = lipgloss.NewStyle().
			Foreground(colorText)

	styleDisabled = lipgloss.NewStyle().
			Foreground(colorMuted)

	styleSubtle = lipgloss.NewStyle().
			Foreground(colorSubtle)

	styleSuccess = lipgloss.NewStyle().
			Foreground(colorSuccess)

	styleDanger = lipgloss.NewStyle().
			Foreground(colorDanger)

	styleAccent = lipgloss.NewStyle().
			Foreground(colorAccent)

	styleKeyBadge = lipgloss.NewStyle().
			Foreground(colorBgDark).
			Background(colorAccentDark).
			Padding(0, 1)

	styleStatusBar = lipgloss.NewStyle().
			Foreground(colorSubtle).
			Padding(0, 1)

	styleInput = lipgloss.NewStyle().
			Foreground(colorAccent).
			Border(lipgloss.NormalBorder()).
			BorderForeground(colorAccentDark).
			Padding(0, 1)

	styleHelp = lipgloss.NewStyle().
			Foreground(colorMuted).
			Italic(true)
)
