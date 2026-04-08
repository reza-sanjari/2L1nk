package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"2L1nk/internal/utils"

	tea "github.com/charmbracelet/bubbletea"
)

type nukeModel struct {
	confirmed bool
	working   bool
	done      bool
	err       error

	dbPath      string
	logPath     string
	pidPath     string
	optsPath    string
	tunnelsPath string

	serverPID int
	srvState  serverState

	closeDB func() error // closes CLI's own DB connection before deletion

	width  int
	height int
}

type nukeDoneMsg struct{ err error }
type nukeCancelMsg struct{}

func newNukeModel(dbPath, logPath, pidPath, optsPath, tunnelsPath string, serverPID int, srvState serverState, closeDB func() error) nukeModel {
	return nukeModel{
		dbPath:      dbPath,
		logPath:     logPath,
		pidPath:     pidPath,
		optsPath:    optsPath,
		tunnelsPath: tunnelsPath,
		serverPID:   serverPID,
		srvState:    srvState,
		closeDB:     closeDB,
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
	tunnelsPath := m.tunnelsPath
	closeDB := m.closeDB

	return func() tea.Msg {
		// 1. Kill server and wait for the OS to fully release its file handles.
		//    On Windows this uses WaitForSingleObject (not a poll loop).
		//    On Unix it is a no-op — SIGKILL releases handles immediately.
		if srvRunning {
			if process, err := os.FindProcess(pid); err == nil {
				_ = process.Kill()
			}
			waitForProcessExit(pid)
		}

		// 2. Close the CLI's own DB connection, releasing the final file lock.
		_ = closeDB()

		var errs []string

		// 3. Delete SQLite files with os.Remove, not SecureDelete.
		//    SecureDelete overwrites content before deleting; if the delete then
		//    fails (unexpected lock) it leaves a corrupted file. os.Remove either
		//    succeeds or fails cleanly. DB content is E2E-encrypted so byte-wiping
		//    provides no additional security over deletion.
		for _, path := range []string{dbPath + "-wal", dbPath + "-shm", dbPath} {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				errs = append(errs, fmt.Sprintf("%s: %v", path, err))
			}
		}

		// 4. SecureDelete non-DB files (logs may contain plaintext server data).
		for _, path := range []string{logPath, pidPath, optsPath, tunnelsPath} {
			if err := utils.SecureDelete(path); err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", path, err))
			}
		}

		// 5. SecureDelete all tunnel log files (*.tunnel.log in the DB directory).
		tunnelLogs, _ := filepath.Glob(filepath.Join(filepath.Dir(dbPath), "*.tunnel.log"))
		for _, path := range tunnelLogs {
			if err := utils.SecureDelete(path); err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", path, err))
			}
		}

		if len(errs) > 0 {
			return nukeDoneMsg{err: fmt.Errorf("%s", strings.Join(errs, "; "))}
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
		{m.tunnelsPath, "tunnel config"},
	}
	for _, f := range files {
		b.WriteString(styleSubtle.Render(fmt.Sprintf("    • %-20s %s", f.path, f.desc)) + "\n")
	}

	b.WriteString(styleSubtle.Render("    • *.tunnel.log             tunnel logs (all providers)") + "\n")
	b.WriteString("\n" + styleSubtle.Render("  Data is overwritten with random bytes before deletion.") + "\n")
	b.WriteString(styleSubtle.Render("  Best-effort on SSDs (wear leveling may retain copies).") + "\n")

	b.WriteString("\n" + divider + "\n\n")
	b.WriteString(styleNormal.Render("  ") + styleAccent.Render("[ ENTER to confirm ]") + styleNormal.Render("   ") + styleDisabled.Render("[ ESC to cancel ]") + "\n")

	return b.String()
}
