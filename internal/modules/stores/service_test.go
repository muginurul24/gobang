package stores

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mugiew/onixggr/internal/modules/auth"
	"github.com/mugiew/onixggr/internal/platform/security"
)

func TestCreateStoreGeneratesInitialTokenAndDefaultThreshold(t *testing.T) {
	now := time.Date(2026, 3, 30, 10, 0, 0, 0, time.UTC)
	repository := newFakeRepository(now)
	service := NewService(repository, fakePasswordHasher{}, fixedClock{now: now}, 150000).(*service)
	service.tokenFactory = func() (string, error) {
		return "store_live_fixed_token", nil
	}

	store, err := service.CreateStore(context.Background(), auth.Subject{
		UserID: "owner-1",
		Role:   auth.RoleOwner,
	}, CreateStoreInput{
		Name: "Alpha Store",
		Slug: "alpha-store",
	}, auth.RequestMetadata{
		IPAddress: "127.0.0.1",
		UserAgent: "stores-test",
	})
	if err != nil {
		t.Fatalf("CreateStore returned error: %v", err)
	}

	if store.APIToken == nil || *store.APIToken != "store_live_fixed_token" {
		t.Fatalf("APIToken = %v, want generated token", store.APIToken)
	}

	if repository.lastCreatedStore.APITokenHash != security.HashStoreToken("store_live_fixed_token") {
		t.Fatal("expected repository to persist hashed store token")
	}

	if store.LowBalanceThreshold == nil || *store.LowBalanceThreshold != "150000" {
		t.Fatalf("LowBalanceThreshold = %v, want default threshold", store.LowBalanceThreshold)
	}

	if len(repository.auditLogs) != 2 {
		t.Fatalf("audit logs = %#v, want store.create and store.token_created entries", repository.auditLogs)
	}
	if repository.auditLogs[0].action != "store.create" || repository.auditLogs[1].action != "store.token_created" {
		t.Fatalf("audit logs = %#v, want store.create then store.token_created", repository.auditLogs)
	}
}

func TestUpdateStorePreservesExistingValuesWhenFieldsOmitted(t *testing.T) {
	now := time.Date(2026, 3, 30, 11, 0, 0, 0, time.UTC)
	repository := newFakeRepository(now)
	threshold := "100000"
	repository.stores["store-1"] = Store{
		ID:                  "store-1",
		OwnerUserID:         "owner-1",
		Name:                "Stable Store",
		Slug:                "stable-store",
		Status:              StatusActive,
		LowBalanceThreshold: &threshold,
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	service := NewService(repository, fakePasswordHasher{}, fixedClock{now: now}, 150000)
	updated, err := service.UpdateStore(context.Background(), auth.Subject{
		UserID: "owner-1",
		Role:   auth.RoleOwner,
	}, "store-1", UpdateStoreInput{}, auth.RequestMetadata{})
	if err != nil {
		t.Fatalf("UpdateStore returned error: %v", err)
	}

	if updated.Name != "Stable Store" || updated.Status != StatusActive {
		t.Fatalf("updated store = %#v, want original name and status", updated)
	}

	if updated.LowBalanceThreshold == nil || *updated.LowBalanceThreshold != "100000" {
		t.Fatalf("LowBalanceThreshold = %v, want existing threshold", updated.LowBalanceThreshold)
	}
}

func TestAssignStoreStaffRejectsCrossOwnerRelation(t *testing.T) {
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)
	repository := newFakeRepository(now)
	owner2 := "owner-2"
	repository.stores["store-1"] = Store{
		ID:          "store-1",
		OwnerUserID: "owner-1",
		Name:        "Owner One Store",
		Slug:        "owner-one-store",
		Status:      StatusActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	repository.employees["employee-1"] = StaffUser{
		ID:              "employee-1",
		Email:           "employee@example.com",
		Username:        "employee",
		Role:            string(auth.RoleKaryawan),
		CreatedByUserID: &owner2,
		CreatedAt:       now,
	}

	service := NewService(repository, fakePasswordHasher{}, fixedClock{now: now}, 150000)
	_, err := service.AssignStoreStaff(context.Background(), auth.Subject{
		UserID: "owner-1",
		Role:   auth.RoleOwner,
	}, "store-1", AssignStaffInput{UserID: "employee-1"}, auth.RequestMetadata{})
	if !errors.Is(err, ErrEmployeeScopeMismatch) {
		t.Fatalf("AssignStoreStaff error = %v, want ErrEmployeeScopeMismatch", err)
	}
}

func TestListStoresHidesCallbackURLForKaryawan(t *testing.T) {
	now := time.Date(2026, 3, 30, 13, 0, 0, 0, time.UTC)
	repository := newFakeRepository(now)
	callback := "https://merchant.example.com/callback"
	repository.staffStores["employee-1"] = []Store{
		{
			ID:          "store-1",
			OwnerUserID: "owner-1",
			Name:        "Scoped Store",
			Slug:        "scoped-store",
			Status:      StatusActive,
			CallbackURL: callback,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
	}

	service := NewService(repository, fakePasswordHasher{}, fixedClock{now: now}, 150000)
	stores, err := service.ListStores(context.Background(), auth.Subject{
		UserID: "employee-1",
		Role:   auth.RoleKaryawan,
	})
	if err != nil {
		t.Fatalf("ListStores returned error: %v", err)
	}

	if len(stores) != 1 {
		t.Fatalf("len(stores) = %d, want 1", len(stores))
	}

	if stores[0].CallbackURL != "" {
		t.Fatalf("CallbackURL = %q, want empty for karyawan", stores[0].CallbackURL)
	}
}

func TestListStoreDirectoryHidesCallbackURLForKaryawan(t *testing.T) {
	now := time.Date(2026, 3, 30, 13, 30, 0, 0, time.UTC)
	repository := newFakeRepository(now)
	callback := "https://merchant.example.com/callback"
	repository.staffStores["employee-1"] = []Store{
		{
			ID:          "store-1",
			OwnerUserID: "owner-1",
			Name:        "Scoped Store",
			Slug:        "scoped-store",
			Status:      StatusActive,
			CallbackURL: callback,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
	}

	service := NewService(repository, fakePasswordHasher{}, fixedClock{now: now}, 150000)
	page, err := service.ListStoreDirectory(context.Background(), auth.Subject{
		UserID: "employee-1",
		Role:   auth.RoleKaryawan,
	}, ListStoreDirectoryFilter{Limit: 10})
	if err != nil {
		t.Fatalf("ListStoreDirectory returned error: %v", err)
	}

	if len(page.Items) != 1 {
		t.Fatalf("len(page.Items) = %d, want 1", len(page.Items))
	}
	if page.Items[0].CallbackURL != "" {
		t.Fatalf("CallbackURL = %q, want empty for karyawan", page.Items[0].CallbackURL)
	}
}

func TestListEmployeeDirectoryRejectsNonOwner(t *testing.T) {
	service := NewService(newFakeRepository(time.Now()), fakePasswordHasher{}, fixedClock{now: time.Now()}, 150000)

	_, err := service.ListEmployeeDirectory(context.Background(), auth.Subject{
		UserID: "dev-1",
		Role:   auth.RoleDev,
	}, ListEmployeesFilter{})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("ListEmployeeDirectory error = %v, want ErrForbidden", err)
	}
}

func TestRotateStoreTokenBlocksDevVisibility(t *testing.T) {
	now := time.Date(2026, 3, 30, 14, 0, 0, 0, time.UTC)
	repository := newFakeRepository(now)
	repository.stores["store-1"] = Store{
		ID:          "store-1",
		OwnerUserID: "owner-1",
		Name:        "Scoped Store",
		Slug:        "scoped-store",
		Status:      StatusActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	service := NewService(repository, fakePasswordHasher{}, fixedClock{now: now}, 150000).(*service)
	service.tokenFactory = func() (string, error) {
		return "store_live_new_token", nil
	}

	_, err := service.RotateStoreToken(context.Background(), auth.Subject{
		UserID: "dev-1",
		Role:   auth.RoleDev,
	}, "store-1", auth.RequestMetadata{})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("RotateStoreToken error = %v, want ErrForbidden", err)
	}
}

func TestRotateStoreTokenRehashesTokenAndWritesAuditTrail(t *testing.T) {
	now := time.Date(2026, 3, 30, 14, 5, 0, 0, time.UTC)
	repository := newFakeRepository(now)
	repository.stores["store-1"] = Store{
		ID:          "store-1",
		OwnerUserID: "owner-1",
		Name:        "Scoped Store",
		Slug:        "scoped-store",
		Status:      StatusActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	service := NewService(repository, fakePasswordHasher{}, fixedClock{now: now}, 150000).(*service)
	service.tokenFactory = func() (string, error) {
		return "store_live_rotated_token", nil
	}

	token, err := service.RotateStoreToken(context.Background(), auth.Subject{
		UserID: "owner-1",
		Role:   auth.RoleOwner,
	}, "store-1", auth.RequestMetadata{
		IPAddress: "127.0.0.1",
		UserAgent: "stores-test",
	})
	if err != nil {
		t.Fatalf("RotateStoreToken returned error: %v", err)
	}

	if token.Token != "store_live_rotated_token" {
		t.Fatalf("token = %q, want store_live_rotated_token", token.Token)
	}
	if repository.lastRotate.APITokenHash != security.HashStoreToken("store_live_rotated_token") {
		t.Fatalf("lastRotate.APITokenHash = %q, want rotated token hash", repository.lastRotate.APITokenHash)
	}
	if len(repository.auditLogs) != 2 {
		t.Fatalf("audit logs = %#v, want token_rotated and token_revoked", repository.auditLogs)
	}
	if repository.auditLogs[0].action != "store.token_rotated" || repository.auditLogs[1].action != "store.token_revoked" {
		t.Fatalf("audit logs = %#v, want token_rotated then token_revoked", repository.auditLogs)
	}
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time {
	return c.now
}

type fakePasswordHasher struct{}

func (fakePasswordHasher) Hash(password string) (string, error) {
	return "hash:" + password, nil
}

type fakeRepository struct {
	now              time.Time
	stores           map[string]Store
	staffStores      map[string][]Store
	employees        map[string]StaffUser
	storeStaff       map[string][]StaffUser
	lastCreatedStore CreateStoreParams
	lastRotate       RotateTokenParams
	auditLogs        []fakeAuditLog
}

type fakeAuditLog struct {
	action string
}

func newFakeRepository(now time.Time) *fakeRepository {
	return &fakeRepository{
		now:         now,
		stores:      map[string]Store{},
		staffStores: map[string][]Store{},
		employees:   map[string]StaffUser{},
		storeStaff:  map[string][]StaffUser{},
	}
}

func (r *fakeRepository) ListStoresForOwner(_ context.Context, ownerUserID string) ([]Store, error) {
	var stores []Store
	for _, store := range r.stores {
		if store.OwnerUserID == ownerUserID && store.DeletedAt == nil {
			stores = append(stores, store)
		}
	}

	return stores, nil
}

func (r *fakeRepository) ListStoresForStaff(_ context.Context, userID string) ([]Store, error) {
	return append([]Store(nil), r.staffStores[userID]...), nil
}

func (r *fakeRepository) ListAllStores(_ context.Context) ([]Store, error) {
	var stores []Store
	for _, store := range r.stores {
		if store.DeletedAt == nil {
			stores = append(stores, store)
		}
	}

	return stores, nil
}

func (r *fakeRepository) ListStoreDirectoryForOwner(_ context.Context, ownerUserID string, filter ListStoreDirectoryFilter) (StorePage, error) {
	var items []Store
	for _, store := range r.stores {
		if store.OwnerUserID != ownerUserID || store.DeletedAt != nil {
			continue
		}
		if !matchesStoreDirectory(store, filter) {
			continue
		}
		items = append(items, store)
	}

	return paginateStorePage(items, filter), nil
}

func (r *fakeRepository) ListStoreDirectoryForStaff(_ context.Context, userID string, filter ListStoreDirectoryFilter) (StorePage, error) {
	var items []Store
	for _, store := range r.staffStores[userID] {
		if store.DeletedAt != nil {
			continue
		}
		if !matchesStoreDirectory(store, filter) {
			continue
		}
		items = append(items, store)
	}

	return paginateStorePage(items, filter), nil
}

func (r *fakeRepository) ListStoreDirectoryForPlatform(_ context.Context, filter ListStoreDirectoryFilter) (StorePage, error) {
	var items []Store
	for _, store := range r.stores {
		if store.DeletedAt != nil {
			continue
		}
		if !matchesStoreDirectory(store, filter) {
			continue
		}
		items = append(items, store)
	}

	return paginateStorePage(items, filter), nil
}

func (r *fakeRepository) GetStoreByID(_ context.Context, storeID string) (Store, error) {
	store, ok := r.stores[storeID]
	if !ok {
		return Store{}, ErrNotFound
	}

	return store, nil
}

func (r *fakeRepository) IsStoreStaff(_ context.Context, storeID string, userID string) (bool, error) {
	for _, store := range r.staffStores[userID] {
		if store.ID == storeID {
			return true, nil
		}
	}

	return false, nil
}

func (r *fakeRepository) CreateStore(_ context.Context, params CreateStoreParams) (Store, error) {
	r.lastCreatedStore = params
	store := Store{
		ID:                  "store-created",
		OwnerUserID:         params.OwnerUserID,
		Name:                params.Name,
		Slug:                params.Slug,
		Status:              StatusActive,
		LowBalanceThreshold: params.LowBalanceThreshold,
		CurrentBalance:      "0",
		CreatedAt:           params.OccurredAt,
		UpdatedAt:           params.OccurredAt,
	}
	r.stores[store.ID] = store
	return store, nil
}

func (r *fakeRepository) UpdateStore(_ context.Context, params UpdateStoreParams) (Store, error) {
	store, ok := r.stores[params.StoreID]
	if !ok {
		return Store{}, ErrNotFound
	}

	store.Name = params.Name
	store.Status = params.Status
	store.LowBalanceThreshold = params.LowBalanceThreshold
	store.UpdatedAt = params.OccurredAt
	r.stores[store.ID] = store
	return store, nil
}

func (r *fakeRepository) SoftDeleteStore(_ context.Context, params SoftDeleteStoreParams) error {
	store, ok := r.stores[params.StoreID]
	if !ok {
		return ErrNotFound
	}

	store.Status = StatusDeleted
	store.DeletedAt = &params.OccurredAt
	r.stores[store.ID] = store
	return nil
}

func (r *fakeRepository) RotateToken(_ context.Context, params RotateTokenParams) error {
	if _, ok := r.stores[params.StoreID]; !ok {
		return ErrNotFound
	}

	r.lastRotate = params
	return nil
}

func (r *fakeRepository) UpdateCallbackURL(_ context.Context, params UpdateCallbackParams) (Store, error) {
	store, ok := r.stores[params.StoreID]
	if !ok {
		return Store{}, ErrNotFound
	}

	store.CallbackURL = params.CallbackURL
	store.UpdatedAt = params.OccurredAt
	r.stores[store.ID] = store
	return store, nil
}

func (r *fakeRepository) CreateEmployee(_ context.Context, params CreateEmployeeParams) (StaffUser, error) {
	user := StaffUser{
		ID:              "employee-created",
		Email:           params.Email,
		Username:        params.Username,
		Role:            string(auth.RoleKaryawan),
		CreatedByUserID: &params.OwnerUserID,
		CreatedAt:       params.OccurredAt,
	}
	r.employees[user.ID] = user
	return user, nil
}

func (r *fakeRepository) ListEmployeesByOwner(_ context.Context, ownerUserID string) ([]StaffUser, error) {
	var users []StaffUser
	for _, employee := range r.employees {
		if employee.CreatedByUserID != nil && *employee.CreatedByUserID == ownerUserID {
			users = append(users, employee)
		}
	}

	return users, nil
}

func (r *fakeRepository) ListEmployeeDirectoryByOwner(_ context.Context, ownerUserID string, filter ListEmployeesFilter) (StaffUserPage, error) {
	var items []StaffUser
	for _, employee := range r.employees {
		if employee.CreatedByUserID == nil || *employee.CreatedByUserID != ownerUserID {
			continue
		}

		if filter.Query != "" {
			search := strings.ToLower(filter.Query)
			if !strings.Contains(strings.ToLower(employee.Email), search) && !strings.Contains(strings.ToLower(employee.Username), search) {
				continue
			}
		}

		items = append(items, employee)
	}

	return paginateStaffPage(items, filter.Limit, filter.Offset), nil
}

func (r *fakeRepository) GetEmployeeByID(_ context.Context, userID string) (StaffUser, error) {
	user, ok := r.employees[userID]
	if !ok {
		return StaffUser{}, ErrEmployeeNotFound
	}

	return user, nil
}

func (r *fakeRepository) ListStoreStaff(_ context.Context, storeID string) ([]StaffUser, error) {
	return append([]StaffUser(nil), r.storeStaff[storeID]...), nil
}

func (r *fakeRepository) ListStoreStaffPage(_ context.Context, filter ListStoreStaffFilter) (StaffUserPage, error) {
	var items []StaffUser
	for _, user := range r.storeStaff[filter.StoreID] {
		if filter.Query != "" {
			search := strings.ToLower(filter.Query)
			if !strings.Contains(strings.ToLower(user.Email), search) && !strings.Contains(strings.ToLower(user.Username), search) {
				continue
			}
		}

		items = append(items, user)
	}

	return paginateStaffPage(items, filter.Limit, filter.Offset), nil
}

func (r *fakeRepository) AssignStaff(_ context.Context, params AssignStaffParams) error {
	user, ok := r.employees[params.UserID]
	if !ok {
		return ErrEmployeeNotFound
	}

	r.storeStaff[params.StoreID] = append(r.storeStaff[params.StoreID], user)
	return nil
}

func (r *fakeRepository) UnassignStaff(_ context.Context, storeID string, userID string) error {
	staff := r.storeStaff[storeID]
	next := make([]StaffUser, 0, len(staff))
	found := false
	for _, user := range staff {
		if user.ID == userID {
			found = true
			continue
		}

		next = append(next, user)
	}

	if !found {
		return ErrNotFound
	}

	r.storeStaff[storeID] = next
	return nil
}

func (r *fakeRepository) InsertAuditLog(_ context.Context, _ *string, _ string, _ *string, action string, _ string, _ *string, _ map[string]any, _ string, _ string, _ time.Time) error {
	r.auditLogs = append(r.auditLogs, fakeAuditLog{action: action})
	return nil
}

func matchesStoreDirectory(store Store, filter ListStoreDirectoryFilter) bool {
	if filter.Query != "" {
		search := strings.ToLower(filter.Query)
		haystack := strings.ToLower(store.Name + " " + store.Slug + " " + store.CallbackURL)
		if !strings.Contains(haystack, search) {
			return false
		}
	}

	if filter.Status != nil && store.Status != *filter.Status {
		return false
	}

	return true
}

func paginateStorePage(items []Store, filter ListStoreDirectoryFilter) StorePage {
	start := filter.Offset
	if start > len(items) {
		start = len(items)
	}
	end := start + filter.Limit
	if end > len(items) {
		end = len(items)
	}

	summary := StoreDirectorySummary{
		TotalCount: len(items),
	}
	for _, item := range items {
		switch item.Status {
		case StatusActive:
			summary.ActiveCount++
		case StatusInactive:
			summary.InactiveCount++
		case StatusBanned:
			summary.BannedCount++
		case StatusDeleted:
			summary.DeletedCount++
		}
	}

	return StorePage{
		Items:   append([]Store(nil), items[start:end]...),
		Summary: summary,
		Limit:   filter.Limit,
		Offset:  filter.Offset,
	}
}

func paginateStaffPage(items []StaffUser, limit int, offset int) StaffUserPage {
	start := offset
	if start > len(items) {
		start = len(items)
	}
	end := start + limit
	if end > len(items) {
		end = len(items)
	}

	return StaffUserPage{
		Items:      append([]StaffUser(nil), items[start:end]...),
		TotalCount: len(items),
		Limit:      limit,
		Offset:     offset,
	}
}
