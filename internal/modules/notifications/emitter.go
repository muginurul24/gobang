package notifications

import (
	"context"
	"log/slog"
)

// AsyncEmitter implements the Emitter interface for domain services. It fires
// notification creation in a background goroutine so the calling transaction
// path is never blocked by notification persistence or realtime push.
type AsyncEmitter struct {
	service Service
	logger  *slog.Logger
}

func NewAsyncEmitter(service Service, logger *slog.Logger) *AsyncEmitter {
	if logger == nil {
		logger = slog.Default()
	}

	return &AsyncEmitter{
		service: service,
		logger:  logger,
	}
}

func (e *AsyncEmitter) Emit(params CreateParams) error {
	go func() {
		_, err := e.service.Create(context.Background(), params)
		if err != nil {
			e.logger.Warn("async notification emit failed",
				"event_type", params.EventType,
				"scope_type", string(params.ScopeType),
				"scope_id", params.ScopeID,
				"error", err,
			)
		}
	}()

	return nil
}
