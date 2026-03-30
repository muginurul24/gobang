package bankaccounts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mugiew/onixggr/internal/platform/bankdirectory"
)

type BankDirectory interface {
	Search(query string, limit int) []bankdirectory.Entry
	PrimaryByCode(code string) (bankdirectory.Entry, bool)
	HasCode(code string) bool
}

type AccountSealer interface {
	Seal(plain string) (string, error)
}

type InquiryVerifier interface {
	Verify(ctx context.Context, request InquiryRequest) (InquiryResult, error)
}

type InquiryVerifierConfig struct {
	BaseURL      string
	Client       string
	ClientKey    string
	GlobalUUID   string
	Amount       int64
	TransferType int
	Timeout      time.Duration
}

func NewInquiryVerifier(cfg InquiryVerifierConfig, directory BankDirectory) InquiryVerifier {
	if shouldUseMockInquiry(cfg) {
		return &mockInquiryVerifier{directory: directory}
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	return &httpInquiryVerifier{
		baseURL:      strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		client:       strings.TrimSpace(cfg.Client),
		clientKey:    strings.TrimSpace(cfg.ClientKey),
		globalUUID:   strings.TrimSpace(cfg.GlobalUUID),
		amount:       cfg.Amount,
		transferType: cfg.TransferType,
		httpClient:   &http.Client{Timeout: timeout},
	}
}

type httpInquiryVerifier struct {
	baseURL      string
	client       string
	clientKey    string
	globalUUID   string
	amount       int64
	transferType int
	httpClient   *http.Client
}

func (v *httpInquiryVerifier) Verify(ctx context.Context, request InquiryRequest) (InquiryResult, error) {
	payload := map[string]any{
		"client":         v.client,
		"client_key":     v.clientKey,
		"uuid":           v.globalUUID,
		"amount":         v.amount,
		"bank_code":      request.BankCode,
		"account_number": request.AccountNumber,
		"type":           v.transferType,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return InquiryResult{}, fmt.Errorf("encode inquiry request: %w", err)
	}

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, v.baseURL+"/api/inquiry", bytes.NewReader(body))
	if err != nil {
		return InquiryResult{}, fmt.Errorf("build inquiry request: %w", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")

	response, err := v.httpClient.Do(httpRequest)
	if err != nil {
		return InquiryResult{}, fmt.Errorf("%w: %v", ErrInquiryUnavailable, err)
	}
	defer response.Body.Close()

	var envelope struct {
		Status bool `json:"status"`
		Data   struct {
			AccountNumber string `json:"account_number"`
			AccountName   string `json:"account_name"`
			BankCode      string `json:"bank_code"`
			BankName      string `json:"bank_name"`
		} `json:"data"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return InquiryResult{}, fmt.Errorf("decode inquiry response: %w", err)
	}

	if response.StatusCode >= http.StatusBadRequest {
		if strings.TrimSpace(envelope.Error) != "" {
			return InquiryResult{}, fmt.Errorf("%w: %s", ErrInquiryFailed, envelope.Error)
		}

		return InquiryResult{}, ErrInquiryFailed
	}

	if !envelope.Status {
		if strings.TrimSpace(envelope.Error) != "" {
			return InquiryResult{}, fmt.Errorf("%w: %s", ErrInquiryFailed, envelope.Error)
		}

		return InquiryResult{}, ErrInquiryFailed
	}

	return InquiryResult{
		BankCode:      strings.TrimSpace(envelope.Data.BankCode),
		BankName:      strings.TrimSpace(envelope.Data.BankName),
		AccountNumber: strings.TrimSpace(envelope.Data.AccountNumber),
		AccountName:   strings.TrimSpace(envelope.Data.AccountName),
	}, nil
}

type mockInquiryVerifier struct {
	directory BankDirectory
}

func (v *mockInquiryVerifier) Verify(_ context.Context, request InquiryRequest) (InquiryResult, error) {
	if !v.directory.HasCode(request.BankCode) {
		return InquiryResult{}, ErrInvalidBankCode
	}

	entry, _ := v.directory.PrimaryByCode(request.BankCode)
	last4 := request.AccountNumber
	if len(last4) > 4 {
		last4 = last4[len(last4)-4:]
	}

	return InquiryResult{
		BankCode:      request.BankCode,
		BankName:      entry.BankName,
		AccountNumber: request.AccountNumber,
		AccountName:   "DEMO ACCOUNT " + last4,
	}, nil
}

func shouldUseMockInquiry(cfg InquiryVerifierConfig) bool {
	baseURL := strings.TrimSpace(cfg.BaseURL)

	return baseURL == "" ||
		strings.Contains(baseURL, "example-qris.local") ||
		strings.TrimSpace(cfg.Client) == "" ||
		strings.TrimSpace(cfg.ClientKey) == "" ||
		strings.TrimSpace(cfg.GlobalUUID) == ""
}
