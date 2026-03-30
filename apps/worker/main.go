package main

import (
	"context"
	"log"
	"log/slog"
	"os/signal"
	"syscall"
	"time"

	"github.com/mugiew/onixggr/internal/modules/callbacks"
	"github.com/mugiew/onixggr/internal/modules/game"
	"github.com/mugiew/onixggr/internal/modules/ledger"
	"github.com/mugiew/onixggr/internal/platform/config"
	"github.com/mugiew/onixggr/internal/platform/db"
	"github.com/mugiew/onixggr/internal/platform/nexusggr"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	log.Printf("worker booted for %s in %s", cfg.App.Name, cfg.App.Env)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := db.Open(ctx, cfg.Database)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer pool.Close()

	reconcileService := game.NewReconcileService(game.ReconcileOptions{
		Repository: game.NewRepository(pool),
		Upstream: nexusggr.NewClient(nexusggr.Config{
			BaseURL:    cfg.NexusGGR.BaseURL,
			AgentCode:  cfg.NexusGGR.AgentCode,
			AgentToken: cfg.NexusGGR.AgentToken,
			Timeout:    cfg.NexusGGR.Timeout,
		}, slog.Default(), nil),
		Ledger: ledger.NewService(ledger.NewRepository(pool)),
	})
	callbackService := callbacks.NewService(callbacks.Options{
		Repository:    callbacks.NewRepository(pool),
		Dispatcher:    callbacks.NewHTTPDispatcher(cfg.Callback.DeliveryTimeout),
		SigningSecret: cfg.Callback.SigningSecret,
	})

	interval := cfg.Worker.GameReconcileInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}

	batchSize := cfg.Worker.GameReconcileBatchSize
	if batchSize <= 0 {
		batchSize = 50
	}

	go runGameReconcileLoop(ctx, reconcileService, interval, batchSize)

	callbackInterval := cfg.Worker.CallbackRetryInterval
	if callbackInterval <= 0 {
		callbackInterval = 15 * time.Second
	}

	callbackBatchSize := cfg.Worker.CallbackRetryBatchSize
	if callbackBatchSize <= 0 {
		callbackBatchSize = 50
	}

	go runCallbackLoop(ctx, callbackService, callbackInterval, callbackBatchSize)

	<-ctx.Done()
	log.Println("worker stopped")
}

func runGameReconcileLoop(ctx context.Context, service game.ReconcileService, interval time.Duration, batchSize int) {
	runOnce := func() {
		summary, err := service.RunPending(ctx, batchSize)
		if err != nil {
			log.Printf("game reconcile run failed: %v", err)
		}

		if summary.Scanned == 0 && summary.FinalizedSuccess == 0 && summary.FinalizedFailed == 0 && summary.StillPending == 0 {
			return
		}

		log.Printf(
			"game reconcile run: scanned=%d success=%d failed=%d pending=%d skipped=%d",
			summary.Scanned,
			summary.FinalizedSuccess,
			summary.FinalizedFailed,
			summary.StillPending,
			summary.Skipped,
		)
	}

	runOnce()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runOnce()
		}
	}
}

func runCallbackLoop(ctx context.Context, service callbacks.Service, interval time.Duration, batchSize int) {
	runOnce := func() {
		summary, err := service.RunPending(ctx, batchSize)
		if err != nil {
			log.Printf("callback delivery run failed: %v", err)
		}

		if summary.Scanned == 0 && summary.Delivered == 0 && summary.Retrying == 0 && summary.Failed == 0 && summary.Skipped == 0 {
			return
		}

		log.Printf(
			"callback delivery run: scanned=%d delivered=%d retrying=%d failed=%d skipped=%d",
			summary.Scanned,
			summary.Delivered,
			summary.Retrying,
			summary.Failed,
			summary.Skipped,
		)
	}

	runOnce()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runOnce()
		}
	}
}
