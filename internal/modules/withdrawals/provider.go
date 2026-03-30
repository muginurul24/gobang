package withdrawals

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/mugiew/onixggr/internal/platform/bankdirectory"
	"github.com/mugiew/onixggr/internal/platform/qris"
)

type BankDirectory interface {
	HasCode(code string) bool
	PrimaryByCode(code string) (bankdirectory.Entry, bool)
}

type Provider interface {
	Inquiry(ctx context.Context, input ProviderInquiryInput) (ProviderInquiryResult, error)
	Transfer(ctx context.Context, input ProviderTransferInput) (ProviderTransferResult, error)
}

type ProviderConfig struct {
	BaseURL      string
	Client       string
	ClientKey    string
	GlobalUUID   string
	TransferType int
	Timeout      time.Duration
}

func NewProvider(cfg ProviderConfig, directory BankDirectory) Provider {
	if shouldUseMockProvider(cfg) {
		return &mockProvider{directory: directory}
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	client := qris.NewClient(qris.Config{
		BaseURL:              cfg.BaseURL,
		Client:               cfg.Client,
		ClientKey:            cfg.ClientKey,
		GlobalUUID:           cfg.GlobalUUID,
		DefaultExpireSeconds: 300,
		Timeout:              timeout,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)

	return &qrisProvider{
		client:       client,
		transferType: cfg.TransferType,
	}
}

type qrisProvider struct {
	client       *qris.Client
	transferType int
}

func (p *qrisProvider) Inquiry(ctx context.Context, input ProviderInquiryInput) (ProviderInquiryResult, error) {
	result, err := p.client.InquiryTransfer(ctx, qris.InquiryTransferInput{
		Amount:        input.Amount,
		BankCode:      strings.TrimSpace(input.BankCode),
		AccountNumber: strings.TrimSpace(input.AccountNumber),
		TransferType:  p.transferType,
		ClientRefID:   sanitizeClientRefID(input.IdempotencyKey),
	})
	if err != nil {
		return ProviderInquiryResult{}, err
	}

	return ProviderInquiryResult{
		AccountName:  strings.TrimSpace(result.AccountName),
		BankCode:     strings.TrimSpace(result.BankCode),
		BankName:     strings.TrimSpace(result.BankName),
		PartnerRefNo: strings.TrimSpace(result.PartnerRefNo),
		InquiryID:    strconv.FormatInt(result.InquiryID, 10),
		ExternalFee:  result.Fee * 100,
	}, nil
}

func (p *qrisProvider) Transfer(ctx context.Context, input ProviderTransferInput) (ProviderTransferResult, error) {
	inquiryID, err := strconv.ParseInt(strings.TrimSpace(input.InquiryID), 10, 64)
	if err != nil || inquiryID <= 0 {
		return ProviderTransferResult{}, qris.ErrInvalidRequest
	}

	result, err := p.client.Transfer(ctx, qris.TransferInput{
		Amount:        input.Amount,
		BankCode:      strings.TrimSpace(input.BankCode),
		AccountNumber: strings.TrimSpace(input.AccountNumber),
		TransferType:  p.transferType,
		InquiryID:     inquiryID,
	})
	if err != nil {
		return ProviderTransferResult{}, err
	}

	return ProviderTransferResult{Accepted: result.Accepted}, nil
}

type mockProvider struct {
	directory BankDirectory
}

func (p *mockProvider) Inquiry(_ context.Context, input ProviderInquiryInput) (ProviderInquiryResult, error) {
	bankCode := strings.TrimSpace(input.BankCode)
	if !p.directory.HasCode(bankCode) {
		return ProviderInquiryResult{}, &qris.BusinessError{Code: "INVALID_BANK_CODE", Message: "invalid bank code"}
	}

	entry, _ := p.directory.PrimaryByCode(bankCode)
	accountNumber := strings.TrimSpace(input.AccountNumber)
	last4 := accountNumber
	if len(last4) > 4 {
		last4 = last4[len(last4)-4:]
	}

	sum := sha256.Sum256([]byte(bankCode + ":" + accountNumber + ":" + strings.TrimSpace(input.IdempotencyKey)))
	inquiryID := int64(binary.BigEndian.Uint32(sum[:4])) + 1
	partnerRefNo := fmt.Sprintf("%x", sum[:20])

	return ProviderInquiryResult{
		AccountName:  "DEMO ACCOUNT " + last4,
		BankCode:     bankCode,
		BankName:     entry.BankName,
		PartnerRefNo: partnerRefNo,
		InquiryID:    strconv.FormatInt(inquiryID, 10),
		ExternalFee:  1800 * 100,
	}, nil
}

func (p *mockProvider) Transfer(context.Context, ProviderTransferInput) (ProviderTransferResult, error) {
	return ProviderTransferResult{Accepted: true}, nil
}

func shouldUseMockProvider(cfg ProviderConfig) bool {
	baseURL := strings.TrimSpace(cfg.BaseURL)

	return baseURL == "" ||
		strings.Contains(baseURL, "example-qris.local") ||
		strings.TrimSpace(cfg.Client) == "" ||
		strings.TrimSpace(cfg.ClientKey) == "" ||
		strings.TrimSpace(cfg.GlobalUUID) == ""
}

func classifyInquiryError(err error) error {
	var businessErr *qris.BusinessError

	switch {
	case errors.Is(err, qris.ErrTimeout), errors.Is(err, qris.ErrUpstreamUnavailable), errors.Is(err, qris.ErrUnexpectedHTTP), errors.Is(err, qris.ErrNotConfigured):
		return ErrInquiryUnavailable
	case errors.As(err, &businessErr), errors.Is(err, qris.ErrInvalidRequest), errors.Is(err, qris.ErrInvalidResponse):
		return ErrInquiryFailed
	default:
		return ErrInquiryFailed
	}
}

func classifyTransferError(err error) (error, bool) {
	var businessErr *qris.BusinessError

	switch {
	case errors.Is(err, qris.ErrTimeout), errors.Is(err, qris.ErrUpstreamUnavailable), errors.Is(err, qris.ErrUnexpectedHTTP):
		return ErrTransferUnavailable, true
	case errors.Is(err, qris.ErrNotConfigured):
		return ErrTransferUnavailable, true
	case errors.As(err, &businessErr), errors.Is(err, qris.ErrInvalidRequest), errors.Is(err, qris.ErrInvalidResponse):
		return ErrTransferFailed, false
	default:
		return ErrTransferFailed, false
	}
}
