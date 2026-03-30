package nexusggr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type Client struct {
	baseURL    string
	agentCode  string
	agentToken string
	httpClient HTTPClient
	logger     *slog.Logger
}

type responseEnvelope struct {
	Status int    `json:"status"`
	Msg    string `json:"msg"`
	Error  string `json:"error"`
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

	return &Client{
		baseURL:    strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		agentCode:  strings.TrimSpace(cfg.AgentCode),
		agentToken: strings.TrimSpace(cfg.AgentToken),
		httpClient: httpClient,
		logger:     logger,
	}
}

func (c *Client) ProviderList(ctx context.Context) (ProviderListResult, error) {
	var result ProviderListResult
	err := c.invoke(ctx, "provider_list", nil, &result)
	return result, err
}

func (c *Client) GameList(ctx context.Context, providerCode string) (GameListResult, error) {
	if strings.TrimSpace(providerCode) == "" {
		return GameListResult{}, ErrInvalidRequest
	}

	var result GameListResult
	err := c.invoke(ctx, "game_list", map[string]any{
		"provider_code": strings.TrimSpace(providerCode),
	}, &result)
	return result, err
}

func (c *Client) GameLaunch(ctx context.Context, input GameLaunchInput) (GameLaunchResult, error) {
	if strings.TrimSpace(input.UserCode) == "" || strings.TrimSpace(input.ProviderCode) == "" || strings.TrimSpace(input.Lang) == "" {
		return GameLaunchResult{}, ErrInvalidRequest
	}

	var result GameLaunchResult
	err := c.invoke(ctx, "game_launch", map[string]any{
		"user_code":     strings.TrimSpace(input.UserCode),
		"provider_code": strings.TrimSpace(input.ProviderCode),
		"game_code":     strings.TrimSpace(input.GameCode),
		"lang":          strings.TrimSpace(input.Lang),
	}, &result)
	return result, err
}

func (c *Client) MoneyInfo(ctx context.Context, input MoneyInfoInput) (MoneyInfoResult, error) {
	if strings.TrimSpace(input.UserCode) == "" && !input.AllUsers {
		return MoneyInfoResult{}, ErrInvalidRequest
	}

	payload := map[string]any{}
	if strings.TrimSpace(input.UserCode) != "" {
		payload["user_code"] = strings.TrimSpace(input.UserCode)
	}
	if input.AllUsers {
		payload["all_users"] = true
	}

	var result MoneyInfoResult
	err := c.invoke(ctx, "money_info", payload, &result)
	return result, err
}

func (c *Client) UserCreate(ctx context.Context, input UserCreateInput) (UserCreateResult, error) {
	if strings.TrimSpace(input.UserCode) == "" {
		return UserCreateResult{}, ErrInvalidRequest
	}

	var result UserCreateResult
	err := c.invoke(ctx, "user_create", map[string]any{
		"user_code": strings.TrimSpace(input.UserCode),
	}, &result)
	return result, err
}

func (c *Client) UserDeposit(ctx context.Context, input TransferInput) (TransferResult, error) {
	return c.transfer(ctx, "user_deposit", input)
}

func (c *Client) UserWithdraw(ctx context.Context, input TransferInput) (TransferResult, error) {
	return c.transfer(ctx, "user_withdraw", input)
}

func (c *Client) UserWithdrawReset(ctx context.Context, input UserWithdrawResetInput) (UserWithdrawResetResult, error) {
	if strings.TrimSpace(input.UserCode) == "" && !input.AllUsers {
		return UserWithdrawResetResult{}, ErrInvalidRequest
	}

	payload := map[string]any{}
	if strings.TrimSpace(input.UserCode) != "" {
		payload["user_code"] = strings.TrimSpace(input.UserCode)
	}
	if input.AllUsers {
		payload["all_users"] = true
	}

	var result UserWithdrawResetResult
	err := c.invoke(ctx, "user_withdraw_reset", payload, &result)
	return result, err
}

func (c *Client) TransferStatus(ctx context.Context, input TransferStatusInput) (TransferStatusResult, error) {
	if strings.TrimSpace(input.UserCode) == "" || strings.TrimSpace(input.AgentSign) == "" {
		return TransferStatusResult{}, ErrInvalidRequest
	}

	var result TransferStatusResult
	err := c.invoke(ctx, "transfer_status", map[string]any{
		"user_code":  strings.TrimSpace(input.UserCode),
		"agent_sign": strings.TrimSpace(input.AgentSign),
	}, &result)
	return result, err
}

func (c *Client) transfer(ctx context.Context, method string, input TransferInput) (TransferResult, error) {
	if strings.TrimSpace(input.UserCode) == "" || input.Amount <= 0 {
		return TransferResult{}, ErrInvalidRequest
	}

	payload := map[string]any{
		"user_code": strings.TrimSpace(input.UserCode),
		"amount":    input.Amount,
	}
	if strings.TrimSpace(input.AgentSign) != "" {
		payload["agent_sign"] = strings.TrimSpace(input.AgentSign)
	}

	var result TransferResult
	err := c.invoke(ctx, method, payload, &result)
	return result, err
}

func (c *Client) invoke(ctx context.Context, method string, payload map[string]any, target any) error {
	if c.baseURL == "" || c.agentCode == "" || c.agentToken == "" {
		return ErrNotConfigured
	}

	requestPayload := map[string]any{
		"method":      method,
		"agent_code":  c.agentCode,
		"agent_token": c.agentToken,
	}
	for key, value := range payload {
		requestPayload[key] = value
	}

	body, err := json.Marshal(requestPayload)
	if err != nil {
		return fmt.Errorf("marshal %s request: %w", method, err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create %s request: %w", method, err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")

	startedAt := time.Now()
	response, err := c.httpClient.Do(request)
	if err != nil {
		duration := time.Since(startedAt)
		if errors.Is(err, context.DeadlineExceeded) {
			c.logger.Warn("nexusggr_timeout",
				slog.String("method", method),
				slog.Duration("duration", duration),
				slog.Any("request_masked", maskedRequest(method, requestPayload)),
			)
			return ErrTimeout
		}

		c.logger.Error("nexusggr_transport_error",
			slog.String("method", method),
			slog.Duration("duration", duration),
			slog.String("error", err.Error()),
			slog.Any("request_masked", maskedRequest(method, requestPayload)),
		)
		return fmt.Errorf("%w: %v", ErrUpstreamUnavailable, err)
	}
	defer response.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read %s response: %w", method, err)
	}

	duration := time.Since(startedAt)
	if response.StatusCode != http.StatusOK {
		c.logger.Error("nexusggr_http_error",
			slog.String("method", method),
			slog.Int("status_code", response.StatusCode),
			slog.Duration("duration", duration),
			slog.Any("request_masked", maskedRequest(method, requestPayload)),
			slog.Any("response_masked", maskedResponse(method, raw)),
		)
		return fmt.Errorf("%w: %d", ErrUnexpectedHTTP, response.StatusCode)
	}

	var envelope responseEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		c.logger.Error("nexusggr_invalid_response",
			slog.String("method", method),
			slog.Duration("duration", duration),
			slog.Any("request_masked", maskedRequest(method, requestPayload)),
		)
		return fmt.Errorf("%w: %v", ErrInvalidResponse, err)
	}

	message := firstNonEmpty(envelope.Msg, envelope.Error)
	if envelope.Status != 1 {
		normalized := normalizeBusinessCode(message)
		c.logger.Warn("nexusggr_business_failure",
			slog.String("method", method),
			slog.String("code", normalized),
			slog.Duration("duration", duration),
			slog.Any("request_masked", maskedRequest(method, requestPayload)),
			slog.Any("response_masked", maskedResponse(method, raw)),
		)
		return &BusinessError{
			Method:  method,
			Code:    normalized,
			Message: message,
		}
	}

	if target != nil {
		if err := json.Unmarshal(raw, target); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidResponse, err)
		}
	}

	c.logger.Info("nexusggr_request",
		slog.String("method", method),
		slog.Duration("duration", duration),
		slog.Any("request_masked", maskedRequest(method, requestPayload)),
		slog.Any("response_masked", maskedResponse(method, raw)),
	)

	return nil
}

func maskedRequest(method string, payload map[string]any) map[string]any {
	masked := map[string]any{
		"method": method,
	}

	if value, ok := payload["provider_code"].(string); ok && value != "" {
		masked["provider_code"] = value
	}
	if value, ok := payload["game_code"].(string); ok && value != "" {
		masked["game_code"] = value
	}
	if value, ok := payload["lang"].(string); ok && value != "" {
		masked["lang"] = value
	}
	if value, ok := payload["user_code"].(string); ok && value != "" {
		masked["user_code_masked"] = maskValue(value)
	}
	if value, ok := payload["agent_sign"].(string); ok && value != "" {
		masked["agent_sign_masked"] = maskValue(value)
	}
	if value, ok := payload["amount"]; ok {
		masked["amount"] = value
	}
	if value, ok := payload["all_users"].(bool); ok {
		masked["all_users"] = value
	}

	return masked
}

func maskedResponse(method string, raw []byte) map[string]any {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return map[string]any{
			"method": method,
			"body":   "[unparseable]",
		}
	}

	masked := map[string]any{
		"method": method,
	}
	if value, ok := payload["status"]; ok {
		masked["status"] = value
	}
	if value := firstNonEmpty(stringValue(payload["msg"]), stringValue(payload["error"])); value != "" {
		masked["message"] = value
	}
	if value, ok := payload["agent_balance"]; ok {
		masked["agent_balance"] = value
	}
	if value, ok := payload["user_balance"]; ok {
		masked["user_balance"] = value
	}
	if value, ok := payload["amount"]; ok {
		masked["amount"] = value
	}
	if value, ok := payload["type"]; ok {
		masked["type"] = value
	}
	if providers, ok := payload["providers"].([]any); ok {
		masked["providers_count"] = len(providers)
	}
	if games, ok := payload["games"].([]any); ok {
		masked["games_count"] = len(games)
	}
	if users, ok := payload["user_list"].([]any); ok {
		masked["users_count"] = len(users)
	}
	if _, ok := payload["launch_url"]; ok {
		masked["launch_url"] = "[masked]"
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}

	return ""
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}
