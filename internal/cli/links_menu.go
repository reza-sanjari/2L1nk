package cli

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type linkItem struct {
	label string
	url   string
}

type linksMenuModel struct {
	port     int
	runtimes map[string]*tunnelRuntime // shared pointer — always up-to-date
	localIP  string
	globalIP string // "" = fetching, "error" = unavailable, else valid IP
	cursor   int
	width    int
	height   int
	msg      string
}

type linksDoneMsg struct{}
type linkSelectedMsg struct {
	label string
	url   string
}
type globalIPFetchedMsg struct{ ip string }

func newLinksMenuModel(port int, runtimes map[string]*tunnelRuntime) linksMenuModel {
	return linksMenuModel{
		port:     port,
		runtimes: runtimes,
		localIP:  detectLocalIP(),
	}
}

func detectLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ip4 := ipnet.IP.To4(); ip4 != nil {
				return ip4.String()
			}
		}
	}
	return "127.0.0.1"
}

func cmdFetchGlobalIP() tea.Cmd {
	return func() tea.Msg {
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Get("https://api.ipify.org")
		if err != nil {
			return globalIPFetchedMsg{ip: ""}
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return globalIPFetchedMsg{ip: ""}
		}
		return globalIPFetchedMsg{ip: strings.TrimSpace(string(body))}
	}
}

func (m linksMenuModel) items() []linkItem {
	var items []linkItem

	// Local IP — always present
	items = append(items, linkItem{
		label: "Local Network",
		url:   fmt.Sprintf("http://%s:%d", m.localIP, m.port),
	})

	// Global IP
	switch m.globalIP {
	case "":
		items = append(items, linkItem{label: "Global IP  (fetching...)", url: ""})
	case "error":
		items = append(items, linkItem{label: "Global IP  (unavailable)", url: ""})
	default:
		items = append(items, linkItem{
			label: "Global IP",
			url:   fmt.Sprintf("http://%s:%d", m.globalIP, m.port),
		})
	}

	// Running tunnels with detected URLs — sorted for stable order
	if m.runtimes != nil {
		names := make([]string, 0, len(m.runtimes))
		for name := range m.runtimes {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			rt := m.runtimes[name]
			if rt.status == tunnelRunning && rt.detectedURL != "" {
				items = append(items, linkItem{label: name, url: rt.detectedURL})
			}
		}
	}

	items = append(items, linkItem{label: "← Back", url: ""})
	return items
}

func (m linksMenuModel) Update(msg tea.Msg) (linksMenuModel, tea.Cmd) {
	items := m.items()
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			return m, func() tea.Msg { return linksDoneMsg{} }
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

func (m linksMenuModel) handleSelect() (linksMenuModel, tea.Cmd) {
	items := m.items()
	if m.cursor >= len(items) {
		return m, nil
	}
	item := items[m.cursor]

	if item.label == "← Back" {
		return m, func() tea.Msg { return linksDoneMsg{} }
	}

	if item.url == "" {
		// Not yet available (fetching or error)
		return m, nil
	}

	label, url := item.label, item.url
	return m, func() tea.Msg { return linkSelectedMsg{label: label, url: url} }
}

func (m linksMenuModel) View() string {
	var b strings.Builder

	b.WriteString(styleTitle.Render("  Links") + "\n")
	divider := styleDivider.Render(strings.Repeat("─", max(40, m.width-4)))
	b.WriteString(divider + "\n\n")

	items := m.items()
	for i, item := range items {
		prefix := "   "
		if i == m.cursor {
			prefix = " ▶ "
		}

		isBack := item.label == "← Back"
		isUnavailable := item.url == "" && !isBack

		var line string
		switch {
		case isUnavailable:
			line = styleDisabled.Render(prefix + item.label)
		case i == m.cursor:
			line = styleSelected.Render(prefix+item.label) + styleSubtle.Render("  "+item.url)
		default:
			line = styleNormal.Render(prefix+item.label) + styleSubtle.Render("  "+item.url)
		}
		b.WriteString(line + "\n")
	}

	if m.msg != "" {
		b.WriteString("\n" + styleAccent.Render("  "+m.msg) + "\n")
	}

	b.WriteString("\n" + divider + "\n")
	b.WriteString("\n" + styleHelp.Render("  ↑↓ navigate  enter select  esc back") + "\n")

	return b.String()
}
