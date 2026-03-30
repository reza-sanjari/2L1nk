package cli

import (
	"fmt"
	"os"
	"strings"
	"time"

	"2L1nk/internal/utils"

	tea "github.com/charmbracelet/bubbletea"
)

type nukeModel struct {
	confirmed bool
	working   bool
	done      bool
	err       error

	dbPath   string
	logPath  string
	pidPath  string
	optsPath string

	serverPID int
	srvState  serverState

	width  int
	height int
}

type nukeDoneMsg struct{ err error }
type nukeCancelMsg struct{}

func newNukeModel(dbPath, logPath, pidPath, optsPath string, serverPID int, srvState serverState) nukeModel {
	return nukeModel{
		dbPath:    dbPath,
		logPath:   logPath,
		pidPath:   pidPath,
		optsPath:  optsPath,
		serverPID: serverPID,
		srvState:  srvState,
	}
}

func (m nukeModel) Update(msg tea.Msg) (nukeModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.working || m.done {
			return m, nil
		}
		switch msg.String() {
		case "esc", "q":
			return m, func() tea.Msg { return nukeCancelMsg{} }
		case "enter":
			m.working = true
			return m, m.cmdNuke()
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case nukeDoneMsg:
		m.working = false
		m.done = true
		m.err = msg.err
	}
	return m, nil
}

func (m nukeModel) cmdNuke() tea.Cmd {
	pid := m.serverPID
	srvRunning := m.srvState == stateRunning
	dbPath := m.dbPath
	logPath := m.logPath
	pidPath := m.pidPath
	optsPath := m.optsPath

	return func() tea.Msg {
		// Stop server first
		if srvRunning {
			if process, err := os.FindProcess(pid); err == nil {
				_ = process.Kill()
			}
			time.Sleep(200 * time.Millisecond)
		}

		var errs []string
		for _, path := range []string{
			dbPath, dbPath + "-shm", dbPath + "-wal",
			logPath, pidPath, optsPath,
		} {
			if err := utils.SecureDelete(path); err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", path, err))
			}
		}

		if len(errs) > 0 {
			return nukeDoneMsg{err: fmt.Errorf(strings.Join(errs, "; "))}
		}
		return nukeDoneMsg{}
	}
}

func (m nukeModel) View() string {
	var b strings.Builder

	b.WriteString(styleDanger.Render("  ☢  NUKE — THIS IS IRREVERSIBLE") + "\n")
	divider := styleDivider.Render(strings.Repeat("─", max(50, m.width-4)))
	b.WriteString(divider + "\n\n")

	if m.working {
		b.WriteString(styleAccent.Render("  Securely wiping all data...") + "\n")
		return b.String()
	}

	if m.done {
		if m.err != nil {
			b.WriteString(styleDanger.Render("  ✗ Errors during wipe:") + "\n")
			b.WriteString(styleSubtle.Render("  "+m.err.Error()) + "\n")
		} else {
			b.WriteString(styleSuccess.Render("  ✓ All data wiped.") + "\n")
		}
		b.WriteString("\n" + styleHelp.Render("  Exiting...") + "\n")
		return b.String()
	}

	b.WriteString(styleNormal.Render("  Will securely wipe (3-pass overwrite + delete):") + "\n\n")

	files := []struct{ path, desc string }{
		{m.dbPath, "database"},
		{m.dbPath + "-shm", "database shared memory"},
		{m.dbPath + "-wal", "database write-ahead log"},
		{m.logPath, "server logs"},
		{m.pidPath, "process file"},
		{m.optsPath, "options"},
	}
	for _, f := range files {
		b.WriteString(styleSubtle.Render(fmt.Sprintf("    • %-20s %s", f.path, f.desc)) + "\n")
	}

	b.WriteString("\n" + styleSubtle.Render("  Data is overwritten with random bytes before deletion.") + "\n")
	b.WriteString(styleSubtle.Render("  Best-effort on SSDs (wear leveling may retain copies).") + "\n")

	b.WriteString("\n" + divider + "\n\n")
	b.WriteString(styleNormal.Render("  ") + styleAccent.Render("[ ENTER to confirm ]") + styleNormal.Render("   ") + styleDisabled.Render("[ ESC to cancel ]") + "\n")

	return b.String()
}
