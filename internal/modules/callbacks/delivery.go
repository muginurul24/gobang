package callbacks

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type HTTPDispatcher struct {
	client *http.Client
}

func NewHTTPDispatcher(timeout time.Duration) *HTTPDispatcher {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	return &HTTPDispatcher{
		client: &http.Client{Timeout: timeout},
	}
}

func (d *HTTPDispatcher) Dispatch(ctx context.Context, callback DueOutboundCallback) (DispatchResult, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSpace(callback.CallbackURL), bytes.NewReader(callback.PayloadJSON))
	if err != nil {
		return DispatchResult{}, fmt.Errorf("build callback request: %w", err)
	}

	request.Header.Set("Content-Type", "application/json; charset=utf-8")
	request.Header.Set("User-Agent", "onixggr-callback/1.0")
	request.Header.Set("X-Onixggr-Event", callback.EventType)
	request.Header.Set("X-Onixggr-Signature", callback.Signature)
	request.Header.Set("X-Onixggr-Delivery-ID", callback.ID)
	request.Header.Set("X-Onixggr-Reference-Type", callback.ReferenceType)
	request.Header.Set("X-Onixggr-Reference-ID", callback.ReferenceID)

	response, err := d.client.Do(request)
	if err != nil {
		return DispatchResult{}, fmt.Errorf("send callback request: %w", err)
	}
	defer response.Body.Close()

	body, readErr := io.ReadAll(io.LimitReader(response.Body, 4096))
	if readErr != nil {
		return DispatchResult{}, fmt.Errorf("read callback response: %w", readErr)
	}

	statusCode := response.StatusCode
	result := DispatchResult{
		HTTPStatus:         &statusCode,
		ResponseBodyMasked: maskResponseBody(string(body)),
		Success:            response.StatusCode >= 200 && response.StatusCode < 300,
	}

	return result, nil
}
