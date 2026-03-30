package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"2L1nk/internal/app"
	"2L1nk/internal/cli"
	"2L1nk/internal/config"
	"2L1nk/internal/utils"
	"2L1nk/internal/gate"
)

func main() {
	switch {
	case hasArg("--server"):
		runServer(false)
	case hasArg("--tempserver"):
		runServer(true)
	default:
		if err := cli.RunTUI(); err != nil {
			log.Fatalf("TUI error: %v", err)
		}
	}
}

func runServer(tempMode bool) {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	g, err := gate.New(0)
	if err != nil {
		log.Fatalf("failed to initialize gate: %v", err)
	}
	if key := os.Getenv("_2L1NK_GATE_KEY"); key != "" {
		g.SetKey(key)
	}

	noLogs := os.Getenv("_2L1NK_NO_LOGS") == "1"
	logPath := ""
	if !noLogs {
		logPath = derivePathWithExt(cfg.DBPath, ".log")
	}

	pidPath := derivePathWithExt(cfg.DBPath, ".pid")

	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0644); err != nil {
		log.Fatalf("failed to write pid file: %v", err)
	}
	defer func() {
		os.Remove(pidPath)
		if tempMode {
			// Give the server a moment to release file handles.
			time.Sleep(100 * time.Millisecond)
			_ = utils.SecureDelete(cfg.DBPath)
			_ = utils.SecureDelete(cfg.DBPath + "-shm")
			_ = utils.SecureDelete(cfg.DBPath + "-wal")
			if logPath != "" {
				_ = utils.SecureDelete(logPath)
			}
		}
	}()

	a := app.New(cfg, g, logPath)
	if err := a.Start(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("application failed: %v", err)
	}
}

func hasArg(flag string) bool {
	for _, a := range os.Args[1:] {
		if a == flag {
			return true
		}
	}
	return false
}

func derivePathWithExt(dbPath, ext string) string {
	return strings.TrimSuffix(dbPath, filepath.Ext(dbPath)) + ext
}
