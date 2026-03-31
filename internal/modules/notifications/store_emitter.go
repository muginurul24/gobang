package notifications

// StoreEmitter wraps the AsyncEmitter and fulfils the simplified
// NotificationEmitter interfaces used by domain modules (game, paymentsqris,
// withdrawals, callbacks). Those interfaces accept (storeID, eventType, title,
// body) and this adapter maps them to a full CreateParams with ScopeStore.
type StoreEmitter struct {
	inner Emitter
}

func NewStoreEmitter(inner Emitter) *StoreEmitter {
	return &StoreEmitter{inner: inner}
}

func (e *StoreEmitter) Emit(storeID string, eventType string, title string, body string) {
	_ = e.inner.Emit(CreateParams{
		ScopeType: ScopeStore,
		ScopeID:   storeID,
		EventType: eventType,
		Title:     title,
		Body:      body,
	})
}

type PlatformRoleEmitter struct {
	inner Emitter
}

func NewPlatformRoleEmitter(inner Emitter) *PlatformRoleEmitter {
	return &PlatformRoleEmitter{inner: inner}
}

func (e *PlatformRoleEmitter) Emit(eventType string, title string, body string) {
	if e.inner == nil {
		return
	}

	for _, role := range []string{"dev", "superadmin"} {
		_ = e.inner.Emit(CreateParams{
			ScopeType: ScopeRole,
			ScopeID:   role,
			EventType: eventType,
			Title:     title,
			Body:      body,
		})
	}
}
