package stores

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mugiew/onixggr/internal/modules/auth"
	"github.com/mugiew/onixggr/internal/platform/clock"
	"github.com/mugiew/onixggr/internal/platform/security"
)

type PasswordHasher interface {
	Hash(password string) (string, error)
}

type RepositoryContract interface {
	ListStoresForOwner(ctx context.Context, ownerUserID string) ([]Store, error)
	ListStoresForStaff(ctx context.Context, userID string) ([]Store, error)
	ListAllStores(ctx context.Context) ([]Store, error)
	ListStoreDirectoryForOwner(ctx context.Context, ownerUserID string, filter ListStoreDirectoryFilter) (StorePage, error)
	ListStoreDirectoryForStaff(ctx context.Context, userID string, filter ListStoreDirectoryFilter) (StorePage, error)
	ListStoreDirectoryForPlatform(ctx context.Context, filter ListStoreDirectoryFilter) (StorePage, error)
	GetStoreByID(ctx context.Context, storeID string) (Store, error)
	IsStoreStaff(ctx context.Context, storeID string, userID string) (bool, error)
	CreateStore(ctx context.Context, params CreateStoreParams) (Store, error)
	UpdateStore(ctx context.Context, params UpdateStoreParams) (Store, error)
	SoftDeleteStore(ctx context.Context, params SoftDeleteStoreParams) error
	RotateToken(ctx context.Context, params RotateTokenParams) error
	UpdateCallbackURL(ctx context.Context, params UpdateCallbackParams) (Store, error)
	CreateEmployee(ctx context.Context, params CreateEmployeeParams) (StaffUser, error)
	ListEmployeesByOwner(ctx context.Context, ownerUserID string) ([]StaffUser, error)
	ListEmployeeDirectoryByOwner(ctx context.Context, ownerUserID string, filter ListEmployeesFilter) (StaffUserPage, error)
	GetEmployeeByID(ctx context.Context, userID string) (StaffUser, error)
	ListStoreStaff(ctx context.Context, storeID string) ([]StaffUser, error)
	ListStoreStaffPage(ctx context.Context, filter ListStoreStaffFilter) (StaffUserPage, error)
	AssignStaff(ctx context.Context, params AssignStaffParams) error
	UnassignStaff(ctx context.Context, storeID string, userID string) error
	InsertAuditLog(
		ctx context.Context,
		actorUserID *string,
		actorRole string,
		storeID *string,
		action string,
		targetType string,
		targetID *string,
		payload map[string]any,
		ipAddress string,
		userAgent string,
		occurredAt time.Time,
	) error
}

type Service interface {
	ListStores(ctx context.Context, subject auth.Subject) ([]Store, error)
	ListStoreDirectory(ctx context.Context, subject auth.Subject, filter ListStoreDirectoryFilter) (StorePage, error)
	GetStore(ctx context.Context, subject auth.Subject, storeID string) (Store, error)
	CreateStore(ctx context.Context, subject auth.Subject, input CreateStoreInput, metadata auth.RequestMetadata) (Store, error)
	UpdateStore(ctx context.Context, subject auth.Subject, storeID string, input UpdateStoreInput, metadata auth.RequestMetadata) (Store, error)
	DeleteStore(ctx context.Context, subject auth.Subject, storeID string, metadata auth.RequestMetadata) error
	RotateStoreToken(ctx context.Context, subject auth.Subject, storeID string, metadata auth.RequestMetadata) (StoreToken, error)
	UpdateCallbackURL(ctx context.Context, subject auth.Subject, storeID string, input UpdateCallbackInput, metadata auth.RequestMetadata) (Store, error)
	CreateEmployee(ctx context.Context, subject auth.Subject, input CreateEmployeeInput, metadata auth.RequestMetadata) (StaffUser, error)
	ListEmployees(ctx context.Context, subject auth.Subject) ([]StaffUser, error)
	ListEmployeeDirectory(ctx context.Context, subject auth.Subject, filter ListEmployeesFilter) (StaffUserPage, error)
	ListStoreStaff(ctx context.Context, subject auth.Subject, storeID string) ([]StaffUser, error)
	ListStoreStaffDirectory(ctx context.Context, subject auth.Subject, filter ListStoreStaffFilter) (StaffUserPage, error)
	AssignStoreStaff(ctx context.Context, subject auth.Subject, storeID string, input AssignStaffInput, metadata auth.RequestMetadata) ([]StaffUser, error)
	UnassignStoreStaff(ctx context.Context, subject auth.Subject, storeID string, userID string, metadata auth.RequestMetadata) ([]StaffUser, error)
}

type service struct {
	repository                 RepositoryContract
	passwords                  PasswordHasher
	clock                      clock.Clock
	tokenFactory               func() (string, error)
	defaultLowBalanceThreshold *string
}

func NewService(repository RepositoryContract, passwords PasswordHasher, now clock.Clock, defaultLowBalanceThreshold int64) Service {
	if now == nil {
		now = clock.SystemClock{}
	}

	return &service{
		repository:                 repository,
		passwords:                  passwords,
		clock:                      now,
		tokenFactory:               security.NewStoreToken,
		defaultLowBalanceThreshold: normalizeDefaultThreshold(defaultLowBalanceThreshold),
	}
}

func (s *service) ListStores(ctx context.Context, subject auth.Subject) ([]Store, error) {
	var (
		stores []Store
		err    error
	)

	switch subject.Role {
	case auth.RoleOwner:
		stores, err = s.repository.ListStoresForOwner(ctx, subject.UserID)
	case auth.RoleKaryawan:
		stores, err = s.repository.ListStoresForStaff(ctx, subject.UserID)
	case auth.RoleDev, auth.RoleSuperadmin:
		stores, err = s.repository.ListAllStores(ctx)
	default:
		return nil, ErrForbidden
	}
	if err != nil {
		return nil, err
	}

	for index := range stores {
		stores[index] = s.sanitizeStore(subject, stores[index])
	}

	return stores, nil
}

func (s *service) ListStoreDirectory(ctx context.Context, subject auth.Subject, filter ListStoreDirectoryFilter) (StorePage, error) {
	filter.Query = strings.TrimSpace(filter.Query)
	filter.Limit = normalizeLimit(filter.Limit, 12, 100)
	if filter.Offset < 0 {
		filter.Offset = 0
	}

	var (
		page StorePage
		err  error
	)

	switch subject.Role {
	case auth.RoleOwner:
		page, err = s.repository.ListStoreDirectoryForOwner(ctx, subject.UserID, filter)
	case auth.RoleKaryawan:
		page, err = s.repository.ListStoreDirectoryForStaff(ctx, subject.UserID, filter)
	case auth.RoleDev, auth.RoleSuperadmin:
		page, err = s.repository.ListStoreDirectoryForPlatform(ctx, filter)
	default:
		return StorePage{}, ErrForbidden
	}
	if err != nil {
		return StorePage{}, err
	}

	for index := range page.Items {
		page.Items[index] = s.sanitizeStore(subject, page.Items[index])
	}

	return page, nil
}

func (s *service) GetStore(ctx context.Context, subject auth.Subject, storeID string) (Store, error) {
	store, err := s.repository.GetStoreByID(ctx, strings.TrimSpace(storeID))
	if err != nil {
		return Store{}, err
	}

	if store.DeletedAt != nil {
		return Store{}, ErrNotFound
	}

	allowed, err := s.canViewStore(ctx, subject, store)
	if err != nil {
		return Store{}, err
	}

	if !allowed {
		return Store{}, ErrForbidden
	}

	return s.sanitizeStore(subject, store), nil
}

func (s *service) CreateStore(ctx context.Context, subject auth.Subject, input CreateStoreInput, metadata auth.RequestMetadata) (Store, error) {
	if subject.Role != auth.RoleOwner {
		return Store{}, ErrForbidden
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		return Store{}, ErrInvalidStoreName
	}

	slug := normalizeSlug(input.Slug)
	if !validSlug(slug) {
		return Store{}, ErrInvalidSlug
	}

	threshold, err := s.resolveCreateThreshold(input.LowBalanceThreshold)
	if err != nil {
		return Store{}, err
	}

	token, err := s.tokenFactory()
	if err != nil {
		return Store{}, fmt.Errorf("generate store token: %w", err)
	}

	now := s.clock.Now().UTC()
	store, err := s.repository.CreateStore(ctx, CreateStoreParams{
		OwnerUserID:         subject.UserID,
		Name:                name,
		Slug:                slug,
		APITokenHash:        security.HashStoreToken(token),
		LowBalanceThreshold: threshold,
		OccurredAt:          now,
	})
	if err != nil {
		return Store{}, err
	}

	store.APIToken = cloneStringPtr(token)

	if err := s.repository.InsertAuditLog(
		ctx,
		&subject.UserID,
		string(subject.Role),
		&store.ID,
		"store.create",
		"store",
		&store.ID,
		map[string]any{
			"slug":                  slug,
			"name":                  name,
			"low_balance_threshold": threshold,
			"token_state":           "created",
		},
		metadata.IPAddress,
		metadata.UserAgent,
		now,
	); err != nil {
		return Store{}, err
	}

	if err := s.repository.InsertAuditLog(
		ctx,
		&subject.UserID,
		string(subject.Role),
		&store.ID,
		"store.token_created",
		"store",
		&store.ID,
		map[string]any{
			"token_state": "created",
		},
		metadata.IPAddress,
		metadata.UserAgent,
		now,
	); err != nil {
		return Store{}, err
	}

	return s.sanitizeStore(subject, store), nil
}

func (s *service) UpdateStore(ctx context.Context, subject auth.Subject, storeID string, input UpdateStoreInput, metadata auth.RequestMetadata) (Store, error) {
	store, err := s.repository.GetStoreByID(ctx, strings.TrimSpace(storeID))
	if err != nil {
		return Store{}, err
	}

	if store.DeletedAt != nil {
		return Store{}, ErrNotFound
	}

	if !s.canManageStore(subject, store) {
		return Store{}, ErrForbidden
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = store.Name
	}

	status := store.Status
	if strings.TrimSpace(input.Status) != "" {
		status, err = parseStatus(input.Status)
		if err != nil {
			return Store{}, err
		}
	}

	if subject.Role == auth.RoleOwner && status == StatusBanned {
		return Store{}, ErrForbidden
	}

	threshold, err := s.resolveUpdateThreshold(store.LowBalanceThreshold, input.LowBalanceThreshold)
	if err != nil {
		return Store{}, err
	}

	now := s.clock.Now().UTC()
	updated, err := s.repository.UpdateStore(ctx, UpdateStoreParams{
		StoreID:             store.ID,
		Name:                name,
		Status:              status,
		LowBalanceThreshold: threshold,
		OccurredAt:          now,
	})
	if err != nil {
		return Store{}, err
	}

	if err := s.repository.InsertAuditLog(
		ctx,
		&subject.UserID,
		string(subject.Role),
		&updated.ID,
		"store.update",
		"store",
		&updated.ID,
		map[string]any{
			"name":                  name,
			"status":                status,
			"low_balance_threshold": threshold,
		},
		metadata.IPAddress,
		metadata.UserAgent,
		now,
	); err != nil {
		return Store{}, err
	}

	return s.sanitizeStore(subject, updated), nil
}

func (s *service) DeleteStore(ctx context.Context, subject auth.Subject, storeID string, metadata auth.RequestMetadata) error {
	store, err := s.repository.GetStoreByID(ctx, strings.TrimSpace(storeID))
	if err != nil {
		return err
	}

	if store.DeletedAt != nil {
		return ErrNotFound
	}

	if !s.canManageStore(subject, store) {
		return ErrForbidden
	}

	now := s.clock.Now().UTC()
	if err := s.repository.SoftDeleteStore(ctx, SoftDeleteStoreParams{
		StoreID:    store.ID,
		OccurredAt: now,
	}); err != nil {
		return err
	}

	return s.repository.InsertAuditLog(
		ctx,
		&subject.UserID,
		string(subject.Role),
		&store.ID,
		"store.delete",
		"store",
		&store.ID,
		map[string]any{
			"slug": store.Slug,
		},
		metadata.IPAddress,
		metadata.UserAgent,
		now,
	)
}

func (s *service) RotateStoreToken(ctx context.Context, subject auth.Subject, storeID string, metadata auth.RequestMetadata) (StoreToken, error) {
	store, err := s.repository.GetStoreByID(ctx, strings.TrimSpace(storeID))
	if err != nil {
		return StoreToken{}, err
	}

	if store.DeletedAt != nil {
		return StoreToken{}, ErrNotFound
	}

	if !s.canRotateStoreToken(subject, store) {
		return StoreToken{}, ErrForbidden
	}

	token, err := s.tokenFactory()
	if err != nil {
		return StoreToken{}, fmt.Errorf("generate store token: %w", err)
	}

	now := s.clock.Now().UTC()
	if err := s.repository.RotateToken(ctx, RotateTokenParams{
		StoreID:      store.ID,
		APITokenHash: security.HashStoreToken(token),
		OccurredAt:   now,
	}); err != nil {
		return StoreToken{}, err
	}

	if err := s.repository.InsertAuditLog(
		ctx,
		&subject.UserID,
		string(subject.Role),
		&store.ID,
		"store.token_rotated",
		"store",
		&store.ID,
		map[string]any{
			"token_state": "rotated",
		},
		metadata.IPAddress,
		metadata.UserAgent,
		now,
	); err != nil {
		return StoreToken{}, err
	}

	if err := s.repository.InsertAuditLog(
		ctx,
		&subject.UserID,
		string(subject.Role),
		&store.ID,
		"store.token_revoked",
		"store",
		&store.ID,
		map[string]any{
			"reason":      "rotate",
			"token_state": "revoked",
		},
		metadata.IPAddress,
		metadata.UserAgent,
		now,
	); err != nil {
		return StoreToken{}, err
	}

	return StoreToken{Token: token}, nil
}

func (s *service) UpdateCallbackURL(ctx context.Context, subject auth.Subject, storeID string, input UpdateCallbackInput, metadata auth.RequestMetadata) (Store, error) {
	store, err := s.repository.GetStoreByID(ctx, strings.TrimSpace(storeID))
	if err != nil {
		return Store{}, err
	}

	if store.DeletedAt != nil {
		return Store{}, ErrNotFound
	}

	if !s.canManageStore(subject, store) {
		return Store{}, ErrForbidden
	}

	callbackURL, err := validateCallbackURL(input.CallbackURL)
	if err != nil {
		return Store{}, err
	}

	now := s.clock.Now().UTC()
	updated, err := s.repository.UpdateCallbackURL(ctx, UpdateCallbackParams{
		StoreID:     store.ID,
		CallbackURL: callbackURL,
		OccurredAt:  now,
	})
	if err != nil {
		return Store{}, err
	}

	if err := s.repository.InsertAuditLog(
		ctx,
		&subject.UserID,
		string(subject.Role),
		&store.ID,
		"store.callback_url_updated",
		"store",
		&store.ID,
		map[string]any{
			"callback_url": callbackURL,
		},
		metadata.IPAddress,
		metadata.UserAgent,
		now,
	); err != nil {
		return Store{}, err
	}

	return s.sanitizeStore(subject, updated), nil
}

func (s *service) CreateEmployee(ctx context.Context, subject auth.Subject, input CreateEmployeeInput, metadata auth.RequestMetadata) (StaffUser, error) {
	if subject.Role != auth.RoleOwner {
		return StaffUser{}, ErrForbidden
	}

	email := strings.ToLower(strings.TrimSpace(input.Email))
	username := strings.ToLower(strings.TrimSpace(input.Username))
	password := strings.TrimSpace(input.Password)
	if email == "" || username == "" || password == "" {
		return StaffUser{}, ErrInvalidEmployeeInput
	}

	hash, err := s.passwords.Hash(password)
	if err != nil {
		return StaffUser{}, fmt.Errorf("hash employee password: %w", err)
	}

	now := s.clock.Now().UTC()
	user, err := s.repository.CreateEmployee(ctx, CreateEmployeeParams{
		OwnerUserID:  subject.UserID,
		Email:        email,
		Username:     username,
		PasswordHash: hash,
		OccurredAt:   now,
	})
	if err != nil {
		return StaffUser{}, err
	}

	if err := s.repository.InsertAuditLog(
		ctx,
		&subject.UserID,
		string(subject.Role),
		nil,
		"staff.user_created",
		"user",
		&user.ID,
		map[string]any{
			"username": username,
			"email":    email,
		},
		metadata.IPAddress,
		metadata.UserAgent,
		now,
	); err != nil {
		return StaffUser{}, err
	}

	return user, nil
}

func (s *service) ListEmployees(ctx context.Context, subject auth.Subject) ([]StaffUser, error) {
	if subject.Role != auth.RoleOwner {
		return nil, ErrForbidden
	}

	return s.repository.ListEmployeesByOwner(ctx, subject.UserID)
}

func (s *service) ListEmployeeDirectory(ctx context.Context, subject auth.Subject, filter ListEmployeesFilter) (StaffUserPage, error) {
	if subject.Role != auth.RoleOwner {
		return StaffUserPage{}, ErrForbidden
	}

	filter.Query = strings.TrimSpace(filter.Query)
	filter.Limit = normalizeLimit(filter.Limit, 12, 100)
	if filter.Offset < 0 {
		filter.Offset = 0
	}

	return s.repository.ListEmployeeDirectoryByOwner(ctx, subject.UserID, filter)
}

func (s *service) ListStoreStaff(ctx context.Context, subject auth.Subject, storeID string) ([]StaffUser, error) {
	store, err := s.repository.GetStoreByID(ctx, strings.TrimSpace(storeID))
	if err != nil {
		return nil, err
	}

	if store.DeletedAt != nil {
		return nil, ErrNotFound
	}

	allowed, err := s.canViewStore(ctx, subject, store)
	if err != nil {
		return nil, err
	}

	if !allowed || subject.Role == auth.RoleKaryawan {
		return nil, ErrForbidden
	}

	return s.repository.ListStoreStaff(ctx, store.ID)
}

func (s *service) ListStoreStaffDirectory(ctx context.Context, subject auth.Subject, filter ListStoreStaffFilter) (StaffUserPage, error) {
	store, err := s.repository.GetStoreByID(ctx, strings.TrimSpace(filter.StoreID))
	if err != nil {
		return StaffUserPage{}, err
	}

	if store.DeletedAt != nil {
		return StaffUserPage{}, ErrNotFound
	}

	allowed, err := s.canViewStore(ctx, subject, store)
	if err != nil {
		return StaffUserPage{}, err
	}

	if !allowed || subject.Role == auth.RoleKaryawan {
		return StaffUserPage{}, ErrForbidden
	}

	filter.StoreID = store.ID
	filter.Query = strings.TrimSpace(filter.Query)
	filter.Limit = normalizeLimit(filter.Limit, 8, 100)
	if filter.Offset < 0 {
		filter.Offset = 0
	}

	return s.repository.ListStoreStaffPage(ctx, filter)
}

func (s *service) AssignStoreStaff(ctx context.Context, subject auth.Subject, storeID string, input AssignStaffInput, metadata auth.RequestMetadata) ([]StaffUser, error) {
	if subject.Role != auth.RoleOwner {
		return nil, ErrForbidden
	}

	store, err := s.repository.GetStoreByID(ctx, strings.TrimSpace(storeID))
	if err != nil {
		return nil, err
	}

	if store.DeletedAt != nil || store.OwnerUserID != subject.UserID {
		return nil, ErrForbidden
	}

	employee, err := s.repository.GetEmployeeByID(ctx, strings.TrimSpace(input.UserID))
	if err != nil {
		return nil, err
	}

	if employee.Role != string(auth.RoleKaryawan) || employee.CreatedByUserID == nil || *employee.CreatedByUserID != subject.UserID {
		return nil, ErrEmployeeScopeMismatch
	}

	now := s.clock.Now().UTC()
	if err := s.repository.AssignStaff(ctx, AssignStaffParams{
		StoreID:          store.ID,
		UserID:           employee.ID,
		CreatedByOwnerID: subject.UserID,
		OccurredAt:       now,
	}); err != nil {
		return nil, err
	}

	if err := s.repository.InsertAuditLog(
		ctx,
		&subject.UserID,
		string(subject.Role),
		&store.ID,
		"store.staff_assigned",
		"user",
		&employee.ID,
		map[string]any{
			"store_slug": store.Slug,
		},
		metadata.IPAddress,
		metadata.UserAgent,
		now,
	); err != nil {
		return nil, err
	}

	return s.repository.ListStoreStaff(ctx, store.ID)
}

func (s *service) UnassignStoreStaff(ctx context.Context, subject auth.Subject, storeID string, userID string, metadata auth.RequestMetadata) ([]StaffUser, error) {
	if subject.Role != auth.RoleOwner {
		return nil, ErrForbidden
	}

	store, err := s.repository.GetStoreByID(ctx, strings.TrimSpace(storeID))
	if err != nil {
		return nil, err
	}

	if store.DeletedAt != nil || store.OwnerUserID != subject.UserID {
		return nil, ErrForbidden
	}

	employee, err := s.repository.GetEmployeeByID(ctx, strings.TrimSpace(userID))
	if err != nil {
		return nil, err
	}

	if employee.CreatedByUserID == nil || *employee.CreatedByUserID != subject.UserID {
		return nil, ErrEmployeeScopeMismatch
	}

	now := s.clock.Now().UTC()
	if err := s.repository.UnassignStaff(ctx, store.ID, employee.ID); err != nil {
		return nil, err
	}

	if err := s.repository.InsertAuditLog(
		ctx,
		&subject.UserID,
		string(subject.Role),
		&store.ID,
		"store.staff_unassigned",
		"user",
		&employee.ID,
		map[string]any{
			"store_slug": store.Slug,
		},
		metadata.IPAddress,
		metadata.UserAgent,
		now,
	); err != nil {
		return nil, err
	}

	return s.repository.ListStoreStaff(ctx, store.ID)
}

func (s *service) sanitizeStore(subject auth.Subject, store Store) Store {
	if subject.Role == auth.RoleKaryawan {
		store.CallbackURL = ""
	}

	if subject.Role != auth.RoleOwner && subject.Role != auth.RoleSuperadmin {
		store.APIToken = nil
	}

	return store
}

func (s *service) canManageStore(subject auth.Subject, store Store) bool {
	switch subject.Role {
	case auth.RoleDev, auth.RoleSuperadmin:
		return true
	case auth.RoleOwner:
		return store.OwnerUserID == subject.UserID
	default:
		return false
	}
}

func (s *service) canRotateStoreToken(subject auth.Subject, store Store) bool {
	switch subject.Role {
	case auth.RoleSuperadmin:
		return true
	case auth.RoleOwner:
		return store.OwnerUserID == subject.UserID
	default:
		return false
	}
}

func (s *service) canViewStore(ctx context.Context, subject auth.Subject, store Store) (bool, error) {
	switch subject.Role {
	case auth.RoleDev, auth.RoleSuperadmin:
		return true, nil
	case auth.RoleOwner:
		return store.OwnerUserID == subject.UserID, nil
	case auth.RoleKaryawan:
		return s.repository.IsStoreStaff(ctx, store.ID, subject.UserID)
	default:
		return false, nil
	}
}

func (s *service) resolveCreateThreshold(input *string) (*string, error) {
	if input != nil {
		return normalizeThreshold(input)
	}

	return cloneNullableString(s.defaultLowBalanceThreshold), nil
}

func (s *service) resolveUpdateThreshold(current *string, input *string) (*string, error) {
	if input == nil {
		return cloneNullableString(current), nil
	}

	return normalizeThreshold(input)
}

func normalizeDefaultThreshold(value int64) *string {
	if value <= 0 {
		return nil
	}

	normalized := strconv.FormatInt(value, 10)
	return &normalized
}

func cloneNullableString(value *string) *string {
	if value == nil {
		return nil
	}

	trimmed := strings.TrimSpace(*value)
	return &trimmed
}

func cloneStringPtr(value string) *string {
	trimmed := strings.TrimSpace(value)
	return &trimmed
}

func normalizeLimit(value int, fallback int, max int) int {
	if value <= 0 {
		return fallback
	}
	if value > max {
		return max
	}

	return value
}
