package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mugiew/onixggr/internal/platform/config"
	"github.com/mugiew/onixggr/internal/platform/db"
	"github.com/mugiew/onixggr/internal/platform/health"
	"github.com/mugiew/onixggr/internal/platform/httpserver"
	"github.com/mugiew/onixggr/internal/platform/observability"
	platformrealtime "github.com/mugiew/onixggr/internal/platform/realtime"
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

	var metrics *observability.Metrics
	if cfg.Observability.MetricsEnabled {
		metrics = observability.NewMetrics()
	}

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

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	realtimeHub, err := platformrealtime.NewHub(ctx, redis)
	if err != nil {
		return fmt.Errorf("bootstrap realtime hub: %w", err)
	}
	defer func() {
		if err := realtimeHub.Close(); err != nil {
			log.Printf("close realtime hub: %v", err)
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
		health.Checker{
			Name:     "nexusggr",
			Severity: health.SeverityDegraded,
			Check: func(context.Context) error {
				if strings.TrimSpace(cfg.NexusGGR.BaseURL) == "" || strings.TrimSpace(cfg.NexusGGR.AgentCode) == "" || strings.TrimSpace(cfg.NexusGGR.AgentToken) == "" {
					return fmt.Errorf("nexusggr not fully configured")
				}
				return nil
			},
		},
		health.Checker{
			Name:     "qris",
			Severity: health.SeverityDegraded,
			Check: func(context.Context) error {
				if strings.TrimSpace(cfg.QRIS.BaseURL) == "" || strings.TrimSpace(cfg.QRIS.Client) == "" || strings.TrimSpace(cfg.QRIS.ClientKey) == "" || strings.TrimSpace(cfg.QRIS.GlobalUUID) == "" {
					return fmt.Errorf("qris not fully configured")
				}
				return nil
			},
		},
	)

	server := &http.Server{
		Addr: cfg.HTTP.Address,
		Handler: httpserver.NewHandler(cfg, httpserver.Dependencies{
			Health:   healthService,
			Logger:   logger,
			DB:       postgres,
			Redis:    redis,
			Realtime: realtimeHub,
			Metrics:  metrics,
		}),
		ReadHeaderTimeout: 5 * time.Second,
	}

	serverErrors := make(chan error, 1)

	var metricsServer *http.Server
	if metrics != nil {
		metricsMux := http.NewServeMux()
		metricsMux.Handle("GET /metrics", metrics.Handler())

		metricsServer = &http.Server{
			Addr:              ":" + strconv.Itoa(cfg.Observability.PrometheusPort),
			Handler:           metricsMux,
			ReadHeaderTimeout: 5 * time.Second,
		}

		go func() {
			log.Printf("metrics listening on %s", metricsServer.Addr)
			if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				serverErrors <- err
			}
		}()

		go observability.RunSnapshotLoop(ctx, metrics, observability.NewSnapshotter(postgres), realtimeHub.ConnectionCount, 15*time.Second)
		go observability.RunHealthLoop(ctx, metrics, healthService, 15*time.Second)
	}

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

	if metricsServer != nil {
		if err := metricsServer.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown metrics server: %w", err)
		}
	}

	return nil
}
