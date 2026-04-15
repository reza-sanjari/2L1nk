package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"2L1nk/internal/config"
	"2L1nk/internal/gate"
	"2L1nk/internal/utils"

	tea "github.com/charmbracelet/bubbletea"
)

type serverState int

const (
	stateStopped serverState = iota
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
	screenTunnels
	screenTunnelDetail
	screenTunnelAdd
	screenTunnelLog
	screenLinks
	screenLinkDetail
)

type tunnelStatus int

const (
	tunnelStopped tunnelStatus = iota
	tunnelStarting
	tunnelRunning
	tunnelStopping
)

type tunnelRuntime struct {
	pid         int
	status      tunnelStatus
	logPath     string
	pidPath     string
	detectedURL string
}

type model struct {
	screen      screen
	menu        menuModel
	gateMenu    gateMenuModel
	logs        logsModel
	optionsMenu optionsModel
	nukeScreen  nukeModel

	// Tunnel screens
	tunnelMenu   tunnelMenuModel
	tunnelDetail tunnelDetailModel
	tunnelAdd    tunnelAddModel
	tunnelLog    tunnelLogModel

	// Tunnel state
	tunnelRuntimes map[string]*tunnelRuntime
	tunnelsConfig  TunnelsConfig
	tunnelsPath    string

	// Links screens
	linksMenu  linksMenuModel
	linkDetail linkDetailModel
	globalIP   string // cached public IP; "" = not yet fetched, "error" = unavailable

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
type tunnelStartedMsg struct {
	name string
	pid  int
}
type tunnelStoppedMsg struct {
	name string
	err  error
}

func newModel(g *gate.Gate, cfg *config.Config, pidPath, logPath, optsPath, tunnelsPath string, opts Options, tunnelsCfg TunnelsConfig, pid int, running bool) model {
	srvState := stateStopped
	if running {
		srvState = stateRunning
	}
	menu := newMenuModel()
	menu.serverState = srvState
	menu.port = cfg.Port
	menu.noLogs = opts.NoLogs

	runtimes := checkRunningTunnels(tunnelsCfg, cfg.DBPath)

	return model{
		screen:         screenMenu,
		menu:           menu,
		gateMenu:       newGateMenuModel(g),
		tunnelRuntimes: runtimes,
		tunnelsConfig:  tunnelsCfg,
		tunnelsPath:    tunnelsPath,
		g:              g,
		cfg:            cfg,
		opts:           opts,
		optsPath:       optsPath,
		pidPath:        pidPath,
		logPath:        logPath,
		serverPID:      pid,
		srvState:       srvState,
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
		m.tunnelMenu.width = msg.Width
		m.tunnelMenu.height = msg.Height
		m.tunnelDetail.width = msg.Width
		m.tunnelDetail.height = msg.Height
		m.tunnelAdd.width = msg.Width
		m.tunnelAdd.height = msg.Height
		m.linksMenu.width = msg.Width
		m.linksMenu.height = msg.Height
		m.linkDetail.width = msg.Width
		m.linkDetail.height = msg.Height
		if m.screen == screenLogs {
			updated, _ := m.logs.Update(msg)
			m.logs = updated
		}
		if m.screen == screenTunnelLog {
			updated, _ := m.tunnelLog.Update(msg)
			m.tunnelLog = updated
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
		case screenTunnels:
			updated, cmd := m.tunnelMenu.Update(msg)
			m.tunnelMenu = updated
			return m, cmd
		case screenTunnelDetail:
			updated, cmd := m.tunnelDetail.Update(msg)
			m.tunnelDetail = updated
			return m, cmd
		case screenTunnelAdd:
			updated, cmd := m.tunnelAdd.Update(msg)
			m.tunnelAdd = updated
			return m, cmd
		case screenTunnelLog:
			updated, cmd := m.tunnelLog.Update(msg)
			m.tunnelLog = updated
			return m, cmd
		case screenLinks:
			updated, cmd := m.linksMenu.Update(msg)
			m.linksMenu = updated
			return m, cmd
		case screenLinkDetail:
			updated, cmd := m.linkDetail.Update(msg)
			m.linkDetail = updated
			return m, cmd
		}

	case serverStartedMsg:
		m.srvState = stateRunning
		m.menu.serverState = stateRunning
		m.menu.port = msg.port
		m.serverPID = msg.pid
		// Auto-start tunnels marked with auto_start
		var cmds []tea.Cmd
		for i := range m.tunnelsConfig.Tunnels {
			t := m.tunnelsConfig.Tunnels[i]
			if t.AutoStart {
				rt := m.getOrCreateRuntime(t)
				if rt.status == tunnelStopped {
					rt.status = tunnelStarting
					cmds = append(cmds, m.cmdStartTunnel(t, rt))
				}
			}
		}
		return m, tea.Batch(cmds...)

	case serverStoppedMsg:
		m.srvState = stateStopped
		m.menu.serverState = stateStopped
		m.serverPID = 0
		if msg.err != nil {
			m.menu.flashMsg = "Error: " + msg.err.Error()
		}
		return m, nil

	case tunnelStartedMsg:
		if rt, ok := m.tunnelRuntimes[msg.name]; ok {
			rt.status = tunnelRunning
			rt.pid = msg.pid
		}
		m.tunnelMenu = newTunnelMenuModel(m.tunnelsConfig.Tunnels, m.tunnelRuntimes)
		m.tunnelMenu.width = m.width
		m.tunnelMenu.height = m.height
		if m.screen == screenTunnelDetail && m.tunnelDetail.entry.Name == msg.name {
			m.tunnelDetail.runtime = m.tunnelRuntimes[msg.name]
		}
		return m, nil

	case tunnelStoppedMsg:
		if rt, ok := m.tunnelRuntimes[msg.name]; ok {
			rt.status = tunnelStopped
			rt.pid = 0
			rt.detectedURL = ""
		}
		m.tunnelMenu = newTunnelMenuModel(m.tunnelsConfig.Tunnels, m.tunnelRuntimes)
		m.tunnelMenu.width = m.width
		m.tunnelMenu.height = m.height
		if m.screen == screenTunnelDetail && m.tunnelDetail.entry.Name == msg.name {
			m.tunnelDetail.runtime = m.tunnelRuntimes[msg.name]
			if msg.err != nil {
				m.tunnelDetail.msg = "Error: " + msg.err.Error()
				m.tunnelDetail.msgErr = true
			}
		}
		return m, nil

	case tunnelURLDetectedMsg:
		if rt, ok := m.tunnelRuntimes[msg.name]; ok {
			rt.detectedURL = msg.url
		}
		if m.screen == screenTunnelDetail && m.tunnelDetail.entry.Name == msg.name {
			m.tunnelDetail.runtime = m.tunnelRuntimes[msg.name]
		}
		return m, nil

	case globalIPFetchedMsg:
		if msg.ip == "" {
			m.globalIP = "error"
		} else {
			m.globalIP = msg.ip
		}
		m.linksMenu.globalIP = m.globalIP
		return m, nil

	case linksDoneMsg:
		m.screen = screenMenu
		return m, nil

	case linkSelectedMsg:
		m.linkDetail = newLinkDetailModel(msg.label, msg.url)
		m.linkDetail.width = m.width
		m.linkDetail.height = m.height
		m.screen = screenLinkDetail
		return m, nil

	case linkDetailDoneMsg:
		m.screen = screenLinks
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

	case tunnelsDoneMsg:
		m.screen = screenMenu
		return m, nil

	case tunnelSelectedMsg:
		if msg.index >= 0 && msg.index < len(m.tunnelsConfig.Tunnels) {
			entry := m.tunnelsConfig.Tunnels[msg.index]
			rt := m.getOrCreateRuntime(entry)
			m.tunnelDetail = newTunnelDetailModel(entry, rt)
			m.tunnelDetail.width = m.width
			m.tunnelDetail.height = m.height
			m.screen = screenTunnelDetail
		}
		return m, nil

	case tunnelAddRequestMsg:
		m.tunnelAdd = newTunnelAddModel()
		m.tunnelAdd.width = m.width
		m.tunnelAdd.height = m.height
		m.screen = screenTunnelAdd
		return m, nil

	case tunnelAddDoneMsg:
		// Check for duplicate name
		for _, t := range m.tunnelsConfig.Tunnels {
			if t.Name == msg.entry.Name {
				m.tunnelMenu.msg = fmt.Sprintf("Tunnel \"%s\" already exists.", msg.entry.Name)
				m.screen = screenTunnels
				return m, nil
			}
		}
		m.tunnelsConfig.Tunnels = append(m.tunnelsConfig.Tunnels, msg.entry)
		_ = saveTunnels(m.tunnelsPath, m.tunnelsConfig)
		m.tunnelMenu = newTunnelMenuModel(m.tunnelsConfig.Tunnels, m.tunnelRuntimes)
		m.tunnelMenu.width = m.width
		m.tunnelMenu.height = m.height
		m.tunnelMenu.cursor = len(m.tunnelsConfig.Tunnels) - 1
		m.screen = screenTunnels
		return m, nil

	case tunnelAddCancelMsg:
		m.screen = screenTunnels
		return m, nil

	case tunnelDetailDoneMsg:
		m.tunnelMenu = newTunnelMenuModel(m.tunnelsConfig.Tunnels, m.tunnelRuntimes)
		m.tunnelMenu.width = m.width
		m.tunnelMenu.height = m.height
		m.screen = screenTunnels
		return m, nil

	case tunnelDetailStartMsg:
		if rt, ok := m.tunnelRuntimes[msg.name]; ok && rt.status == tunnelStopped {
			rt.status = tunnelStarting
			entry := m.findTunnel(msg.name)
			if entry != nil {
				return m, m.cmdStartTunnel(*entry, rt)
			}
		}
		return m, nil

	case tunnelDetailStopMsg:
		if rt, ok := m.tunnelRuntimes[msg.name]; ok && rt.status == tunnelRunning {
			rt.status = tunnelStopping
			return m, cmdStopTunnel(msg.name, rt)
		}
		return m, nil

	case tunnelDetailDeleteMsg:
		m.deleteTunnel(msg.name)
		m.tunnelMenu = newTunnelMenuModel(m.tunnelsConfig.Tunnels, m.tunnelRuntimes)
		m.tunnelMenu.width = m.width
		m.tunnelMenu.height = m.height
		m.screen = screenTunnels
		return m, nil

	case tunnelDetailLogsMsg:
		rt := m.tunnelRuntimes[msg.name]
		logPath := ""
		urlPattern := ""
		if rt != nil {
			logPath = rt.logPath
		}
		if entry := m.findTunnel(msg.name); entry != nil {
			if logPath == "" {
				logPath = tunnelLogPath(m.cfg.DBPath, entry.Name)
			}
			urlPattern = entry.URLPattern
			if urlPattern == "" {
				for _, p := range tunnelPresets {
					if p.Name == entry.Name {
						urlPattern = p.URLPattern
						break
					}
				}
			}
		}
		m.tunnelLog = newTunnelLogModel(msg.name, logPath, urlPattern, m.width, m.height)
		m.screen = screenTunnelLog
		return m, tickCmd()

	case tunnelLogDoneMsg:
		m.screen = screenTunnelDetail
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
		if m.screen == screenTunnelLog {
			updated, cmd := m.tunnelLog.Update(msg)
			m.tunnelLog = updated
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
	case screenTunnels:
		updated, cmd := m.tunnelMenu.Update(msg)
		m.tunnelMenu = updated
		return m, cmd
	case screenTunnelDetail:
		updated, cmd := m.tunnelDetail.Update(msg)
		m.tunnelDetail = updated
		return m, cmd
	case screenTunnelAdd:
		updated, cmd := m.tunnelAdd.Update(msg)
		m.tunnelAdd = updated
		return m, cmd
	case screenTunnelLog:
		updated, cmd := m.tunnelLog.Update(msg)
		m.tunnelLog = updated
		return m, cmd
	case screenLinks:
		updated, cmd := m.linksMenu.Update(msg)
		m.linksMenu = updated
		return m, cmd
	case screenLinkDetail:
		updated, cmd := m.linkDetail.Update(msg)
		m.linkDetail = updated
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
			_ = m.g.RefreshFromDB()
			gm := newGateMenuModel(m.g)
			gm.width = m.width
			gm.height = m.height
			m.gateMenu = gm
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

		case "tunnels":
			m.tunnelMenu = newTunnelMenuModel(m.tunnelsConfig.Tunnels, m.tunnelRuntimes)
			m.tunnelMenu.width = m.width
			m.tunnelMenu.height = m.height
			m.screen = screenTunnels
			return m, nil

		case "links":
			lm := newLinksMenuModel(m.cfg.Port, m.tunnelRuntimes)
			lm.globalIP = m.globalIP
			lm.width = m.width
			lm.height = m.height
			m.linksMenu = lm
			m.screen = screenLinks
			var cmd tea.Cmd
			if m.globalIP == "" {
				cmd = cmdFetchGlobalIP()
			}
			return m, cmd

		case "nuke":
			m.nukeScreen = newNukeModel(m.cfg.DBPath, m.logPath, m.pidPath, m.optsPath, m.tunnelsPath, m.serverPID, m.srvState, m.g.Close)
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
		if m.gateMenu.viewingHistory {
			return m.gateMenu.View()
		}
		return styleBorder.Width(m.width - 4).Render(m.gateMenu.View())
	case screenLogs:
		return m.logs.View()
	case screenOptions:
		return styleBorder.Width(m.width - 4).Render(m.optionsMenu.View())
	case screenNuke:
		return styleBorder.Width(m.width - 4).Render(m.nukeScreen.View())
	case screenTunnels:
		return styleBorder.Width(m.width - 4).Render(m.tunnelMenu.View())
	case screenTunnelDetail:
		return styleBorder.Width(m.width - 4).Render(m.tunnelDetail.View())
	case screenTunnelAdd:
		return styleBorder.Width(m.width - 4).Render(m.tunnelAdd.View())
	case screenTunnelLog:
		return m.tunnelLog.View()
	case screenLinks:
		return styleBorder.Width(m.width - 4).Render(m.linksMenu.View())
	case screenLinkDetail:
		return styleBorder.Width(m.width - 4).Render(m.linkDetail.View())
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

	return func() tea.Msg {
		exe, err := os.Executable()
		if err != nil {
			return serverStoppedMsg{err: err}
		}

		cmd := exec.Command(exe, "--server")

		if noLogs {
			cmd.Stdout = io.Discard
			cmd.Stderr = io.Discard
			cmd.Env = append(os.Environ(), "_2L1NK_NO_LOGS=1", "_2L1NK_SUBPROCESS=1")
		} else {
			logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				return serverStoppedMsg{err: err}
			}
			cmd.Stdout = logFile
			cmd.Stderr = logFile
			cmd.Env = append(os.Environ(), "_2L1NK_SUBPROCESS=1")
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
	tunnelsPath := m.tunnelsPath
	closeDB := m.g.Close

	return func() tea.Msg {
		killRunningTunnels(tunnelsPath, dbPath)

		process, err := os.FindProcess(pid)
		if err == nil {
			_ = process.Kill()
		}
		_ = os.Remove(pidPath)

		if tempServer {
			waitForProcessExit(pid)
			_ = closeDB()
			_ = os.Remove(dbPath + "-wal")
			_ = os.Remove(dbPath + "-shm")
			_ = os.Remove(dbPath)
			_ = utils.SecureDelete(logPath)
		}

		return serverStoppedMsg{}
	}
}

// getOrCreateRuntime returns the existing runtime for a tunnel or creates a new stopped one.
func (m *model) getOrCreateRuntime(entry TunnelEntry) *tunnelRuntime {
	if m.tunnelRuntimes == nil {
		m.tunnelRuntimes = make(map[string]*tunnelRuntime)
	}
	if rt, ok := m.tunnelRuntimes[entry.Name]; ok {
		return rt
	}
	rt := &tunnelRuntime{
		status:  tunnelStopped,
		logPath: tunnelLogPath(m.cfg.DBPath, entry.Name),
		pidPath: tunnelPIDPath(m.cfg.DBPath, entry.Name),
	}
	m.tunnelRuntimes[entry.Name] = rt
	return rt
}

// findTunnel returns a pointer to the TunnelEntry with the given name, or nil.
func (m *model) findTunnel(name string) *TunnelEntry {
	for i := range m.tunnelsConfig.Tunnels {
		if m.tunnelsConfig.Tunnels[i].Name == name {
			return &m.tunnelsConfig.Tunnels[i]
		}
	}
	return nil
}

// deleteTunnel removes a tunnel from config and stops it if running.
func (m *model) deleteTunnel(name string) {
	if rt, ok := m.tunnelRuntimes[name]; ok && (rt.status == tunnelRunning || rt.status == tunnelStarting) {
		// Best-effort kill
		if rt.pid != 0 {
			if proc, err := os.FindProcess(rt.pid); err == nil {
				_ = proc.Kill()
			}
			_ = os.Remove(rt.pidPath)
		}
		delete(m.tunnelRuntimes, name)
	}
	tunnels := m.tunnelsConfig.Tunnels[:0]
	for _, t := range m.tunnelsConfig.Tunnels {
		if t.Name != name {
			tunnels = append(tunnels, t)
		}
	}
	m.tunnelsConfig.Tunnels = tunnels
	_ = saveTunnels(m.tunnelsPath, m.tunnelsConfig)
}

// cmdStartTunnel spawns the tunnel command as a detached background process.
func (m *model) cmdStartTunnel(entry TunnelEntry, rt *tunnelRuntime) tea.Cmd {
	name := entry.Name
	command := resolveCommand(entry.Command, entry.Port)
	logPath := rt.logPath
	pidPath := rt.pidPath

	return func() tea.Msg {
		parts := strings.Fields(command)
		if len(parts) == 0 {
			return tunnelStoppedMsg{name: name, err: errors.New("empty command")}
		}

		logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return tunnelStoppedMsg{name: name, err: err}
		}
		defer logFile.Close()

		cmd := exec.Command(parts[0], parts[1:]...)
		cmd.Stdout = logFile
		cmd.Stderr = logFile
		detachProcess(cmd)

		if err := cmd.Start(); err != nil {
			return tunnelStoppedMsg{name: name, err: err}
		}

		pid := cmd.Process.Pid
		if err := os.WriteFile(pidPath, []byte(strconv.Itoa(pid)), 0644); err != nil {
			// Non-fatal: tunnel is running but we can't persist PID
			_ = err
		}

		return tunnelStartedMsg{name: name, pid: pid}
	}
}

// cmdStopTunnel kills a running tunnel process.
func cmdStopTunnel(name string, rt *tunnelRuntime) tea.Cmd {
	pid := rt.pid
	pidPath := rt.pidPath

	return func() tea.Msg {
		if pid != 0 {
			if proc, err := os.FindProcess(pid); err == nil {
				_ = proc.Kill()
			}
		}
		_ = os.Remove(pidPath)
		return tunnelStoppedMsg{name: name}
	}
}

// checkRunningTunnels reads PID files for all configured tunnels and returns
// a runtime map with any that are still alive.
func checkRunningTunnels(cfg TunnelsConfig, dbPath string) map[string]*tunnelRuntime {
	runtimes := make(map[string]*tunnelRuntime)
	for _, t := range cfg.Tunnels {
		lp := tunnelLogPath(dbPath, t.Name)
		pp := tunnelPIDPath(dbPath, t.Name)
		rt := &tunnelRuntime{
			status:  tunnelStopped,
			logPath: lp,
			pidPath: pp,
		}
		if data, err := os.ReadFile(pp); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && isProcessAlive(pid) {
				rt.status = tunnelRunning
				rt.pid = pid
			}
		}
		runtimes[t.Name] = rt
	}
	return runtimes
}

// cmdResetDB stops the server (if running), deletes the DB file, and re-runs migrations.
func (m *model) cmdResetDB() tea.Cmd {
	pid := m.serverPID
	pidPath := m.pidPath
	dbPath := m.cfg.DBPath
	tunnelsPath := m.tunnelsPath
	srvRunning := m.srvState == stateRunning
	closeDB := m.g.Close

	if srvRunning {
		m.srvState = stateStopping
		m.menu.serverState = stateStopping
	}

	return func() tea.Msg {
		killRunningTunnels(tunnelsPath, dbPath)

		if srvRunning {
			if process, err := os.FindProcess(pid); err == nil {
				_ = process.Kill()
			}
			_ = os.Remove(pidPath)
			waitForProcessExit(pid)
		}

		// Release the CLI's own DB connection before deletion.
		_ = closeDB()

		// Remove WAL/SHM first, then the main DB file.
		_ = os.Remove(dbPath + "-wal")
		_ = os.Remove(dbPath + "-shm")
		if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
			return dbResetDoneMsg{err: err}
		}

		return dbResetDoneMsg{err: resetDatabase(dbPath)}
	}
}
