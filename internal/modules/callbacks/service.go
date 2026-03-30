package callbacks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mugiew/onixggr/internal/platform/clock"
)

const memberPaymentSuccessEvent = "member_payment.success"

type RepositoryContract interface {
	FindMemberPaymentCallbackSource(ctx context.Context, qrisTransactionID string) (MemberPaymentCallbackSource, error)
	EnqueueOutboundCallback(ctx context.Context, params EnqueueOutboundCallbackParams) (OutboundCallback, error)
	ListDueOutboundCallbacks(ctx context.Context, now time.Time, limit int) ([]DueOutboundCallback, error)
	RecordAttempt(ctx context.Context, params RecordAttemptParams) error
}

type Dispatcher interface {
	Dispatch(ctx context.Context, callback DueOutboundCallback) (DispatchResult, error)
}

type Service interface {
	EnqueueMemberPaymentSuccess(ctx context.Context, qrisTransactionID string) error
	RunPending(ctx context.Context, limit int) (RunSummary, error)
}

type Options struct {
	Repository    RepositoryContract
	Dispatcher    Dispatcher
	Clock         clock.Clock
	SigningSecret string
}

type service struct {
	repository RepositoryContract
	dispatcher Dispatcher
	clock      clock.Clock
	signer     signer
}

func NewService(options Options) Service {
	now := options.Clock
	if now == nil {
		now = clock.SystemClock{}
	}

	dispatcher := options.Dispatcher
	if dispatcher == nil {
		dispatcher = noopDispatcher{}
	}

	secret := strings.TrimSpace(options.SigningSecret)
	if secret == "" {
		secret = "change-me-callback-signing-secret"
	}

	return &service{
		repository: options.Repository,
		dispatcher: dispatcher,
		clock:      now,
		signer:     newSigner(secret),
	}
}

func (s *service) EnqueueMemberPaymentSuccess(ctx context.Context, qrisTransactionID string) error {
	source, err := s.repository.FindMemberPaymentCallbackSource(ctx, qrisTransactionID)
	if err != nil {
		return err
	}

	if strings.TrimSpace(source.TransactionStatus) != "success" {
		return nil
	}

	payload := MemberPaymentSuccessPayload{
		EventType:     memberPaymentSuccessEvent,
		OccurredAt:    source.TransactionUpdateAt.UTC(),
		ReferenceType: "qris_transaction",
		ReferenceID:   source.QRISTransactionID,
		Data: MemberPaymentSuccessPayloadData{
			QRISTransactionID:   source.QRISTransactionID,
			StoreID:             source.StoreID,
			StoreMemberID:       source.StoreMemberID,
			RealUsername:        source.RealUsername,
			Status:              source.TransactionStatus,
			CustomRef:           source.CustomRef,
			ProviderTrxID:       source.ProviderTrxID,
			AmountGross:         source.AmountGross,
			PlatformFeeAmount:   source.PlatformFeeAmount,
			StoreCreditAmount:   source.StoreCreditAmount,
			TransactionUpdateAt: source.TransactionUpdateAt.UTC(),
		},
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal member payment callback payload: %w", err)
	}

	_, err = s.repository.EnqueueOutboundCallback(ctx, EnqueueOutboundCallbackParams{
		StoreID:       source.StoreID,
		EventType:     memberPaymentSuccessEvent,
		ReferenceType: "qris_transaction",
		ReferenceID:   source.QRISTransactionID,
		PayloadJSON:   payloadJSON,
		Signature:     s.signer.Sign(payloadJSON),
		OccurredAt:    s.clock.Now().UTC(),
	})
	if err != nil {
		return err
	}

	return nil
}

func (s *service) RunPending(ctx context.Context, limit int) (RunSummary, error) {
	if limit <= 0 {
		limit = 50
	}

	callbacks, err := s.repository.ListDueOutboundCallbacks(ctx, s.clock.Now().UTC(), limit)
	if err != nil {
		return RunSummary{}, err
	}

	summary := RunSummary{Scanned: len(callbacks)}

	var runErr error
	for _, callback := range callbacks {
		if err := s.processCallback(ctx, callback, &summary); err != nil {
			summary.Skipped++
			if runErr == nil {
				runErr = fmt.Errorf("process callback %s: %w", callback.ID, err)
			}
		}
	}

	return summary, runErr
}

func (s *service) processCallback(ctx context.Context, callback DueOutboundCallback, summary *RunSummary) error {
	now := s.clock.Now().UTC()
	attemptNo := callback.AttemptNo + 1

	result, dispatchErr := s.dispatcher.Dispatch(ctx, callback)
	if dispatchErr == nil && result.Success {
		err := s.repository.RecordAttempt(ctx, RecordAttemptParams{
			OutboundCallbackID: callback.ID,
			AttemptNo:          attemptNo,
			HTTPStatus:         result.HTTPStatus,
			Status:             AttemptStatusSuccess,
			ResponseBodyMasked: result.ResponseBodyMasked,
			NextRetryAt:        nil,
			CallbackStatus:     StatusSuccess,
			OccurredAt:         now,
		})
		if err != nil {
			return err
		}

		summary.Delivered++
		return nil
	}

	failureBody := result.ResponseBodyMasked
	if dispatchErr != nil {
		failureBody = maskResponseBody(dispatchErr.Error())
	}

	nextRetry := nextRetryAt(now, attemptNo)
	status := StatusRetrying
	var notification *NotificationParams
	if nextRetry == nil {
		status = StatusFailed
		notification = &NotificationParams{
			StoreID:   callback.StoreID,
			EventType: "callback.delivery_failed",
			Title:     "Callback delivery gagal",
			Body:      fmt.Sprintf("Callback %s ke endpoint toko gagal setelah %d percobaan.", callback.EventType, attemptNo),
		}
	}

	err := s.repository.RecordAttempt(ctx, RecordAttemptParams{
		OutboundCallbackID: callback.ID,
		AttemptNo:          attemptNo,
		HTTPStatus:         result.HTTPStatus,
		Status:             AttemptStatusFailed,
		ResponseBodyMasked: failureBody,
		NextRetryAt:        nextRetry,
		CallbackStatus:     status,
		OccurredAt:         now,
		Notification:       notification,
	})
	if err != nil {
		if errors.Is(err, ErrDuplicateAttempt) {
			return nil
		}
		return err
	}

	if status == StatusFailed {
		summary.Failed++
	} else {
		summary.Retrying++
	}

	return nil
}

type noopDispatcher struct{}

func (noopDispatcher) Dispatch(context.Context, DueOutboundCallback) (DispatchResult, error) {
	return DispatchResult{}, ErrNotFound
}
