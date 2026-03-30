package cli

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"time"

	"2L1nk/internal/config"
	"2L1nk/internal/utils"
	"2L1nk/internal/gate"

	tea "github.com/charmbracelet/bubbletea"
)

type serverState int

const (
	stateStopped  serverState = iota
	stateRunning
	stateStopping
)

type screen int

const (
	screenMenu screen = iota
	screenGate
	screenLogs
	screenOptions
	screenNuke
)

type model struct {
	screen      screen
	menu        menuModel
	gateMenu    gateMenuModel
	logs        logsModel
	optionsMenu optionsModel
	nukeScreen  nukeModel

	g         *gate.Gate
	cfg       *config.Config
	opts      Options
	optsPath  string
	pidPath   string
	logPath   string
	serverPID int
	srvState  serverState

	width  int
	height int
}

type serverStartedMsg struct{ port, pid int }
type serverStoppedMsg struct{ err error }
type dbResetDoneMsg struct{ err error }

func newModel(g *gate.Gate, cfg *config.Config, pidPath, logPath, optsPath string, opts Options, pid int, running bool) model {
	srvState := stateStopped
	if running {
		srvState = stateRunning
	}
	menu := newMenuModel()
	menu.serverState = srvState
	menu.port = cfg.Port
	menu.noLogs = opts.NoLogs

	return model{
		screen:    screenMenu,
		menu:      menu,
		gateMenu:  newGateMenuModel(g),
		g:         g,
		cfg:       cfg,
		opts:      opts,
		optsPath:  optsPath,
		pidPath:   pidPath,
		logPath:   logPath,
		serverPID: pid,
		srvState:  srvState,
	}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.menu.width = msg.Width
		m.menu.height = msg.Height
		m.gateMenu.width = msg.Width
		m.gateMenu.height = msg.Height
		m.optionsMenu.width = msg.Width
		m.optionsMenu.height = msg.Height
		if m.screen == screenLogs {
			updated, _ := m.logs.Update(msg)
			m.logs = updated
		}
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		switch m.screen {
		case screenMenu:
			return m.handleMenuKey(msg)
		case screenGate:
			updated, cmd := m.gateMenu.Update(msg)
			m.gateMenu = updated
			return m, cmd
		case screenLogs:
			updated, cmd := m.logs.Update(msg)
			m.logs = updated
			return m, cmd
		case screenOptions:
			updated, cmd := m.optionsMenu.Update(msg)
			m.optionsMenu = updated
			return m, cmd
		case screenNuke:
			updated, cmd := m.nukeScreen.Update(msg)
			m.nukeScreen = updated
			return m, cmd
		}

	case serverStartedMsg:
		m.srvState = stateRunning
		m.menu.serverState = stateRunning
		m.menu.port = msg.port
		m.serverPID = msg.pid
		return m, nil

	case serverStoppedMsg:
		m.srvState = stateStopped
		m.menu.serverState = stateStopped
		m.serverPID = 0
		if msg.err != nil {
			m.menu.flashMsg = "Error: " + msg.err.Error()
		}
		return m, nil

	case dbResetDoneMsg:
		m.srvState = stateStopped
		m.menu.serverState = stateStopped
		m.serverPID = 0
		if msg.err != nil {
			m.menu.flashMsg = "Reset failed: " + msg.err.Error()
		} else {
			m.menu.flashMsg = "Database reset successfully."
		}
		return m, nil

	case gateDoneMsg:
		m.screen = screenMenu
		return m, nil

	case logsDoneMsg:
		m.screen = screenMenu
		return m, nil

	case optionsDoneMsg:
		m.opts = msg.opts
		m.menu.noLogs = m.opts.NoLogs
		m.screen = screenMenu
		return m, nil

	case nukeDoneMsg:
		updated, _ := m.nukeScreen.Update(msg)
		m.nukeScreen = updated
		// Show result briefly, then quit
		return m, tea.Tick(1500*time.Millisecond, func(_ time.Time) tea.Msg {
			return tea.QuitMsg{}
		})

	case nukeCancelMsg:
		m.screen = screenMenu
		return m, nil

	case flashMsg:
		updated, cmd := m.menu.Update(msg)
		m.menu = updated
		return m, cmd

	case tickMsg:
		if m.screen == screenLogs {
			updated, cmd := m.logs.Update(msg)
			m.logs = updated
			return m, cmd
		}
		return m, nil
	}

	// Propagate unhandled messages (e.g. textinput updates)
	switch m.screen {
	case screenGate:
		updated, cmd := m.gateMenu.Update(msg)
		m.gateMenu = updated
		return m, cmd
	case screenLogs:
		updated, cmd := m.logs.Update(msg)
		m.logs = updated
		return m, cmd
	case screenOptions:
		updated, cmd := m.optionsMenu.Update(msg)
		m.optionsMenu = updated
		return m, cmd
	case screenNuke:
		updated, cmd := m.nukeScreen.Update(msg)
		m.nukeScreen = updated
		return m, cmd
	}

	return m, nil
}

func (m model) handleMenuKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "q" {
		return m, tea.Quit
	}

	if msg.String() == "enter" {
		id := m.menu.selectedItem()
		switch id {
		case "run":
			if m.srvState == stateRunning {
				return m, cmdFlash("Server is already running.")
			}
			return m, m.cmdStartServer()

		case "stop":
			if m.srvState != stateRunning {
				return m, cmdFlash("Server is not running.")
			}
			m.srvState = stateStopping
			m.menu.serverState = stateStopping
			return m, m.cmdStopServer()

		case "gate":
			m.gateMenu = newGateMenuModel(m.g)
			m.screen = screenGate
			return m, nil

		case "logs":
			if m.opts.NoLogs {
				return m, cmdFlash("Logging is disabled. Enable in Options.")
			}
			m.logs = newLogsModel(m.logPath, m.width, m.height)
			m.screen = screenLogs
			return m, tickCmd()

		case "reset":
			return m, m.cmdResetDB()

		case "options":
			m.optionsMenu = newOptionsModel(m.opts, m.optsPath)
			m.screen = screenOptions
			return m, nil

		case "nuke":
			m.nukeScreen = newNukeModel(m.cfg.DBPath, m.logPath, m.pidPath, m.optsPath, m.serverPID, m.srvState)
			m.screen = screenNuke
			return m, nil
		}
		return m, nil
	}

	updated, cmd := m.menu.Update(msg)
	m.menu = updated
	return m, cmd
}

func (m model) View() string {
	switch m.screen {
	case screenGate:
		return styleBorder.Width(m.width - 4).Render(m.gateMenu.View())
	case screenLogs:
		return m.logs.View()
	case screenOptions:
		return styleBorder.Width(m.width - 4).Render(m.optionsMenu.View())
	case screenNuke:
		return styleBorder.Width(m.width - 4).Render(m.nukeScreen.View())
	default:
		return styleBorder.Width(m.width - 4).Render(m.menu.View())
	}
}

// cmdStartServer spawns a detached subprocess with --server flag.
func (m *model) cmdStartServer() tea.Cmd {
	pidPath := m.pidPath
	logPath := m.logPath
	port := m.cfg.Port
	noLogs := m.opts.NoLogs
	gateKey := m.g.Key()

	return func() tea.Msg {
		exe, err := os.Executable()
		if err != nil {
			return serverStoppedMsg{err: err}
		}

		cmd := exec.Command(exe, "--server")

		if noLogs {
			cmd.Stdout = io.Discard
			cmd.Stderr = io.Discard
			cmd.Env = append(os.Environ(), "_2L1NK_NO_LOGS=1", "_2L1NK_GATE_KEY="+gateKey)
		} else {
			logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				return serverStoppedMsg{err: err}
			}
			cmd.Stdout = logFile
			cmd.Stderr = logFile
			cmd.Env = append(os.Environ(), "_2L1NK_GATE_KEY="+gateKey)
			defer logFile.Close()
		}

		detachProcess(cmd)

		if err := cmd.Start(); err != nil {
			return serverStoppedMsg{err: err}
		}

		// Wait for subprocess to write its PID file (up to 2s).
		for i := 0; i < 20; i++ {
			time.Sleep(100 * time.Millisecond)
			pid, alive := checkRunningServer(pidPath)
			if alive {
				return serverStartedMsg{port: port, pid: pid}
			}
		}
		return serverStoppedMsg{err: errors.New("server did not start within 2s")}
	}
}

// cmdStopServer kills the server subprocess by PID.
// If TempServer is enabled, securely wipes DB + log after stopping.
func (m *model) cmdStopServer() tea.Cmd {
	pid := m.serverPID
	pidPath := m.pidPath
	tempServer := m.opts.TempServer
	dbPath := m.cfg.DBPath
	logPath := m.logPath

	return func() tea.Msg {
		process, err := os.FindProcess(pid)
		if err == nil {
			_ = process.Kill()
		}
		_ = os.Remove(pidPath)

		if tempServer {
			time.Sleep(150 * time.Millisecond)
			_ = utils.SecureDelete(dbPath)
			_ = utils.SecureDelete(dbPath + "-shm")
			_ = utils.SecureDelete(dbPath + "-wal")
			_ = utils.SecureDelete(logPath)
		}

		return serverStoppedMsg{}
	}
}

// cmdResetDB stops the server (if running), deletes the DB file, and re-runs migrations.
func (m *model) cmdResetDB() tea.Cmd {
	pid := m.serverPID
	pidPath := m.pidPath
	dbPath := m.cfg.DBPath
	srvRunning := m.srvState == stateRunning

	if srvRunning {
		m.srvState = stateStopping
		m.menu.serverState = stateStopping
	}

	return func() tea.Msg {
		if srvRunning {
			if process, err := os.FindProcess(pid); err == nil {
				_ = process.Kill()
			}
			_ = os.Remove(pidPath)
			time.Sleep(200 * time.Millisecond)
		}

		if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
			return dbResetDoneMsg{err: err}
		}

		return dbResetDoneMsg{err: resetDatabase(dbPath)}
	}
}
