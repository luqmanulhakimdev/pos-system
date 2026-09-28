package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luqmanulhakimdev/pos-system/internal/application"
)

const userAdminGuardLock int64 = 746219533

type UserManagementStore struct{ pool *pgxpool.Pool }

func NewUserManagementStore(pool *pgxpool.Pool) *UserManagementStore {
	return &UserManagementStore{pool: pool}
}

func (s *UserManagementStore) CreateManagedUser(ctx context.Context, actorID int64, email, displayName string, passwordHash []byte, role string) (application.ManagedUser, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return application.ManagedUser{}, fmt.Errorf("begin user creation: %w", err)
	}
	defer tx.Rollback(context.Background())
	var roleID int64
	if err := tx.QueryRow(ctx, "SELECT id FROM roles WHERE name=$1", role).Scan(&roleID); errors.Is(err, pgx.ErrNoRows) {
		return application.ManagedUser{}, application.ErrInvalidManagedRole
	} else if err != nil {
		return application.ManagedUser{}, fmt.Errorf("load user role: %w", err)
	}
	var user application.ManagedUser
	err = tx.QueryRow(ctx, `INSERT INTO users(email,password_hash,display_name) VALUES($1,$2,$3) RETURNING id,email,display_name,active,created_at`, email, string(passwordHash), displayName).Scan(&user.ID, &user.Email, &user.DisplayName, &user.Active, &user.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return application.ManagedUser{}, application.ErrDuplicateUserEmail
		}
		return application.ManagedUser{}, fmt.Errorf("insert user: %w", err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO user_roles(user_id,role_id) VALUES($1,$2)", user.ID, roleID); err != nil {
		return application.ManagedUser{}, fmt.Errorf("assign initial user role: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_logs(actor_user_id,action,entity_type,entity_id,details) VALUES($1,'user.created','user',$2,jsonb_build_object('role',$3::text))`, actorID, fmt.Sprint(user.ID), role); err != nil {
		return application.ManagedUser{}, fmt.Errorf("audit user creation: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return application.ManagedUser{}, fmt.Errorf("commit user creation: %w", err)
	}
	user.Roles = []string{role}
	return user, nil
}

func (s *UserManagementStore) ListManagedUsers(ctx context.Context) ([]application.ManagedUser, error) {
	rows, err := s.pool.Query(ctx, `SELECT u.id,u.email,u.display_name,u.active,u.created_at,COALESCE(array_agg(r.name ORDER BY r.name) FILTER(WHERE r.name IS NOT NULL),'{}')
		FROM users u LEFT JOIN user_roles ur ON ur.user_id=u.id LEFT JOIN roles r ON r.id=ur.role_id
		GROUP BY u.id ORDER BY u.id`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()
	users := make([]application.ManagedUser, 0)
	for rows.Next() {
		var user application.ManagedUser
		if err := rows.Scan(&user.ID, &user.Email, &user.DisplayName, &user.Active, &user.CreatedAt, &user.Roles); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read users: %w", err)
	}
	return users, nil
}

func (s *UserManagementStore) SetManagedUserRole(ctx context.Context, actorID, userID int64, role string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin role change: %w", err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", userAdminGuardLock); err != nil {
		return fmt.Errorf("lock admin role changes: %w", err)
	}
	var active bool
	if err := tx.QueryRow(ctx, "SELECT active FROM users WHERE id=$1 FOR UPDATE", userID).Scan(&active); errors.Is(err, pgx.ErrNoRows) {
		return application.ErrManagedUserNotFound
	} else if err != nil {
		return fmt.Errorf("lock managed user: %w", err)
	}
	if !active {
		return application.ErrManagedUserNotFound
	}
	var isAdmin bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM user_roles ur JOIN roles r ON r.id=ur.role_id WHERE ur.user_id=$1 AND r.name='admin')`, userID).Scan(&isAdmin); err != nil {
		return fmt.Errorf("check immutable admin role: %w", err)
	}
	if isAdmin {
		return application.ErrManagedAdminImmutable
	}
	var roleID int64
	if err := tx.QueryRow(ctx, "SELECT id FROM roles WHERE name=$1", role).Scan(&roleID); errors.Is(err, pgx.ErrNoRows) {
		return application.ErrInvalidManagedRole
	} else if err != nil {
		return fmt.Errorf("load replacement role: %w", err)
	}
	if _, err := tx.Exec(ctx, "DELETE FROM user_roles WHERE user_id=$1", userID); err != nil {
		return fmt.Errorf("replace user roles: %w", err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO user_roles(user_id,role_id) VALUES($1,$2)", userID, roleID); err != nil {
		return fmt.Errorf("assign replacement user role: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_logs(actor_user_id,action,entity_type,entity_id,details) VALUES($1,'user.role_changed','user',$2,jsonb_build_object('role',$3::text))`, actorID, fmt.Sprint(userID), role); err != nil {
		return fmt.Errorf("audit role change: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit role change: %w", err)
	}
	return nil
}

func (s *UserManagementStore) DeactivateManagedUser(ctx context.Context, actorID, userID int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin user deactivation: %w", err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", userAdminGuardLock); err != nil {
		return fmt.Errorf("lock administrator deactivation: %w", err)
	}
	var active bool
	if err := tx.QueryRow(ctx, "SELECT active FROM users WHERE id=$1 FOR UPDATE", userID).Scan(&active); errors.Is(err, pgx.ErrNoRows) {
		return application.ErrManagedUserNotFound
	} else if err != nil {
		return fmt.Errorf("lock user for deactivation: %w", err)
	}
	if !active {
		return application.ErrManagedUserNotFound
	}
	var isAdmin bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM user_roles ur JOIN roles r ON r.id=ur.role_id WHERE ur.user_id=$1 AND r.name='admin')`, userID).Scan(&isAdmin); err != nil {
		return fmt.Errorf("check administrator status: %w", err)
	}
	if isAdmin {
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM users u JOIN user_roles ur ON ur.user_id=u.id JOIN roles r ON r.id=ur.role_id WHERE u.active=TRUE AND r.name='admin'`).Scan(&count); err != nil {
			return fmt.Errorf("count active administrators: %w", err)
		}
		if count <= 1 {
			return application.ErrLastActiveAdmin
		}
	}
	if _, err := tx.Exec(ctx, "UPDATE users SET active=FALSE,updated_at=now() WHERE id=$1", userID); err != nil {
		return fmt.Errorf("deactivate user: %w", err)
	}
	if _, err := tx.Exec(ctx, "UPDATE user_sessions SET revoked_at=now() WHERE user_id=$1 AND revoked_at IS NULL", userID); err != nil {
		return fmt.Errorf("revoke deactivated user sessions: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_logs(actor_user_id,action,entity_type,entity_id,details) VALUES($1,'user.deactivated','user',$2,jsonb_build_object('user_id',$2::bigint))`, actorID, fmt.Sprint(userID)); err != nil {
		return fmt.Errorf("audit user deactivation: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit user deactivation: %w", err)
	}
	return nil
}

var _ application.UserManagementStore = (*UserManagementStore)(nil)
