package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/database"
	"gorm.io/gorm"
)

func TestOAuthServiceFindOrCreateUserContextHonorsCanceledContext(t *testing.T) {
	setupAuthServiceContextTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := (&OAuthService{}).findOrCreateUserContext(ctx, "github", "123", "alice", "alice@example.com", "")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("findOrCreateUserContext() error = %v, want context.Canceled", err)
	}
}

func TestOAuthServiceFindOrCreateUserContextUsesInjectedStoresForExistingBinding(t *testing.T) {
	oldDB := database.DB
	database.DB = nil
	t.Cleanup(func() {
		database.DB = oldDB
	})

	bindings := &stubOAuthBindingStore{
		binding: &model.OAuthBinding{UserID: 42},
	}
	users := &stubOAuthUserStore{
		user: &model.User{ID: 42, Username: "alice"},
	}
	svc := &OAuthService{
		bindingDAO: bindings,
		userDAO:    users,
	}

	got, err := svc.findOrCreateUserContext(context.Background(), "github", "123", "alice", "alice@example.com", "")
	if err != nil {
		t.Fatalf("findOrCreateUserContext() error = %v", err)
	}
	if got != users.user {
		t.Fatalf("findOrCreateUserContext() user = %#v, want injected user", got)
	}
	if bindings.getProvider != "github" || bindings.getProviderUserID != "123" {
		t.Fatalf("binding lookup = (%q, %q), want (github, 123)", bindings.getProvider, bindings.getProviderUserID)
	}
	if users.getID != 42 {
		t.Fatalf("user lookup id = %d, want 42", users.getID)
	}
	if bindings.createCalled || users.createCalled {
		t.Fatal("existing binding path must not create a user or binding")
	}
}

type stubOAuthBindingStore struct {
	binding           *model.OAuthBinding
	getErr            error
	createErr         error
	getProvider       string
	getProviderUserID string
	createCalled      bool
}

func (s *stubOAuthBindingStore) GetByProviderUserContext(ctx context.Context, provider, providerUserID string) (*model.OAuthBinding, error) {
	s.getProvider = provider
	s.getProviderUserID = providerUserID
	if s.getErr != nil {
		return nil, s.getErr
	}
	if s.binding == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return s.binding, nil
}

func (s *stubOAuthBindingStore) CreateContext(ctx context.Context, binding *model.OAuthBinding) error {
	s.createCalled = true
	if s.createErr != nil {
		return s.createErr
	}
	s.binding = binding
	return nil
}

type stubOAuthUserStore struct {
	user         *model.User
	getErr       error
	createErr    error
	getID        uint
	createCalled bool
}

func (s *stubOAuthUserStore) GetUserByIDContext(ctx context.Context, id uint) (*model.User, error) {
	s.getID = id
	if s.getErr != nil {
		return nil, s.getErr
	}
	return s.user, nil
}

func (s *stubOAuthUserStore) CreateUserContext(ctx context.Context, user *model.User) error {
	s.createCalled = true
	if s.createErr != nil {
		return s.createErr
	}
	if user.ID == 0 {
		user.ID = 100
	}
	s.user = user
	return nil
}
