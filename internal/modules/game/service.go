package game

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mugiew/onixggr/internal/modules/ledger"
	"github.com/mugiew/onixggr/internal/platform/clock"
	"github.com/mugiew/onixggr/internal/platform/nexusggr"
	"github.com/mugiew/onixggr/internal/platform/security"
	"golang.org/x/sync/singleflight"
)

type RepositoryContract interface {
	AuthenticateStore(ctx context.Context, tokenHash string) (StoreScope, error)
	FindStoreMemberByUsername(ctx context.Context, storeID string, username string) (StoreMember, error)
	FindProviderGame(ctx context.Context, providerCode string, gameCode string) (ProviderGame, error)
	HasUpstreamUserCode(ctx context.Context, upstreamUserCode string) (bool, error)
	CreateStoreMember(ctx context.Context, params CreateStoreMemberParams) (StoreMember, error)
	FindGameTransactionByTrxID(ctx context.Context, storeID string, trxID string) (GameTransaction, error)
	CreateGameTransaction(ctx context.Context, params CreateGameTransactionParams) (GameTransaction, error)
	CreateGameLaunchLog(ctx context.Context, params CreateGameLaunchLogParams) error
	UpdateGameTransaction(ctx context.Context, params UpdateGameTransactionParams) (GameTransaction, error)
	InsertAuditLog(ctx context.Context, storeID string, action string, targetType string, targetID *string, payload map[string]any, ipAddress string, userAgent string, occurredAt time.Time) error
}

type UpstreamClient interface {
	MoneyInfo(ctx context.Context, input nexusggr.MoneyInfoInput) (nexusggr.MoneyInfoResult, error)
	GameLaunch(ctx context.Context, input nexusggr.GameLaunchInput) (nexusggr.GameLaunchResult, error)
	UserCreate(ctx context.Context, input nexusggr.UserCreateInput) (nexusggr.UserCreateResult, error)
	UserDeposit(ctx context.Context, input nexusggr.TransferInput) (nexusggr.TransferResult, error)
	UserWithdraw(ctx context.Context, input nexusggr.TransferInput) (nexusggr.TransferResult, error)
}

type LedgerService interface {
	GetBalance(ctx context.Context, storeID string) (ledger.BalanceSnapshot, error)
	Credit(ctx context.Context, storeID string, input ledger.PostEntryInput) (ledger.PostingResult, error)
	Reserve(ctx context.Context, storeID string, input ledger.ReserveInput) (ledger.ReservationResult, error)
	CommitReservation(ctx context.Context, storeID string, input ledger.CommitReservationInput) (ledger.CommitReservationResult, error)
	ReleaseReservation(ctx context.Context, storeID string, input ledger.ReleaseReservationInput) (ledger.ReservationResult, error)
}

type Service interface {
	CreateUser(ctx context.Context, storeToken string, input CreateUserInput, metadata RequestMetadata) (StoreMember, error)
	GetBalance(ctx context.Context, storeToken string, input CreateGameBalanceInput) (GameBalanceResult, error)
	Launch(ctx context.Context, storeToken string, input CreateLaunchInput, metadata RequestMetadata) (LaunchResult, error)
	Deposit(ctx context.Context, storeToken string, input CreateDepositInput, metadata RequestMetadata) (DepositResult, error)
	Withdraw(ctx context.Context, storeToken string, input CreateWithdrawInput, metadata RequestMetadata) (WithdrawResult, error)
}

type NotificationEmitter interface {
	Emit(storeID string, eventType string, title string, body string)
}

type Options struct {
	Repository           RepositoryContract
	Upstream             UpstreamClient
	Ledger               LedgerService
	BalanceCache         BalanceCache
	Notifications        NotificationEmitter
	Clock                clock.Clock
	MinTransactionAmount int64
}

type service struct {
	repository           RepositoryContract
	upstream             UpstreamClient
	ledger               LedgerService
	balanceCache         BalanceCache
	notifications        NotificationEmitter
	clock                clock.Clock
	codeFactory          func() (string, error)
	agentSignFactory     func() (string, error)
	minTransactionAmount money
	balanceTTL           time.Duration
	balanceGroup         singleflight.Group
}

func NewService(options Options) Service {
	now := options.Clock
	if now == nil {
		now = clock.SystemClock{}
	}

	upstream := options.Upstream
	if upstream == nil {
		upstream = noopUpstream{}
	}

	ledgerService := options.Ledger
	if ledgerService == nil {
		ledgerService = noopLedger{}
	}

	cache := options.BalanceCache
	if cache == nil {
		cache = noopBalanceCache{}
	}

	notifs := options.Notifications
	if notifs == nil {
		notifs = noopNotificationEmitter{}
	}

	return &service{
		repository:           options.Repository,
		upstream:             upstream,
		ledger:               ledgerService,
		balanceCache:         cache,
		notifications:        notifs,
		clock:                now,
		codeFactory:          newUpstreamUserCode,
		agentSignFactory:     newAgentSign,
		minTransactionAmount: money(options.MinTransactionAmount * 100),
		balanceTTL:           5 * time.Second,
	}
}

func (s *service) CreateUser(ctx context.Context, storeToken string, input CreateUserInput, metadata RequestMetadata) (StoreMember, error) {
	store, err := s.authenticateStore(ctx, storeToken)
	if err != nil {
		return StoreMember{}, err
	}

	username := normalizeUsername(input.Username)
	if !validUsername(username) {
		return StoreMember{}, ErrInvalidUsername
	}

	_, err = s.repository.FindStoreMemberByUsername(ctx, store.ID, username)
	switch {
	case err == nil:
		return StoreMember{}, ErrDuplicateUsername
	case errors.Is(err, ErrNotFound):
	default:
		return StoreMember{}, err
	}

	return s.createMemberMapping(ctx, store, username, metadata, "store_api")
}

func (s *service) GetBalance(ctx context.Context, storeToken string, input CreateGameBalanceInput) (GameBalanceResult, error) {
	store, err := s.authenticateStore(ctx, storeToken)
	if err != nil {
		return GameBalanceResult{}, err
	}

	username := normalizeUsername(input.Username)
	if !validUsername(username) {
		return GameBalanceResult{}, ErrInvalidUsername
	}

	member, err := s.repository.FindStoreMemberByUsername(ctx, store.ID, username)
	if err != nil {
		return GameBalanceResult{}, err
	}
	if member.Status != MemberStatusActive {
		return GameBalanceResult{}, ErrMemberInactive
	}

	cached, ok, err := s.balanceCache.Get(ctx, store.ID, member.ID)
	if err == nil && ok {
		return cached, nil
	}

	coalescedKey := store.ID + ":" + member.ID
	value, err, _ := s.balanceGroup.Do(coalescedKey, func() (any, error) {
		cached, ok, err := s.balanceCache.Get(ctx, store.ID, member.ID)
		if err == nil && ok {
			return cached, nil
		}

		upstreamResult, err := s.upstream.MoneyInfo(ctx, nexusggr.MoneyInfoInput{
			UserCode: member.UpstreamUserCode,
		})
		if err != nil {
			return GameBalanceResult{}, err
		}
		if upstreamResult.User == nil {
			return GameBalanceResult{}, nexusggr.ErrInvalidResponse
		}

		result := GameBalanceResult{
			Username:         member.RealUsername,
			UpstreamUserCode: member.UpstreamUserCode,
			Balance:          moneyFromFloat64(upstreamResult.User.Balance).String(),
		}
		_ = s.balanceCache.Set(ctx, store.ID, member.ID, result, s.balanceTTL)

		return result, nil
	})
	if err != nil {
		return GameBalanceResult{}, err
	}

	result, ok := value.(GameBalanceResult)
	if !ok {
		return GameBalanceResult{}, nexusggr.ErrInvalidResponse
	}

	return result, nil
}

func (s *service) Launch(ctx context.Context, storeToken string, input CreateLaunchInput, metadata RequestMetadata) (LaunchResult, error) {
	store, err := s.authenticateStore(ctx, storeToken)
	if err != nil {
		return LaunchResult{}, err
	}

	username := normalizeUsername(input.Username)
	if !validUsername(username) {
		return LaunchResult{}, ErrInvalidUsername
	}

	providerCode := normalizeProviderCode(input.ProviderCode)
	gameCode := normalizeGameCode(input.GameCode)
	if providerCode == "" || gameCode == "" {
		return LaunchResult{}, ErrInvalidProviderGame
	}

	lang := normalizeLang(input.Lang)

	if _, err := s.repository.FindProviderGame(ctx, providerCode, gameCode); err != nil {
		if errors.Is(err, ErrNotFound) {
			return LaunchResult{}, ErrInvalidProviderGame
		}

		return LaunchResult{}, err
	}

	member, err := s.findOrCreateLaunchMember(ctx, store, username, metadata)
	if err != nil {
		return LaunchResult{}, err
	}

	now := s.clock.Now().UTC()
	upstreamResult, err := s.upstream.GameLaunch(ctx, nexusggr.GameLaunchInput{
		UserCode:     member.UpstreamUserCode,
		ProviderCode: providerCode,
		GameCode:     gameCode,
		Lang:         lang,
	})
	if err != nil {
		if logErr := s.repository.CreateGameLaunchLog(ctx, CreateGameLaunchLogParams{
			StoreID:       store.ID,
			StoreMemberID: member.ID,
			ProviderCode:  providerCode,
			GameCode:      gameCode,
			Lang:          lang,
			Status:        "failed",
			UpstreamPayloadMasked: map[string]any{
				"error": err.Error(),
			},
			OccurredAt: now,
		}); logErr != nil {
			return LaunchResult{}, logErr
		}

		return LaunchResult{}, err
	}

	if err := s.repository.CreateGameLaunchLog(ctx, CreateGameLaunchLogParams{
		StoreID:       store.ID,
		StoreMemberID: member.ID,
		ProviderCode:  providerCode,
		GameCode:      gameCode,
		Lang:          lang,
		Status:        "success",
		UpstreamPayloadMasked: map[string]any{
			"message":    upstreamResult.Message,
			"launch_url": upstreamResult.LaunchURL,
		},
		OccurredAt: now,
	}); err != nil {
		return LaunchResult{}, err
	}

	return LaunchResult{
		Username:         member.RealUsername,
		UpstreamUserCode: member.UpstreamUserCode,
		ProviderCode:     providerCode,
		GameCode:         gameCode,
		Lang:             lang,
		LaunchURL:        upstreamResult.LaunchURL,
	}, nil
}

func (s *service) Deposit(ctx context.Context, storeToken string, input CreateDepositInput, metadata RequestMetadata) (DepositResult, error) {
	store, err := s.authenticateStore(ctx, storeToken)
	if err != nil {
		return DepositResult{}, err
	}

	username := normalizeUsername(input.Username)
	if !validUsername(username) {
		return DepositResult{}, ErrInvalidUsername
	}

	trxID := normalizeTransactionID(input.TrxID)
	if trxID == "" {
		return DepositResult{}, ErrInvalidTransactionID
	}

	amount, err := parseMoney(input.Amount.String())
	if err != nil || amount.LessThan(1) || amount.LessThan(s.minTransactionAmount) {
		return DepositResult{}, ErrInvalidAmount
	}

	member, err := s.repository.FindStoreMemberByUsername(ctx, store.ID, username)
	if err != nil {
		return DepositResult{}, err
	}
	if member.Status != MemberStatusActive {
		return DepositResult{}, ErrMemberInactive
	}

	_, err = s.repository.FindGameTransactionByTrxID(ctx, store.ID, trxID)
	switch {
	case err == nil:
		return DepositResult{}, ErrDuplicateTransactionID
	case errors.Is(err, ErrNotFound):
	default:
		return DepositResult{}, err
	}

	balance, err := s.ledger.GetBalance(ctx, store.ID)
	if err != nil {
		return DepositResult{}, err
	}

	availableBalance, err := parseMoney(balance.AvailableBalance)
	if err != nil {
		return DepositResult{}, err
	}
	if availableBalance.LessThan(amount) {
		return DepositResult{}, ErrInsufficientBalance
	}

	now := s.clock.Now().UTC()
	transaction, err := s.createPendingDeposit(ctx, store, member, trxID, amount, now)
	if err != nil {
		return DepositResult{}, err
	}

	if _, err := s.ledger.Reserve(ctx, store.ID, ledger.ReserveInput{
		Amount:        amount.String(),
		ReferenceType: "game_transaction",
		ReferenceID:   transaction.ID,
	}); err != nil {
		if errors.Is(err, ledger.ErrInsufficientFunds) {
			if _, updateErr := s.markFailed(ctx, transaction.ID, "INSUFFICIENT_BALANCE", map[string]any{
				"message": "insufficient balance during reserve",
			}, now); updateErr != nil {
				return DepositResult{}, updateErr
			}

			return DepositResult{}, ErrInsufficientBalance
		}

		return DepositResult{}, err
	}

	upstreamResult, err := s.upstream.UserDeposit(ctx, nexusggr.TransferInput{
		UserCode:  member.UpstreamUserCode,
		Amount:    amount.Float64(),
		AgentSign: transaction.AgentSign,
	})
	if err == nil {
		commitResult, commitErr := s.ledger.CommitReservation(ctx, store.ID, ledger.CommitReservationInput{
			ReferenceType: "game_transaction",
			ReferenceID:   transaction.ID,
			Entries: []ledger.ReservationCommitEntryInput{
				{
					EntryType: ledger.EntryTypeGameDeposit,
					Amount:    amount.String(),
					Metadata: map[string]any{
						"trx_id":             transaction.TrxID,
						"real_username":      member.RealUsername,
						"upstream_user_code": member.UpstreamUserCode,
					},
				},
			},
		})
		if commitErr != nil {
			pending, pendingErr := s.markPendingReconcile(ctx, transaction.ID, nil, map[string]any{
				"message":       upstreamResult.Message,
				"agent_balance": upstreamResult.AgentBalance,
				"user_balance":  upstreamResult.UserBalance,
				"reason":        "ledger_commit_failed",
			}, now)
			if pendingErr != nil {
				return DepositResult{}, pendingErr
			}

			if auditErr := s.repository.InsertAuditLog(ctx, store.ID, "game.deposit_pending_reconcile", "game_transaction", &pending.ID, map[string]any{
				"trx_id": pending.TrxID,
				"reason": "ledger_commit_failed",
			}, metadata.IPAddress, metadata.UserAgent, now); auditErr != nil {
				return DepositResult{}, auditErr
			}

			return DepositResult{Transaction: pending}, nil
		}

		transaction, err = s.repository.UpdateGameTransaction(ctx, UpdateGameTransactionParams{
			GameTransactionID: transaction.ID,
			Status:            TransactionStatusSuccess,
			UpstreamResponseMasked: map[string]any{
				"message":       upstreamResult.Message,
				"agent_balance": upstreamResult.AgentBalance,
				"user_balance":  upstreamResult.UserBalance,
			},
			OccurredAt: now,
		})
		if err != nil {
			return DepositResult{}, err
		}

		if err := s.repository.InsertAuditLog(ctx, store.ID, "game.deposit_success", "game_transaction", &transaction.ID, map[string]any{
			"trx_id":             transaction.TrxID,
			"real_username":      member.RealUsername,
			"amount":             amount.String(),
			"upstream_user_code": transaction.UpstreamUserCode,
		}, metadata.IPAddress, metadata.UserAgent, now); err != nil {
			return DepositResult{}, err
		}

		s.notifications.Emit(store.ID, "game.deposit.success",
			"Game deposit berhasil",
			fmt.Sprintf("Deposit %s untuk %s berhasil (trx: %s)", amount.String(), member.RealUsername, transaction.TrxID),
		)

		return DepositResult{
			Transaction: transaction,
			Balance: &BalanceSnapshot{
				StoreID:          commitResult.Balance.StoreID,
				LedgerAccountID:  commitResult.Balance.LedgerAccountID,
				Currency:         commitResult.Balance.Currency,
				CurrentBalance:   commitResult.Balance.CurrentBalance,
				ReservedAmount:   commitResult.Balance.ReservedAmount,
				AvailableBalance: commitResult.Balance.AvailableBalance,
			},
		}, nil
	}

	var businessErr *nexusggr.BusinessError
	switch {
	case errors.As(err, &businessErr), errors.Is(err, nexusggr.ErrNotConfigured):
		if _, releaseErr := s.ledger.ReleaseReservation(ctx, store.ID, ledger.ReleaseReservationInput{
			ReferenceType: "game_transaction",
			ReferenceID:   transaction.ID,
		}); releaseErr != nil {
			return DepositResult{}, releaseErr
		}

		code := "UPSTREAM_NOT_CONFIGURED"
		response := map[string]any{}
		if errors.As(err, &businessErr) {
			code = businessErr.Code
			response["message"] = businessErr.Message
			response["code"] = businessErr.Code
		}

		transaction, updateErr := s.markFailed(ctx, transaction.ID, code, response, now)
		if updateErr != nil {
			return DepositResult{}, updateErr
		}

		if auditErr := s.repository.InsertAuditLog(ctx, store.ID, "game.deposit_failed", "game_transaction", &transaction.ID, map[string]any{
			"trx_id": transaction.TrxID,
			"code":   code,
		}, metadata.IPAddress, metadata.UserAgent, now); auditErr != nil {
			return DepositResult{}, auditErr
		}

		return DepositResult{}, err
	case errors.Is(err, nexusggr.ErrTimeout), errors.Is(err, nexusggr.ErrUpstreamUnavailable), errors.Is(err, nexusggr.ErrUnexpectedHTTP), errors.Is(err, nexusggr.ErrInvalidResponse):
		transaction, updateErr := s.markPendingReconcile(ctx, transaction.ID, nil, map[string]any{
			"reason": err.Error(),
		}, now)
		if updateErr != nil {
			return DepositResult{}, updateErr
		}

		if auditErr := s.repository.InsertAuditLog(ctx, store.ID, "game.deposit_pending_reconcile", "game_transaction", &transaction.ID, map[string]any{
			"trx_id": transaction.TrxID,
			"reason": err.Error(),
		}, metadata.IPAddress, metadata.UserAgent, now); auditErr != nil {
			return DepositResult{}, auditErr
		}

		return DepositResult{Transaction: transaction}, nil
	default:
		return DepositResult{}, err
	}
}

func (s *service) Withdraw(ctx context.Context, storeToken string, input CreateWithdrawInput, metadata RequestMetadata) (WithdrawResult, error) {
	store, err := s.authenticateStore(ctx, storeToken)
	if err != nil {
		return WithdrawResult{}, err
	}

	username := normalizeUsername(input.Username)
	if !validUsername(username) {
		return WithdrawResult{}, ErrInvalidUsername
	}

	trxID := normalizeTransactionID(input.TrxID)
	if trxID == "" {
		return WithdrawResult{}, ErrInvalidTransactionID
	}

	amount, err := parseMoney(input.Amount.String())
	if err != nil || amount.LessThan(1) || amount.LessThan(s.minTransactionAmount) {
		return WithdrawResult{}, ErrInvalidAmount
	}

	member, err := s.repository.FindStoreMemberByUsername(ctx, store.ID, username)
	if err != nil {
		return WithdrawResult{}, err
	}
	if member.Status != MemberStatusActive {
		return WithdrawResult{}, ErrMemberInactive
	}

	// Idempotency: if same trx_id already exists, return the old result.
	existing, err := s.repository.FindGameTransactionByTrxID(ctx, store.ID, trxID)
	switch {
	case err == nil:
		return WithdrawResult{Transaction: existing}, nil
	case errors.Is(err, ErrNotFound):
	default:
		return WithdrawResult{}, err
	}

	now := s.clock.Now().UTC()
	transaction, err := s.createPendingWithdraw(ctx, store, member, trxID, amount, now)
	if err != nil {
		return WithdrawResult{}, err
	}

	upstreamResult, err := s.upstream.UserWithdraw(ctx, nexusggr.TransferInput{
		UserCode:  member.UpstreamUserCode,
		Amount:    amount.Float64(),
		AgentSign: transaction.AgentSign,
	})
	if err == nil {
		creditResult, creditErr := s.ledger.Credit(ctx, store.ID, ledger.PostEntryInput{
			EntryType:     ledger.EntryTypeGameWithdraw,
			Amount:        amount.String(),
			ReferenceType: "game_transaction",
			ReferenceID:   transaction.ID,
			Metadata: map[string]any{
				"trx_id":             transaction.TrxID,
				"real_username":      member.RealUsername,
				"upstream_user_code": member.UpstreamUserCode,
			},
		})
		if creditErr != nil {
			pending, pendingErr := s.markPendingReconcile(ctx, transaction.ID, nil, map[string]any{
				"message":       upstreamResult.Message,
				"agent_balance": upstreamResult.AgentBalance,
				"user_balance":  upstreamResult.UserBalance,
				"reason":        "ledger_credit_failed",
			}, now)
			if pendingErr != nil {
				return WithdrawResult{}, pendingErr
			}

			if auditErr := s.repository.InsertAuditLog(ctx, store.ID, "game.withdraw_pending_reconcile", "game_transaction", &pending.ID, map[string]any{
				"trx_id": pending.TrxID,
				"reason": "ledger_credit_failed",
			}, metadata.IPAddress, metadata.UserAgent, now); auditErr != nil {
				return WithdrawResult{}, auditErr
			}

			return WithdrawResult{Transaction: pending}, nil
		}

		transaction, err = s.repository.UpdateGameTransaction(ctx, UpdateGameTransactionParams{
			GameTransactionID: transaction.ID,
			Status:            TransactionStatusSuccess,
			UpstreamResponseMasked: map[string]any{
				"message":       upstreamResult.Message,
				"agent_balance": upstreamResult.AgentBalance,
				"user_balance":  upstreamResult.UserBalance,
			},
			OccurredAt: now,
		})
		if err != nil {
			return WithdrawResult{}, err
		}

		if err := s.repository.InsertAuditLog(ctx, store.ID, "game.withdraw_success", "game_transaction", &transaction.ID, map[string]any{
			"trx_id":             transaction.TrxID,
			"real_username":      member.RealUsername,
			"amount":             amount.String(),
			"upstream_user_code": transaction.UpstreamUserCode,
		}, metadata.IPAddress, metadata.UserAgent, now); err != nil {
			return WithdrawResult{}, err
		}

		s.notifications.Emit(store.ID, "game.withdraw.success",
			"Game withdraw berhasil",
			fmt.Sprintf("Withdraw %s untuk %s berhasil (trx: %s)", amount.String(), member.RealUsername, transaction.TrxID),
		)

		return WithdrawResult{
			Transaction: transaction,
			Balance: &BalanceSnapshot{
				StoreID:          creditResult.Balance.StoreID,
				LedgerAccountID:  creditResult.Balance.LedgerAccountID,
				Currency:         creditResult.Balance.Currency,
				CurrentBalance:   creditResult.Balance.CurrentBalance,
				ReservedAmount:   creditResult.Balance.ReservedAmount,
				AvailableBalance: creditResult.Balance.AvailableBalance,
			},
		}, nil
	}

	var businessErr *nexusggr.BusinessError
	switch {
	case errors.As(err, &businessErr), errors.Is(err, nexusggr.ErrNotConfigured):
		code := "UPSTREAM_NOT_CONFIGURED"
		response := map[string]any{}
		if errors.As(err, &businessErr) {
			code = businessErr.Code
			response["message"] = businessErr.Message
			response["code"] = businessErr.Code
		}

		transaction, updateErr := s.markFailed(ctx, transaction.ID, code, response, now)
		if updateErr != nil {
			return WithdrawResult{}, updateErr
		}

		if auditErr := s.repository.InsertAuditLog(ctx, store.ID, "game.withdraw_failed", "game_transaction", &transaction.ID, map[string]any{
			"trx_id": transaction.TrxID,
			"code":   code,
		}, metadata.IPAddress, metadata.UserAgent, now); auditErr != nil {
			return WithdrawResult{}, auditErr
		}

		return WithdrawResult{}, err
	case errors.Is(err, nexusggr.ErrTimeout), errors.Is(err, nexusggr.ErrUpstreamUnavailable), errors.Is(err, nexusggr.ErrUnexpectedHTTP), errors.Is(err, nexusggr.ErrInvalidResponse):
		transaction, updateErr := s.markPendingReconcile(ctx, transaction.ID, nil, map[string]any{
			"reason": err.Error(),
		}, now)
		if updateErr != nil {
			return WithdrawResult{}, updateErr
		}

		if auditErr := s.repository.InsertAuditLog(ctx, store.ID, "game.withdraw_pending_reconcile", "game_transaction", &transaction.ID, map[string]any{
			"trx_id": transaction.TrxID,
			"reason": err.Error(),
		}, metadata.IPAddress, metadata.UserAgent, now); auditErr != nil {
			return WithdrawResult{}, auditErr
		}

		return WithdrawResult{Transaction: transaction}, nil
	default:
		return WithdrawResult{}, err
	}
}

func (s *service) authenticateStore(ctx context.Context, storeToken string) (StoreScope, error) {
	token := strings.TrimSpace(storeToken)
	if token == "" {
		return StoreScope{}, ErrUnauthorized
	}

	store, err := s.repository.AuthenticateStore(ctx, security.HashStoreToken(token))
	if err != nil {
		return StoreScope{}, err
	}

	if store.DeletedAt != nil || store.Status != StoreStatusActive {
		return StoreScope{}, ErrStoreInactive
	}

	return store, nil
}

func (s *service) findOrCreateLaunchMember(ctx context.Context, store StoreScope, username string, metadata RequestMetadata) (StoreMember, error) {
	member, err := s.repository.FindStoreMemberByUsername(ctx, store.ID, username)
	switch {
	case err == nil:
		if member.Status != MemberStatusActive {
			return StoreMember{}, ErrMemberInactive
		}

		return member, nil
	case !errors.Is(err, ErrNotFound):
		return StoreMember{}, err
	}

	member, err = s.createMemberMapping(ctx, store, username, metadata, "launch_auto_create")
	if err != nil {
		if errors.Is(err, ErrDuplicateUsername) {
			member, findErr := s.repository.FindStoreMemberByUsername(ctx, store.ID, username)
			if findErr != nil {
				return StoreMember{}, findErr
			}
			if member.Status != MemberStatusActive {
				return StoreMember{}, ErrMemberInactive
			}

			return member, nil
		}

		return StoreMember{}, err
	}

	return member, nil
}

func (s *service) createMemberMapping(ctx context.Context, store StoreScope, username string, metadata RequestMetadata, origin string) (StoreMember, error) {
	now := s.clock.Now().UTC()
	for range 8 {
		upstreamUserCode, err := s.codeFactory()
		if err != nil {
			return StoreMember{}, fmt.Errorf("generate upstream user code: %w", err)
		}

		exists, err := s.repository.HasUpstreamUserCode(ctx, upstreamUserCode)
		if err != nil {
			return StoreMember{}, err
		}
		if exists {
			continue
		}

		if _, err := s.upstream.UserCreate(ctx, nexusggr.UserCreateInput{
			UserCode: upstreamUserCode,
		}); err != nil {
			return StoreMember{}, err
		}

		member, err := s.repository.CreateStoreMember(ctx, CreateStoreMemberParams{
			StoreID:          store.ID,
			RealUsername:     username,
			UpstreamUserCode: upstreamUserCode,
			Status:           MemberStatusActive,
			OccurredAt:       now,
		})
		if err != nil {
			if errors.Is(err, ErrDuplicateUsername) {
				return StoreMember{}, err
			}

			if errors.Is(err, ErrDuplicateUpstreamUserCode) {
				continue
			}

			return StoreMember{}, err
		}

		if err := s.repository.InsertAuditLog(ctx, store.ID, "game.user_created", "store_member", &member.ID, map[string]any{
			"real_username":      member.RealUsername,
			"upstream_user_code": member.UpstreamUserCode,
			"origin":             origin,
		}, metadata.IPAddress, metadata.UserAgent, now); err != nil {
			return StoreMember{}, err
		}

		return member, nil
	}

	return StoreMember{}, ErrCodeGenerationExhausted
}

func (s *service) createPendingDeposit(ctx context.Context, store StoreScope, member StoreMember, trxID string, amount money, occurredAt time.Time) (GameTransaction, error) {
	for range 8 {
		agentSign, err := s.agentSignFactory()
		if err != nil {
			return GameTransaction{}, fmt.Errorf("generate agent sign: %w", err)
		}

		transaction, err := s.repository.CreateGameTransaction(ctx, CreateGameTransactionParams{
			StoreID:          store.ID,
			StoreMemberID:    member.ID,
			Action:           GameActionDeposit,
			TrxID:            trxID,
			UpstreamUserCode: member.UpstreamUserCode,
			Amount:           amount.String(),
			AgentSign:        agentSign,
			Status:           TransactionStatusPending,
			OccurredAt:       occurredAt,
		})
		if err == nil {
			return transaction, nil
		}

		if errors.Is(err, ErrAgentSignExhausted) {
			continue
		}

		return GameTransaction{}, err
	}

	return GameTransaction{}, ErrAgentSignExhausted
}

func (s *service) createPendingWithdraw(ctx context.Context, store StoreScope, member StoreMember, trxID string, amount money, occurredAt time.Time) (GameTransaction, error) {
	for range 8 {
		agentSign, err := s.agentSignFactory()
		if err != nil {
			return GameTransaction{}, fmt.Errorf("generate agent sign: %w", err)
		}

		transaction, err := s.repository.CreateGameTransaction(ctx, CreateGameTransactionParams{
			StoreID:          store.ID,
			StoreMemberID:    member.ID,
			Action:           GameActionWithdraw,
			TrxID:            trxID,
			UpstreamUserCode: member.UpstreamUserCode,
			Amount:           amount.String(),
			AgentSign:        agentSign,
			Status:           TransactionStatusPending,
			OccurredAt:       occurredAt,
		})
		if err == nil {
			return transaction, nil
		}

		if errors.Is(err, ErrAgentSignExhausted) {
			continue
		}

		return GameTransaction{}, err
	}

	return GameTransaction{}, ErrAgentSignExhausted
}

func (s *service) markFailed(ctx context.Context, transactionID string, code string, response map[string]any, occurredAt time.Time) (GameTransaction, error) {
	return s.repository.UpdateGameTransaction(ctx, UpdateGameTransactionParams{
		GameTransactionID:      transactionID,
		Status:                 TransactionStatusFailed,
		UpstreamErrorCode:      nullableString(code),
		UpstreamResponseMasked: response,
		OccurredAt:             occurredAt,
	})
}

func (s *service) markPendingReconcile(ctx context.Context, transactionID string, code *string, response map[string]any, occurredAt time.Time) (GameTransaction, error) {
	status := ReconcileStatusPending
	return s.repository.UpdateGameTransaction(ctx, UpdateGameTransactionParams{
		GameTransactionID:      transactionID,
		Status:                 TransactionStatusPending,
		ReconcileStatus:        &status,
		UpstreamErrorCode:      code,
		UpstreamResponseMasked: response,
		OccurredAt:             occurredAt,
	})
}

type noopUpstream struct{}

func (noopUpstream) MoneyInfo(context.Context, nexusggr.MoneyInfoInput) (nexusggr.MoneyInfoResult, error) {
	return nexusggr.MoneyInfoResult{}, nexusggr.ErrNotConfigured
}

func (noopUpstream) GameLaunch(context.Context, nexusggr.GameLaunchInput) (nexusggr.GameLaunchResult, error) {
	return nexusggr.GameLaunchResult{}, nexusggr.ErrNotConfigured
}

func (noopUpstream) UserCreate(context.Context, nexusggr.UserCreateInput) (nexusggr.UserCreateResult, error) {
	return nexusggr.UserCreateResult{}, nexusggr.ErrNotConfigured
}

func (noopUpstream) UserDeposit(context.Context, nexusggr.TransferInput) (nexusggr.TransferResult, error) {
	return nexusggr.TransferResult{}, nexusggr.ErrNotConfigured
}

func (noopUpstream) UserWithdraw(context.Context, nexusggr.TransferInput) (nexusggr.TransferResult, error) {
	return nexusggr.TransferResult{}, nexusggr.ErrNotConfigured
}

type noopLedger struct{}

func (noopLedger) GetBalance(context.Context, string) (ledger.BalanceSnapshot, error) {
	return ledger.BalanceSnapshot{}, ledger.ErrNotFound
}

func (noopLedger) Credit(context.Context, string, ledger.PostEntryInput) (ledger.PostingResult, error) {
	return ledger.PostingResult{}, ledger.ErrNotFound
}

func (noopLedger) Reserve(context.Context, string, ledger.ReserveInput) (ledger.ReservationResult, error) {
	return ledger.ReservationResult{}, ledger.ErrNotFound
}

func (noopLedger) CommitReservation(context.Context, string, ledger.CommitReservationInput) (ledger.CommitReservationResult, error) {
	return ledger.CommitReservationResult{}, ledger.ErrNotFound
}

func (noopLedger) ReleaseReservation(context.Context, string, ledger.ReleaseReservationInput) (ledger.ReservationResult, error) {
	return ledger.ReservationResult{}, ledger.ErrNotFound
}

type noopNotificationEmitter struct{}

func (noopNotificationEmitter) Emit(string, string, string, string) {}
