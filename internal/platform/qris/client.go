package qris

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type Client struct {
	baseURL              string
	client               string
	clientKey            string
	globalUUID           string
	defaultExpireSeconds int
	httpClient           HTTPClient
	logger               *slog.Logger
}

func NewClient(cfg Config, logger *slog.Logger, httpClient HTTPClient) *Client {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	}
	if logger == nil {
		logger = slog.Default()
	}

	defaultExpire := cfg.DefaultExpireSeconds
	if defaultExpire <= 0 {
		defaultExpire = 300
	}

	return &Client{
		baseURL:              strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		client:               strings.TrimSpace(cfg.Client),
		clientKey:            strings.TrimSpace(cfg.ClientKey),
		globalUUID:           strings.TrimSpace(cfg.GlobalUUID),
		defaultExpireSeconds: defaultExpire,
		httpClient:           httpClient,
		logger:               logger,
	}
}

func (c *Client) Generate(ctx context.Context, input GenerateInput) (GenerateResult, error) {
	username := strings.TrimSpace(input.Username)
	if username == "" || input.Amount <= 0 {
		return GenerateResult{}, ErrInvalidRequest
	}

	uuid := c.resolveUUID(input.UUID)
	if uuid == "" || c.baseURL == "" {
		return GenerateResult{}, ErrNotConfigured
	}

	expire := input.ExpireSeconds
	if expire <= 0 {
		expire = c.defaultExpireSeconds
	}

	payload := map[string]any{
		"username": username,
		"amount":   input.Amount,
		"uuid":     uuid,
		"expire":   expire,
	}
	if customRef := strings.TrimSpace(input.CustomRef); customRef != "" {
		payload["custom_ref"] = customRef
	}

	raw, err := c.post(ctx, "/api/generate", payload, configRequirements{})
	if err != nil {
		return GenerateResult{}, err
	}

	var envelope struct {
		Status    bool   `json:"status"`
		Data      string `json:"data"`
		TrxID     string `json:"trx_id"`
		ExpiredAt *int   `json:"expired_at"`
		Error     string `json:"error"`
	}
	if err := decodeJSON(raw, &envelope); err != nil {
		return GenerateResult{}, err
	}
	if !envelope.Status {
		return GenerateResult{}, newBusinessError(envelope.Error)
	}
	if strings.TrimSpace(envelope.Data) == "" || strings.TrimSpace(envelope.TrxID) == "" {
		return GenerateResult{}, ErrInvalidResponse
	}

	return GenerateResult{
		RawValue:  strings.TrimSpace(envelope.Data),
		TrxID:     strings.TrimSpace(envelope.TrxID),
		ExpiredAt: envelope.ExpiredAt,
		IsVA:      envelope.ExpiredAt != nil,
	}, nil
}

func (c *Client) CheckStatus(ctx context.Context, input CheckStatusInput) (PaymentStatusResult, error) {
	trxID := strings.TrimSpace(input.TrxID)
	if trxID == "" {
		return PaymentStatusResult{}, ErrInvalidRequest
	}

	uuid := c.resolveUUID(input.UUID)
	payload := map[string]any{
		"uuid":   uuid,
		"client": c.client,
	}
	if c.clientKey != "" {
		payload["client_key"] = c.clientKey
	}

	raw, err := c.post(ctx, "/api/checkstatus/v2/"+url.PathEscape(trxID), payload, configRequirements{
		RequireClient: true,
		RequireUUID:   true,
	})
	if err != nil {
		return PaymentStatusResult{}, err
	}

	if businessErr := decodePossibleBusinessError(raw); businessErr != nil {
		return PaymentStatusResult{}, businessErr
	}

	var result struct {
		Amount     json.Number `json:"amount"`
		MerchantID string      `json:"merchant_id"`
		TrxID      string      `json:"trx_id"`
		RRN        string      `json:"rrn"`
		Status     string      `json:"status"`
		CreatedAt  string      `json:"created_at"`
		FinishAt   string      `json:"finish_at"`
		TerminalID string      `json:"terminal_id"`
		CustomRef  string      `json:"custom_ref"`
		Vendor     string      `json:"vendor"`
	}
	if err := decodeJSON(raw, &result); err != nil {
		return PaymentStatusResult{}, err
	}

	amount, err := parseIntegerNumber(result.Amount)
	if err != nil {
		return PaymentStatusResult{}, err
	}

	status := strings.TrimSpace(result.Status)
	if status == "" {
		return PaymentStatusResult{}, ErrInvalidResponse
	}

	return PaymentStatusResult{
		Amount:     amount,
		MerchantID: strings.TrimSpace(result.MerchantID),
		TrxID:      strings.TrimSpace(result.TrxID),
		RRN:        strings.TrimSpace(result.RRN),
		Status:     status,
		CreatedAt:  parseOptionalTime(result.CreatedAt),
		FinishAt:   parseOptionalTime(result.FinishAt),
		TerminalID: strings.TrimSpace(result.TerminalID),
		CustomRef:  strings.TrimSpace(result.CustomRef),
		Vendor:     strings.TrimSpace(result.Vendor),
	}, nil
}

func (c *Client) InquiryTransfer(ctx context.Context, input InquiryTransferInput) (InquiryTransferResult, error) {
	bankCode := strings.TrimSpace(input.BankCode)
	accountNumber := strings.TrimSpace(input.AccountNumber)
	if bankCode == "" || accountNumber == "" || input.Amount <= 0 || input.TransferType <= 0 {
		return InquiryTransferResult{}, ErrInvalidRequest
	}

	payload := map[string]any{
		"client":         c.client,
		"client_key":     c.clientKey,
		"uuid":           c.resolveUUID(input.UUID),
		"amount":         input.Amount,
		"bank_code":      bankCode,
		"account_number": accountNumber,
		"type":           input.TransferType,
	}
	if note := strings.TrimSpace(input.Note); note != "" {
		payload["note"] = note
	}
	if clientRefID := strings.TrimSpace(input.ClientRefID); clientRefID != "" {
		payload["client_ref_id"] = clientRefID
	}

	raw, err := c.post(ctx, "/api/inquiry", payload, configRequirements{
		RequireClient:    true,
		RequireClientKey: true,
		RequireUUID:      true,
	})
	if err != nil {
		return InquiryTransferResult{}, err
	}

	var envelope struct {
		Status bool `json:"status"`
		Data   struct {
			AccountNumber string      `json:"account_number"`
			AccountName   string      `json:"account_name"`
			BankCode      string      `json:"bank_code"`
			BankName      string      `json:"bank_name"`
			PartnerRefNo  string      `json:"partner_ref_no"`
			VendorRefNo   string      `json:"vendor_ref_no"`
			Amount        json.Number `json:"amount"`
			Fee           json.Number `json:"fee"`
			InquiryID     json.Number `json:"inquiry_id"`
		} `json:"data"`
		Error string `json:"error"`
	}
	if err := decodeJSON(raw, &envelope); err != nil {
		return InquiryTransferResult{}, err
	}
	if !envelope.Status {
		return InquiryTransferResult{}, newBusinessError(envelope.Error)
	}

	amount, err := parseIntegerNumber(envelope.Data.Amount)
	if err != nil {
		return InquiryTransferResult{}, err
	}
	fee, err := parseIntegerNumber(envelope.Data.Fee)
	if err != nil {
		return InquiryTransferResult{}, err
	}
	inquiryID, err := parseIntegerNumber(envelope.Data.InquiryID)
	if err != nil {
		return InquiryTransferResult{}, err
	}

	return InquiryTransferResult{
		AccountNumber: strings.TrimSpace(envelope.Data.AccountNumber),
		AccountName:   strings.TrimSpace(envelope.Data.AccountName),
		BankCode:      strings.TrimSpace(envelope.Data.BankCode),
		BankName:      strings.TrimSpace(envelope.Data.BankName),
		PartnerRefNo:  strings.TrimSpace(envelope.Data.PartnerRefNo),
		VendorRefNo:   strings.TrimSpace(envelope.Data.VendorRefNo),
		Amount:        amount,
		Fee:           fee,
		InquiryID:     inquiryID,
	}, nil
}

func (c *Client) Transfer(ctx context.Context, input TransferInput) (TransferResult, error) {
	bankCode := strings.TrimSpace(input.BankCode)
	accountNumber := strings.TrimSpace(input.AccountNumber)
	if bankCode == "" || accountNumber == "" || input.Amount <= 0 || input.TransferType <= 0 || input.InquiryID <= 0 {
		return TransferResult{}, ErrInvalidRequest
	}

	payload := map[string]any{
		"client":         c.client,
		"client_key":     c.clientKey,
		"uuid":           c.resolveUUID(input.UUID),
		"amount":         input.Amount,
		"bank_code":      bankCode,
		"account_number": accountNumber,
		"type":           input.TransferType,
		"inquiry_id":     input.InquiryID,
	}

	raw, err := c.post(ctx, "/api/transfer", payload, configRequirements{
		RequireClient:    true,
		RequireClientKey: true,
		RequireUUID:      true,
	})
	if err != nil {
		return TransferResult{}, err
	}

	var envelope struct {
		Status bool   `json:"status"`
		Error  string `json:"error"`
	}
	if err := decodeJSON(raw, &envelope); err != nil {
		return TransferResult{}, err
	}
	if !envelope.Status {
		return TransferResult{}, newBusinessError(envelope.Error)
	}

	return TransferResult{Accepted: true}, nil
}

func (c *Client) CheckDisbursementStatus(ctx context.Context, input CheckDisbursementStatusInput) (DisbursementStatusResult, error) {
	partnerRefNo := strings.TrimSpace(input.PartnerRefNo)
	if partnerRefNo == "" {
		return DisbursementStatusResult{}, ErrInvalidRequest
	}

	payload := map[string]any{
		"client": c.client,
		"uuid":   c.resolveUUID(input.UUID),
	}

	raw, err := c.post(ctx, "/api/disbursement/check-status/"+url.PathEscape(partnerRefNo), payload, configRequirements{
		RequireClient: true,
		RequireUUID:   true,
	})
	if err != nil {
		return DisbursementStatusResult{}, err
	}

	if businessErr := decodePossibleBusinessError(raw); businessErr != nil {
		return DisbursementStatusResult{}, businessErr
	}

	var result struct {
		Amount       json.Number `json:"amount"`
		Fee          json.Number `json:"fee"`
		PartnerRefNo string      `json:"partner_ref_no"`
		MerchantUUID string      `json:"merchant_uuid"`
		Status       string      `json:"status"`
	}
	if err := decodeJSON(raw, &result); err != nil {
		return DisbursementStatusResult{}, err
	}

	amount, err := parseIntegerNumber(result.Amount)
	if err != nil {
		return DisbursementStatusResult{}, err
	}
	fee, err := parseIntegerNumber(result.Fee)
	if err != nil {
		return DisbursementStatusResult{}, err
	}

	if strings.TrimSpace(result.Status) == "" {
		return DisbursementStatusResult{}, ErrInvalidResponse
	}

	return DisbursementStatusResult{
		Amount:       amount,
		Fee:          fee,
		PartnerRefNo: strings.TrimSpace(result.PartnerRefNo),
		MerchantUUID: strings.TrimSpace(result.MerchantUUID),
		Status:       strings.TrimSpace(result.Status),
	}, nil
}

func ParsePaymentWebhook(raw []byte) (PaymentWebhook, error) {
	var payload struct {
		Amount     json.Number `json:"amount"`
		TerminalID string      `json:"terminal_id"`
		TrxID      string      `json:"trx_id"`
		RRN        string      `json:"rrn"`
		CustomRef  string      `json:"custom_ref"`
		Vendor     string      `json:"vendor"`
		Status     string      `json:"status"`
		CreatedAt  string      `json:"create_at"`
		FinishAt   string      `json:"finish_at"`
	}
	if err := decodeJSON(raw, &payload); err != nil {
		return PaymentWebhook{}, err
	}

	amount, err := parseIntegerNumber(payload.Amount)
	if err != nil {
		return PaymentWebhook{}, err
	}
	if strings.TrimSpace(payload.TrxID) == "" || strings.TrimSpace(payload.Status) == "" {
		return PaymentWebhook{}, ErrInvalidResponse
	}

	return PaymentWebhook{
		Amount:     amount,
		TerminalID: strings.TrimSpace(payload.TerminalID),
		TrxID:      strings.TrimSpace(payload.TrxID),
		RRN:        strings.TrimSpace(payload.RRN),
		CustomRef:  strings.TrimSpace(payload.CustomRef),
		Vendor:     strings.TrimSpace(payload.Vendor),
		Status:     strings.TrimSpace(payload.Status),
		CreatedAt:  parseOptionalTime(payload.CreatedAt),
		FinishAt:   parseOptionalTime(payload.FinishAt),
	}, nil
}

func ParseTransferWebhook(raw []byte) (TransferWebhook, error) {
	var payload struct {
		Amount          json.Number `json:"amount"`
		PartnerRefNo    string      `json:"partner_ref_no"`
		Status          string      `json:"status"`
		TransactionDate string      `json:"transaction_date"`
		MerchantID      string      `json:"merchant_id"`
	}
	if err := decodeJSON(raw, &payload); err != nil {
		return TransferWebhook{}, err
	}

	amount, err := parseIntegerNumber(payload.Amount)
	if err != nil {
		return TransferWebhook{}, err
	}
	if strings.TrimSpace(payload.PartnerRefNo) == "" || strings.TrimSpace(payload.Status) == "" {
		return TransferWebhook{}, ErrInvalidResponse
	}

	return TransferWebhook{
		Amount:          amount,
		PartnerRefNo:    strings.TrimSpace(payload.PartnerRefNo),
		Status:          strings.TrimSpace(payload.Status),
		TransactionDate: parseOptionalTime(payload.TransactionDate),
		MerchantID:      strings.TrimSpace(payload.MerchantID),
	}, nil
}

type configRequirements struct {
	RequireClient    bool
	RequireClientKey bool
	RequireUUID      bool
}

func (c *Client) post(ctx context.Context, path string, payload map[string]any, requirements configRequirements) ([]byte, error) {
	if c.baseURL == "" {
		return nil, ErrNotConfigured
	}
	if requirements.RequireClient && strings.TrimSpace(c.client) == "" {
		return nil, ErrNotConfigured
	}
	if requirements.RequireClientKey && strings.TrimSpace(c.clientKey) == "" {
		return nil, ErrNotConfigured
	}
	if requirements.RequireUUID {
		if uuid, _ := payload["uuid"].(string); strings.TrimSpace(uuid) == "" {
			return nil, ErrNotConfigured
		}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal qris request: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build qris request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")

	startedAt := time.Now()
	response, err := c.httpClient.Do(request)
	if err != nil {
		duration := time.Since(startedAt)
		if errors.Is(err, context.DeadlineExceeded) {
			c.logger.Warn("qris_timeout",
				slog.String("path", path),
				slog.Duration("duration", duration),
				slog.Any("request_masked", maskedRequest(payload)),
			)
			return nil, ErrTimeout
		}

		c.logger.Error("qris_transport_error",
			slog.String("path", path),
			slog.Duration("duration", duration),
			slog.String("error", err.Error()),
			slog.Any("request_masked", maskedRequest(payload)),
		)
		return nil, fmt.Errorf("%w: %v", ErrUpstreamUnavailable, err)
	}
	defer response.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read qris response: %w", err)
	}

	duration := time.Since(startedAt)
	if response.StatusCode != http.StatusOK {
		c.logger.Error("qris_http_error",
			slog.String("path", path),
			slog.Int("status_code", response.StatusCode),
			slog.Duration("duration", duration),
			slog.Any("request_masked", maskedRequest(payload)),
			slog.Any("response_masked", maskedResponse(raw)),
		)
		return nil, fmt.Errorf("%w: %d", ErrUnexpectedHTTP, response.StatusCode)
	}

	c.logger.Info("qris_request",
		slog.String("path", path),
		slog.Duration("duration", duration),
		slog.Any("request_masked", maskedRequest(payload)),
		slog.Any("response_masked", maskedResponse(raw)),
	)

	return raw, nil
}

func (c *Client) resolveUUID(override string) string {
	if strings.TrimSpace(override) != "" {
		return strings.TrimSpace(override)
	}

	return c.globalUUID
}

func decodeJSON(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidResponse, err)
	}

	return nil
}

func decodePossibleBusinessError(raw []byte) *BusinessError {
	var payload struct {
		Status json.RawMessage `json:"status"`
		Error  string          `json:"error"`
	}
	if err := decodeJSON(raw, &payload); err != nil {
		return nil
	}
	if string(payload.Status) != "false" {
		return nil
	}

	return newBusinessError(payload.Error)
}

func newBusinessError(message string) *BusinessError {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		trimmed = "unknown error"
	}

	return &BusinessError{
		Code:    normalizeBusinessCode(trimmed),
		Message: trimmed,
	}
}

func parseIntegerNumber(number json.Number) (int64, error) {
	if strings.TrimSpace(number.String()) == "" {
		return 0, ErrInvalidResponse
	}

	if value, err := number.Int64(); err == nil {
		return value, nil
	}

	floatValue, err := strconv.ParseFloat(number.String(), 64)
	if err != nil {
		return 0, ErrInvalidResponse
	}

	return int64(floatValue), nil
}

func parseOptionalTime(raw string) *time.Time {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}

	parsed, err := time.Parse(time.RFC3339, trimmed)
	if err != nil {
		return nil
	}

	return &parsed
}

func maskedRequest(payload map[string]any) map[string]any {
	masked := map[string]any{}
	for key, value := range payload {
		switch key {
		case "username":
			masked["username_masked"] = maskValue(stringValue(value))
		case "client_key":
			masked["client_key_masked"] = maskValue(stringValue(value))
		case "account_number":
			masked["account_number_masked"] = maskValue(stringValue(value))
		case "uuid":
			masked["uuid_masked"] = maskValue(stringValue(value))
		case "client_ref_id":
			masked["client_ref_id"] = stringValue(value)
		default:
			masked[key] = value
		}
	}

	return masked
}

func maskedResponse(raw []byte) map[string]any {
	var payload map[string]any
	if err := decodeJSON(raw, &payload); err != nil {
		return map[string]any{
			"body": "[unparseable]",
		}
	}

	masked := map[string]any{}
	for key, value := range payload {
		switch key {
		case "data":
			if text, ok := value.(string); ok {
				masked["data_masked"] = maskValue(text)
				masked["data_length"] = len(text)
				continue
			}

			if nested, ok := value.(map[string]any); ok {
				masked["data"] = maskedNestedData(nested)
				continue
			}
			masked[key] = value
		case "trx_id":
			masked["trx_id_masked"] = maskValue(stringValue(value))
		case "partner_ref_no":
			masked["partner_ref_no_masked"] = maskValue(stringValue(value))
		case "rrn":
			masked["rrn_masked"] = maskValue(stringValue(value))
		default:
			masked[key] = value
		}
	}

	return masked
}

func maskedNestedData(payload map[string]any) map[string]any {
	masked := map[string]any{}
	for key, value := range payload {
		switch key {
		case "account_number":
			masked["account_number_masked"] = maskValue(stringValue(value))
		case "account_name":
			masked["account_name_masked"] = maskName(stringValue(value))
		case "partner_ref_no":
			masked["partner_ref_no_masked"] = maskValue(stringValue(value))
		default:
			masked[key] = value
		}
	}

	return masked
}

func normalizeBusinessCode(raw string) string {
	trimmed := strings.TrimSpace(strings.Trim(raw, "."))
	if trimmed == "" {
		return "UNKNOWN_ERROR"
	}

	var builder strings.Builder
	lastUnderscore := false
	for _, char := range strings.ToUpper(trimmed) {
		if (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') {
			builder.WriteRune(char)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			builder.WriteByte('_')
			lastUnderscore = true
		}
	}

	return strings.Trim(builder.String(), "_")
}

func maskValue(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if len(trimmed) <= 4 {
		return strings.Repeat("*", len(trimmed))
	}

	return trimmed[:2] + strings.Repeat("*", len(trimmed)-4) + trimmed[len(trimmed)-2:]
}

func maskName(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}

	parts := strings.Fields(trimmed)
	for index, part := range parts {
		if len(part) <= 1 {
			continue
		}

		parts[index] = part[:1] + strings.Repeat("*", len(part)-1)
	}

	return strings.Join(parts, " ")
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	default:
		return fmt.Sprint(value)
	}
}
