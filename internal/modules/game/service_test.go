package game

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
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

func TestGetBalanceCachesForFiveSeconds(t *testing.T) {
	repository := newFakeRepository(time.Date(2026, 3, 30, 20, 45, 0, 0, time.UTC))
	upstream := &fakeUpstream{
		moneyInfoResult: nexusggr.MoneyInfoResult{
			Message: "SUCCESS",
			User: &nexusggr.Balance{
				UserCode: "MEMBER000001",
				Balance:  1234.56,
			},
		},
	}
	cache := newFakeBalanceCache()

	service := NewService(Options{
		Repository:   repository,
		Upstream:     upstream,
		Ledger:       newFakeLedger("100000.00"),
		BalanceCache: cache,
		Clock:        fixedClock{now: repository.now},
	})

	first, err := service.GetBalance(context.Background(), "store_live_demo", CreateGameBalanceInput{
		Username: "member-demo",
	})
	if err != nil {
		t.Fatalf("GetBalance first call returned error: %v", err)
	}

	second, err := service.GetBalance(context.Background(), "store_live_demo", CreateGameBalanceInput{
		Username: "member-demo",
	})
	if err != nil {
		t.Fatalf("GetBalance second call returned error: %v", err)
	}

	if first.Balance != "1234.56" || second.Balance != "1234.56" {
		t.Fatalf("Balance results = %#v / %#v, want 1234.56", first, second)
	}

	if upstream.moneyInfoCalls != 1 {
		t.Fatalf("money info calls = %d, want 1", upstream.moneyInfoCalls)
	}

	if cache.lastTTL != 5*time.Second {
		t.Fatalf("cache ttl = %s, want 5s", cache.lastTTL)
	}
}

func TestGetBalanceCoalescesConcurrentRequests(t *testing.T) {
	repository := newFakeRepository(time.Date(2026, 3, 30, 20, 50, 0, 0, time.UTC))
	wait := make(chan struct{})
	upstream := &fakeUpstream{
		moneyInfoResult: nexusggr.MoneyInfoResult{
			Message: "SUCCESS",
			User: &nexusggr.Balance{
				UserCode: "MEMBER000001",
				Balance:  777.00,
			},
		},
		moneyInfoWait: wait,
	}

	service := NewService(Options{
		Repository:   repository,
		Upstream:     upstream,
		Ledger:       newFakeLedger("100000.00"),
		BalanceCache: newFakeBalanceCache(),
		Clock:        fixedClock{now: repository.now},
	})

	var wg sync.WaitGroup
	results := make([]GameBalanceResult, 2)
	errs := make([]error, 2)
	for index := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = service.GetBalance(context.Background(), "store_live_demo", CreateGameBalanceInput{
				Username: "member-demo",
			})
		}(index)
	}

	time.Sleep(20 * time.Millisecond)
	close(wait)
	wg.Wait()

	for index, err := range errs {
		if err != nil {
			t.Fatalf("GetBalance call %d returned error: %v", index, err)
		}
	}

	if upstream.moneyInfoCalls != 1 {
		t.Fatalf("money info calls = %d, want 1", upstream.moneyInfoCalls)
	}

	if results[0].Balance != "777.00" || results[1].Balance != "777.00" {
		t.Fatalf("coalesced results = %#v", results)
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

func TestLaunchAutoCreatesMemberWithoutDeposit(t *testing.T) {
	repository := newFakeRepository(time.Date(2026, 3, 30, 22, 40, 0, 0, time.UTC))
	repository.members = map[string]StoreMember{}
	upstream := &fakeUpstream{
		launchResult: nexusggr.GameLaunchResult{
			Message:   "SUCCESS",
			LaunchURL: "https://launch.example/session",
		},
	}

	service := NewService(Options{
		Repository: repository,
		Upstream:   upstream,
		Ledger:     newFakeLedger("0.00"),
		Clock:      fixedClock{now: repository.now},
	}).(*service)
	service.codeFactory = func() (string, error) {
		return "AUTOUSER0001", nil
	}

	result, err := service.Launch(context.Background(), "store_live_demo", CreateLaunchInput{
		Username:     "member-launch",
		ProviderCode: "pragmatic",
		GameCode:     "vs20doghouse",
	}, RequestMetadata{
		IPAddress: "127.0.0.1",
		UserAgent: "game-launch-test",
	})
	if err != nil {
		t.Fatalf("Launch returned error: %v", err)
	}

	if result.LaunchURL != "https://launch.example/session" {
		t.Fatalf("LaunchURL = %q, want https://launch.example/session", result.LaunchURL)
	}

	if result.Lang != "id" {
		t.Fatalf("Lang = %q, want id", result.Lang)
	}

	if upstream.calls != 1 {
		t.Fatalf("user create calls = %d, want 1", upstream.calls)
	}

	if upstream.launchCalls != 1 {
		t.Fatalf("launch calls = %d, want 1", upstream.launchCalls)
	}

	if len(repository.members) != 1 {
		t.Fatalf("member count = %d, want 1", len(repository.members))
	}

	if len(repository.launchLogs) != 1 || repository.launchLogs[0].Status != "success" {
		t.Fatalf("launch logs = %#v, want one success log", repository.launchLogs)
	}
}

func TestLaunchRejectsUnknownProviderBeforeAutoCreate(t *testing.T) {
	repository := newFakeRepository(time.Date(2026, 3, 30, 22, 45, 0, 0, time.UTC))
	repository.members = map[string]StoreMember{}
	upstream := &fakeUpstream{}

	service := NewService(Options{
		Repository: repository,
		Upstream:   upstream,
		Ledger:     newFakeLedger("0.00"),
		Clock:      fixedClock{now: repository.now},
	}).(*service)
	service.codeFactory = func() (string, error) {
		return "AUTOUSER0001", nil
	}

	_, err := service.Launch(context.Background(), "store_live_demo", CreateLaunchInput{
		Username:     "member-launch",
		ProviderCode: "unknown",
		GameCode:     "missing",
	}, RequestMetadata{})
	if !errors.Is(err, ErrInvalidProviderGame) {
		t.Fatalf("Launch error = %v, want ErrInvalidProviderGame", err)
	}

	if upstream.calls != 0 {
		t.Fatalf("user create calls = %d, want 0", upstream.calls)
	}

	if upstream.launchCalls != 0 {
		t.Fatalf("launch calls = %d, want 0", upstream.launchCalls)
	}
}

func TestLaunchHasNoIdempotency(t *testing.T) {
	repository := newFakeRepository(time.Date(2026, 3, 30, 22, 50, 0, 0, time.UTC))
	upstream := &fakeUpstream{
		launchResult: nexusggr.GameLaunchResult{
			Message:   "SUCCESS",
			LaunchURL: "https://launch.example/session",
		},
	}

	service := NewService(Options{
		Repository: repository,
		Upstream:   upstream,
		Ledger:     newFakeLedger("0.00"),
		Clock:      fixedClock{now: repository.now},
	})

	for range 2 {
		_, err := service.Launch(context.Background(), "store_live_demo", CreateLaunchInput{
			Username:     "member-demo",
			ProviderCode: "PRAGMATIC",
			GameCode:     "vs20doghouse",
			Lang:         "en",
		}, RequestMetadata{})
		if err != nil {
			t.Fatalf("Launch returned error: %v", err)
		}
	}

	if upstream.launchCalls != 2 {
		t.Fatalf("launch calls = %d, want 2", upstream.launchCalls)
	}

	if len(repository.launchLogs) != 2 {
		t.Fatalf("launch log count = %d, want 2", len(repository.launchLogs))
	}
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time {
	return c.now
}

type fakeRepository struct {
	now           time.Time
	store         StoreScope
	members       map[string]StoreMember
	providerGames map[string]ProviderGame
	transactions  map[string]GameTransaction
	launchLogs    []CreateGameLaunchLogParams
	auditActions  []string
	notifications []string
	locked        map[string]bool
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
		providerGames: map[string]ProviderGame{
			"PRAGMATIC:vs20doghouse": {
				ProviderCode: "PRAGMATIC",
				GameCode:     "vs20doghouse",
			},
		},
		transactions: map[string]GameTransaction{},
		locked:       map[string]bool{},
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

func (r *fakeRepository) FindStoreMemberByID(_ context.Context, memberID string) (StoreMember, error) {
	member, ok := r.members[memberID]
	if !ok {
		return StoreMember{}, ErrNotFound
	}

	return member, nil
}

func (r *fakeRepository) FindProviderGame(_ context.Context, providerCode string, gameCode string) (ProviderGame, error) {
	providerGame, ok := r.providerGames[providerCode+":"+gameCode]
	if !ok {
		return ProviderGame{}, ErrNotFound
	}

	return providerGame, nil
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

func (r *fakeRepository) CreateGameLaunchLog(_ context.Context, params CreateGameLaunchLogParams) error {
	r.launchLogs = append(r.launchLogs, params)
	return nil
}

func (r *fakeRepository) ListPendingReconcileTransactions(_ context.Context, limit int) ([]GameTransaction, error) {
	transactions := []GameTransaction{}
	for _, transaction := range r.transactions {
		if transaction.Status == TransactionStatusPending && transaction.ReconcileStatus != nil && *transaction.ReconcileStatus == ReconcileStatusPending {
			transactions = append(transactions, transaction)
		}
		if limit > 0 && len(transactions) >= limit {
			break
		}
	}

	return transactions, nil
}

func (r *fakeRepository) FindGameTransactionByID(_ context.Context, transactionID string) (GameTransaction, error) {
	transaction, ok := r.transactions[transactionID]
	if !ok {
		return GameTransaction{}, ErrNotFound
	}

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

func (r *fakeRepository) AcquireReconcileLock(_ context.Context, transactionID string) (ReconcileLock, bool, error) {
	if r.locked[transactionID] {
		return nil, false, nil
	}

	r.locked[transactionID] = true
	return &fakeReconcileLock{
		repository:    r,
		transactionID: transactionID,
	}, true, nil
}

func (r *fakeRepository) FinalizeGameTransactionReconcile(_ context.Context, params FinalizeGameTransactionReconcileParams) (GameTransaction, error) {
	transaction, ok := r.transactions[params.GameTransactionID]
	if !ok {
		return GameTransaction{}, ErrNotFound
	}

	transaction.Status = params.Status
	transaction.ReconcileStatus = &params.ReconcileStatus
	transaction.UpstreamErrorCode = params.UpstreamErrorCode
	transaction.UpdatedAt = params.OccurredAt
	transaction.UpstreamResponse = json.RawMessage(toJSON(params.UpstreamResponseMasked))
	r.transactions[transaction.ID] = transaction
	r.auditActions = append(r.auditActions, params.AuditAction)
	r.notifications = append(r.notifications, params.NotificationEventType)

	return transaction, nil
}

func (r *fakeRepository) InsertAuditLog(_ context.Context, _ string, action string, _ string, _ *string, _ map[string]any, _ string, _ string, _ time.Time) error {
	r.auditActions = append(r.auditActions, action)
	return nil
}

type fakeReconcileLock struct {
	repository    *fakeRepository
	transactionID string
}

func (l *fakeReconcileLock) Unlock(_ context.Context) error {
	if l == nil || l.repository == nil {
		return nil
	}

	delete(l.repository.locked, l.transactionID)
	return nil
}

type fakeUpstream struct {
	calls                int
	err                  error
	moneyInfoCalls       int
	moneyInfoErr         error
	moneyInfoResult      nexusggr.MoneyInfoResult
	moneyInfoWait        <-chan struct{}
	launchCalls          int
	launchErr            error
	launchResult         nexusggr.GameLaunchResult
	depositCalls         int
	depositErr           error
	withdrawCalls        int
	withdrawErr          error
	transferStatusCalls  int
	transferStatusErr    error
	transferStatusResult nexusggr.TransferStatusResult
}

func (u *fakeUpstream) MoneyInfo(_ context.Context, _ nexusggr.MoneyInfoInput) (nexusggr.MoneyInfoResult, error) {
	u.moneyInfoCalls++
	if u.moneyInfoWait != nil {
		<-u.moneyInfoWait
	}
	if u.moneyInfoErr != nil {
		return nexusggr.MoneyInfoResult{}, u.moneyInfoErr
	}
	if u.moneyInfoResult.User == nil {
		u.moneyInfoResult = nexusggr.MoneyInfoResult{
			Message: "SUCCESS",
			User: &nexusggr.Balance{
				UserCode: "MEMBER000001",
				Balance:  0,
			},
		}
	}

	return u.moneyInfoResult, nil
}

func (u *fakeUpstream) GameLaunch(_ context.Context, _ nexusggr.GameLaunchInput) (nexusggr.GameLaunchResult, error) {
	u.launchCalls++
	if u.launchErr != nil {
		return nexusggr.GameLaunchResult{}, u.launchErr
	}
	if u.launchResult.LaunchURL == "" {
		u.launchResult = nexusggr.GameLaunchResult{
			Message:   "SUCCESS",
			LaunchURL: "https://launch.example/default",
		}
	}

	return u.launchResult, nil
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

func (u *fakeUpstream) TransferStatus(_ context.Context, _ nexusggr.TransferStatusInput) (nexusggr.TransferStatusResult, error) {
	u.transferStatusCalls++
	if u.transferStatusErr != nil {
		return nexusggr.TransferStatusResult{}, u.transferStatusErr
	}
	if u.transferStatusResult.Message == "" {
		u.transferStatusResult = nexusggr.TransferStatusResult{
			Message:      "SUCCESS",
			Amount:       5000,
			AgentBalance: 250000,
			UserBalance:  5000,
			Type:         "user_deposit",
		}
	}

	return u.transferStatusResult, nil
}

type fakeLedger struct {
	balance          ledger.BalanceSnapshot
	reserveCalls     int
	commitCalls      int
	releaseCalls     int
	creditCalls      int
	referenceEntries map[string]bool
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
		referenceEntries: map[string]bool{},
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
	l.referenceEntries[input.ReferenceType+":"+input.ReferenceID] = true
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
	l.referenceEntries[input.ReferenceType+":"+input.ReferenceID] = true
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

type fakeBalanceCache struct {
	mu      sync.Mutex
	entries map[string]GameBalanceResult
	lastTTL time.Duration
}

func newFakeBalanceCache() *fakeBalanceCache {
	return &fakeBalanceCache{
		entries: map[string]GameBalanceResult{},
	}
}

func (l *fakeLedger) HasReferenceEntries(_ context.Context, referenceType string, referenceID string) (bool, error) {
	return l.referenceEntries[referenceType+":"+referenceID], nil
}

func (c *fakeBalanceCache) Get(_ context.Context, storeID string, memberID string) (GameBalanceResult, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	result, ok := c.entries[storeID+":"+memberID]
	return result, ok, nil
}

func (c *fakeBalanceCache) Set(_ context.Context, storeID string, memberID string, result GameBalanceResult, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[storeID+":"+memberID] = result
	c.lastTTL = ttl
	return nil
}
