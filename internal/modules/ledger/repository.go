package ledger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

type accountState struct {
	storeID         string
	ledgerAccountID string
	currency        string
	currentBalance  money
	reservedAmount  money
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) GetBalance(ctx context.Context, storeID string) (BalanceSnapshot, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return BalanceSnapshot{}, fmt.Errorf("begin get balance transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	state, err := r.loadLockedState(ctx, tx, storeID)
	if err != nil {
		return BalanceSnapshot{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return BalanceSnapshot{}, fmt.Errorf("commit get balance transaction: %w", err)
	}

	return state.snapshot(), nil
}

func (r *Repository) PostEntry(ctx context.Context, params postEntryParams) (PostingResult, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return PostingResult{}, fmt.Errorf("begin post entry transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	state, err := r.loadLockedState(ctx, tx, params.StoreID)
	if err != nil {
		return PostingResult{}, err
	}

	amount, err := parseMoney(params.Amount)
	if err != nil {
		return PostingResult{}, ErrInvalidAmount
	}

	nextBalance := state.currentBalance
	if params.Direction == DirectionDebit {
		if state.availableBalance().LessThan(amount) {
			return PostingResult{}, ErrInsufficientFunds
		}

		nextBalance = state.currentBalance.Sub(amount)
	} else {
		nextBalance = state.currentBalance.Add(amount)
	}

	now := time.Now().UTC()
	entry, err := insertEntryTx(ctx, tx, insertEntryParams{
		storeID:         state.storeID,
		ledgerAccountID: state.ledgerAccountID,
		direction:       params.Direction,
		entryType:       params.EntryType,
		amount:          amount.String(),
		balanceAfter:    nextBalance.String(),
		referenceType:   params.ReferenceType,
		referenceID:     params.ReferenceID,
		metadata:        params.Metadata,
		occurredAt:      now,
	})
	if err != nil {
		return PostingResult{}, err
	}

	if err := updateStoreBalanceTx(ctx, tx, state.storeID, nextBalance.String(), now); err != nil {
		return PostingResult{}, err
	}

	state.currentBalance = nextBalance

	if err := tx.Commit(ctx); err != nil {
		return PostingResult{}, fmt.Errorf("commit post entry transaction: %w", err)
	}

	return PostingResult{
		Entry:   entry,
		Balance: state.snapshot(),
	}, nil
}

func (r *Repository) PostEntries(ctx context.Context, params postEntriesParams) (BatchPostingResult, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return BatchPostingResult{}, fmt.Errorf("begin post entries transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	state, err := r.loadLockedState(ctx, tx, params.StoreID)
	if err != nil {
		return BatchPostingResult{}, err
	}

	now := time.Now().UTC()
	nextBalance := state.currentBalance
	entries := make([]LedgerEntry, 0, len(params.Entries))
	for _, rawEntry := range params.Entries {
		amount, err := parseMoney(rawEntry.Amount)
		if err != nil {
			return BatchPostingResult{}, ErrInvalidAmount
		}

		if rawEntry.Direction == DirectionDebit {
			if nextBalance.Sub(state.reservedAmount).LessThan(amount) {
				return BatchPostingResult{}, ErrInsufficientFunds
			}
			nextBalance = nextBalance.Sub(amount)
		} else {
			nextBalance = nextBalance.Add(amount)
		}

		entry, err := insertEntryTx(ctx, tx, insertEntryParams{
			storeID:         state.storeID,
			ledgerAccountID: state.ledgerAccountID,
			direction:       rawEntry.Direction,
			entryType:       rawEntry.EntryType,
			amount:          amount.String(),
			balanceAfter:    nextBalance.String(),
			referenceType:   params.ReferenceType,
			referenceID:     params.ReferenceID,
			metadata:        rawEntry.Metadata,
			occurredAt:      now,
		})
		if err != nil {
			return BatchPostingResult{}, err
		}

		entries = append(entries, entry)
	}

	if err := updateStoreBalanceTx(ctx, tx, state.storeID, nextBalance.String(), now); err != nil {
		return BatchPostingResult{}, err
	}

	state.currentBalance = nextBalance

	if err := tx.Commit(ctx); err != nil {
		return BatchPostingResult{}, fmt.Errorf("commit post entries transaction: %w", err)
	}

	return BatchPostingResult{
		Entries: entries,
		Balance: state.snapshot(),
	}, nil
}

func (r *Repository) HasReferenceEntries(ctx context.Context, referenceType string, referenceID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM ledger_entries
			WHERE reference_type = $1 AND reference_id = $2
		)
	`, referenceType, referenceID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check ledger entries by reference: %w", err)
	}

	return exists, nil
}

func (r *Repository) Reserve(ctx context.Context, params reserveParams) (ReservationResult, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return ReservationResult{}, fmt.Errorf("begin reserve transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	state, err := r.loadLockedState(ctx, tx, params.StoreID)
	if err != nil {
		return ReservationResult{}, err
	}

	amount, err := parseMoney(params.Amount)
	if err != nil {
		return ReservationResult{}, ErrInvalidAmount
	}

	existing, err := findReservationTx(ctx, tx, params.ReferenceType, params.ReferenceID)
	if err == nil {
		if existing.StoreID != params.StoreID {
			return ReservationResult{}, ErrDuplicateReference
		}

		if existing.Status == ReservationStatusPending && mustParseMoney(existing.Amount).Equal(amount) {
			return ReservationResult{
				Reservation: existing,
				Balance:     state.snapshot(),
			}, nil
		}

		return ReservationResult{}, ErrDuplicateReference
	}
	if err != nil && !errors.Is(err, ErrNotFound) {
		return ReservationResult{}, err
	}

	if state.availableBalance().LessThan(amount) {
		return ReservationResult{}, ErrInsufficientFunds
	}

	now := time.Now().UTC()
	reservation, err := insertReservationTx(ctx, tx, insertReservationParams{
		storeID:       params.StoreID,
		referenceType: params.ReferenceType,
		referenceID:   params.ReferenceID,
		amount:        amount.String(),
		status:        ReservationStatusPending,
		occurredAt:    now,
	})
	if err != nil {
		return ReservationResult{}, err
	}

	state.reservedAmount = state.reservedAmount.Add(amount)

	if err := tx.Commit(ctx); err != nil {
		return ReservationResult{}, fmt.Errorf("commit reserve transaction: %w", err)
	}

	return ReservationResult{
		Reservation: reservation,
		Balance:     state.snapshot(),
	}, nil
}

func (r *Repository) CommitReservation(ctx context.Context, params commitReservationParams) (CommitReservationResult, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return CommitReservationResult{}, fmt.Errorf("begin commit reservation transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	state, err := r.loadLockedState(ctx, tx, params.StoreID)
	if err != nil {
		return CommitReservationResult{}, err
	}

	reservation, err := findReservationTx(ctx, tx, params.ReferenceType, params.ReferenceID)
	if err != nil {
		return CommitReservationResult{}, err
	}

	if reservation.StoreID != params.StoreID {
		return CommitReservationResult{}, ErrNotFound
	}

	if reservation.Status == ReservationStatusReleased {
		return CommitReservationResult{}, ErrReservationFinalized
	}

	expectedAmount := mustParseMoney(reservation.Amount)
	total := money(0)
	for _, entry := range params.Entries {
		total = total.Add(mustParseMoney(entry.Amount))
	}
	if !total.Equal(expectedAmount) {
		return CommitReservationResult{}, ErrInvalidReservationCommit
	}

	if reservation.Status == ReservationStatusCommitted {
		entries, entryErr := listEntriesByReferenceTx(ctx, tx, params.ReferenceType, params.ReferenceID)
		if entryErr != nil {
			return CommitReservationResult{}, entryErr
		}

		return CommitReservationResult{
			Reservation: reservation,
			Entries:     entries,
			Balance:     state.snapshot(),
		}, nil
	}

	if state.currentBalance.LessThan(expectedAmount) {
		return CommitReservationResult{}, ErrInsufficientFunds
	}

	runningBalance := state.currentBalance
	entries := make([]LedgerEntry, 0, len(params.Entries))
	now := time.Now().UTC()

	for _, item := range params.Entries {
		amount := mustParseMoney(item.Amount)
		runningBalance = runningBalance.Sub(amount)

		entry, entryErr := insertEntryTx(ctx, tx, insertEntryParams{
			storeID:         state.storeID,
			ledgerAccountID: state.ledgerAccountID,
			direction:       DirectionDebit,
			entryType:       item.EntryType,
			amount:          amount.String(),
			balanceAfter:    runningBalance.String(),
			referenceType:   params.ReferenceType,
			referenceID:     params.ReferenceID,
			metadata:        item.Metadata,
			occurredAt:      now,
		})
		if entryErr != nil {
			return CommitReservationResult{}, entryErr
		}

		entries = append(entries, entry)
	}

	if err := updateStoreBalanceTx(ctx, tx, state.storeID, runningBalance.String(), now); err != nil {
		return CommitReservationResult{}, err
	}

	reservation, err = updateReservationStatusTx(ctx, tx, reservation.ID, ReservationStatusCommitted, now)
	if err != nil {
		return CommitReservationResult{}, err
	}

	state.currentBalance = runningBalance
	state.reservedAmount = state.reservedAmount.Sub(expectedAmount)

	if err := tx.Commit(ctx); err != nil {
		return CommitReservationResult{}, fmt.Errorf("commit reservation transaction: %w", err)
	}

	return CommitReservationResult{
		Reservation: reservation,
		Entries:     entries,
		Balance:     state.snapshot(),
	}, nil
}

func (r *Repository) ReleaseReservation(ctx context.Context, params releaseReservationParams) (ReservationResult, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return ReservationResult{}, fmt.Errorf("begin release reservation transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	state, err := r.loadLockedState(ctx, tx, params.StoreID)
	if err != nil {
		return ReservationResult{}, err
	}

	reservation, err := findReservationTx(ctx, tx, params.ReferenceType, params.ReferenceID)
	if err != nil {
		return ReservationResult{}, err
	}

	if reservation.StoreID != params.StoreID {
		return ReservationResult{}, ErrNotFound
	}

	if reservation.Status == ReservationStatusCommitted {
		return ReservationResult{}, ErrReservationFinalized
	}

	if reservation.Status == ReservationStatusReleased {
		return ReservationResult{
			Reservation: reservation,
			Balance:     state.snapshot(),
		}, nil
	}

	now := time.Now().UTC()
	reservation, err = updateReservationStatusTx(ctx, tx, reservation.ID, ReservationStatusReleased, now)
	if err != nil {
		return ReservationResult{}, err
	}

	state.reservedAmount = state.reservedAmount.Sub(mustParseMoney(reservation.Amount))

	if err := tx.Commit(ctx); err != nil {
		return ReservationResult{}, fmt.Errorf("commit release reservation transaction: %w", err)
	}

	return ReservationResult{
		Reservation: reservation,
		Balance:     state.snapshot(),
	}, nil
}

func (r *Repository) loadLockedState(ctx context.Context, tx pgx.Tx, storeID string) (accountState, error) {
	var currentBalanceRaw string
	err := tx.QueryRow(ctx, `
		SELECT id, current_balance::text
		FROM stores
		WHERE id = $1 AND deleted_at IS NULL
		FOR UPDATE
	`, storeID).Scan(&storeID, &currentBalanceRaw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return accountState{}, ErrNotFound
		}

		return accountState{}, fmt.Errorf("lock store: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO ledger_accounts (store_id, currency)
		VALUES ($1, 'IDR')
		ON CONFLICT (store_id) DO NOTHING
	`, storeID); err != nil {
		return accountState{}, fmt.Errorf("ensure ledger account: %w", err)
	}

	var ledgerAccountID string
	var currency string
	err = tx.QueryRow(ctx, `
		SELECT id, currency
		FROM ledger_accounts
		WHERE store_id = $1
		LIMIT 1
		FOR UPDATE
	`, storeID).Scan(&ledgerAccountID, &currency)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return accountState{}, ErrNotFound
		}

		return accountState{}, fmt.Errorf("load ledger account: %w", err)
	}

	var reservedAmountRaw string
	err = tx.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount), 0)::text
		FROM ledger_reservations
		WHERE store_id = $1 AND status = 'pending'
	`, storeID).Scan(&reservedAmountRaw)
	if err != nil {
		return accountState{}, fmt.Errorf("load reserved amount: %w", err)
	}

	currentBalance, err := parseMoney(currentBalanceRaw)
	if err != nil {
		return accountState{}, fmt.Errorf("parse current balance: %w", err)
	}

	reservedAmount, err := parseMoney(reservedAmountRaw)
	if err != nil {
		return accountState{}, fmt.Errorf("parse reserved amount: %w", err)
	}

	return accountState{
		storeID:         storeID,
		ledgerAccountID: ledgerAccountID,
		currency:        currency,
		currentBalance:  currentBalance,
		reservedAmount:  reservedAmount,
	}, nil
}

func (s accountState) availableBalance() money {
	return s.currentBalance.Sub(s.reservedAmount)
}

func (s accountState) snapshot() BalanceSnapshot {
	return BalanceSnapshot{
		StoreID:          s.storeID,
		LedgerAccountID:  s.ledgerAccountID,
		Currency:         s.currency,
		CurrentBalance:   s.currentBalance.String(),
		ReservedAmount:   s.reservedAmount.String(),
		AvailableBalance: s.availableBalance().String(),
	}
}

type insertEntryParams struct {
	storeID         string
	ledgerAccountID string
	direction       Direction
	entryType       EntryType
	amount          string
	balanceAfter    string
	referenceType   string
	referenceID     string
	metadata        map[string]any
	occurredAt      time.Time
}

func insertEntryTx(ctx context.Context, tx pgx.Tx, params insertEntryParams) (LedgerEntry, error) {
	var encoded []byte
	if params.metadata == nil {
		encoded = []byte("{}")
	} else {
		var err error
		encoded, err = json.Marshal(params.metadata)
		if err != nil {
			return LedgerEntry{}, fmt.Errorf("marshal ledger metadata: %w", err)
		}
	}

	var entry LedgerEntry
	err := tx.QueryRow(ctx, `
		INSERT INTO ledger_entries (
			store_id,
			ledger_account_id,
			direction,
			entry_type,
			amount,
			balance_after,
			reference_type,
			reference_id,
			metadata_json,
			created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10)
		RETURNING
			id,
			store_id,
			ledger_account_id,
			direction,
			entry_type,
			amount::text,
			balance_after::text,
			reference_type,
			reference_id::text,
			metadata_json,
			created_at
	`, params.storeID, params.ledgerAccountID, params.direction, params.entryType, params.amount, params.balanceAfter, params.referenceType, params.referenceID, string(encoded), params.occurredAt).Scan(
		&entry.ID,
		&entry.StoreID,
		&entry.LedgerAccountID,
		&entry.Direction,
		&entry.EntryType,
		&entry.Amount,
		&entry.BalanceAfter,
		&entry.ReferenceType,
		&entry.ReferenceID,
		&entry.Metadata,
		&entry.CreatedAt,
	)
	if err != nil {
		if isUniqueViolation(err, "ledger_entries_reference_type_reference_id_direction_entry_type_unique") {
			return LedgerEntry{}, ErrDuplicateReference
		}

		return LedgerEntry{}, fmt.Errorf("insert ledger entry: %w", err)
	}

	return entry, nil
}

func updateStoreBalanceTx(ctx context.Context, tx pgx.Tx, storeID string, balance string, occurredAt time.Time) error {
	commandTag, err := tx.Exec(ctx, `
		UPDATE stores
		SET current_balance = $2, updated_at = $3
		WHERE id = $1 AND deleted_at IS NULL
	`, storeID, balance, occurredAt)
	if err != nil {
		return fmt.Errorf("update store balance: %w", err)
	}

	if commandTag.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

type insertReservationParams struct {
	storeID       string
	referenceType string
	referenceID   string
	amount        string
	status        ReservationStatus
	occurredAt    time.Time
}

func insertReservationTx(ctx context.Context, tx pgx.Tx, params insertReservationParams) (LedgerReservation, error) {
	var reservation LedgerReservation
	err := tx.QueryRow(ctx, `
		INSERT INTO ledger_reservations (
			store_id,
			reference_type,
			reference_id,
			amount,
			status,
			created_at,
			updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $6)
		RETURNING
			id,
			store_id,
			reference_type,
			reference_id::text,
			amount::text,
			status,
			created_at,
			updated_at
	`, params.storeID, params.referenceType, params.referenceID, params.amount, params.status, params.occurredAt).Scan(
		&reservation.ID,
		&reservation.StoreID,
		&reservation.ReferenceType,
		&reservation.ReferenceID,
		&reservation.Amount,
		&reservation.Status,
		&reservation.CreatedAt,
		&reservation.UpdatedAt,
	)
	if err != nil {
		if isUniqueViolation(err, "ledger_reservations_reference_type_reference_id_unique") {
			return LedgerReservation{}, ErrDuplicateReference
		}

		return LedgerReservation{}, fmt.Errorf("insert ledger reservation: %w", err)
	}

	return reservation, nil
}

func findReservationTx(ctx context.Context, tx pgx.Tx, referenceType string, referenceID string) (LedgerReservation, error) {
	var reservation LedgerReservation
	err := tx.QueryRow(ctx, `
		SELECT
			id,
			store_id,
			reference_type,
			reference_id::text,
			amount::text,
			status,
			created_at,
			updated_at
		FROM ledger_reservations
		WHERE reference_type = $1 AND reference_id = $2
		LIMIT 1
		FOR UPDATE
	`, referenceType, referenceID).Scan(
		&reservation.ID,
		&reservation.StoreID,
		&reservation.ReferenceType,
		&reservation.ReferenceID,
		&reservation.Amount,
		&reservation.Status,
		&reservation.CreatedAt,
		&reservation.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return LedgerReservation{}, ErrNotFound
		}

		return LedgerReservation{}, fmt.Errorf("find ledger reservation: %w", err)
	}

	return reservation, nil
}

func updateReservationStatusTx(ctx context.Context, tx pgx.Tx, reservationID string, status ReservationStatus, occurredAt time.Time) (LedgerReservation, error) {
	var reservation LedgerReservation
	err := tx.QueryRow(ctx, `
		UPDATE ledger_reservations
		SET status = $2, updated_at = $3
		WHERE id = $1
		RETURNING
			id,
			store_id,
			reference_type,
			reference_id::text,
			amount::text,
			status,
			created_at,
			updated_at
	`, reservationID, status, occurredAt).Scan(
		&reservation.ID,
		&reservation.StoreID,
		&reservation.ReferenceType,
		&reservation.ReferenceID,
		&reservation.Amount,
		&reservation.Status,
		&reservation.CreatedAt,
		&reservation.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return LedgerReservation{}, ErrNotFound
		}

		return LedgerReservation{}, fmt.Errorf("update ledger reservation status: %w", err)
	}

	return reservation, nil
}

func listEntriesByReferenceTx(ctx context.Context, tx pgx.Tx, referenceType string, referenceID string) ([]LedgerEntry, error) {
	rows, err := tx.Query(ctx, `
		SELECT
			id,
			store_id,
			ledger_account_id,
			direction,
			entry_type,
			amount::text,
			balance_after::text,
			reference_type,
			reference_id::text,
			metadata_json,
			created_at
		FROM ledger_entries
		WHERE reference_type = $1 AND reference_id = $2
		ORDER BY created_at ASC, id ASC
	`, referenceType, referenceID)
	if err != nil {
		return nil, fmt.Errorf("list ledger entries by reference: %w", err)
	}
	defer rows.Close()

	var entries []LedgerEntry
	for rows.Next() {
		var entry LedgerEntry
		if err := rows.Scan(
			&entry.ID,
			&entry.StoreID,
			&entry.LedgerAccountID,
			&entry.Direction,
			&entry.EntryType,
			&entry.Amount,
			&entry.BalanceAfter,
			&entry.ReferenceType,
			&entry.ReferenceID,
			&entry.Metadata,
			&entry.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan ledger entry: %w", err)
		}

		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ledger entries: %w", err)
	}

	return entries, nil
}

func isUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}

	return pgErr.Code == "23505" && pgErr.ConstraintName == constraint
}
