package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/luqmanulhakimdev/pos-system/internal/application"
)

func TestUserManagementStoreIntegrationCreateAssignAndDeactivate(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := NewPool(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := ApplyMigrations(ctx, pool); err != nil {
		t.Fatal(err)
	}
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	var adminID, adminRoleID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users(email,password_hash,display_name) VALUES($1,'unused','Admin fixture') RETURNING id`, "user-management-"+suffix+"@example.test").Scan(&adminID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT id FROM roles WHERE name='admin'").Scan(&adminRoleID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO user_roles(user_id,role_id) VALUES($1,$2)", adminID, adminRoleID); err != nil {
		t.Fatal(err)
	}
	store := NewUserManagementStore(pool)
	service := application.NewUserManagement(store)
	user, err := service.Create(ctx, adminID, application.CreateManagedUserRequest{Email: "new-" + suffix + "@example.test", DisplayName: "New Cashier", Password: "integration-password-123", Role: "manager"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM audit_logs WHERE actor_user_id=$1", adminID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE id IN ($1,$2)", adminID, user.ID)
	})
	if err := service.SetRole(ctx, adminID, user.ID, "cashier"); err != nil {
		t.Fatal(err)
	}
	users, err := service.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range users {
		if item.ID == user.ID && len(item.Roles) == 1 && item.Roles[0] == "cashier" {
			found = true
		}
	}
	if !found {
		t.Fatalf("role change not present in user list: %#v", users)
	}
	if err := service.Deactivate(ctx, adminID, user.ID); err != nil {
		t.Fatal(err)
	}
	var active bool
	if err := pool.QueryRow(ctx, "SELECT active FROM users WHERE id=$1", user.ID).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if active {
		t.Fatal("user remained active after deactivation")
	}
	if err := service.Deactivate(ctx, adminID, adminID); !errors.Is(err, application.ErrCannotDeactivateSelf) {
		t.Fatalf("self deactivation error=%v", err)
	}
	if err := store.DeactivateManagedUser(ctx, adminID, adminID); !errors.Is(err, application.ErrLastActiveAdmin) {
		t.Fatalf("last admin deactivation error=%v", err)
	}
	if err := store.SetManagedUserRole(ctx, adminID, adminID, "cashier"); !errors.Is(err, application.ErrManagedAdminImmutable) {
		t.Fatalf("admin role change error=%v", err)
	}
}
