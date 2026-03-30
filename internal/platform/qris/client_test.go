package qris

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestGenerateUsesDefaultsAndParsesSuccess(t *testing.T) {
	client := NewClient(Config{
		BaseURL:              "https://qris.test",
		GlobalUUID:           "merchant-uuid",
		DefaultExpireSeconds: 300,
		Timeout:              time.Second,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)), stubHTTPClient(func(req *http.Request) (*http.Response, error) {
		payload, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("ReadAll returned error: %v", err)
		}

		if !strings.Contains(string(payload), `"uuid":"merchant-uuid"`) {
			t.Fatalf("payload = %s, want default uuid", string(payload))
		}
		if !strings.Contains(string(payload), `"expire":300`) {
			t.Fatalf("payload = %s, want default expire", string(payload))
		}

		return jsonResponse(http.StatusOK, `{"status":true,"data":"000201...","trx_id":"trx-123"}`), nil
	}), nil)

	result, err := client.Generate(context.Background(), GenerateInput{
		Username: "owner-demo",
		Amount:   10000,
	})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	if result.TrxID != "trx-123" {
		t.Fatalf("TrxID = %q, want trx-123", result.TrxID)
	}
	if result.IsVA {
		t.Fatal("IsVA = true, want false")
	}
}

func TestCheckStatusParsesPendingSuccess(t *testing.T) {
	client := NewClient(Config{
		BaseURL:    "https://qris.test",
		Client:     "demo-client",
		ClientKey:  "demo-key",
		GlobalUUID: "merchant-uuid",
		Timeout:    time.Second,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)), stubHTTPClient(func(req *http.Request) (*http.Response, error) {
		if !strings.Contains(req.URL.Path, "/api/checkstatus/v2/trx-123") {
			t.Fatalf("path = %s, want checkstatus path", req.URL.Path)
		}

		return jsonResponse(http.StatusOK, `{"amount":1000,"merchant_id":"merchant-uuid","trx_id":"trx-123","status":"pending","created_at":"2024-05-06T09:35:44.000Z","finish_at":"2024-05-06T09:35:44.000Z"}`), nil
	}), nil)

	result, err := client.CheckStatus(context.Background(), CheckStatusInput{TrxID: "trx-123"})
	if err != nil {
		t.Fatalf("CheckStatus returned error: %v", err)
	}

	if result.Status != "pending" {
		t.Fatalf("Status = %q, want pending", result.Status)
	}
	if result.Amount != 1000 {
		t.Fatalf("Amount = %d, want 1000", result.Amount)
	}
}

func TestInquiryTransferParsesSuccess(t *testing.T) {
	client := NewClient(Config{
		BaseURL:    "https://qris.test",
		Client:     "demo-client",
		ClientKey:  "demo-key",
		GlobalUUID: "merchant-uuid",
		Timeout:    time.Second,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)), stubHTTPClient(func(req *http.Request) (*http.Response, error) {
		payload, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("ReadAll returned error: %v", err)
		}
		if !strings.Contains(string(payload), `"bank_code":"542"`) {
			t.Fatalf("payload = %s, want bank code", string(payload))
		}

		return jsonResponse(http.StatusOK, `{"status":true,"data":{"account_number":"100009689749","account_name":"SISKA DAMAYANTI","bank_code":"542","bank_name":"PT. BANK ARTOS INDONESIA (Bank Jago)","partner_ref_no":"03ce198c","vendor_ref_no":"","amount":700000,"fee":1800,"inquiry_id":2949850}}`), nil
	}), nil)

	result, err := client.InquiryTransfer(context.Background(), InquiryTransferInput{
		Amount:        700000,
		BankCode:      "542",
		AccountNumber: "100009689749",
		TransferType:  2,
	})
	if err != nil {
		t.Fatalf("InquiryTransfer returned error: %v", err)
	}

	if result.InquiryID != 2949850 {
		t.Fatalf("InquiryID = %d, want 2949850", result.InquiryID)
	}
	if result.Fee != 1800 {
		t.Fatalf("Fee = %d, want 1800", result.Fee)
	}
}

func TestTransferBusinessFailureNormalizesError(t *testing.T) {
	client := NewClient(Config{
		BaseURL:    "https://qris.test",
		Client:     "demo-client",
		ClientKey:  "demo-key",
		GlobalUUID: "merchant-uuid",
		Timeout:    time.Second,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)), stubHTTPClient(func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"status":false,"error":"Invalid client"}`), nil
	}), nil)

	_, err := client.Transfer(context.Background(), TransferInput{
		Amount:        25000,
		BankCode:      "014",
		AccountNumber: "0234567",
		TransferType:  2,
		InquiryID:     1,
	})

	var businessErr *BusinessError
	if !errors.As(err, &businessErr) {
		t.Fatalf("Transfer error = %v, want BusinessError", err)
	}
	if businessErr.Code != "INVALID_CLIENT" {
		t.Fatalf("BusinessError.Code = %s, want INVALID_CLIENT", businessErr.Code)
	}
}

func TestCheckDisbursementStatusParsesSuccess(t *testing.T) {
	client := NewClient(Config{
		BaseURL:    "https://qris.test",
		Client:     "demo-client",
		GlobalUUID: "merchant-uuid",
		Timeout:    time.Second,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)), stubHTTPClient(func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"amount":25000,"fee":1800,"partner_ref_no":"123444","merchant_uuid":"uuid-toko","status":"success"}`), nil
	}), nil)

	result, err := client.CheckDisbursementStatus(context.Background(), CheckDisbursementStatusInput{
		PartnerRefNo: "123444",
	})
	if err != nil {
		t.Fatalf("CheckDisbursementStatus returned error: %v", err)
	}

	if result.Status != "success" {
		t.Fatalf("Status = %q, want success", result.Status)
	}
}

func TestWebhookParsers(t *testing.T) {
	payment, err := ParsePaymentWebhook([]byte(`{"amount":1000,"terminal_id":"member-alpha","trx_id":"trx-123","rrn":"rrn-1","custom_ref":"abc","vendor":"qris-vendor","status":"success","create_at":"2024-05-06T09:35:44.000Z","finish_at":"2024-05-06T09:35:54.000Z"}`))
	if err != nil {
		t.Fatalf("ParsePaymentWebhook returned error: %v", err)
	}
	if payment.TrxID != "trx-123" {
		t.Fatalf("payment.TrxID = %q, want trx-123", payment.TrxID)
	}

	transfer, err := ParseTransferWebhook([]byte(`{"amount":25000,"partner_ref_no":"partner-1","status":"failed","transaction_date":"2026-02-11T12:45:42.000Z","merchant_id":"uuid-toko"}`))
	if err != nil {
		t.Fatalf("ParseTransferWebhook returned error: %v", err)
	}
	if transfer.Status != "failed" {
		t.Fatalf("transfer.Status = %q, want failed", transfer.Status)
	}
}

func TestLogsMaskSensitiveFields(t *testing.T) {
	var buffer bytes.Buffer
	client := NewClient(Config{
		BaseURL:    "https://qris.test",
		Client:     "demo-client",
		ClientKey:  "super-secret-key",
		GlobalUUID: "merchant-uuid",
		Timeout:    time.Second,
	}, slog.New(slog.NewTextHandler(&buffer, nil)), stubHTTPClient(func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"status":true,"data":{"account_number":"100009689749","account_name":"SISKA DAMAYANTI","bank_code":"542","bank_name":"PT. BANK ARTOS INDONESIA (Bank Jago)","partner_ref_no":"03ce198c0d2561c8","vendor_ref_no":"","amount":700000,"fee":1800,"inquiry_id":2949850}}`), nil
	}), nil)

	_, err := client.InquiryTransfer(context.Background(), InquiryTransferInput{
		Amount:        700000,
		BankCode:      "542",
		AccountNumber: "100009689749",
		TransferType:  2,
	})
	if err != nil {
		t.Fatalf("InquiryTransfer returned error: %v", err)
	}

	logOutput := buffer.String()
	if strings.Contains(logOutput, "super-secret-key") {
		t.Fatal("log output leaked client key")
	}
	if strings.Contains(logOutput, "100009689749") {
		t.Fatal("log output leaked account number")
	}
	if strings.Contains(logOutput, "SISKA DAMAYANTI") {
		t.Fatal("log output leaked account name")
	}
}

type stubHTTPClient func(req *http.Request) (*http.Response, error)

func (f stubHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

func jsonResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
		},
		Body: io.NopCloser(strings.NewReader(body)),
	}
}
