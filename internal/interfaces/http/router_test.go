package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/luqmanulhakimdev/pos-system/internal/application"
)

type testAuthStore struct {
	hash    string
	revoked bool
}

func (s *testAuthStore) VerifyCredentials(_ context.Context, email, password string) (int64, error) {
	if email != "admin@example.com" || password != "password-strong" {
		return 0, application.ErrInvalidCredentials
	}
	return 1, nil
}
func (s *testAuthStore) CreateSession(_ context.Context, _ int64, hash string, _ time.Time) error {
	s.hash = hash
	return nil
}
func (s *testAuthStore) LoadSession(_ context.Context, hash string) (application.AuthenticatedUser, error) {
	if hash != s.hash || s.revoked {
		return application.AuthenticatedUser{}, application.ErrInvalidSession
	}
	return application.AuthenticatedUser{ID: 1, Email: "admin@example.com", Permissions: []string{"orders.read"}}, nil
}
func (s *testAuthStore) RevokeSession(_ context.Context, hash string) error {
	if hash != s.hash || s.revoked {
		return application.ErrInvalidSession
	}
	s.revoked = true
	return nil
}

func TestHealthz(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	NewRouter(nil, nil, nil).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := recorder.Body.String(); got != "ok\n" {
		t.Fatalf("body = %q, want %q", got, "ok\n")
	}
}

func TestLoginMeAndLogout(t *testing.T) {
	store := &testAuthStore{}
	router := NewRouter(func(context.Context) error { return nil }, application.NewAuthService(store, time.Hour), nil)
	login := httptest.NewRequest(http.MethodPost, "/v1/auth/login", strings.NewReader(`{"email":"admin@example.com","password":"password-strong"}`))
	loginRecorder := httptest.NewRecorder()
	router.ServeHTTP(loginRecorder, login)
	if loginRecorder.Code != http.StatusCreated {
		t.Fatalf("login status = %d, body %s", loginRecorder.Code, loginRecorder.Body)
	}
	var session application.LoginSession
	if err := json.Unmarshal(loginRecorder.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	me := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	me.Header.Set("Authorization", "Bearer "+session.Token)
	meRecorder := httptest.NewRecorder()
	router.ServeHTTP(meRecorder, me)
	if meRecorder.Code != http.StatusOK || !strings.Contains(meRecorder.Body.String(), "orders.read") {
		t.Fatalf("me response = %d %s", meRecorder.Code, meRecorder.Body)
	}
	logout := httptest.NewRequest(http.MethodPost, "/v1/auth/logout", nil)
	logout.Header.Set("Authorization", "Bearer "+session.Token)
	logoutRecorder := httptest.NewRecorder()
	router.ServeHTTP(logoutRecorder, logout)
	if logoutRecorder.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d", logoutRecorder.Code)
	}
}

func TestAuthenticatedAndPermissionMiddlewareReject(t *testing.T) {
	router := NewRouter(nil, application.NewAuthService(&testAuthStore{}, time.Hour), nil)
	request := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want unauthorized", recorder.Code)
	}
	store := &testAuthStore{}
	hash, _ := application.HashSessionToken("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	store.hash = hash
	permissionHandler := RequirePermission(application.NewAuthService(store, time.Hour), "products.write", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	request = httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.Header.Set("Authorization", "Bearer AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	recorder = httptest.NewRecorder()
	permissionHandler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want forbidden", recorder.Code)
	}
}

func TestHealthzRejectsOtherMethods(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/healthz", nil)

	NewRouter(nil, nil, nil).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusMethodNotAllowed)
	}
}

func TestReadinessChecksDatabase(t *testing.T) {
	tests := []struct {
		name  string
		check func(context.Context) error
		want  int
	}{
		{"healthy", func(context.Context) error { return nil }, http.StatusOK},
		{"database unavailable", func(context.Context) error { return errors.New("connection refused") }, http.StatusServiceUnavailable},
		{"missing database check", nil, http.StatusServiceUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
			NewRouter(tt.check, nil, nil).ServeHTTP(recorder, request)
			if recorder.Code != tt.want {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.want)
			}
		})
	}
}
