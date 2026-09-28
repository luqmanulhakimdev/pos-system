package application

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidManagedUser    = errors.New("invalid managed user")
	ErrDuplicateUserEmail    = errors.New("user email already exists")
	ErrManagedUserNotFound   = errors.New("managed user not found")
	ErrInvalidManagedRole    = errors.New("invalid managed role")
	ErrLastActiveAdmin       = errors.New("cannot deactivate the last active administrator")
	ErrCannotDeactivateSelf  = errors.New("cannot deactivate the current user")
	ErrManagedAdminImmutable = errors.New("administrator role cannot be changed through this API")
)

type ManagedUser struct {
	ID          int64     `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	Active      bool      `json:"active"`
	Roles       []string  `json:"roles"`
	CreatedAt   time.Time `json:"created_at"`
}

type CreateManagedUserRequest struct {
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"`
	Role        string `json:"role"`
}

type UserManagementStore interface {
	CreateManagedUser(context.Context, int64, string, string, []byte, string) (ManagedUser, error)
	ListManagedUsers(context.Context) ([]ManagedUser, error)
	SetManagedUserRole(context.Context, int64, int64, string) error
	DeactivateManagedUser(context.Context, int64, int64) error
}

type UserManagement struct{ store UserManagementStore }

func NewUserManagement(store UserManagementStore) *UserManagement {
	return &UserManagement{store: store}
}

func (m *UserManagement) Create(ctx context.Context, actorID int64, input CreateManagedUserRequest) (ManagedUser, error) {
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.Role = strings.TrimSpace(input.Role)
	parsed, err := mail.ParseAddress(input.Email)
	if m == nil || m.store == nil || actorID <= 0 || err != nil || parsed.Address != input.Email || len(input.Email) > 254 || input.DisplayName == "" || len(input.DisplayName) > 120 || len(input.Password) < 12 || len(input.Password) > 72 {
		return ManagedUser{}, ErrInvalidManagedUser
	}
	if !validManagedRole(input.Role) {
		return ManagedUser{}, ErrInvalidManagedRole
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return ManagedUser{}, err
	}
	return m.store.CreateManagedUser(ctx, actorID, input.Email, input.DisplayName, hash, input.Role)
}

func (m *UserManagement) List(ctx context.Context) ([]ManagedUser, error) {
	if m == nil || m.store == nil {
		return nil, ErrInvalidManagedUser
	}
	return m.store.ListManagedUsers(ctx)
}

func (m *UserManagement) SetRole(ctx context.Context, actorID, userID int64, role string) error {
	role = strings.TrimSpace(role)
	if m == nil || m.store == nil || actorID <= 0 || userID <= 0 {
		return ErrInvalidManagedUser
	}
	if !validManagedRole(role) {
		return ErrInvalidManagedRole
	}
	return m.store.SetManagedUserRole(ctx, actorID, userID, role)
}

func (m *UserManagement) Deactivate(ctx context.Context, actorID, userID int64) error {
	if m == nil || m.store == nil || actorID <= 0 || userID <= 0 {
		return ErrInvalidManagedUser
	}
	if actorID == userID {
		return ErrCannotDeactivateSelf
	}
	return m.store.DeactivateManagedUser(ctx, actorID, userID)
}

func validManagedRole(role string) bool {
	return role == "manager" || role == "cashier" || role == "inventory_staff"
}
