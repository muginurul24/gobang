package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/mugiew/onixggr/internal/platform/config"
	"github.com/mugiew/onixggr/internal/platform/db"
	"github.com/mugiew/onixggr/internal/platform/health"
	"github.com/mugiew/onixggr/internal/platform/httpserver"
	"github.com/mugiew/onixggr/internal/platform/observability"
	redisclient "github.com/mugiew/onixggr/internal/platform/redis"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := observability.NewLogger(cfg.App)
	slog.SetDefault(logger)

	bootCtx, bootCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer bootCancel()

	postgres, err := db.Open(bootCtx, cfg.Database)
	if err != nil {
		return fmt.Errorf("bootstrap postgres: %w", err)
	}
	defer postgres.Close()

	redis, err := redisclient.Open(bootCtx, cfg.Redis)
	if err != nil {
		return fmt.Errorf("bootstrap redis: %w", err)
	}
	defer func() {
		if err := redis.Close(); err != nil {
			log.Printf("close redis: %v", err)
		}
	}()

	healthService := health.New(
		cfg.App.Name,
		cfg.App.Env,
		2*time.Second,
		health.Checker{Name: "postgres", Check: postgres.Ping},
		health.Checker{
			Name: "redis",
			Check: func(ctx context.Context) error {
				return redis.Ping(ctx).Err()
			},
		},
	)

	server := &http.Server{
		Addr: cfg.HTTP.Address,
		Handler: httpserver.NewHandler(cfg, httpserver.Dependencies{
			Health: healthService,
			Logger: logger,
			DB:     postgres,
			Redis:  redis,
		}),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	serverErrors := make(chan error, 1)

	go func() {
		log.Printf("api listening on %s", cfg.HTTP.Address)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErrors <- err
		}
	}()

	select {
	case <-ctx.Done():
	case err := <-serverErrors:
		return fmt.Errorf("listen and serve: %w", err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown server: %w", err)
	}

	return nil
}
