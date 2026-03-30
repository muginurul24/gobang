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
)

type RepositoryContract interface {
	AuthenticateStore(ctx context.Context, tokenHash string) (StoreScope, error)
	FindStoreMemberByUsername(ctx context.Context, storeID string, username string) (StoreMember, error)
	HasUpstreamUserCode(ctx context.Context, upstreamUserCode string) (bool, error)
	CreateStoreMember(ctx context.Context, params CreateStoreMemberParams) (StoreMember, error)
	FindGameTransactionByTrxID(ctx context.Context, storeID string, trxID string) (GameTransaction, error)
	CreateGameTransaction(ctx context.Context, params CreateGameTransactionParams) (GameTransaction, error)
	UpdateGameTransaction(ctx context.Context, params UpdateGameTransactionParams) (GameTransaction, error)
	InsertAuditLog(ctx context.Context, storeID string, action string, targetType string, targetID *string, payload map[string]any, ipAddress string, userAgent string, occurredAt time.Time) error
}

type UpstreamClient interface {
	UserCreate(ctx context.Context, input nexusggr.UserCreateInput) (nexusggr.UserCreateResult, error)
	UserDeposit(ctx context.Context, input nexusggr.TransferInput) (nexusggr.TransferResult, error)
}

type LedgerService interface {
	GetBalance(ctx context.Context, storeID string) (ledger.BalanceSnapshot, error)
	Reserve(ctx context.Context, storeID string, input ledger.ReserveInput) (ledger.ReservationResult, error)
	CommitReservation(ctx context.Context, storeID string, input ledger.CommitReservationInput) (ledger.CommitReservationResult, error)
	ReleaseReservation(ctx context.Context, storeID string, input ledger.ReleaseReservationInput) (ledger.ReservationResult, error)
}

type Service interface {
	CreateUser(ctx context.Context, storeToken string, input CreateUserInput, metadata RequestMetadata) (StoreMember, error)
	Deposit(ctx context.Context, storeToken string, input CreateDepositInput, metadata RequestMetadata) (DepositResult, error)
}

type Options struct {
	Repository           RepositoryContract
	Upstream             UpstreamClient
	Ledger               LedgerService
	Clock                clock.Clock
	MinTransactionAmount int64
}

type service struct {
	repository           RepositoryContract
	upstream             UpstreamClient
	ledger               LedgerService
	clock                clock.Clock
	codeFactory          func() (string, error)
	agentSignFactory     func() (string, error)
	minTransactionAmount money
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

	return &service{
		repository:           options.Repository,
		upstream:             upstream,
		ledger:               ledgerService,
		clock:                now,
		codeFactory:          newUpstreamUserCode,
		agentSignFactory:     newAgentSign,
		minTransactionAmount: money(options.MinTransactionAmount * 100),
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
				return StoreMember{}, ErrCodeGenerationExhausted
			}

			return StoreMember{}, err
		}

		if err := s.repository.InsertAuditLog(ctx, store.ID, "game.user_created", "store_member", &member.ID, map[string]any{
			"real_username":      member.RealUsername,
			"upstream_user_code": member.UpstreamUserCode,
			"origin":             "store_api",
		}, metadata.IPAddress, metadata.UserAgent, now); err != nil {
			return StoreMember{}, err
		}

		return member, nil
	}

	return StoreMember{}, ErrCodeGenerationExhausted
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

func (noopUpstream) UserCreate(context.Context, nexusggr.UserCreateInput) (nexusggr.UserCreateResult, error) {
	return nexusggr.UserCreateResult{}, nexusggr.ErrNotConfigured
}

func (noopUpstream) UserDeposit(context.Context, nexusggr.TransferInput) (nexusggr.TransferResult, error) {
	return nexusggr.TransferResult{}, nexusggr.ErrNotConfigured
}

type noopLedger struct{}

func (noopLedger) GetBalance(context.Context, string) (ledger.BalanceSnapshot, error) {
	return ledger.BalanceSnapshot{}, ledger.ErrNotFound
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
