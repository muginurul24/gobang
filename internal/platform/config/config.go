package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	App             AppConfig
	HTTP            HTTPConfig
	Database        DatabaseConfig
	Redis           RedisConfig
	Auth            AuthConfig
	Business        BusinessConfig
	Callback        CallbackConfig
	QRIS            QRISConfig
	NexusGGR        NexusGGRConfig
	ProviderCatalog ProviderCatalogConfig
	Worker          WorkerConfig
	Realtime        RealtimeConfig
	Observability   ObservabilityConfig
}

type AppConfig struct {
	Name     string
	Env      string
	URL      string
	Timezone string
	LogLevel string
}

type HTTPConfig struct {
	Address string
}

type DatabaseConfig struct {
	URL             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

type RedisConfig struct {
	URL      string
	Password string
	DB       int
}

type AuthConfig struct {
	JWTAccessSecret               string
	JWTAccessTTL                  time.Duration
	SessionTTL                    time.Duration
	BcryptCost                    int
	EncryptionKey                 string
	TOTPEnrollmentTTL             time.Duration
	LoginAttemptWindow            time.Duration
	LoginMaxAttemptsPerIP         int
	LoginMaxAttemptsPerIdentifier int
}

type BusinessConfig struct {
	MinTransactionAmount        int64
	StoreLowBalanceThreshold    int64
	MemberPaymentPlatformFeePct float64
	StoreWithdrawPlatformFeePct float64
}

type CallbackConfig struct {
	SigningSecret   string
	DeliveryTimeout time.Duration
}

type QRISConfig struct {
	BaseURL              string
	Client               string
	ClientKey            string
	GlobalUUID           string
	DefaultExpireSeconds int
	BankInquiryAmount    int64
	BankInquiryType      int
	WebhookSharedSecret  string
}

type NexusGGRConfig struct {
	BaseURL    string
	AgentCode  string
	AgentToken string
	Timeout    time.Duration
}

type ProviderCatalogConfig struct {
	SyncInterval time.Duration
}

type WorkerConfig struct {
	GameReconcileInterval      time.Duration
	GameReconcileBatchSize     int
	QRISReconcileInterval      time.Duration
	QRISReconcileBatchSize     int
	WithdrawReconcileInterval  time.Duration
	WithdrawReconcileBatchSize int
	CallbackRetryInterval      time.Duration
	CallbackRetryBatchSize     int
}

type RealtimeConfig struct {
	HeartbeatSeconds int
}

type ObservabilityConfig struct {
	MetricsEnabled bool
	PrometheusPort int
}

func Load() (Config, error) {
	databaseConnMaxLifetime, err := envDuration("DATABASE_CONN_MAX_LIFETIME", 15*time.Minute)
	if err != nil {
		return Config{}, err
	}

	jwtAccessTTL, err := envDuration("JWT_ACCESS_TTL", time.Hour)
	if err != nil {
		return Config{}, err
	}

	sessionTTL, err := envDuration("SESSION_TTL", 168*time.Hour)
	if err != nil {
		return Config{}, err
	}

	totpEnrollmentTTL, err := envDuration("TOTP_ENROLLMENT_TTL", 10*time.Minute)
	if err != nil {
		return Config{}, err
	}

	loginAttemptWindow, err := envDuration("LOGIN_ATTEMPT_WINDOW", 15*time.Minute)
	if err != nil {
		return Config{}, err
	}

	nexusTimeout, err := envDuration("NEXUSGGR_TIMEOUT", 10*time.Second)
	if err != nil {
		return Config{}, err
	}

	providerCatalogSyncInterval, err := envDuration("PROVIDER_CATALOG_SYNC_INTERVAL", 30*time.Minute)
	if err != nil {
		return Config{}, err
	}

	callbackDeliveryTimeout, err := envDuration("CALLBACK_DELIVERY_TIMEOUT", 10*time.Second)
	if err != nil {
		return Config{}, err
	}

	gameReconcileInterval, err := envDuration("GAME_RECONCILE_INTERVAL", 30*time.Second)
	if err != nil {
		return Config{}, err
	}

	qrisReconcileInterval, err := envDuration("QRIS_RECONCILE_INTERVAL", 30*time.Second)
	if err != nil {
		return Config{}, err
	}

	withdrawReconcileInterval, err := envDuration("WITHDRAW_RECONCILE_INTERVAL", 30*time.Second)
	if err != nil {
		return Config{}, err
	}

	callbackRetryInterval, err := envDuration("CALLBACK_RETRY_INTERVAL", 15*time.Second)
	if err != nil {
		return Config{}, err
	}

	return Config{
		App: AppConfig{
			Name:     envString("APP_NAME", "onixggr"),
			Env:      envString("APP_ENV", "development"),
			URL:      envString("APP_URL", "http://localhost:5173"),
			Timezone: envString("APP_TIMEZONE", "Asia/Jakarta"),
			LogLevel: envString("APP_LOG_LEVEL", "debug"),
		},
		HTTP: HTTPConfig{
			Address: envString("HTTP_ADDRESS", ":8080"),
		},
		Database: DatabaseConfig{
			URL:             envString("DATABASE_URL", "postgresql://postgres:postgres@127.0.0.1:15432/onixggr?sslmode=disable"),
			MaxOpenConns:    envInt("DATABASE_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    envInt("DATABASE_MAX_IDLE_CONNS", 25),
			ConnMaxLifetime: databaseConnMaxLifetime,
		},
		Redis: RedisConfig{
			URL:      envString("REDIS_URL", "redis://127.0.0.1:16379"),
			Password: envString("REDIS_PASSWORD", ""),
			DB:       envInt("REDIS_DB", 0),
		},
		Auth: AuthConfig{
			JWTAccessSecret:               envString("JWT_ACCESS_SECRET", "change-me"),
			JWTAccessTTL:                  jwtAccessTTL,
			SessionTTL:                    sessionTTL,
			BcryptCost:                    envInt("PASSWORD_BCRYPT_COST", 12),
			EncryptionKey:                 envString("AUTH_ENCRYPTION_KEY", "change-me-auth-encryption-key"),
			TOTPEnrollmentTTL:             totpEnrollmentTTL,
			LoginAttemptWindow:            loginAttemptWindow,
			LoginMaxAttemptsPerIP:         envInt("LOGIN_MAX_ATTEMPTS_PER_IP", 20),
			LoginMaxAttemptsPerIdentifier: envInt("LOGIN_MAX_ATTEMPTS_PER_IDENTIFIER", 5),
		},
		Business: BusinessConfig{
			MinTransactionAmount:        envInt64("MIN_TRANSACTION_AMOUNT", 5000),
			StoreLowBalanceThreshold:    envInt64("STORE_LOW_BALANCE_THRESHOLD", 100000),
			MemberPaymentPlatformFeePct: envFloat64("MEMBER_PAYMENT_PLATFORM_FEE_PERCENT", 3),
			StoreWithdrawPlatformFeePct: envFloat64("STORE_WITHDRAW_PLATFORM_FEE_PERCENT", 12),
		},
		Callback: CallbackConfig{
			SigningSecret:   envString("CALLBACK_SIGNING_SECRET", "change-me-callback-signing-secret"),
			DeliveryTimeout: callbackDeliveryTimeout,
		},
		QRIS: QRISConfig{
			BaseURL:              envString("QRIS_BASE_URL", "https://example-qris.local"),
			Client:               envString("QRIS_CLIENT", ""),
			ClientKey:            envString("QRIS_CLIENT_KEY", ""),
			GlobalUUID:           envString("QRIS_GLOBAL_UUID", "GLOBAL_QRIS_UUID"),
			DefaultExpireSeconds: envInt("QRIS_DEFAULT_EXPIRE_SECONDS", 300),
			BankInquiryAmount:    envInt64("QRIS_BANK_INQUIRY_AMOUNT", 10000),
			BankInquiryType:      envInt("QRIS_BANK_INQUIRY_TYPE", 2),
			WebhookSharedSecret:  envString("QRIS_WEBHOOK_SHARED_SECRET", ""),
		},
		NexusGGR: NexusGGRConfig{
			BaseURL:    envString("NEXUSGGR_BASE_URL", "https://api.nexusggr.com"),
			AgentCode:  envString("NEXUSGGR_AGENT_CODE", ""),
			AgentToken: envString("NEXUSGGR_AGENT_TOKEN", ""),
			Timeout:    nexusTimeout,
		},
		ProviderCatalog: ProviderCatalogConfig{
			SyncInterval: providerCatalogSyncInterval,
		},
		Worker: WorkerConfig{
			GameReconcileInterval:      gameReconcileInterval,
			GameReconcileBatchSize:     envInt("GAME_RECONCILE_BATCH_SIZE", 50),
			QRISReconcileInterval:      qrisReconcileInterval,
			QRISReconcileBatchSize:     envInt("QRIS_RECONCILE_BATCH_SIZE", 50),
			WithdrawReconcileInterval:  withdrawReconcileInterval,
			WithdrawReconcileBatchSize: envInt("WITHDRAW_RECONCILE_BATCH_SIZE", 50),
			CallbackRetryInterval:      callbackRetryInterval,
			CallbackRetryBatchSize:     envInt("CALLBACK_RETRY_BATCH_SIZE", 50),
		},
		Realtime: RealtimeConfig{
			HeartbeatSeconds: envInt("WS_HEARTBEAT_SECONDS", 30),
		},
		Observability: ObservabilityConfig{
			MetricsEnabled: envBool("METRICS_ENABLED", true),
			PrometheusPort: envInt("PROMETHEUS_PORT", 9090),
		},
	}, nil
}

func envString(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}

func envInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return parsed
}

func envInt64(key string, fallback int64) int64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}

	return parsed
}

func envFloat64(key string, fallback float64) float64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}

	return parsed
}

func envBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}

	return parsed
}

func envDuration(key string, fallback time.Duration) (time.Duration, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}

	return parsed, nil
}
