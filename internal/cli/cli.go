package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"2L1nk/internal/config"
	"2L1nk/internal/gate"

	tea "github.com/charmbracelet/bubbletea"
)

// RunTUI launches the interactive terminal UI.
// It returns when the user quits (server subprocess keeps running unless stopped).
func RunTUI() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	g, err := gate.New(0)
	if err != nil {
		return err
	}

	pidPath := derivePathWithExt(cfg.DBPath, ".pid")
	logPath := derivePathWithExt(cfg.DBPath, ".log")
	optsPath := derivePathWithExt(cfg.DBPath, ".opts")

	opts := loadOptions(optsPath)
	pid, running := checkRunningServer(pidPath)

	m := newModel(g, cfg, pidPath, logPath, optsPath, opts, pid, running)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err = p.Run()
	return err
}

// checkRunningServer reads the PID file and checks if the process is alive.
func checkRunningServer(pidPath string) (pid int, alive bool) {
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return 0, false
	}
	pid, err = strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, false
	}
	return pid, isProcessAlive(pid)
}

// derivePathWithExt replaces the extension of dbPath with ext.
// e.g. "2L1nk.db" + ".pid" → "2L1nk.pid"
func derivePathWithExt(dbPath, ext string) string {
	return strings.TrimSuffix(dbPath, filepath.Ext(dbPath)) + ext
}
