package main

import (
	"log"

	"2L1nk/internal/app"
	"2L1nk/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	a := app.New(cfg)

	if err := a.Start(); err != nil {
		log.Fatalf("application failed: %v", err)
	}
}
