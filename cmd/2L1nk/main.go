package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"2L1nk/internal/app"
	"2L1nk/internal/cli"
	"2L1nk/internal/config"
	"2L1nk/internal/gate"
	"2L1nk/internal/utils"
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

	isSubprocess := os.Getenv("_2L1NK_SUBPROCESS") == "1"
	noLogs := os.Getenv("_2L1NK_NO_LOGS") == "1"

	// Only create a log file for direct --server invocations (not tempserver, not TUI subprocess).
	logPath := ""
	if !tempMode && !isSubprocess && !noLogs {
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
			_ = utils.SecureDelete(derivePathWithExt(cfg.DBPath, ".log"))
		}
	}()

	suppressStdout := tempMode && !isSubprocess
	a := app.New(cfg, g, logPath, suppressStdout)

	if !isSubprocess {
		fmt.Printf("\n  Port : %d\n  Key  : %s\n\n", cfg.Port, g.Key())
	}

	// Handle SIGINT/SIGTERM for graceful shutdown (runs deferred cleanup).
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	serverDone := make(chan error, 1)
	go func() {
		serverDone <- a.Start()
	}()

	select {
	case <-sigCh:
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = a.Stop(ctx)
		<-serverDone // wait for listener to close before returning (releases port)
	case err := <-serverDone:
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("application failed: %v", err)
		}
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
