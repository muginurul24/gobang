package ledger

import (
	"context"
	"strings"
)

type repositoryContract interface {
	GetBalance(ctx context.Context, storeID string) (BalanceSnapshot, error)
	PostEntry(ctx context.Context, params postEntryParams) (PostingResult, error)
	PostEntries(ctx context.Context, params postEntriesParams) (BatchPostingResult, error)
	HasReferenceEntries(ctx context.Context, referenceType string, referenceID string) (bool, error)
	Reserve(ctx context.Context, params reserveParams) (ReservationResult, error)
	CommitReservation(ctx context.Context, params commitReservationParams) (CommitReservationResult, error)
	ReleaseReservation(ctx context.Context, params releaseReservationParams) (ReservationResult, error)
}

type Service interface {
	GetBalance(ctx context.Context, storeID string) (BalanceSnapshot, error)
	Credit(ctx context.Context, storeID string, input PostEntryInput) (PostingResult, error)
	Debit(ctx context.Context, storeID string, input PostEntryInput) (PostingResult, error)
	PostEntries(ctx context.Context, storeID string, input PostEntriesInput) (BatchPostingResult, error)
	HasReferenceEntries(ctx context.Context, referenceType string, referenceID string) (bool, error)
	Reserve(ctx context.Context, storeID string, input ReserveInput) (ReservationResult, error)
	CommitReservation(ctx context.Context, storeID string, input CommitReservationInput) (CommitReservationResult, error)
	ReleaseReservation(ctx context.Context, storeID string, input ReleaseReservationInput) (ReservationResult, error)
}

type service struct {
	repository repositoryContract
}

func NewService(repository repositoryContract) Service {
	return &service{repository: repository}
}

func (s *service) GetBalance(ctx context.Context, storeID string) (BalanceSnapshot, error) {
	if strings.TrimSpace(storeID) == "" {
		return BalanceSnapshot{}, ErrNotFound
	}

	return s.repository.GetBalance(ctx, strings.TrimSpace(storeID))
}

func (s *service) Credit(ctx context.Context, storeID string, input PostEntryInput) (PostingResult, error) {
	params, err := normalizePostEntryParams(storeID, DirectionCredit, input)
	if err != nil {
		return PostingResult{}, err
	}

	return s.repository.PostEntry(ctx, params)
}

func (s *service) Debit(ctx context.Context, storeID string, input PostEntryInput) (PostingResult, error) {
	params, err := normalizePostEntryParams(storeID, DirectionDebit, input)
	if err != nil {
		return PostingResult{}, err
	}

	return s.repository.PostEntry(ctx, params)
}

func (s *service) PostEntries(ctx context.Context, storeID string, input PostEntriesInput) (BatchPostingResult, error) {
	normalizedStoreID := strings.TrimSpace(storeID)
	if normalizedStoreID == "" {
		return BatchPostingResult{}, ErrNotFound
	}

	if invalidReference(input.ReferenceType, input.ReferenceID) {
		return BatchPostingResult{}, ErrInvalidReference
	}

	if len(input.Entries) == 0 {
		return BatchPostingResult{}, ErrInvalidAmount
	}

	entries := make([]batchPostEntryParams, 0, len(input.Entries))
	for _, entry := range input.Entries {
		amount, err := parseMoney(entry.Amount)
		if err != nil || amount.LessThan(1) {
			return BatchPostingResult{}, ErrInvalidAmount
		}
		if !validDirection(entry.Direction) {
			return BatchPostingResult{}, ErrInvalidDirection
		}
		if !validEntryType(entry.EntryType) {
			return BatchPostingResult{}, ErrInvalidEntryType
		}

		entries = append(entries, batchPostEntryParams{
			Direction: entry.Direction,
			EntryType: entry.EntryType,
			Amount:    amount.String(),
			Metadata:  entry.Metadata,
		})
	}

	return s.repository.PostEntries(ctx, postEntriesParams{
		StoreID:       normalizedStoreID,
		ReferenceType: strings.TrimSpace(input.ReferenceType),
		ReferenceID:   strings.TrimSpace(input.ReferenceID),
		Entries:       entries,
	})
}

func (s *service) HasReferenceEntries(ctx context.Context, referenceType string, referenceID string) (bool, error) {
	if invalidReference(referenceType, referenceID) {
		return false, ErrInvalidReference
	}

	return s.repository.HasReferenceEntries(ctx, strings.TrimSpace(referenceType), strings.TrimSpace(referenceID))
}

func (s *service) Reserve(ctx context.Context, storeID string, input ReserveInput) (ReservationResult, error) {
	normalizedStoreID := strings.TrimSpace(storeID)
	if normalizedStoreID == "" {
		return ReservationResult{}, ErrNotFound
	}

	amount, err := parseMoney(input.Amount)
	if err != nil || amount.LessThan(1) {
		return ReservationResult{}, ErrInvalidAmount
	}

	if invalidReference(input.ReferenceType, input.ReferenceID) {
		return ReservationResult{}, ErrInvalidReference
	}

	return s.repository.Reserve(ctx, reserveParams{
		StoreID:       normalizedStoreID,
		Amount:        amount.String(),
		ReferenceType: strings.TrimSpace(input.ReferenceType),
		ReferenceID:   strings.TrimSpace(input.ReferenceID),
	})
}

func (s *service) CommitReservation(ctx context.Context, storeID string, input CommitReservationInput) (CommitReservationResult, error) {
	normalizedStoreID := strings.TrimSpace(storeID)
	if normalizedStoreID == "" {
		return CommitReservationResult{}, ErrNotFound
	}

	if invalidReference(input.ReferenceType, input.ReferenceID) {
		return CommitReservationResult{}, ErrInvalidReference
	}

	if len(input.Entries) == 0 {
		return CommitReservationResult{}, ErrInvalidReservationCommit
	}

	entries := make([]commitEntryParams, 0, len(input.Entries))
	total := money(0)
	for _, entry := range input.Entries {
		amount, err := parseMoney(entry.Amount)
		if err != nil || amount.LessThan(1) {
			return CommitReservationResult{}, ErrInvalidReservationCommit
		}

		if !validEntryType(entry.EntryType) {
			return CommitReservationResult{}, ErrInvalidEntryType
		}

		total = total.Add(amount)
		entries = append(entries, commitEntryParams{
			EntryType: entry.EntryType,
			Amount:    amount.String(),
			Metadata:  entry.Metadata,
		})
	}

	if total.LessThan(1) {
		return CommitReservationResult{}, ErrInvalidReservationCommit
	}

	return s.repository.CommitReservation(ctx, commitReservationParams{
		StoreID:       normalizedStoreID,
		ReferenceType: strings.TrimSpace(input.ReferenceType),
		ReferenceID:   strings.TrimSpace(input.ReferenceID),
		Entries:       entries,
	})
}

func (s *service) ReleaseReservation(ctx context.Context, storeID string, input ReleaseReservationInput) (ReservationResult, error) {
	normalizedStoreID := strings.TrimSpace(storeID)
	if normalizedStoreID == "" {
		return ReservationResult{}, ErrNotFound
	}

	if invalidReference(input.ReferenceType, input.ReferenceID) {
		return ReservationResult{}, ErrInvalidReference
	}

	return s.repository.ReleaseReservation(ctx, releaseReservationParams{
		StoreID:       normalizedStoreID,
		ReferenceType: strings.TrimSpace(input.ReferenceType),
		ReferenceID:   strings.TrimSpace(input.ReferenceID),
	})
}

func normalizePostEntryParams(storeID string, direction Direction, input PostEntryInput) (postEntryParams, error) {
	normalizedStoreID := strings.TrimSpace(storeID)
	if normalizedStoreID == "" {
		return postEntryParams{}, ErrNotFound
	}

	amount, err := parseMoney(input.Amount)
	if err != nil || amount.LessThan(1) {
		return postEntryParams{}, ErrInvalidAmount
	}

	if !validEntryType(input.EntryType) {
		return postEntryParams{}, ErrInvalidEntryType
	}

	if invalidReference(input.ReferenceType, input.ReferenceID) {
		return postEntryParams{}, ErrInvalidReference
	}

	return postEntryParams{
		StoreID:       normalizedStoreID,
		Direction:     direction,
		EntryType:     input.EntryType,
		Amount:        amount.String(),
		ReferenceType: strings.TrimSpace(input.ReferenceType),
		ReferenceID:   strings.TrimSpace(input.ReferenceID),
		Metadata:      input.Metadata,
	}, nil
}

func invalidReference(referenceType string, referenceID string) bool {
	return strings.TrimSpace(referenceType) == "" || strings.TrimSpace(referenceID) == ""
}

func validDirection(direction Direction) bool {
	switch direction {
	case DirectionDebit, DirectionCredit:
		return true
	default:
		return false
	}
}

func validEntryType(entryType EntryType) bool {
	switch entryType {
	case EntryTypeGameDeposit,
		EntryTypeGameWithdraw,
		EntryTypeStoreTopup,
		EntryTypeMemberPaymentCredit,
		EntryTypeMemberPaymentFee,
		EntryTypeWithdrawReserve,
		EntryTypeWithdrawCommit,
		EntryTypeWithdrawRelease,
		EntryTypeWithdrawPlatformFee,
		EntryTypeWithdrawExternalFee:
		return true
	default:
		return false
	}
}
