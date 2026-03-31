package config

import (
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	for _, key := range []string{
		"APP_NAME",
		"APP_ENV",
		"APP_URL",
		"APP_TIMEZONE",
		"APP_LOG_LEVEL",
		"HTTP_ADDRESS",
		"DATABASE_URL",
		"DATABASE_MAX_OPEN_CONNS",
		"DATABASE_MAX_IDLE_CONNS",
		"DATABASE_CONN_MAX_LIFETIME",
		"REDIS_URL",
		"REDIS_PASSWORD",
		"REDIS_DB",
		"JWT_ACCESS_SECRET",
		"JWT_ACCESS_TTL",
		"SESSION_TTL",
		"SESSION_CLEANUP_INTERVAL",
		"PASSWORD_BCRYPT_COST",
		"MIN_TRANSACTION_AMOUNT",
		"STORE_LOW_BALANCE_THRESHOLD",
		"MEMBER_PAYMENT_PLATFORM_FEE_PERCENT",
		"STORE_WITHDRAW_PLATFORM_FEE_PERCENT",
		"CALLBACK_SIGNING_SECRET",
		"CALLBACK_DELIVERY_TIMEOUT",
		"CALLBACK_ATTEMPT_RETENTION_PERIOD",
		"CALLBACK_ATTEMPT_PRUNE_INTERVAL",
		"QRIS_BASE_URL",
		"QRIS_CLIENT",
		"QRIS_CLIENT_KEY",
		"QRIS_GLOBAL_UUID",
		"QRIS_DEFAULT_EXPIRE_SECONDS",
		"QRIS_BANK_INQUIRY_AMOUNT",
		"QRIS_BANK_INQUIRY_TYPE",
		"QRIS_WEBHOOK_SHARED_SECRET",
		"NEXUSGGR_BASE_URL",
		"NEXUSGGR_AGENT_CODE",
		"NEXUSGGR_AGENT_TOKEN",
		"NEXUSGGR_TIMEOUT",
		"PROVIDER_CATALOG_SYNC_INTERVAL",
		"LOW_BALANCE_SWEEP_INTERVAL",
		"LOW_BALANCE_ALERT_COOLDOWN",
		"GAME_RECONCILE_INTERVAL",
		"GAME_RECONCILE_BATCH_SIZE",
		"QRIS_RECONCILE_INTERVAL",
		"QRIS_RECONCILE_BATCH_SIZE",
		"WITHDRAW_RECONCILE_INTERVAL",
		"WITHDRAW_RECONCILE_BATCH_SIZE",
		"CALLBACK_RETRY_INTERVAL",
		"CALLBACK_RETRY_BATCH_SIZE",
		"AUDIT_RETENTION_PERIOD",
		"AUDIT_PRUNE_INTERVAL",
		"CHAT_RETENTION_PERIOD",
		"CHAT_PRUNE_INTERVAL",
		"WS_HEARTBEAT_SECONDS",
		"METRICS_ENABLED",
		"PROMETHEUS_PORT",
	} {
		t.Setenv(key, "")
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.App.Name != "onixggr" {
		t.Fatalf("App.Name = %q, want onixggr", cfg.App.Name)
	}

	if cfg.HTTP.Address != ":8080" {
		t.Fatalf("HTTP.Address = %q, want :8080", cfg.HTTP.Address)
	}

	if cfg.Database.ConnMaxLifetime != 15*time.Minute {
		t.Fatalf("Database.ConnMaxLifetime = %v, want 15m", cfg.Database.ConnMaxLifetime)
	}

	if cfg.Auth.JWTAccessTTL != time.Hour {
		t.Fatalf("Auth.JWTAccessTTL = %v, want 1h", cfg.Auth.JWTAccessTTL)
	}

	if cfg.Auth.SessionCleanupInterval != time.Hour {
		t.Fatalf("Auth.SessionCleanupInterval = %v, want 1h", cfg.Auth.SessionCleanupInterval)
	}

	if cfg.Callback.AttemptRetentionPeriod != 30*24*time.Hour {
		t.Fatalf("Callback.AttemptRetentionPeriod = %v, want 720h", cfg.Callback.AttemptRetentionPeriod)
	}

	if cfg.Callback.AttemptPruneInterval != 24*time.Hour {
		t.Fatalf("Callback.AttemptPruneInterval = %v, want 24h", cfg.Callback.AttemptPruneInterval)
	}

	if !cfg.Observability.MetricsEnabled {
		t.Fatal("Observability.MetricsEnabled = false, want true")
	}
}

func TestLoadOverrides(t *testing.T) {
	t.Setenv("APP_NAME", "demo-app")
	t.Setenv("HTTP_ADDRESS", ":9090")
	t.Setenv("JWT_ACCESS_TTL", "2h")
	t.Setenv("SESSION_CLEANUP_INTERVAL", "90m")
	t.Setenv("STORE_WITHDRAW_PLATFORM_FEE_PERCENT", "10.5")
	t.Setenv("CALLBACK_SIGNING_SECRET", "callback-secret")
	t.Setenv("CALLBACK_DELIVERY_TIMEOUT", "12s")
	t.Setenv("CALLBACK_ATTEMPT_RETENTION_PERIOD", "480h")
	t.Setenv("CALLBACK_ATTEMPT_PRUNE_INTERVAL", "6h")
	t.Setenv("NEXUSGGR_TIMEOUT", "25s")
	t.Setenv("PROVIDER_CATALOG_SYNC_INTERVAL", "45m")
	t.Setenv("LOW_BALANCE_SWEEP_INTERVAL", "20m")
	t.Setenv("LOW_BALANCE_ALERT_COOLDOWN", "8h")
	t.Setenv("GAME_RECONCILE_INTERVAL", "45s")
	t.Setenv("GAME_RECONCILE_BATCH_SIZE", "77")
	t.Setenv("QRIS_RECONCILE_INTERVAL", "35s")
	t.Setenv("QRIS_RECONCILE_BATCH_SIZE", "21")
	t.Setenv("WITHDRAW_RECONCILE_INTERVAL", "40s")
	t.Setenv("WITHDRAW_RECONCILE_BATCH_SIZE", "17")
	t.Setenv("CALLBACK_RETRY_INTERVAL", "20s")
	t.Setenv("CALLBACK_RETRY_BATCH_SIZE", "31")
	t.Setenv("AUDIT_RETENTION_PERIOD", "2200h")
	t.Setenv("AUDIT_PRUNE_INTERVAL", "3h")
	t.Setenv("CHAT_RETENTION_PERIOD", "200h")
	t.Setenv("CHAT_PRUNE_INTERVAL", "2h")
	t.Setenv("METRICS_ENABLED", "false")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.App.Name != "demo-app" {
		t.Fatalf("App.Name = %q, want demo-app", cfg.App.Name)
	}

	if cfg.HTTP.Address != ":9090" {
		t.Fatalf("HTTP.Address = %q, want :9090", cfg.HTTP.Address)
	}

	if cfg.Auth.JWTAccessTTL != 2*time.Hour {
		t.Fatalf("Auth.JWTAccessTTL = %v, want 2h", cfg.Auth.JWTAccessTTL)
	}

	if cfg.Auth.SessionCleanupInterval != 90*time.Minute {
		t.Fatalf("Auth.SessionCleanupInterval = %v, want 90m", cfg.Auth.SessionCleanupInterval)
	}

	if cfg.Business.StoreWithdrawPlatformFeePct != 10.5 {
		t.Fatalf("Business.StoreWithdrawPlatformFeePct = %v, want 10.5", cfg.Business.StoreWithdrawPlatformFeePct)
	}

	if cfg.Callback.SigningSecret != "callback-secret" {
		t.Fatalf("Callback.SigningSecret = %q, want callback-secret", cfg.Callback.SigningSecret)
	}

	if cfg.Callback.DeliveryTimeout != 12*time.Second {
		t.Fatalf("Callback.DeliveryTimeout = %v, want 12s", cfg.Callback.DeliveryTimeout)
	}

	if cfg.Callback.AttemptRetentionPeriod != 480*time.Hour {
		t.Fatalf("Callback.AttemptRetentionPeriod = %v, want 480h", cfg.Callback.AttemptRetentionPeriod)
	}

	if cfg.Callback.AttemptPruneInterval != 6*time.Hour {
		t.Fatalf("Callback.AttemptPruneInterval = %v, want 6h", cfg.Callback.AttemptPruneInterval)
	}

	if cfg.NexusGGR.Timeout != 25*time.Second {
		t.Fatalf("NexusGGR.Timeout = %v, want 25s", cfg.NexusGGR.Timeout)
	}

	if cfg.ProviderCatalog.SyncInterval != 45*time.Minute {
		t.Fatalf("ProviderCatalog.SyncInterval = %v, want 45m", cfg.ProviderCatalog.SyncInterval)
	}

	if cfg.Alert.LowBalanceSweepInterval != 20*time.Minute {
		t.Fatalf("Alert.LowBalanceSweepInterval = %v, want 20m", cfg.Alert.LowBalanceSweepInterval)
	}

	if cfg.Alert.LowBalanceCooldown != 8*time.Hour {
		t.Fatalf("Alert.LowBalanceCooldown = %v, want 8h", cfg.Alert.LowBalanceCooldown)
	}

	if cfg.Worker.GameReconcileInterval != 45*time.Second {
		t.Fatalf("Worker.GameReconcileInterval = %v, want 45s", cfg.Worker.GameReconcileInterval)
	}

	if cfg.Worker.GameReconcileBatchSize != 77 {
		t.Fatalf("Worker.GameReconcileBatchSize = %d, want 77", cfg.Worker.GameReconcileBatchSize)
	}

	if cfg.Worker.QRISReconcileInterval != 35*time.Second {
		t.Fatalf("Worker.QRISReconcileInterval = %v, want 35s", cfg.Worker.QRISReconcileInterval)
	}

	if cfg.Worker.QRISReconcileBatchSize != 21 {
		t.Fatalf("Worker.QRISReconcileBatchSize = %d, want 21", cfg.Worker.QRISReconcileBatchSize)
	}

	if cfg.Worker.WithdrawReconcileInterval != 40*time.Second {
		t.Fatalf("Worker.WithdrawReconcileInterval = %v, want 40s", cfg.Worker.WithdrawReconcileInterval)
	}

	if cfg.Worker.WithdrawReconcileBatchSize != 17 {
		t.Fatalf("Worker.WithdrawReconcileBatchSize = %d, want 17", cfg.Worker.WithdrawReconcileBatchSize)
	}

	if cfg.Worker.CallbackRetryInterval != 20*time.Second {
		t.Fatalf("Worker.CallbackRetryInterval = %v, want 20s", cfg.Worker.CallbackRetryInterval)
	}

	if cfg.Worker.CallbackRetryBatchSize != 31 {
		t.Fatalf("Worker.CallbackRetryBatchSize = %d, want 31", cfg.Worker.CallbackRetryBatchSize)
	}

	if cfg.Audit.RetentionPeriod != 2200*time.Hour {
		t.Fatalf("Audit.RetentionPeriod = %v, want 2200h", cfg.Audit.RetentionPeriod)
	}

	if cfg.Audit.PruneInterval != 3*time.Hour {
		t.Fatalf("Audit.PruneInterval = %v, want 3h", cfg.Audit.PruneInterval)
	}

	if cfg.Chat.RetentionPeriod != 200*time.Hour {
		t.Fatalf("Chat.RetentionPeriod = %v, want 200h", cfg.Chat.RetentionPeriod)
	}

	if cfg.Chat.PruneInterval != 2*time.Hour {
		t.Fatalf("Chat.PruneInterval = %v, want 2h", cfg.Chat.PruneInterval)
	}

	if cfg.Observability.MetricsEnabled {
		t.Fatal("Observability.MetricsEnabled = true, want false")
	}
}

func TestLoadRejectsInvalidDuration(t *testing.T) {
	t.Setenv("JWT_ACCESS_TTL", "not-a-duration")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want invalid duration error")
	}
}
