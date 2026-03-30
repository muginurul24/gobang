package bankaccounts

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/mugiew/onixggr/internal/platform/bankdirectory"
	"github.com/mugiew/onixggr/internal/platform/qris"
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

	return &qrisInquiryVerifier{
		client: qris.NewClient(qris.Config{
			BaseURL:              cfg.BaseURL,
			Client:               cfg.Client,
			ClientKey:            cfg.ClientKey,
			GlobalUUID:           cfg.GlobalUUID,
			DefaultExpireSeconds: 300,
			Timeout:              timeout,
		}, slog.New(slog.NewTextHandler(io.Discard, nil)), nil, nil),
		amount:       cfg.Amount,
		transferType: cfg.TransferType,
	}
}

type qrisInquiryClient interface {
	InquiryTransfer(ctx context.Context, input qris.InquiryTransferInput) (qris.InquiryTransferResult, error)
}

type qrisInquiryVerifier struct {
	client       qrisInquiryClient
	amount       int64
	transferType int
}

func (v *qrisInquiryVerifier) Verify(ctx context.Context, request InquiryRequest) (InquiryResult, error) {
	result, err := v.client.InquiryTransfer(ctx, qris.InquiryTransferInput{
		Amount:        v.amount,
		BankCode:      request.BankCode,
		AccountNumber: request.AccountNumber,
		TransferType:  v.transferType,
	})
	if err != nil {
		var businessErr *qris.BusinessError
		switch {
		case errors.Is(err, qris.ErrTimeout), errors.Is(err, qris.ErrUpstreamUnavailable), errors.Is(err, qris.ErrUnexpectedHTTP), errors.Is(err, qris.ErrNotConfigured):
			return InquiryResult{}, fmt.Errorf("%w: %v", ErrInquiryUnavailable, err)
		case errors.As(err, &businessErr):
			return InquiryResult{}, fmt.Errorf("%w: %s", ErrInquiryFailed, businessErr.Message)
		default:
			return InquiryResult{}, fmt.Errorf("%w: %v", ErrInquiryFailed, err)
		}
	}

	return InquiryResult{
		BankCode:      strings.TrimSpace(result.BankCode),
		BankName:      strings.TrimSpace(result.BankName),
		AccountNumber: strings.TrimSpace(result.AccountNumber),
		AccountName:   strings.TrimSpace(result.AccountName),
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
