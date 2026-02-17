// @title       	2L1nk API
// @version     	1.0
// @description 	This is the 2L1nk API documentation.
// @contact.name  	Reza Sanjari
// @contact.email 	reza.sanjari@avl.com
// @host        	localhost:8080
// @BasePath    	/
// @schemes     	http

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
