package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/luqmanulhakimdev/pos-system/internal/application"
)

// NewRouter builds the HTTP surface exposed by the service.
func NewRouter(checkDatabase func(context.Context) error, auth *application.AuthService) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		if checkDatabase == nil || checkDatabase(ctx) != nil {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready\n"))
	})
	if auth != nil {
		mux.HandleFunc("POST /v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
			var input struct {
				Email    string `json:"email"`
				Password string `json:"password"`
			}
			if err := decodeJSON(w, r, &input); err != nil {
				writeError(w, http.StatusBadRequest, "invalid request")
				return
			}
			session, err := auth.Login(r.Context(), input.Email, input.Password)
			if errors.Is(err, application.ErrInvalidCredentials) {
				writeError(w, http.StatusUnauthorized, "invalid credentials")
				return
			}
			if err != nil {
				writeError(w, http.StatusInternalServerError, "could not log in")
				return
			}
			writeJSON(w, http.StatusCreated, session)
		})
		mux.Handle("GET /v1/me", Authenticated(auth, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := UserFromContext(r.Context())
			writeJSON(w, http.StatusOK, map[string]any{"id": user.ID, "email": user.Email, "permissions": user.Permissions})
		})))
		mux.Handle("POST /v1/auth/logout", Authenticated(auth, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := auth.Logout(r.Context(), bearerToken(r)); err != nil {
				writeError(w, http.StatusUnauthorized, "invalid session")
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})))
	}
	return mux
}

type userContextKey struct{}

func Authenticated(auth *application.AuthService, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth == nil {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		user, err := auth.Authenticate(r.Context(), bearerToken(r))
		if err != nil {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userContextKey{}, user)))
	})
}

func RequirePermission(auth *application.AuthService, permission string, next http.Handler) http.Handler {
	return Authenticated(auth, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !application.HasPermission(UserFromContext(r.Context()), permission) {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
		next.ServeHTTP(w, r)
	}))
}

func UserFromContext(ctx context.Context) application.AuthenticatedUser {
	user, _ := ctx.Value(userContextKey{}).(application.AuthenticatedUser)
	return user
}

func bearerToken(r *http.Request) string {
	parts := strings.Fields(r.Header.Get("Authorization"))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request must contain one JSON value")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
