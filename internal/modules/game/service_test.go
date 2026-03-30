package game

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/mugiew/onixggr/internal/modules/ledger"
	"github.com/mugiew/onixggr/internal/platform/nexusggr"
)

func TestCreateUserRejectsDuplicateUsernameBeforeUpstream(t *testing.T) {
	repository := newFakeRepository(time.Date(2026, 3, 30, 20, 0, 0, 0, time.UTC))
	repository.members["existing"] = StoreMember{
		ID:               "existing",
		StoreID:          "store-1",
		RealUsername:     "member-alpha",
		UpstreamUserCode: "ABCDEF123456",
		Status:           MemberStatusActive,
		CreatedAt:        repository.now,
		UpdatedAt:        repository.now,
	}
	upstream := &fakeUpstream{}

	service := NewService(Options{
		Repository: repository,
		Upstream:   upstream,
		Ledger:     newFakeLedger("100000.00"),
		Clock:      fixedClock{now: repository.now},
	}).(*service)
	service.codeFactory = func() (string, error) {
		return "ZXCVBN123456", nil
	}

	_, err := service.CreateUser(context.Background(), "store_live_demo", CreateUserInput{
		Username: "member-alpha",
	}, RequestMetadata{})
	if !errors.Is(err, ErrDuplicateUsername) {
		t.Fatalf("CreateUser error = %v, want ErrDuplicateUsername", err)
	}

	if upstream.calls != 0 {
		t.Fatalf("upstream calls = %d, want 0", upstream.calls)
	}
}

func TestCreateUserSavesMappingAfterUpstreamSuccess(t *testing.T) {
	repository := newFakeRepository(time.Date(2026, 3, 30, 20, 15, 0, 0, time.UTC))
	upstream := &fakeUpstream{}

	service := NewService(Options{
		Repository: repository,
		Upstream:   upstream,
		Ledger:     newFakeLedger("100000.00"),
		Clock:      fixedClock{now: repository.now},
	}).(*service)
	service.codeFactory = func() (string, error) {
		return "Q3RKJ1PC0ZMK", nil
	}

	member, err := service.CreateUser(context.Background(), "store_live_demo", CreateUserInput{
		Username: "member-beta",
	}, RequestMetadata{
		IPAddress: "127.0.0.1",
		UserAgent: "game-test",
	})
	if err != nil {
		t.Fatalf("CreateUser returned error: %v", err)
	}

	if member.RealUsername != "member-beta" {
		t.Fatalf("RealUsername = %s, want member-beta", member.RealUsername)
	}

	if member.UpstreamUserCode != "Q3RKJ1PC0ZMK" {
		t.Fatalf("UpstreamUserCode = %s, want Q3RKJ1PC0ZMK", member.UpstreamUserCode)
	}

	if upstream.calls != 1 {
		t.Fatalf("upstream calls = %d, want 1", upstream.calls)
	}

	if repository.auditActions[0] != "game.user_created" {
		t.Fatalf("audit action = %q, want game.user_created", repository.auditActions[0])
	}
}

func TestCreateUserRejectsInactiveStore(t *testing.T) {
	repository := newFakeRepository(time.Date(2026, 3, 30, 20, 30, 0, 0, time.UTC))
	repository.store.Status = "inactive"

	service := NewService(Options{
		Repository: repository,
		Upstream:   &fakeUpstream{},
		Ledger:     newFakeLedger("100000.00"),
		Clock:      fixedClock{now: repository.now},
	})

	_, err := service.CreateUser(context.Background(), "store_live_demo", CreateUserInput{
		Username: "member-gamma",
	}, RequestMetadata{})
	if !errors.Is(err, ErrStoreInactive) {
		t.Fatalf("CreateUser error = %v, want ErrStoreInactive", err)
	}
}

func TestDepositRejectsInsufficientBalance(t *testing.T) {
	repository := newFakeRepository(time.Date(2026, 3, 30, 21, 0, 0, 0, time.UTC))
	ledgerService := newFakeLedger("4999.00")
	upstream := &fakeUpstream{}

	service := NewService(Options{
		Repository:           repository,
		Upstream:             upstream,
		Ledger:               ledgerService,
		Clock:                fixedClock{now: repository.now},
		MinTransactionAmount: 5000,
	})

	_, err := service.Deposit(context.Background(), "store_live_demo", CreateDepositInput{
		Username: "member-demo",
		Amount:   json.Number("5000"),
		TrxID:    "trx-insufficient",
	}, RequestMetadata{})
	if !errors.Is(err, ErrInsufficientBalance) {
		t.Fatalf("Deposit error = %v, want ErrInsufficientBalance", err)
	}

	if upstream.depositCalls != 0 {
		t.Fatalf("deposit calls = %d, want 0", upstream.depositCalls)
	}
}

func TestDepositRejectsDuplicateTransactionID(t *testing.T) {
	repository := newFakeRepository(time.Date(2026, 3, 30, 21, 10, 0, 0, time.UTC))
	repository.transactions["existing"] = GameTransaction{
		ID:            "tx-existing",
		StoreID:       "store-1",
		StoreMemberID: "member-1",
		Action:        GameActionDeposit,
		TrxID:         "trx-duplicate",
		Amount:        "5000.00",
		Status:        TransactionStatusSuccess,
	}
	upstream := &fakeUpstream{}

	service := NewService(Options{
		Repository:           repository,
		Upstream:             upstream,
		Ledger:               newFakeLedger("100000.00"),
		Clock:                fixedClock{now: repository.now},
		MinTransactionAmount: 5000,
	})

	_, err := service.Deposit(context.Background(), "store_live_demo", CreateDepositInput{
		Username: "member-demo",
		Amount:   json.Number("5000"),
		TrxID:    "trx-duplicate",
	}, RequestMetadata{})
	if !errors.Is(err, ErrDuplicateTransactionID) {
		t.Fatalf("Deposit error = %v, want ErrDuplicateTransactionID", err)
	}

	if upstream.depositCalls != 0 {
		t.Fatalf("deposit calls = %d, want 0", upstream.depositCalls)
	}
}

func TestDepositSuccessDebitsLedger(t *testing.T) {
	repository := newFakeRepository(time.Date(2026, 3, 30, 21, 20, 0, 0, time.UTC))
	upstream := &fakeUpstream{}
	ledgerService := newFakeLedger("100000.00")

	service := NewService(Options{
		Repository:           repository,
		Upstream:             upstream,
		Ledger:               ledgerService,
		Clock:                fixedClock{now: repository.now},
		MinTransactionAmount: 5000,
	}).(*service)
	service.agentSignFactory = func() (string, error) {
		return "AGTDEPOSIT000001", nil
	}

	result, err := service.Deposit(context.Background(), "store_live_demo", CreateDepositInput{
		Username: "member-demo",
		Amount:   json.Number("5000"),
		TrxID:    "trx-success",
	}, RequestMetadata{
		IPAddress: "127.0.0.1",
		UserAgent: "game-deposit-test",
	})
	if err != nil {
		t.Fatalf("Deposit returned error: %v", err)
	}

	if result.Transaction.Status != TransactionStatusSuccess {
		t.Fatalf("Transaction.Status = %s, want success", result.Transaction.Status)
	}

	if ledgerService.commitCalls != 1 {
		t.Fatalf("commit calls = %d, want 1", ledgerService.commitCalls)
	}

	if result.Balance == nil || result.Balance.CurrentBalance != "95000.00" {
		t.Fatalf("Balance.CurrentBalance = %#v, want 95000.00", result.Balance)
	}
}

func TestDepositTimeoutMovesToPendingReconcile(t *testing.T) {
	repository := newFakeRepository(time.Date(2026, 3, 30, 21, 30, 0, 0, time.UTC))
	upstream := &fakeUpstream{depositErr: nexusggr.ErrTimeout}
	ledgerService := newFakeLedger("100000.00")

	service := NewService(Options{
		Repository:           repository,
		Upstream:             upstream,
		Ledger:               ledgerService,
		Clock:                fixedClock{now: repository.now},
		MinTransactionAmount: 5000,
	}).(*service)
	service.agentSignFactory = func() (string, error) {
		return "AGTTIMEOUT000001", nil
	}

	result, err := service.Deposit(context.Background(), "store_live_demo", CreateDepositInput{
		Username: "member-demo",
		Amount:   json.Number("5000"),
		TrxID:    "trx-timeout",
	}, RequestMetadata{
		IPAddress: "127.0.0.1",
		UserAgent: "game-timeout-test",
	})
	if err != nil {
		t.Fatalf("Deposit returned error: %v", err)
	}

	if result.Transaction.ReconcileStatus == nil || *result.Transaction.ReconcileStatus != ReconcileStatusPending {
		t.Fatalf("ReconcileStatus = %#v, want pending_reconcile", result.Transaction.ReconcileStatus)
	}

	if ledgerService.releaseCalls != 0 {
		t.Fatalf("release calls = %d, want 0", ledgerService.releaseCalls)
	}

	if repository.auditActions[len(repository.auditActions)-1] != "game.deposit_pending_reconcile" {
		t.Fatalf("last audit action = %q, want game.deposit_pending_reconcile", repository.auditActions[len(repository.auditActions)-1])
	}
}

func TestWithdrawSuccessCreditsLedger(t *testing.T) {
	repository := newFakeRepository(time.Date(2026, 3, 30, 22, 0, 0, 0, time.UTC))
	upstream := &fakeUpstream{}
	ledgerService := newFakeLedger("100000.00")

	service := NewService(Options{
		Repository:           repository,
		Upstream:             upstream,
		Ledger:               ledgerService,
		Clock:                fixedClock{now: repository.now},
		MinTransactionAmount: 5000,
	}).(*service)
	service.agentSignFactory = func() (string, error) {
		return "AGTWITHDRAW00001", nil
	}

	result, err := service.Withdraw(context.Background(), "store_live_demo", CreateWithdrawInput{
		Username: "member-demo",
		Amount:   json.Number("10000"),
		TrxID:    "trx-withdraw-success",
	}, RequestMetadata{
		IPAddress: "127.0.0.1",
		UserAgent: "game-withdraw-test",
	})
	if err != nil {
		t.Fatalf("Withdraw returned error: %v", err)
	}

	if result.Transaction.Status != TransactionStatusSuccess {
		t.Fatalf("Transaction.Status = %s, want success", result.Transaction.Status)
	}

	if result.Transaction.Action != GameActionWithdraw {
		t.Fatalf("Transaction.Action = %s, want withdraw", result.Transaction.Action)
	}

	if ledgerService.creditCalls != 1 {
		t.Fatalf("credit calls = %d, want 1", ledgerService.creditCalls)
	}

	if result.Balance == nil || result.Balance.CurrentBalance != "110000.00" {
		t.Fatalf("Balance.CurrentBalance = %#v, want 110000.00", result.Balance)
	}

	if upstream.withdrawCalls != 1 {
		t.Fatalf("withdraw calls = %d, want 1", upstream.withdrawCalls)
	}
}

func TestWithdrawTimeoutMovesToPendingReconcile(t *testing.T) {
	repository := newFakeRepository(time.Date(2026, 3, 30, 22, 10, 0, 0, time.UTC))
	upstream := &fakeUpstream{withdrawErr: nexusggr.ErrTimeout}
	ledgerService := newFakeLedger("100000.00")

	service := NewService(Options{
		Repository:           repository,
		Upstream:             upstream,
		Ledger:               ledgerService,
		Clock:                fixedClock{now: repository.now},
		MinTransactionAmount: 5000,
	}).(*service)
	service.agentSignFactory = func() (string, error) {
		return "AGTTIMEOUTWD0001", nil
	}

	result, err := service.Withdraw(context.Background(), "store_live_demo", CreateWithdrawInput{
		Username: "member-demo",
		Amount:   json.Number("8000"),
		TrxID:    "trx-withdraw-timeout",
	}, RequestMetadata{
		IPAddress: "127.0.0.1",
		UserAgent: "game-withdraw-timeout-test",
	})
	if err != nil {
		t.Fatalf("Withdraw returned error: %v", err)
	}

	if result.Transaction.ReconcileStatus == nil || *result.Transaction.ReconcileStatus != ReconcileStatusPending {
		t.Fatalf("ReconcileStatus = %#v, want pending_reconcile", result.Transaction.ReconcileStatus)
	}

	if ledgerService.creditCalls != 0 {
		t.Fatalf("credit calls = %d, want 0", ledgerService.creditCalls)
	}

	if repository.auditActions[len(repository.auditActions)-1] != "game.withdraw_pending_reconcile" {
		t.Fatalf("last audit action = %q, want game.withdraw_pending_reconcile", repository.auditActions[len(repository.auditActions)-1])
	}
}

func TestWithdrawRetrySameTrxIDReturnsOldResult(t *testing.T) {
	repository := newFakeRepository(time.Date(2026, 3, 30, 22, 20, 0, 0, time.UTC))
	repository.transactions["tx-existing-wd"] = GameTransaction{
		ID:               "tx-existing-wd",
		StoreID:          "store-1",
		StoreMemberID:    "member-1",
		Action:           GameActionWithdraw,
		TrxID:            "trx-withdraw-dup",
		UpstreamUserCode: "MEMBER000001",
		Amount:           "8000.00",
		Status:           TransactionStatusSuccess,
	}
	upstream := &fakeUpstream{}

	service := NewService(Options{
		Repository:           repository,
		Upstream:             upstream,
		Ledger:               newFakeLedger("100000.00"),
		Clock:                fixedClock{now: repository.now},
		MinTransactionAmount: 5000,
	})

	result, err := service.Withdraw(context.Background(), "store_live_demo", CreateWithdrawInput{
		Username: "member-demo",
		Amount:   json.Number("8000"),
		TrxID:    "trx-withdraw-dup",
	}, RequestMetadata{})
	if err != nil {
		t.Fatalf("Withdraw returned error: %v", err)
	}

	if result.Transaction.ID != "tx-existing-wd" {
		t.Fatalf("Transaction.ID = %s, want tx-existing-wd", result.Transaction.ID)
	}

	if upstream.withdrawCalls != 0 {
		t.Fatalf("withdraw calls = %d, want 0 (should be idempotent)", upstream.withdrawCalls)
	}
}

func TestWithdrawFailNoLedgerChange(t *testing.T) {
	repository := newFakeRepository(time.Date(2026, 3, 30, 22, 30, 0, 0, time.UTC))
	upstream := &fakeUpstream{withdrawErr: &nexusggr.BusinessError{Code: "USER_NOT_FOUND", Message: "user not found"}}
	ledgerService := newFakeLedger("100000.00")

	service := NewService(Options{
		Repository:           repository,
		Upstream:             upstream,
		Ledger:               ledgerService,
		Clock:                fixedClock{now: repository.now},
		MinTransactionAmount: 5000,
	}).(*service)
	service.agentSignFactory = func() (string, error) {
		return "AGTFAILWD0000001", nil
	}

	_, err := service.Withdraw(context.Background(), "store_live_demo", CreateWithdrawInput{
		Username: "member-demo",
		Amount:   json.Number("5000"),
		TrxID:    "trx-withdraw-fail",
	}, RequestMetadata{
		IPAddress: "127.0.0.1",
		UserAgent: "game-withdraw-fail-test",
	})

	var businessErr *nexusggr.BusinessError
	if !errors.As(err, &businessErr) {
		t.Fatalf("Withdraw error = %v, want BusinessError", err)
	}

	if ledgerService.creditCalls != 0 {
		t.Fatalf("credit calls = %d, want 0", ledgerService.creditCalls)
	}

	if repository.auditActions[len(repository.auditActions)-1] != "game.withdraw_failed" {
		t.Fatalf("last audit action = %q, want game.withdraw_failed", repository.auditActions[len(repository.auditActions)-1])
	}
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time {
	return c.now
}

type fakeRepository struct {
	now          time.Time
	store        StoreScope
	members      map[string]StoreMember
	transactions map[string]GameTransaction
	auditActions []string
}

func newFakeRepository(now time.Time) *fakeRepository {
	return &fakeRepository{
		now: now,
		store: StoreScope{
			ID:          "store-1",
			OwnerUserID: "owner-1",
			Name:        "Demo Store",
			Slug:        "demo-store",
			Status:      StoreStatusActive,
		},
		members: map[string]StoreMember{
			"member-1": {
				ID:               "member-1",
				StoreID:          "store-1",
				RealUsername:     "member-demo",
				UpstreamUserCode: "MEMBER000001",
				Status:           MemberStatusActive,
				CreatedAt:        now,
				UpdatedAt:        now,
			},
		},
		transactions: map[string]GameTransaction{},
	}
}

func (r *fakeRepository) AuthenticateStore(_ context.Context, tokenHash string) (StoreScope, error) {
	if tokenHash == "" {
		return StoreScope{}, ErrUnauthorized
	}

	return r.store, nil
}

func (r *fakeRepository) FindStoreMemberByUsername(_ context.Context, storeID string, username string) (StoreMember, error) {
	for _, member := range r.members {
		if member.StoreID == storeID && member.RealUsername == username {
			return member, nil
		}
	}

	return StoreMember{}, ErrNotFound
}

func (r *fakeRepository) HasUpstreamUserCode(_ context.Context, upstreamUserCode string) (bool, error) {
	for _, member := range r.members {
		if member.UpstreamUserCode == upstreamUserCode {
			return true, nil
		}
	}

	return false, nil
}

func (r *fakeRepository) CreateStoreMember(_ context.Context, params CreateStoreMemberParams) (StoreMember, error) {
	for _, member := range r.members {
		if member.StoreID == params.StoreID && member.RealUsername == params.RealUsername {
			return StoreMember{}, ErrDuplicateUsername
		}

		if member.UpstreamUserCode == params.UpstreamUserCode {
			return StoreMember{}, ErrDuplicateUpstreamUserCode
		}
	}

	member := StoreMember{
		ID:               "member-created",
		StoreID:          params.StoreID,
		RealUsername:     params.RealUsername,
		UpstreamUserCode: params.UpstreamUserCode,
		Status:           params.Status,
		CreatedAt:        params.OccurredAt,
		UpdatedAt:        params.OccurredAt,
	}
	r.members[member.ID] = member
	return member, nil
}

func (r *fakeRepository) FindGameTransactionByTrxID(_ context.Context, storeID string, trxID string) (GameTransaction, error) {
	for _, transaction := range r.transactions {
		if transaction.StoreID == storeID && transaction.TrxID == trxID {
			return transaction, nil
		}
	}

	return GameTransaction{}, ErrNotFound
}

func (r *fakeRepository) CreateGameTransaction(_ context.Context, params CreateGameTransactionParams) (GameTransaction, error) {
	for _, transaction := range r.transactions {
		if transaction.StoreID == params.StoreID && transaction.TrxID == params.TrxID {
			return GameTransaction{}, ErrDuplicateTransactionID
		}
		if transaction.AgentSign == params.AgentSign {
			return GameTransaction{}, ErrAgentSignExhausted
		}
	}

	transaction := GameTransaction{
		ID:               "transaction-created",
		StoreID:          params.StoreID,
		StoreMemberID:    params.StoreMemberID,
		Action:           params.Action,
		TrxID:            params.TrxID,
		UpstreamUserCode: params.UpstreamUserCode,
		Amount:           params.Amount,
		AgentSign:        params.AgentSign,
		Status:           params.Status,
		CreatedAt:        params.OccurredAt,
		UpdatedAt:        params.OccurredAt,
	}
	r.transactions[transaction.ID] = transaction
	return transaction, nil
}

func (r *fakeRepository) UpdateGameTransaction(_ context.Context, params UpdateGameTransactionParams) (GameTransaction, error) {
	transaction, ok := r.transactions[params.GameTransactionID]
	if !ok {
		return GameTransaction{}, ErrNotFound
	}

	transaction.Status = params.Status
	transaction.ReconcileStatus = params.ReconcileStatus
	transaction.UpstreamErrorCode = params.UpstreamErrorCode
	transaction.UpdatedAt = params.OccurredAt
	if params.UpstreamResponseMasked != nil {
		transaction.UpstreamResponse = json.RawMessage(toJSON(params.UpstreamResponseMasked))
	}
	r.transactions[transaction.ID] = transaction
	return transaction, nil
}

func (r *fakeRepository) InsertAuditLog(_ context.Context, _ string, action string, _ string, _ *string, _ map[string]any, _ string, _ string, _ time.Time) error {
	r.auditActions = append(r.auditActions, action)
	return nil
}

type fakeUpstream struct {
	calls         int
	err           error
	depositCalls  int
	depositErr    error
	withdrawCalls int
	withdrawErr   error
}

func (u *fakeUpstream) UserCreate(_ context.Context, input nexusggr.UserCreateInput) (nexusggr.UserCreateResult, error) {
	u.calls++
	if u.err != nil {
		return nexusggr.UserCreateResult{}, u.err
	}

	return nexusggr.UserCreateResult{
		Message:  "SUCCESS",
		UserCode: input.UserCode,
	}, nil
}

func (u *fakeUpstream) UserDeposit(_ context.Context, input nexusggr.TransferInput) (nexusggr.TransferResult, error) {
	u.depositCalls++
	if u.depositErr != nil {
		return nexusggr.TransferResult{}, u.depositErr
	}

	return nexusggr.TransferResult{
		Message:      "SUCCESS",
		AgentBalance: 250000,
		UserBalance:  input.Amount,
	}, nil
}

func (u *fakeUpstream) UserWithdraw(_ context.Context, input nexusggr.TransferInput) (nexusggr.TransferResult, error) {
	u.withdrawCalls++
	if u.withdrawErr != nil {
		return nexusggr.TransferResult{}, u.withdrawErr
	}

	return nexusggr.TransferResult{
		Message:      "SUCCESS",
		AgentBalance: 250000,
		UserBalance:  input.Amount,
	}, nil
}

type fakeLedger struct {
	balance      ledger.BalanceSnapshot
	reserveCalls int
	commitCalls  int
	releaseCalls int
	creditCalls  int
}

func newFakeLedger(available string) *fakeLedger {
	return &fakeLedger{
		balance: ledger.BalanceSnapshot{
			StoreID:          "store-1",
			LedgerAccountID:  "ledger-1",
			Currency:         "IDR",
			CurrentBalance:   available,
			ReservedAmount:   "0.00",
			AvailableBalance: available,
		},
	}
}

func (l *fakeLedger) GetBalance(_ context.Context, _ string) (ledger.BalanceSnapshot, error) {
	return l.balance, nil
}

func (l *fakeLedger) Credit(_ context.Context, _ string, input ledger.PostEntryInput) (ledger.PostingResult, error) {
	l.creditCalls++
	amount, _ := parseMoney(input.Amount)
	current, _ := parseMoney(l.balance.CurrentBalance)
	newBalance := money(int64(current) + int64(amount))
	l.balance.CurrentBalance = newBalance.String()
	l.balance.AvailableBalance = newBalance.String()
	return ledger.PostingResult{
		Balance: ledger.BalanceSnapshot{
			StoreID:          l.balance.StoreID,
			LedgerAccountID:  l.balance.LedgerAccountID,
			Currency:         l.balance.Currency,
			CurrentBalance:   l.balance.CurrentBalance,
			ReservedAmount:   l.balance.ReservedAmount,
			AvailableBalance: l.balance.AvailableBalance,
		},
	}, nil
}

func (l *fakeLedger) Reserve(_ context.Context, _ string, input ledger.ReserveInput) (ledger.ReservationResult, error) {
	l.reserveCalls++
	available, _ := parseMoney(l.balance.AvailableBalance)
	amount, _ := parseMoney(input.Amount)
	if available.LessThan(amount) {
		return ledger.ReservationResult{}, ledger.ErrInsufficientFunds
	}

	l.balance.ReservedAmount = amount.String()
	l.balance.AvailableBalance = available.Sub(amount).String()
	return ledger.ReservationResult{
		Balance: ledger.BalanceSnapshot{
			StoreID:          l.balance.StoreID,
			LedgerAccountID:  l.balance.LedgerAccountID,
			Currency:         l.balance.Currency,
			CurrentBalance:   l.balance.CurrentBalance,
			ReservedAmount:   l.balance.ReservedAmount,
			AvailableBalance: l.balance.AvailableBalance,
		},
	}, nil
}

func (l *fakeLedger) CommitReservation(_ context.Context, _ string, input ledger.CommitReservationInput) (ledger.CommitReservationResult, error) {
	l.commitCalls++
	amount, _ := parseMoney(input.Entries[0].Amount)
	current, _ := parseMoney(l.balance.CurrentBalance)
	l.balance.CurrentBalance = current.Sub(amount).String()
	l.balance.ReservedAmount = "0.00"
	l.balance.AvailableBalance = l.balance.CurrentBalance
	return ledger.CommitReservationResult{
		Balance: ledger.BalanceSnapshot{
			StoreID:          l.balance.StoreID,
			LedgerAccountID:  l.balance.LedgerAccountID,
			Currency:         l.balance.Currency,
			CurrentBalance:   l.balance.CurrentBalance,
			ReservedAmount:   l.balance.ReservedAmount,
			AvailableBalance: l.balance.AvailableBalance,
		},
	}, nil
}

func (l *fakeLedger) ReleaseReservation(_ context.Context, _ string, _ ledger.ReleaseReservationInput) (ledger.ReservationResult, error) {
	l.releaseCalls++
	l.balance.ReservedAmount = "0.00"
	l.balance.AvailableBalance = l.balance.CurrentBalance
	return ledger.ReservationResult{
		Balance: ledger.BalanceSnapshot{
			StoreID:          l.balance.StoreID,
			LedgerAccountID:  l.balance.LedgerAccountID,
			Currency:         l.balance.Currency,
			CurrentBalance:   l.balance.CurrentBalance,
			ReservedAmount:   l.balance.ReservedAmount,
			AvailableBalance: l.balance.AvailableBalance,
		},
	}, nil
}
