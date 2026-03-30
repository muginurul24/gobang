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
	"github.com/mugiew/onixggr/internal/modules/notifications"
	"github.com/mugiew/onixggr/internal/modules/paymentsqris"
	"github.com/mugiew/onixggr/internal/modules/withdrawals"
	"github.com/mugiew/onixggr/internal/platform/config"
	"github.com/mugiew/onixggr/internal/platform/db"
	"github.com/mugiew/onixggr/internal/platform/nexusggr"
	"github.com/mugiew/onixggr/internal/platform/qris"
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

	ledgerService := ledger.NewService(ledger.NewRepository(pool))
	notificationService := notifications.NewService(notifications.Options{
		Repository: notifications.NewRepository(pool),
		Logger:     slog.Default(),
	})
	storeNotifier := notifications.NewStoreEmitter(
		notifications.NewAsyncEmitter(notificationService, slog.Default()),
	)
	reconcileService := game.NewReconcileService(game.ReconcileOptions{
		Repository: game.NewRepository(pool),
		Upstream: nexusggr.NewClient(nexusggr.Config{
			BaseURL:    cfg.NexusGGR.BaseURL,
			AgentCode:  cfg.NexusGGR.AgentCode,
			AgentToken: cfg.NexusGGR.AgentToken,
			Timeout:    cfg.NexusGGR.Timeout,
		}, slog.Default(), nil, nil),
		Ledger: ledgerService,
	})
	callbackService := callbacks.NewService(callbacks.Options{
		Repository:    callbacks.NewRepository(pool),
		Dispatcher:    callbacks.NewHTTPDispatcher(cfg.Callback.DeliveryTimeout),
		Notifications: storeNotifier,
		SigningSecret: cfg.Callback.SigningSecret,
	})
	qrisClient := qris.NewClient(qris.Config{
		BaseURL:              cfg.QRIS.BaseURL,
		Client:               cfg.QRIS.Client,
		ClientKey:            cfg.QRIS.ClientKey,
		GlobalUUID:           cfg.QRIS.GlobalUUID,
		DefaultExpireSeconds: cfg.QRIS.DefaultExpireSeconds,
	}, slog.Default(), nil, nil)
	qrisPaymentService := paymentsqris.NewService(paymentsqris.Options{
		Repository:          paymentsqris.NewRepository(pool),
		Ledger:              ledgerService,
		Callbacks:           callbackService,
		Notifications:       storeNotifier,
		MemberPaymentFeePct: cfg.Business.MemberPaymentPlatformFeePct,
	})
	qrisReconcileService := paymentsqris.NewReconcileService(paymentsqris.ReconcileOptions{
		Repository: paymentsqris.NewRepository(pool),
		Upstream:   qrisClient,
		Finalizer:  qrisPaymentService,
	})
	withdrawalService := withdrawals.NewService(withdrawals.Options{
		Repository: withdrawals.NewRepository(pool),
		Provider: withdrawals.NewProvider(withdrawals.ProviderConfig{
			BaseURL:      cfg.QRIS.BaseURL,
			Client:       cfg.QRIS.Client,
			ClientKey:    cfg.QRIS.ClientKey,
			GlobalUUID:   cfg.QRIS.GlobalUUID,
			TransferType: cfg.QRIS.BankInquiryType,
		}, nil),
		Ledger:              ledgerService,
		Notifications:       storeNotifier,
		PlatformFeePercent:  cfg.Business.StoreWithdrawPlatformFeePct,
		StatusCheckInterval: cfg.Worker.WithdrawReconcileInterval,
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

	qrisInterval := cfg.Worker.QRISReconcileInterval
	if qrisInterval <= 0 {
		qrisInterval = 30 * time.Second
	}

	qrisBatchSize := cfg.Worker.QRISReconcileBatchSize
	if qrisBatchSize <= 0 {
		qrisBatchSize = 50
	}

	go runQRISReconcileLoop(ctx, qrisReconcileService, qrisInterval, qrisBatchSize)

	withdrawInterval := cfg.Worker.WithdrawReconcileInterval
	if withdrawInterval <= 0 {
		withdrawInterval = 30 * time.Second
	}

	withdrawBatchSize := cfg.Worker.WithdrawReconcileBatchSize
	if withdrawBatchSize <= 0 {
		withdrawBatchSize = 50
	}

	go runWithdrawalStatusCheckLoop(ctx, withdrawalService, withdrawInterval, withdrawBatchSize)

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

func runQRISReconcileLoop(ctx context.Context, service paymentsqris.ReconcileService, interval time.Duration, batchSize int) {
	runOnce := func() {
		summary, err := service.RunPending(ctx, batchSize)
		if err != nil {
			log.Printf("qris reconcile run failed: %v", err)
		}

		if summary.Scanned == 0 && summary.FinalizedSuccess == 0 && summary.FinalizedExpired == 0 && summary.FinalizedFailed == 0 && summary.StillPending == 0 && summary.Skipped == 0 {
			return
		}

		log.Printf(
			"qris reconcile run: scanned=%d success=%d expired=%d failed=%d pending=%d skipped=%d",
			summary.Scanned,
			summary.FinalizedSuccess,
			summary.FinalizedExpired,
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

func runWithdrawalStatusCheckLoop(ctx context.Context, service withdrawals.Service, interval time.Duration, batchSize int) {
	runOnce := func() {
		summary, err := service.RunPendingChecks(ctx, batchSize)
		if err != nil {
			log.Printf("withdraw status check run failed: %v", err)
		}

		if summary.Scanned == 0 && summary.FinalizedSuccess == 0 && summary.FinalizedFailed == 0 && summary.StillPending == 0 && summary.Skipped == 0 {
			return
		}

		log.Printf(
			"withdraw status check run: scanned=%d success=%d failed=%d pending=%d skipped=%d",
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
