package withdrawals

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mugiew/onixggr/internal/modules/auth"
	"github.com/mugiew/onixggr/internal/modules/ledger"
	"github.com/mugiew/onixggr/internal/platform/qris"
)

const systemActorRole = "system"

func (s *service) HandleTransferWebhook(ctx context.Context, payload qris.TransferWebhook, metadata auth.RequestMetadata) (TransferWebhookResult, error) {
	result := TransferWebhookResult{
		Reference: strings.TrimSpace(payload.PartnerRefNo),
	}
	if result.Reference == "" {
		return result, nil
	}

	withdrawal, err := s.repository.FindByPartnerRefNo(ctx, result.Reference)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return result, nil
		}

		return TransferWebhookResult{}, err
	}

	result.WithdrawalID = &withdrawal.ID

	resolvedStatus, ok := resolveTransferStatus(payload.Status)
	if !ok {
		return result, nil
	}

	lock, locked, err := s.repository.AcquireProcessingLock(ctx, withdrawal.ID)
	if err != nil {
		return TransferWebhookResult{}, err
	}
	if !locked {
		return result, nil
	}
	defer func() {
		_ = lock.Unlock(ctx)
	}()

	withdrawal, err = s.repository.GetByID(ctx, withdrawal.ID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return result, nil
		}

		return TransferWebhookResult{}, err
	}

	finalized, err := s.finalizeTransferStatusLocked(ctx, withdrawal, resolvedStatus, transferStatusPayload(withdrawal, resolvedStatus, "webhook", map[string]any{
		"provider_status":  strings.ToLower(strings.TrimSpace(payload.Status)),
		"partner_ref_no":   strings.TrimSpace(payload.PartnerRefNo),
		"merchant_id":      strings.TrimSpace(payload.MerchantID),
		"transaction_date": formatOptionalTime(payload.TransactionDate),
		"provider_amount":  payload.Amount,
	}), metadata, "webhook")
	if err != nil {
		return TransferWebhookResult{}, err
	}

	result.Processed = true
	result.Status = &finalized.Status
	return result, nil
}

func (s *service) RunPendingChecks(ctx context.Context, limit int) (StatusCheckRunSummary, error) {
	if limit <= 0 {
		limit = 50
	}

	cutoff := s.clock.Now().UTC().Add(-s.statusCheckInterval)
	candidates, err := s.repository.ListDueStatusCheckWithdrawals(ctx, cutoff, limit)
	if err != nil {
		return StatusCheckRunSummary{}, err
	}

	summary := StatusCheckRunSummary{
		Scanned: len(candidates),
	}

	var runErr error
	for _, candidate := range candidates {
		outcome, err := s.runSinglePendingCheck(ctx, candidate.Withdrawal.ID)
		if err != nil {
			summary.StillPending++
			if runErr == nil {
				runErr = fmt.Errorf("status check withdrawal %s: %w", candidate.Withdrawal.ID, err)
			}
			continue
		}

		switch outcome {
		case StatusCheckOutcomeFinalizedSuccess:
			summary.FinalizedSuccess++
		case StatusCheckOutcomeFinalizedFailed:
			summary.FinalizedFailed++
		case StatusCheckOutcomeStillPending:
			summary.StillPending++
		default:
			summary.Skipped++
		}
	}

	return summary, runErr
}

func (s *service) runSinglePendingCheck(ctx context.Context, withdrawalID string) (outcome StatusCheckOutcome, err error) {
	lock, locked, err := s.repository.AcquireProcessingLock(ctx, withdrawalID)
	if err != nil {
		return StatusCheckOutcomeStillPending, err
	}
	if !locked {
		return StatusCheckOutcomeSkipped, nil
	}
	defer func() {
		if unlockErr := lock.Unlock(ctx); unlockErr != nil && err == nil {
			err = unlockErr
		}
	}()

	withdrawal, err := s.repository.GetByID(ctx, withdrawalID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return StatusCheckOutcomeSkipped, nil
		}

		return StatusCheckOutcomeStillPending, err
	}
	if withdrawal.Status != WithdrawalStatusPending || withdrawal.ProviderPartnerRefNo == nil || strings.TrimSpace(*withdrawal.ProviderPartnerRefNo) == "" {
		return StatusCheckOutcomeSkipped, nil
	}

	attemptNo, err := s.repository.NextStatusCheckAttemptNo(ctx, withdrawal.ID)
	if err != nil {
		return StatusCheckOutcomeStillPending, err
	}

	now := s.clock.Now().UTC()
	statusResult, err := s.provider.CheckStatus(ctx, ProviderStatusCheckInput{
		PartnerRefNo: strings.TrimSpace(*withdrawal.ProviderPartnerRefNo),
	})
	if err != nil {
		recordStatus := "upstream_error"
		response := map[string]any{
			"source":         "check_status",
			"partner_ref_no": strings.TrimSpace(*withdrawal.ProviderPartnerRefNo),
			"error":          err.Error(),
		}

		var businessErr *qris.BusinessError
		if errors.As(err, &businessErr) {
			recordStatus = "business_error"
			response["code"] = businessErr.Code
			response["message"] = businessErr.Message
		}

		if recordErr := s.repository.RecordStatusCheck(ctx, RecordStatusCheckParams{
			WithdrawalID:   withdrawal.ID,
			AttemptNo:      attemptNo,
			Status:         recordStatus,
			ResponseMasked: response,
			OccurredAt:     now,
		}); recordErr != nil {
			return StatusCheckOutcomeStillPending, recordErr
		}

		return StatusCheckOutcomeStillPending, nil
	}

	resolvedStatus, ok := resolveTransferStatus(statusResult.Status)
	recordStatus := strings.ToLower(strings.TrimSpace(statusResult.Status))
	if recordStatus == "" {
		recordStatus = "unknown"
	}

	response := map[string]any{
		"source":          "check_status",
		"partner_ref_no":  strings.TrimSpace(statusResult.PartnerRefNo),
		"merchant_id":     strings.TrimSpace(statusResult.MerchantID),
		"provider_amount": statusResult.Amount,
		"external_fee":    formatAmount(money(statusResult.ExternalFee)),
		"status":          recordStatus,
	}
	if err := s.repository.RecordStatusCheck(ctx, RecordStatusCheckParams{
		WithdrawalID:   withdrawal.ID,
		AttemptNo:      attemptNo,
		Status:         recordStatus,
		ResponseMasked: response,
		OccurredAt:     now,
	}); err != nil {
		return StatusCheckOutcomeStillPending, err
	}

	if !ok {
		return StatusCheckOutcomeStillPending, nil
	}

	finalized, err := s.finalizeTransferStatusLocked(ctx, withdrawal, resolvedStatus, transferStatusPayload(withdrawal, resolvedStatus, "status_check", response), auth.RequestMetadata{
		IPAddress: "127.0.0.1",
		UserAgent: "withdraw-status-worker",
	}, "status_check")
	if err != nil {
		return StatusCheckOutcomeStillPending, err
	}

	if finalized.Status == WithdrawalStatusSuccess {
		return StatusCheckOutcomeFinalizedSuccess, nil
	}
	if finalized.Status == WithdrawalStatusFailed {
		return StatusCheckOutcomeFinalizedFailed, nil
	}

	return StatusCheckOutcomeStillPending, nil
}

func (s *service) finalizeTransferStatusLocked(ctx context.Context, withdrawal StoreWithdrawal, desiredStatus WithdrawalStatus, providerPayload map[string]any, metadata auth.RequestMetadata, source string) (StoreWithdrawal, error) {
	if withdrawal.Status == WithdrawalStatusSuccess || withdrawal.Status == WithdrawalStatusFailed {
		return withdrawal, nil
	}

	var (
		finalized StoreWithdrawal
		err       error
	)
	switch desiredStatus {
	case WithdrawalStatusSuccess:
		finalized, err = s.finalizeSuccessLocked(ctx, withdrawal, providerPayload)
	case WithdrawalStatusFailed:
		finalized, err = s.finalizeFailedLocked(ctx, withdrawal, providerPayload)
	default:
		return withdrawal, nil
	}
	if err != nil {
		return StoreWithdrawal{}, err
	}

	action := "withdraw.failed"
	if finalized.Status == WithdrawalStatusSuccess {
		action = "withdraw.success"
	}
	if auditErr := s.insertSystemAudit(ctx, finalized, metadata, action, map[string]any{
		"source": source,
	}); auditErr != nil {
		return StoreWithdrawal{}, auditErr
	}

	switch finalized.Status {
	case WithdrawalStatusSuccess:
		s.notifications.Emit(finalized.StoreID, "withdraw.success",
			"Withdraw berhasil",
			fmt.Sprintf("Withdraw %s ke %s (%s) berhasil.", finalized.NetRequestedAmount, finalized.AccountName, finalized.BankName),
		)
		s.emitLowBalanceIfNeeded(ctx, finalized.StoreID)
	case WithdrawalStatusFailed:
		s.notifications.Emit(finalized.StoreID, "withdraw.failed",
			"Withdraw gagal",
			fmt.Sprintf("Withdraw %s ke %s (%s) gagal.", finalized.NetRequestedAmount, finalized.AccountName, finalized.BankName),
		)
	}

	return finalized, nil
}

func (s *service) finalizeSuccessLocked(ctx context.Context, withdrawal StoreWithdrawal, providerPayload map[string]any) (StoreWithdrawal, error) {
	alreadyPosted, err := s.ledger.HasReferenceEntries(ctx, ledgerReferenceType, withdrawal.ID)
	if err != nil {
		return StoreWithdrawal{}, err
	}

	if !alreadyPosted {
		_, err = s.ledger.CommitReservation(ctx, withdrawal.StoreID, ledger.CommitReservationInput{
			ReferenceType: ledgerReferenceType,
			ReferenceID:   withdrawal.ID,
			Entries:       commitEntriesForWithdrawal(withdrawal),
		})
		if err != nil {
			if !errors.Is(err, ledger.ErrReservationFinalized) {
				return StoreWithdrawal{}, err
			}

			alreadyPosted, err = s.ledger.HasReferenceEntries(ctx, ledgerReferenceType, withdrawal.ID)
			if err != nil {
				return StoreWithdrawal{}, err
			}
			if !alreadyPosted {
				return s.persistFinalStatus(ctx, withdrawal, WithdrawalStatusFailed, transferStatusPayload(withdrawal, WithdrawalStatusFailed, "reservation_state", map[string]any{
					"reason": "reservation_already_released",
				}))
			}
		}
	}

	return s.persistFinalStatus(ctx, withdrawal, WithdrawalStatusSuccess, providerPayload)
}

func (s *service) finalizeFailedLocked(ctx context.Context, withdrawal StoreWithdrawal, providerPayload map[string]any) (StoreWithdrawal, error) {
	alreadyPosted, err := s.ledger.HasReferenceEntries(ctx, ledgerReferenceType, withdrawal.ID)
	if err != nil {
		return StoreWithdrawal{}, err
	}
	if alreadyPosted {
		return s.persistFinalStatus(ctx, withdrawal, WithdrawalStatusSuccess, transferStatusPayload(withdrawal, WithdrawalStatusSuccess, "reservation_state", map[string]any{
			"reason": "already_committed",
		}))
	}

	if _, err := s.ledger.ReleaseReservation(ctx, withdrawal.StoreID, ledger.ReleaseReservationInput{
		ReferenceType: ledgerReferenceType,
		ReferenceID:   withdrawal.ID,
	}); err != nil {
		if !errors.Is(err, ledger.ErrReservationFinalized) {
			return StoreWithdrawal{}, err
		}

		alreadyPosted, err = s.ledger.HasReferenceEntries(ctx, ledgerReferenceType, withdrawal.ID)
		if err != nil {
			return StoreWithdrawal{}, err
		}
		if alreadyPosted {
			return s.persistFinalStatus(ctx, withdrawal, WithdrawalStatusSuccess, transferStatusPayload(withdrawal, WithdrawalStatusSuccess, "reservation_state", map[string]any{
				"reason": "already_committed",
			}))
		}
	}

	return s.persistFinalStatus(ctx, withdrawal, WithdrawalStatusFailed, providerPayload)
}

func (s *service) persistFinalStatus(ctx context.Context, withdrawal StoreWithdrawal, status WithdrawalStatus, payload map[string]any) (StoreWithdrawal, error) {
	return s.repository.UpdateStoreWithdrawal(ctx, UpdateStoreWithdrawalParams{
		WithdrawalID:    withdrawal.ID,
		Status:          statusPtr(status),
		ProviderPayload: payload,
		OccurredAt:      s.clock.Now().UTC(),
	})
}

func (s *service) insertSystemAudit(ctx context.Context, withdrawal StoreWithdrawal, metadata auth.RequestMetadata, action string, payload map[string]any) error {
	masked := map[string]any{
		"idempotency_key":       withdrawal.IdempotencyKey,
		"store_bank_account_id": withdrawal.StoreBankAccountID,
		"bank_code":             withdrawal.BankCode,
		"bank_name":             withdrawal.BankName,
		"account_name":          withdrawal.AccountName,
		"account_number_masked": withdrawal.AccountNumberMasked,
		"net_requested_amount":  withdrawal.NetRequestedAmount,
		"platform_fee_amount":   withdrawal.PlatformFeeAmount,
		"external_fee_amount":   withdrawal.ExternalFeeAmount,
		"total_store_debit":     withdrawal.TotalStoreDebit,
		"status":                withdrawal.Status,
	}
	for key, value := range payload {
		masked[key] = value
	}

	return s.repository.InsertAuditLog(
		ctx,
		nil,
		systemActorRole,
		&withdrawal.StoreID,
		action,
		"store_withdrawal",
		&withdrawal.ID,
		masked,
		metadata.IPAddress,
		metadata.UserAgent,
		s.clock.Now().UTC(),
	)
}

func resolveTransferStatus(raw string) (WithdrawalStatus, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(WithdrawalStatusSuccess):
		return WithdrawalStatusSuccess, true
	case string(WithdrawalStatusFailed):
		return WithdrawalStatusFailed, true
	default:
		return "", false
	}
}

func commitEntriesForWithdrawal(withdrawal StoreWithdrawal) []ledger.ReservationCommitEntryInput {
	entries := make([]ledger.ReservationCommitEntryInput, 0, 3)

	netRequestedAmount, err := parseMoneyString(withdrawal.NetRequestedAmount)
	if err == nil && netRequestedAmount > 0 {
		entries = append(entries, ledger.ReservationCommitEntryInput{
			EntryType: ledger.EntryTypeWithdrawCommit,
			Amount:    formatAmount(netRequestedAmount),
			Metadata: map[string]any{
				"store_withdrawal_id":     withdrawal.ID,
				"provider_partner_ref_no": nullableDereference(withdrawal.ProviderPartnerRefNo),
				"provider_inquiry_id":     nullableDereference(withdrawal.ProviderInquiryID),
			},
		})
	}

	platformFeeAmount, err := parseMoneyString(withdrawal.PlatformFeeAmount)
	if err == nil && platformFeeAmount > 0 {
		entries = append(entries, ledger.ReservationCommitEntryInput{
			EntryType: ledger.EntryTypeWithdrawPlatformFee,
			Amount:    formatAmount(platformFeeAmount),
			Metadata: map[string]any{
				"store_withdrawal_id": withdrawal.ID,
			},
		})
	}

	externalFeeAmount, err := parseMoneyString(withdrawal.ExternalFeeAmount)
	if err == nil && externalFeeAmount > 0 {
		entries = append(entries, ledger.ReservationCommitEntryInput{
			EntryType: ledger.EntryTypeWithdrawExternalFee,
			Amount:    formatAmount(externalFeeAmount),
			Metadata: map[string]any{
				"store_withdrawal_id": withdrawal.ID,
			},
		})
	}

	return entries
}

func transferStatusPayload(withdrawal StoreWithdrawal, status WithdrawalStatus, source string, extra map[string]any) map[string]any {
	payload := map[string]any{
		"provider_state":       providerStateForTransferStatus(status),
		"status_source":        strings.TrimSpace(source),
		"partner_ref_no":       nullableDereference(withdrawal.ProviderPartnerRefNo),
		"inquiry_id":           nullableDereference(withdrawal.ProviderInquiryID),
		"net_requested_amount": withdrawal.NetRequestedAmount,
		"platform_fee_amount":  withdrawal.PlatformFeeAmount,
		"external_fee_amount":  withdrawal.ExternalFeeAmount,
		"total_store_debit":    withdrawal.TotalStoreDebit,
	}
	for key, value := range extra {
		payload[key] = value
	}

	return payload
}

func providerStateForTransferStatus(status WithdrawalStatus) string {
	switch status {
	case WithdrawalStatusSuccess:
		return "transfer_success"
	case WithdrawalStatusFailed:
		return "transfer_failed"
	default:
		return "pending_transfer_confirmation"
	}
}

func formatOptionalTime(value *time.Time) *string {
	if value == nil {
		return nil
	}

	formatted := value.UTC().Format(time.RFC3339)
	return &formatted
}

func nullableDereference(value *string) string {
	if value == nil {
		return ""
	}

	return strings.TrimSpace(*value)
}
