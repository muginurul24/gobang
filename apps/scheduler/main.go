package main

import (
	"context"
	"log"
	"log/slog"
	"os/signal"
	"syscall"
	"time"

	"github.com/mugiew/onixggr/internal/modules/audit"
	"github.com/mugiew/onixggr/internal/modules/chat"
	"github.com/mugiew/onixggr/internal/modules/ledger"
	"github.com/mugiew/onixggr/internal/modules/notifications"
	"github.com/mugiew/onixggr/internal/modules/providercatalog"
	"github.com/mugiew/onixggr/internal/modules/stores"
	"github.com/mugiew/onixggr/internal/platform/config"
	"github.com/mugiew/onixggr/internal/platform/db"
	"github.com/mugiew/onixggr/internal/platform/nexusggr"
	platformrealtime "github.com/mugiew/onixggr/internal/platform/realtime"
	redisclient "github.com/mugiew/onixggr/internal/platform/redis"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	log.Printf("scheduler booted for %s in %s", cfg.App.Name, cfg.App.Env)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := db.Open(ctx, cfg.Database)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer pool.Close()

	var realtimeHub *platformrealtime.Hub
	redis, err := redisclient.Open(ctx, cfg.Redis)
	if err != nil {
		log.Printf("scheduler realtime disabled: %v", err)
	} else {
		defer func() {
			if err := redis.Close(); err != nil {
				log.Printf("close scheduler redis: %v", err)
			}
		}()

		realtimeHub, err = platformrealtime.NewHub(ctx, redis)
		if err != nil {
			log.Printf("scheduler realtime hub disabled: %v", err)
		} else {
			defer func() {
				if err := realtimeHub.Close(); err != nil {
					log.Printf("close scheduler realtime hub: %v", err)
				}
			}()
		}
	}

	service := providercatalog.NewService(providercatalog.Options{
		Repository: providercatalog.NewRepository(pool),
		Upstream: nexusggr.NewClient(nexusggr.Config{
			BaseURL:    cfg.NexusGGR.BaseURL,
			AgentCode:  cfg.NexusGGR.AgentCode,
			AgentToken: cfg.NexusGGR.AgentToken,
			Timeout:    cfg.NexusGGR.Timeout,
		}, slog.Default(), nil, nil),
	})
	auditService := audit.NewService(audit.Options{
		Repository:      audit.NewRepository(pool),
		RetentionPeriod: cfg.Audit.RetentionPeriod,
	})
	chatService := chat.NewService(chat.Options{
		Repository:      chat.NewRepository(pool),
		Clock:           nil,
		RetentionPeriod: cfg.Chat.RetentionPeriod,
	})
	notificationService := notifications.NewService(notifications.Options{
		Repository: notifications.NewRepository(pool),
		Hub:        realtimeHub,
		Logger:     slog.Default(),
	})
	lowBalanceMonitor := stores.NewLowBalanceMonitor(stores.LowBalanceMonitorOptions{
		Repository:    stores.NewRepository(pool),
		Ledger:        ledger.NewService(ledger.NewRepository(pool)),
		Notifications: notifications.NewStoreEmitter(notifications.NewAsyncEmitter(notificationService, slog.Default())),
		Cooldown:      cfg.Alert.LowBalanceCooldown,
	})

	interval := cfg.ProviderCatalog.SyncInterval
	if interval <= 0 {
		interval = 30 * time.Minute
	}

	go runProviderCatalogSync(ctx, service, interval)
	go runAuditRetentionPrune(ctx, auditService, cfg.Audit.PruneInterval)
	go runChatRetentionPrune(ctx, chatService, cfg.Chat.PruneInterval)
	go runLowBalanceSweep(ctx, lowBalanceMonitor, cfg.Alert.LowBalanceSweepInterval)

	<-ctx.Done()
	log.Println("scheduler stopped")
}

func runAuditRetentionPrune(ctx context.Context, service audit.Service, interval time.Duration) {
	if interval <= 0 {
		interval = 24 * time.Hour
	}

	runOnce := func() {
		pruned, err := service.PruneExpired(ctx)
		if err != nil {
			log.Printf("audit retention prune failed: %v", err)
			return
		}

		log.Printf("audit retention prune complete: %d row(s) removed", pruned)
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

func runProviderCatalogSync(ctx context.Context, service providercatalog.Service, interval time.Duration) {
	runOnce := func() {
		summary, err := service.Sync(ctx)
		if err != nil {
			log.Printf("provider catalog sync failed: %v", err)
			return
		}

		log.Printf("provider catalog sync complete: %d provider(s), %d game(s)", summary.ProvidersSynced, summary.GamesSynced)
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

func runChatRetentionPrune(ctx context.Context, service chat.Service, interval time.Duration) {
	if interval <= 0 {
		interval = time.Hour
	}

	runOnce := func() {
		pruned, err := service.PruneExpired(ctx)
		if err != nil {
			log.Printf("chat retention prune failed: %v", err)
			return
		}

		log.Printf("chat retention prune complete: %d message(s) removed", pruned)
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

func runLowBalanceSweep(ctx context.Context, monitor stores.LowBalanceMonitor, interval time.Duration) {
	if interval <= 0 {
		interval = 15 * time.Minute
	}

	runOnce := func() {
		result, err := monitor.Sweep(ctx)
		if err != nil {
			log.Printf("low balance sweep failed: %v", err)
			return
		}

		if result.SkippedLocked {
			log.Printf("low balance sweep skipped: lock already held")
			return
		}

		log.Printf(
			"low balance sweep complete: scanned=%d alerted=%d skipped_healthy=%d skipped_cooldown=%d errors=%d",
			result.Scanned,
			result.Alerted,
			result.SkippedHealthy,
			result.SkippedCooldown,
			result.Errors,
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
