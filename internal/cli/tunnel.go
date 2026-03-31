package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// TunnelEntry holds the configuration for a single outbound tunnel.
type TunnelEntry struct {
	Name        string `json:"name"`
	Command     string `json:"command"`              // may contain {PORT} placeholder
	Description string `json:"description,omitempty"`
	Port        int    `json:"port,omitempty"`
	AutoStart   bool   `json:"auto_start,omitempty"`
}

// TunnelsConfig is the root object persisted to the tunnels JSON file.
type TunnelsConfig struct {
	Tunnels []TunnelEntry `json:"tunnels"`
}

// tunnelPresets are built-in templates shown when adding a new tunnel.
var tunnelPresets = []TunnelEntry{
	{
		Name:        "Cloudflare",
		Command:     "cloudflared tunnel --url http://localhost:{PORT}",
		Description: "Cloudflare quick tunnel (cloudflared required)",
		Port:        3847,
	},
}

// tunnelURLRegex matches any https:// URL in log output.
var tunnelURLRegex = regexp.MustCompile(`https?://[^\s"'<>]+`)

func tunnelsConfigPath(dbPath string) string {
	base := strings.TrimSuffix(dbPath, filepath.Ext(dbPath))
	return base + ".tunnels.json"
}

func loadTunnels(path string) TunnelsConfig {
	data, err := os.ReadFile(path)
	if err != nil {
		return TunnelsConfig{}
	}
	var cfg TunnelsConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return TunnelsConfig{}
	}
	return cfg
}

func saveTunnels(path string, cfg TunnelsConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// tunnelSafeName sanitizes a tunnel name for use in file paths.
func tunnelSafeName(name string) string {
	r := strings.NewReplacer(
		" ", "_",
		"/", "_",
		"\\", "_",
		":", "_",
	)
	return strings.ToLower(r.Replace(name))
}

// tunnelLogPath returns the log file path for a tunnel.
func tunnelLogPath(dbPath, name string) string {
	dir := filepath.Dir(dbPath)
	return filepath.Join(dir, tunnelSafeName(name)+".tunnel.log")
}

// tunnelPIDPath returns the PID file path for a tunnel.
func tunnelPIDPath(dbPath, name string) string {
	dir := filepath.Dir(dbPath)
	return filepath.Join(dir, tunnelSafeName(name)+".tunnel.pid")
}

// resolveCommand substitutes {PORT} in the command string.
func resolveCommand(command string, port int) string {
	if port == 0 {
		return strings.ReplaceAll(command, "{PORT}", "")
	}
	return strings.ReplaceAll(command, "{PORT}", fmt.Sprintf("%d", port))
}
