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
		"PASSWORD_BCRYPT_COST",
		"MIN_TRANSACTION_AMOUNT",
		"STORE_LOW_BALANCE_THRESHOLD",
		"MEMBER_PAYMENT_PLATFORM_FEE_PERCENT",
		"STORE_WITHDRAW_PLATFORM_FEE_PERCENT",
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
		"GAME_RECONCILE_INTERVAL",
		"GAME_RECONCILE_BATCH_SIZE",
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

	if !cfg.Observability.MetricsEnabled {
		t.Fatal("Observability.MetricsEnabled = false, want true")
	}
}

func TestLoadOverrides(t *testing.T) {
	t.Setenv("APP_NAME", "demo-app")
	t.Setenv("HTTP_ADDRESS", ":9090")
	t.Setenv("JWT_ACCESS_TTL", "2h")
	t.Setenv("STORE_WITHDRAW_PLATFORM_FEE_PERCENT", "10.5")
	t.Setenv("NEXUSGGR_TIMEOUT", "25s")
	t.Setenv("PROVIDER_CATALOG_SYNC_INTERVAL", "45m")
	t.Setenv("GAME_RECONCILE_INTERVAL", "45s")
	t.Setenv("GAME_RECONCILE_BATCH_SIZE", "77")
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

	if cfg.Business.StoreWithdrawPlatformFeePct != 10.5 {
		t.Fatalf("Business.StoreWithdrawPlatformFeePct = %v, want 10.5", cfg.Business.StoreWithdrawPlatformFeePct)
	}

	if cfg.NexusGGR.Timeout != 25*time.Second {
		t.Fatalf("NexusGGR.Timeout = %v, want 25s", cfg.NexusGGR.Timeout)
	}

	if cfg.ProviderCatalog.SyncInterval != 45*time.Minute {
		t.Fatalf("ProviderCatalog.SyncInterval = %v, want 45m", cfg.ProviderCatalog.SyncInterval)
	}

	if cfg.Worker.GameReconcileInterval != 45*time.Second {
		t.Fatalf("Worker.GameReconcileInterval = %v, want 45s", cfg.Worker.GameReconcileInterval)
	}

	if cfg.Worker.GameReconcileBatchSize != 77 {
		t.Fatalf("Worker.GameReconcileBatchSize = %d, want 77", cfg.Worker.GameReconcileBatchSize)
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
