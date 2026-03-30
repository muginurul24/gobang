package paymentsqris

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mugiew/onixggr/internal/modules/auth"
	"github.com/mugiew/onixggr/internal/platform/qris"
)

func TestWebhookHandlerDispatchesPaymentPayload(t *testing.T) {
	service := &stubWebhookService{
		paymentResult: WebhookDispatchResult{
			Kind:      WebhookKindPayment,
			Processed: true,
			Reference: "trx-123",
		},
	}
	handler := NewHandler(service, nil)
	mux := http.NewServeMux()
	handler.Register(mux)

	request := httptest.NewRequest(http.MethodPost, "/v1/webhooks/qris", strings.NewReader(`{"amount":1000,"terminal_id":"member-alpha","trx_id":"trx-123","rrn":"rrn-1","custom_ref":"MPAY001","vendor":"NOBU","status":"success","create_at":"2024-05-06T09:35:44.000Z","finish_at":"2024-05-06T09:35:54.000Z"}`))
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if service.paymentCalls != 1 {
		t.Fatalf("paymentCalls = %d, want 1", service.paymentCalls)
	}
	if service.transferCalls != 0 {
		t.Fatalf("transferCalls = %d, want 0", service.transferCalls)
	}
}

func TestWebhookHandlerDispatchesTransferPayload(t *testing.T) {
	service := &stubWebhookService{
		transferResult: WebhookDispatchResult{
			Kind:      WebhookKindWithdrawalStatus,
			Processed: false,
			Reference: "partner-1",
		},
	}
	handler := NewHandler(service, nil)
	mux := http.NewServeMux()
	handler.Register(mux)

	request := httptest.NewRequest(http.MethodPost, "/v1/webhooks/qris", strings.NewReader(`{"amount":25000,"partner_ref_no":"partner-1","status":"failed","transaction_date":"2026-02-11T12:45:42.000Z","merchant_id":"uuid-toko"}`))
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if service.transferCalls != 1 {
		t.Fatalf("transferCalls = %d, want 1", service.transferCalls)
	}
	if service.paymentCalls != 0 {
		t.Fatalf("paymentCalls = %d, want 0", service.paymentCalls)
	}
}

type stubWebhookService struct {
	paymentCalls   int
	transferCalls  int
	paymentResult  WebhookDispatchResult
	transferResult WebhookDispatchResult
}

func (s *stubWebhookService) ListStoreTopups(context.Context, auth.Subject, string) ([]QRISTransaction, error) {
	return nil, nil
}

func (s *stubWebhookService) CreateStoreTopup(context.Context, auth.Subject, string, CreateStoreTopupInput, auth.RequestMetadata) (QRISTransaction, error) {
	return QRISTransaction{}, nil
}

func (s *stubWebhookService) CreateMemberPayment(context.Context, string, CreateMemberPaymentInput, auth.RequestMetadata) (QRISTransaction, error) {
	return QRISTransaction{}, nil
}

func (s *stubWebhookService) HandlePaymentWebhook(context.Context, qris.PaymentWebhook, auth.RequestMetadata) (WebhookDispatchResult, error) {
	s.paymentCalls++
	return s.paymentResult, nil
}

func (s *stubWebhookService) HandleTransferWebhook(context.Context, qris.TransferWebhook, auth.RequestMetadata) (WebhookDispatchResult, error) {
	s.transferCalls++
	return s.transferResult, nil
}
