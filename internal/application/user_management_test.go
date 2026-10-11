package application

import (
	"context"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type userManagementStoreStub struct {
	created       ManagedUser
	passwordHash  []byte
	role          string
	deactivatedID int64
	listed        []ManagedUser
}

func (s *userManagementStoreStub) CreateManagedUser(_ context.Context, _ int64, email, name string, passwordHash []byte, role string) (ManagedUser, error) {
	s.passwordHash = passwordHash
	s.role = role
	s.created = ManagedUser{ID: 3, Email: email, DisplayName: name, Active: true, Roles: []string{role}, CreatedAt: time.Now()}
	return s.created, nil
}
func (s *userManagementStoreStub) ListManagedUsers(context.Context) ([]ManagedUser, error) {
	return s.listed, nil
}
func (s *userManagementStoreStub) SetManagedUserRole(_ context.Context, _, _ int64, role string) error {
	s.role = role
	return nil
}
func (s *userManagementStoreStub) DeactivateManagedUser(_ context.Context, _, userID int64) error {
	s.deactivatedID = userID
	return nil
}

func TestUserManagementCreatesHashedPasswordAndValidatesRoles(t *testing.T) {
	store := &userManagementStoreStub{}
	service := NewUserManagement(store)
	user, err := service.Create(context.Background(), 1, CreateManagedUserRequest{Email: " CASHIER@example.com ", DisplayName: " Cashier ", Password: "strong-password-123", Role: "cashier"})
	if err != nil || user.Email != "cashier@example.com" || user.DisplayName != "Cashier" || store.role != "cashier" {
		t.Fatalf("user=%#v role=%q err=%v", user, store.role, err)
	}
	if err := bcrypt.CompareHashAndPassword(store.passwordHash, []byte("strong-password-123")); err != nil {
		t.Fatal("password was not stored as a bcrypt hash")
	}
	for _, role := range []string{"admin", "unknown"} {
		if _, err := service.Create(context.Background(), 1, CreateManagedUserRequest{Email: "x@example.com", DisplayName: "User", Password: "strong-password-123", Role: role}); err != ErrInvalidManagedRole {
			t.Fatalf("role %q error=%v", role, err)
		}
	}
	if err := service.SetRole(context.Background(), 1, 2, "admin"); err != ErrInvalidManagedRole {
		t.Fatalf("admin assignment error=%v", err)
	}
}

func TestUserManagementProtectsSelfDeactivation(t *testing.T) {
	store := &userManagementStoreStub{}
	if err := NewUserManagement(store).Deactivate(context.Background(), 7, 7); err != ErrCannotDeactivateSelf || store.deactivatedID != 0 {
		t.Fatalf("error=%v deactivated=%d", err, store.deactivatedID)
	}
	if err := NewUserManagement(store).Deactivate(context.Background(), 7, 8); err != nil || store.deactivatedID != 8 {
		t.Fatalf("error=%v deactivated=%d", err, store.deactivatedID)
	}
}
