package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/luqmanulhakimdev/pos-system/internal/application"
	"github.com/luqmanulhakimdev/pos-system/internal/domain"
)

// NewRouter builds the HTTP surface exposed by the service.
func NewRouter(checkDatabase func(context.Context) error, auth *application.AuthService, payments *application.Payments, checkout *application.Checkout, catalog *application.Catalog) http.Handler {
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
		if payments != nil {
			mux.Handle("POST /v1/orders/{orderID}/payments", RequirePermission(auth, "payment.create", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				orderID, err := strconv.ParseInt(r.PathValue("orderID"), 10, 64)
				if err != nil || orderID <= 0 {
					writeError(w, http.StatusBadRequest, "invalid order id")
					return
				}
				user := UserFromContext(r.Context())
				payment, err := payments.ChargeOrder(r.Context(), orderID, user.ID)
				if errors.Is(err, application.ErrOrderNotPayable) {
					writeError(w, http.StatusConflict, "order is not payable")
					return
				}
				if errors.Is(err, application.ErrInvalidPayment) {
					writeError(w, http.StatusBadRequest, "invalid payment request")
					return
				}
				if err != nil {
					writeError(w, http.StatusBadGateway, "payment provider unavailable")
					return
				}
				writeJSON(w, http.StatusOK, payment)
			})))
		}
		if checkout != nil {
			mux.Handle("POST /v1/orders/checkout", RequirePermission(auth, "order.create", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var input struct {
					CustomerID *int64                      `json:"customer_id"`
					Items      []application.RequestedItem `json:"items"`
				}
				if err := decodeJSON(w, r, &input); err != nil {
					writeError(w, http.StatusBadRequest, "invalid request")
					return
				}
				user := UserFromContext(r.Context())
				order, err := checkout.Execute(r.Context(), application.CheckoutRequest{CashierID: user.ID, CustomerID: input.CustomerID, Items: input.Items})
				if errors.Is(err, application.ErrInvalidCheckout) {
					writeError(w, http.StatusBadRequest, "invalid checkout")
					return
				}
				if errors.Is(err, application.ErrProductUnavailable) {
					writeError(w, http.StatusNotFound, "product unavailable")
					return
				}
				if errors.Is(err, domain.ErrInsufficientStock) {
					writeError(w, http.StatusConflict, "insufficient stock")
					return
				}
				if err != nil {
					writeError(w, http.StatusInternalServerError, "checkout failed")
					return
				}
				writeJSON(w, http.StatusCreated, order)
			})))
		}
		if catalog != nil {
			mux.Handle("GET /v1/categories", RequirePermission(auth, "product.read", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				items, err := catalog.ListCategories(r.Context())
				if err != nil {
					writeError(w, http.StatusInternalServerError, "could not load categories")
					return
				}
				writeJSON(w, http.StatusOK, items)
			})))
			mux.Handle("POST /v1/categories", RequirePermission(auth, "product.create", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var input struct {
					Name        string `json:"name"`
					Description string `json:"description"`
				}
				if err := decodeJSON(w, r, &input); err != nil {
					writeError(w, http.StatusBadRequest, "invalid request")
					return
				}
				item, err := catalog.CreateCategory(r.Context(), application.Category{Name: input.Name, Description: input.Description})
				if errors.Is(err, application.ErrInvalidCatalogItem) {
					writeError(w, http.StatusBadRequest, "invalid category")
					return
				}
				if errors.Is(err, application.ErrDuplicateCategory) {
					writeError(w, http.StatusConflict, "category already exists")
					return
				}
				if err != nil {
					writeError(w, http.StatusInternalServerError, "could not create category")
					return
				}
				writeJSON(w, http.StatusCreated, item)
			})))
			mux.Handle("GET /v1/products", RequirePermission(auth, "product.read", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				limit, offset := 50, 0
				if raw := r.URL.Query().Get("limit"); raw != "" {
					n, err := strconv.Atoi(raw)
					if err != nil {
						writeError(w, http.StatusBadRequest, "invalid limit")
						return
					}
					limit = n
				}
				if raw := r.URL.Query().Get("offset"); raw != "" {
					n, err := strconv.Atoi(raw)
					if err != nil {
						writeError(w, http.StatusBadRequest, "invalid offset")
						return
					}
					offset = n
				}
				var categoryID *int64
				if raw := r.URL.Query().Get("category_id"); raw != "" {
					n, err := strconv.ParseInt(raw, 10, 64)
					if err != nil {
						writeError(w, http.StatusBadRequest, "invalid category_id")
						return
					}
					categoryID = &n
				}
				items, err := catalog.ListProducts(r.Context(), application.ProductFilter{Search: r.URL.Query().Get("search"), CategoryID: categoryID, Limit: limit, Offset: offset})
				if errors.Is(err, application.ErrInvalidCatalogItem) {
					writeError(w, http.StatusBadRequest, "invalid product filters")
					return
				}
				if err != nil {
					writeError(w, http.StatusInternalServerError, "could not load products")
					return
				}
				writeJSON(w, http.StatusOK, items)
			})))
			mux.Handle("POST /v1/products", RequirePermission(auth, "product.create", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var input struct {
					CategoryID  *int64 `json:"category_id"`
					SKU         string `json:"sku"`
					Name        string `json:"name"`
					Description string `json:"description"`
					PriceMinor  int64  `json:"price_minor"`
					Currency    string `json:"currency"`
				}
				if err := decodeJSON(w, r, &input); err != nil {
					writeError(w, http.StatusBadRequest, "invalid request")
					return
				}
				item, err := catalog.CreateProduct(r.Context(), application.Product{CategoryID: input.CategoryID, SKU: input.SKU, Name: input.Name, Description: input.Description, PriceMinor: input.PriceMinor, Currency: input.Currency})
				if errors.Is(err, application.ErrInvalidCatalogItem) {
					writeError(w, http.StatusBadRequest, "invalid product")
					return
				}
				if errors.Is(err, application.ErrDuplicateSKU) {
					writeError(w, http.StatusConflict, "SKU already exists")
					return
				}
				if errors.Is(err, application.ErrCategoryNotFound) {
					writeError(w, http.StatusBadRequest, "category not found")
					return
				}
				if err != nil {
					writeError(w, http.StatusInternalServerError, "could not create product")
					return
				}
				writeJSON(w, http.StatusCreated, item)
			})))
		}
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
