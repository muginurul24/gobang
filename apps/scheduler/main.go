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
	"github.com/mugiew/onixggr/internal/modules/providercatalog"
	"github.com/mugiew/onixggr/internal/platform/config"
	"github.com/mugiew/onixggr/internal/platform/db"
	"github.com/mugiew/onixggr/internal/platform/nexusggr"
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

	interval := cfg.ProviderCatalog.SyncInterval
	if interval <= 0 {
		interval = 30 * time.Minute
	}

	go runProviderCatalogSync(ctx, service, interval)
	go runAuditRetentionPrune(ctx, auditService, cfg.Audit.PruneInterval)
	go runChatRetentionPrune(ctx, chatService, cfg.Chat.PruneInterval)

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
