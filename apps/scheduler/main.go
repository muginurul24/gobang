package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"github.com/mugiew/onixggr/internal/platform/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	log.Printf("scheduler booted for %s in %s", cfg.App.Name, cfg.App.Env)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	<-ctx.Done()
	log.Println("scheduler stopped")
}
