package httpserver

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mugiew/onixggr/internal/modules/audit"
	"github.com/mugiew/onixggr/internal/modules/auth"
	"github.com/mugiew/onixggr/internal/modules/bankaccounts"
	"github.com/mugiew/onixggr/internal/modules/game"
	"github.com/mugiew/onixggr/internal/modules/ledger"
	"github.com/mugiew/onixggr/internal/modules/providercatalog"
	"github.com/mugiew/onixggr/internal/modules/storemembers"
	"github.com/mugiew/onixggr/internal/modules/stores"
	"github.com/mugiew/onixggr/internal/platform/bankdirectory"
	"github.com/mugiew/onixggr/internal/platform/config"
	"github.com/mugiew/onixggr/internal/platform/crypto"
	"github.com/mugiew/onixggr/internal/platform/health"
	"github.com/mugiew/onixggr/internal/platform/middleware"
	"github.com/mugiew/onixggr/internal/platform/nexusggr"
	"github.com/mugiew/onixggr/internal/platform/security"
	goredis "github.com/redis/go-redis/v9"
)

type Dependencies struct {
	Health health.Service
	Logger *slog.Logger
	DB     *pgxpool.Pool
	Redis  *goredis.Client
}

type infoResponse struct {
	Name  string   `json:"name"`
	Apps  []string `json:"apps"`
	Docs  []string `json:"docs"`
	Ready bool     `json:"ready"`
}

func NewHandler(cfg config.Config, deps Dependencies) http.Handler {
	mux := http.NewServeMux()

	if deps.DB != nil && deps.Redis != nil {
		passwordHasher := security.NewPasswordHasher(cfg.Auth.BcryptCost)
		sealer := crypto.NewSealer(cfg.Auth.EncryptionKey)
		authService := auth.NewService(auth.Options{
			Repository:        auth.NewRepository(deps.DB),
			Sessions:          auth.NewRedisSessionStore(deps.Redis),
			Enrollments:       auth.NewRedisEnrollmentStore(deps.Redis),
			Limiter:           auth.NewRedisLoginLimiter(deps.Redis, cfg.Auth.LoginAttemptWindow, cfg.Auth.LoginMaxAttemptsPerIP, cfg.Auth.LoginMaxAttemptsPerIdentifier),
			Passwords:         passwordHasher,
			Tokens:            security.NewAccessTokenManager(cfg.Auth.JWTAccessSecret, cfg.Auth.JWTAccessTTL, cfg.App.Name, nil),
			Sealer:            sealer,
			TwoFactor:         security.NewTOTPManager(cfg.App.Name),
			SessionTTL:        cfg.Auth.SessionTTL,
			TOTPEnrollmentTTL: cfg.Auth.TOTPEnrollmentTTL,
		})
		banks := bankdirectory.MustLoadDefault()
		ledgerService := ledger.NewService(ledger.NewRepository(deps.DB))
		nexusClient := nexusggr.NewClient(nexusggr.Config{
			BaseURL:    cfg.NexusGGR.BaseURL,
			AgentCode:  cfg.NexusGGR.AgentCode,
			AgentToken: cfg.NexusGGR.AgentToken,
			Timeout:    cfg.NexusGGR.Timeout,
		}, deps.Logger, nil)

		auth.NewHandler(authService).Register(mux)
		stores.NewHandler(
			stores.NewService(
				stores.NewRepository(deps.DB),
				passwordHasher,
				nil,
				cfg.Business.StoreLowBalanceThreshold,
			),
			authService,
		).Register(mux)
		bankaccounts.NewHandler(
			bankaccounts.NewService(
				bankaccounts.NewRepository(deps.DB),
				banks,
				bankaccounts.NewInquiryVerifier(bankaccounts.InquiryVerifierConfig{
					BaseURL:      cfg.QRIS.BaseURL,
					Client:       cfg.QRIS.Client,
					ClientKey:    cfg.QRIS.ClientKey,
					GlobalUUID:   cfg.QRIS.GlobalUUID,
					Amount:       cfg.QRIS.BankInquiryAmount,
					TransferType: cfg.QRIS.BankInquiryType,
				}, banks),
				sealer,
				nil,
			),
			authService,
		).Register(mux)
		storemembers.NewHandler(
			storemembers.NewService(storemembers.NewRepository(deps.DB), nil),
			authService,
		).Register(mux)
		providercatalog.NewHandler(
			providercatalog.NewService(providercatalog.Options{
				Repository: providercatalog.NewRepository(deps.DB),
				Upstream:   nexusClient,
			}),
			authService,
		).Register(mux)
		game.NewHandler(
			game.NewService(game.Options{
				Repository:           game.NewRepository(deps.DB),
				Upstream:             nexusClient,
				Ledger:               ledgerService,
				BalanceCache:         game.NewRedisBalanceCache(deps.Redis),
				MinTransactionAmount: cfg.Business.MinTransactionAmount,
			}),
		).Register(mux)
		audit.NewHandler(audit.NewService(audit.NewRepository(deps.DB)), authService).Register(mux)
	}

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, deps.Health.Liveness())
	})

	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, deps.Health.Liveness())
	})

	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		report := deps.Health.Readiness(r.Context())
		status := http.StatusOK
		if report.Status != "ready" {
			status = http.StatusServiceUnavailable
		}

		writeJSON(w, status, report)
	})

	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
		report := deps.Health.Readiness(r.Context())
		status := http.StatusOK
		if report.Status != "ready" {
			status = http.StatusServiceUnavailable
		}

		writeJSON(w, status, report)
	})

	mux.HandleFunc("GET /v1/ping", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"message": "pong",
		})
	})

	mux.HandleFunc("GET /", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, infoResponse{
			Name:  cfg.App.Name,
			Apps:  []string{"apps/api", "apps/worker", "apps/scheduler", "apps/web"},
			Docs:  []string{"docs/blueprint.md", "docs/database-final.md", "docs/API Qris & VA V3.postman_collection.json", "docs/nexusggr-openapi-3.1.yaml", "docs/Bank RTOL.json"},
			Ready: true,
		})
	})

	return middleware.RequestID(middleware.Logging(deps.Logger, mux))
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
