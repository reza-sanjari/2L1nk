package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"2L1nk/internal/config"
	"2L1nk/internal/db"
	"2L1nk/internal/gate"
	infradb "2L1nk/internal/infrastructure/db"

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

	// Connect CLI to the same DB so gate key changes are visible to the server.
	// Non-fatal: gate stays in-memory mode if DB is not yet available.
	if database, dbErr := db.Open(cfg.DBPath); dbErr == nil {
		repo := infradb.NewGateRepository(database)
		_ = g.SetRepo(repo)
	}

	pidPath := derivePathWithExt(cfg.DBPath, ".pid")
	logPath := derivePathWithExt(cfg.DBPath, ".log")
	optsPath := derivePathWithExt(cfg.DBPath, ".opts")
	tunnelsPath := tunnelsConfigPath(cfg.DBPath)

	opts := loadOptions(optsPath)
	tunnelsCfg := loadTunnels(tunnelsPath)
	pid, running := checkRunningServer(pidPath)

	m := newModel(g, cfg, pidPath, logPath, optsPath, tunnelsPath, opts, tunnelsCfg, pid, running)
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
