package ledger

import (
	"context"
	"testing"
	"time"
)

func TestCreditDebitAndBalanceAfter(t *testing.T) {
	service := NewService(newFakeRepository(time.Date(2026, 3, 30, 9, 0, 0, 0, time.UTC)))

	credit, err := service.Credit(context.Background(), "store-1", PostEntryInput{
		EntryType:     EntryTypeStoreTopup,
		Amount:        "100.00",
		ReferenceType: "qris_transaction",
		ReferenceID:   "1c7179e7-4cee-4db3-84e8-d63d58a422f9",
	})
	if err != nil {
		t.Fatalf("Credit returned error: %v", err)
	}

	if credit.Entry.BalanceAfter != "100.00" {
		t.Fatalf("credit balance_after = %s, want 100.00", credit.Entry.BalanceAfter)
	}

	debit, err := service.Debit(context.Background(), "store-1", PostEntryInput{
		EntryType:     EntryTypeGameDeposit,
		Amount:        "25.50",
		ReferenceType: "game_transaction",
		ReferenceID:   "6380ca59-2784-4d81-bc8d-d73c4c3dc43f",
	})
	if err != nil {
		t.Fatalf("Debit returned error: %v", err)
	}

	if debit.Entry.BalanceAfter != "74.50" {
		t.Fatalf("debit balance_after = %s, want 74.50", debit.Entry.BalanceAfter)
	}

	balance, err := service.GetBalance(context.Background(), "store-1")
	if err != nil {
		t.Fatalf("GetBalance returned error: %v", err)
	}

	if balance.CurrentBalance != "74.50" || balance.ReservedAmount != "0.00" || balance.AvailableBalance != "74.50" {
		t.Fatalf("balance = %#v, want current=74.50 reserved=0.00 available=74.50", balance)
	}
}

func TestPostEntriesAppliesAtomically(t *testing.T) {
	service := NewService(newFakeRepository(time.Date(2026, 3, 30, 9, 15, 0, 0, time.UTC)))

	result, err := service.PostEntries(context.Background(), "store-1", PostEntriesInput{
		ReferenceType: "qris_transaction",
		ReferenceID:   "1c7179e7-4cee-4db3-84e8-d63d58a422f9",
		Entries: []BatchPostEntryInput{
			{
				Direction: DirectionCredit,
				EntryType: EntryTypeMemberPaymentCredit,
				Amount:    "25000.00",
			},
			{
				Direction: DirectionDebit,
				EntryType: EntryTypeMemberPaymentFee,
				Amount:    "750.00",
			},
		},
	})
	if err != nil {
		t.Fatalf("PostEntries returned error: %v", err)
	}

	if len(result.Entries) != 2 {
		t.Fatalf("len(result.Entries) = %d, want 2", len(result.Entries))
	}
	if result.Entries[0].BalanceAfter != "25000.00" {
		t.Fatalf("first balance_after = %s, want 25000.00", result.Entries[0].BalanceAfter)
	}
	if result.Entries[1].BalanceAfter != "24250.00" {
		t.Fatalf("second balance_after = %s, want 24250.00", result.Entries[1].BalanceAfter)
	}
	if result.Balance.CurrentBalance != "24250.00" || result.Balance.AvailableBalance != "24250.00" {
		t.Fatalf("balance = %#v, want current=24250.00 available=24250.00", result.Balance)
	}
}

func TestNoNegativeBalanceWithPendingReservation(t *testing.T) {
	service := NewService(newFakeRepository(time.Date(2026, 3, 30, 9, 30, 0, 0, time.UTC)))

	if _, err := service.Credit(context.Background(), "store-1", PostEntryInput{
		EntryType:     EntryTypeStoreTopup,
		Amount:        "50.00",
		ReferenceType: "qris_transaction",
		ReferenceID:   "e355c344-d52a-4ef1-a7c3-0498d1c514f4",
	}); err != nil {
		t.Fatalf("Credit returned error: %v", err)
	}

	if _, err := service.Reserve(context.Background(), "store-1", ReserveInput{
		Amount:        "40.00",
		ReferenceType: "store_withdrawal",
		ReferenceID:   "d0ca6ce1-4fc2-44d2-a912-795dff32cfa4",
	}); err != nil {
		t.Fatalf("Reserve returned error: %v", err)
	}

	_, err := service.Debit(context.Background(), "store-1", PostEntryInput{
		EntryType:     EntryTypeGameDeposit,
		Amount:        "20.00",
		ReferenceType: "game_transaction",
		ReferenceID:   "7d5d2fd4-0f6d-4ae1-a6cc-2ef7d20e1eb2",
	})
	if err != ErrInsufficientFunds {
		t.Fatalf("Debit error = %v, want ErrInsufficientFunds", err)
	}
}

func TestReserveCommitAndReleaseLifecycle(t *testing.T) {
	service := NewService(newFakeRepository(time.Date(2026, 3, 30, 10, 0, 0, 0, time.UTC)))

	if _, err := service.Credit(context.Background(), "store-1", PostEntryInput{
		EntryType:     EntryTypeStoreTopup,
		Amount:        "200.00",
		ReferenceType: "qris_transaction",
		ReferenceID:   "a0f14ceb-300d-44f9-a406-fa3f8e8785cf",
	}); err != nil {
		t.Fatalf("Credit returned error: %v", err)
	}

	reserved, err := service.Reserve(context.Background(), "store-1", ReserveInput{
		Amount:        "120.00",
		ReferenceType: "store_withdrawal",
		ReferenceID:   "905cde6e-348f-4c3d-a06d-cd19ca53ce5c",
	})
	if err != nil {
		t.Fatalf("Reserve returned error: %v", err)
	}

	if reserved.Balance.CurrentBalance != "200.00" || reserved.Balance.ReservedAmount != "120.00" || reserved.Balance.AvailableBalance != "80.00" {
		t.Fatalf("reserved balance = %#v, want current=200.00 reserved=120.00 available=80.00", reserved.Balance)
	}

	committed, err := service.CommitReservation(context.Background(), "store-1", CommitReservationInput{
		ReferenceType: "store_withdrawal",
		ReferenceID:   "905cde6e-348f-4c3d-a06d-cd19ca53ce5c",
		Entries: []ReservationCommitEntryInput{
			{EntryType: EntryTypeWithdrawCommit, Amount: "100.00"},
			{EntryType: EntryTypeWithdrawPlatformFee, Amount: "10.00"},
			{EntryType: EntryTypeWithdrawExternalFee, Amount: "10.00"},
		},
	})
	if err != nil {
		t.Fatalf("CommitReservation returned error: %v", err)
	}

	if len(committed.Entries) != 3 {
		t.Fatalf("len(committed entries) = %d, want 3", len(committed.Entries))
	}

	if committed.Entries[2].BalanceAfter != "80.00" {
		t.Fatalf("final balance_after = %s, want 80.00", committed.Entries[2].BalanceAfter)
	}

	if committed.Balance.CurrentBalance != "80.00" || committed.Balance.ReservedAmount != "0.00" || committed.Balance.AvailableBalance != "80.00" {
		t.Fatalf("committed balance = %#v, want current=80.00 reserved=0.00 available=80.00", committed.Balance)
	}

	secondReserved, err := service.Reserve(context.Background(), "store-1", ReserveInput{
		Amount:        "50.00",
		ReferenceType: "store_withdrawal",
		ReferenceID:   "8dbec5d6-cf40-4bb6-9a0f-2482ddc2b52a",
	})
	if err != nil {
		t.Fatalf("second Reserve returned error: %v", err)
	}

	if secondReserved.Balance.AvailableBalance != "30.00" {
		t.Fatalf("available balance after second reserve = %s, want 30.00", secondReserved.Balance.AvailableBalance)
	}

	released, err := service.ReleaseReservation(context.Background(), "store-1", ReleaseReservationInput{
		ReferenceType: "store_withdrawal",
		ReferenceID:   "8dbec5d6-cf40-4bb6-9a0f-2482ddc2b52a",
	})
	if err != nil {
		t.Fatalf("ReleaseReservation returned error: %v", err)
	}

	if released.Balance.CurrentBalance != "80.00" || released.Balance.ReservedAmount != "0.00" || released.Balance.AvailableBalance != "80.00" {
		t.Fatalf("released balance = %#v, want current=80.00 reserved=0.00 available=80.00", released.Balance)
	}
}

type fakeRepository struct {
	now           time.Time
	storeID       string
	ledgerAccount string
	currency      string
	current       money
	reservations  map[string]LedgerReservation
	entries       []LedgerEntry
	sequence      int
}

func newFakeRepository(now time.Time) *fakeRepository {
	return &fakeRepository{
		now:           now,
		storeID:       "store-1",
		ledgerAccount: "ledger-1",
		currency:      "IDR",
		reservations:  map[string]LedgerReservation{},
	}
}

func (r *fakeRepository) GetBalance(_ context.Context, storeID string) (BalanceSnapshot, error) {
	if storeID != r.storeID {
		return BalanceSnapshot{}, ErrNotFound
	}

	return r.snapshot(), nil
}

func (r *fakeRepository) PostEntry(_ context.Context, params postEntryParams) (PostingResult, error) {
	if params.StoreID != r.storeID {
		return PostingResult{}, ErrNotFound
	}

	amount := mustParseMoney(params.Amount)
	if params.Direction == DirectionDebit && r.snapshotMoney().available.LessThan(amount) {
		return PostingResult{}, ErrInsufficientFunds
	}

	if params.Direction == DirectionDebit {
		r.current = r.current.Sub(amount)
	} else {
		r.current = r.current.Add(amount)
	}

	entry := LedgerEntry{
		ID:              r.nextID("entry"),
		StoreID:         r.storeID,
		LedgerAccountID: r.ledgerAccount,
		Direction:       params.Direction,
		EntryType:       params.EntryType,
		Amount:          amount.String(),
		BalanceAfter:    r.current.String(),
		ReferenceType:   params.ReferenceType,
		ReferenceID:     params.ReferenceID,
		CreatedAt:       r.nextTime(),
	}
	r.entries = append(r.entries, entry)

	return PostingResult{Entry: entry, Balance: r.snapshot()}, nil
}

func (r *fakeRepository) PostEntries(_ context.Context, params postEntriesParams) (BatchPostingResult, error) {
	if params.StoreID != r.storeID {
		return BatchPostingResult{}, ErrNotFound
	}

	originalCurrent := r.current
	originalEntries := append([]LedgerEntry(nil), r.entries...)
	originalSequence := r.sequence

	resultEntries := make([]LedgerEntry, 0, len(params.Entries))
	for _, entry := range params.Entries {
		result, err := r.PostEntry(context.Background(), postEntryParams{
			StoreID:       params.StoreID,
			Direction:     entry.Direction,
			EntryType:     entry.EntryType,
			Amount:        entry.Amount,
			ReferenceType: params.ReferenceType,
			ReferenceID:   params.ReferenceID,
			Metadata:      entry.Metadata,
		})
		if err != nil {
			r.current = originalCurrent
			r.entries = originalEntries
			r.sequence = originalSequence
			return BatchPostingResult{}, err
		}

		resultEntries = append(resultEntries, result.Entry)
	}

	return BatchPostingResult{
		Entries: resultEntries,
		Balance: r.snapshot(),
	}, nil
}

func (r *fakeRepository) HasReferenceEntries(_ context.Context, referenceType string, referenceID string) (bool, error) {
	for _, entry := range r.entries {
		if entry.ReferenceType == referenceType && entry.ReferenceID == referenceID {
			return true, nil
		}
	}

	return false, nil
}

func (r *fakeRepository) Reserve(_ context.Context, params reserveParams) (ReservationResult, error) {
	if params.StoreID != r.storeID {
		return ReservationResult{}, ErrNotFound
	}

	key := params.ReferenceType + ":" + params.ReferenceID
	if existing, ok := r.reservations[key]; ok {
		if existing.Status == ReservationStatusPending && existing.Amount == params.Amount {
			return ReservationResult{Reservation: existing, Balance: r.snapshot()}, nil
		}

		return ReservationResult{}, ErrDuplicateReference
	}

	amount := mustParseMoney(params.Amount)
	if r.snapshotMoney().available.LessThan(amount) {
		return ReservationResult{}, ErrInsufficientFunds
	}

	reservation := LedgerReservation{
		ID:            r.nextID("reservation"),
		StoreID:       r.storeID,
		ReferenceType: params.ReferenceType,
		ReferenceID:   params.ReferenceID,
		Amount:        amount.String(),
		Status:        ReservationStatusPending,
		CreatedAt:     r.nextTime(),
		UpdatedAt:     r.nextTime(),
	}
	r.reservations[key] = reservation

	return ReservationResult{Reservation: reservation, Balance: r.snapshot()}, nil
}

func (r *fakeRepository) CommitReservation(_ context.Context, params commitReservationParams) (CommitReservationResult, error) {
	if params.StoreID != r.storeID {
		return CommitReservationResult{}, ErrNotFound
	}

	key := params.ReferenceType + ":" + params.ReferenceID
	reservation, ok := r.reservations[key]
	if !ok {
		return CommitReservationResult{}, ErrNotFound
	}

	if reservation.Status == ReservationStatusReleased {
		return CommitReservationResult{}, ErrReservationFinalized
	}

	total := money(0)
	for _, entry := range params.Entries {
		total = total.Add(mustParseMoney(entry.Amount))
	}
	if !total.Equal(mustParseMoney(reservation.Amount)) {
		return CommitReservationResult{}, ErrInvalidReservationCommit
	}

	if reservation.Status == ReservationStatusCommitted {
		return CommitReservationResult{
			Reservation: reservation,
			Entries:     r.entriesByReference(params.ReferenceType, params.ReferenceID),
			Balance:     r.snapshot(),
		}, nil
	}

	if r.current.LessThan(total) {
		return CommitReservationResult{}, ErrInsufficientFunds
	}

	entries := make([]LedgerEntry, 0, len(params.Entries))
	for _, item := range params.Entries {
		amount := mustParseMoney(item.Amount)
		r.current = r.current.Sub(amount)
		entry := LedgerEntry{
			ID:              r.nextID("entry"),
			StoreID:         r.storeID,
			LedgerAccountID: r.ledgerAccount,
			Direction:       DirectionDebit,
			EntryType:       item.EntryType,
			Amount:          amount.String(),
			BalanceAfter:    r.current.String(),
			ReferenceType:   params.ReferenceType,
			ReferenceID:     params.ReferenceID,
			CreatedAt:       r.nextTime(),
		}
		r.entries = append(r.entries, entry)
		entries = append(entries, entry)
	}

	reservation.Status = ReservationStatusCommitted
	reservation.UpdatedAt = r.nextTime()
	r.reservations[key] = reservation

	return CommitReservationResult{
		Reservation: reservation,
		Entries:     entries,
		Balance:     r.snapshot(),
	}, nil
}

func (r *fakeRepository) ReleaseReservation(_ context.Context, params releaseReservationParams) (ReservationResult, error) {
	if params.StoreID != r.storeID {
		return ReservationResult{}, ErrNotFound
	}

	key := params.ReferenceType + ":" + params.ReferenceID
	reservation, ok := r.reservations[key]
	if !ok {
		return ReservationResult{}, ErrNotFound
	}

	if reservation.Status == ReservationStatusCommitted {
		return ReservationResult{}, ErrReservationFinalized
	}

	if reservation.Status == ReservationStatusReleased {
		return ReservationResult{Reservation: reservation, Balance: r.snapshot()}, nil
	}

	reservation.Status = ReservationStatusReleased
	reservation.UpdatedAt = r.nextTime()
	r.reservations[key] = reservation

	return ReservationResult{Reservation: reservation, Balance: r.snapshot()}, nil
}

type balanceMoney struct {
	current   money
	reserved  money
	available money
}

func (r *fakeRepository) snapshot() BalanceSnapshot {
	values := r.snapshotMoney()
	return BalanceSnapshot{
		StoreID:          r.storeID,
		LedgerAccountID:  r.ledgerAccount,
		Currency:         r.currency,
		CurrentBalance:   values.current.String(),
		ReservedAmount:   values.reserved.String(),
		AvailableBalance: values.available.String(),
	}
}

func (r *fakeRepository) snapshotMoney() balanceMoney {
	reserved := money(0)
	for _, reservation := range r.reservations {
		if reservation.Status == ReservationStatusPending {
			reserved = reserved.Add(mustParseMoney(reservation.Amount))
		}
	}

	return balanceMoney{
		current:   r.current,
		reserved:  reserved,
		available: r.current.Sub(reserved),
	}
}

func (r *fakeRepository) entriesByReference(referenceType string, referenceID string) []LedgerEntry {
	var entries []LedgerEntry
	for _, entry := range r.entries {
		if entry.ReferenceType == referenceType && entry.ReferenceID == referenceID {
			entries = append(entries, entry)
		}
	}

	return entries
}

func (r *fakeRepository) nextID(prefix string) string {
	r.sequence++
	return prefix + "-" + time.Date(2000, 1, 1, 0, 0, r.sequence, 0, time.UTC).Format("150405")
}

func (r *fakeRepository) nextTime() time.Time {
	r.now = r.now.Add(time.Second)
	return r.now
}
