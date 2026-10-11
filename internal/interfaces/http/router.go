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
func NewRouter(checkDatabase func(context.Context) error, auth *application.AuthService, payments *application.Payments, checkout *application.Checkout, catalog *application.Catalog, inventory *application.InventoryService, customers *application.Customers, cancelOrder *application.CancelOrder, refunds *application.Refunds, salesReports *application.SalesReports, orders *application.OrderQueries, userManagement *application.UserManagement) http.Handler {
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
		if userManagement != nil {
			mux.Handle("GET /v1/users", RequirePermission(auth, "user.manage", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				users, err := userManagement.List(r.Context())
				if err != nil {
					writeError(w, http.StatusInternalServerError, "could not load users")
					return
				}
				writeJSON(w, http.StatusOK, users)
			})))
			mux.Handle("POST /v1/users", RequirePermission(auth, "user.manage", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var input application.CreateManagedUserRequest
				if err := decodeJSON(w, r, &input); err != nil {
					writeError(w, http.StatusBadRequest, "invalid request")
					return
				}
				user, err := userManagement.Create(r.Context(), UserFromContext(r.Context()).ID, input)
				if errors.Is(err, application.ErrInvalidManagedUser) || errors.Is(err, application.ErrInvalidManagedRole) {
					writeError(w, http.StatusBadRequest, "invalid user details or role")
					return
				}
				if errors.Is(err, application.ErrDuplicateUserEmail) {
					writeError(w, http.StatusConflict, "user email already exists")
					return
				}
				if err != nil {
					writeError(w, http.StatusInternalServerError, "could not create user")
					return
				}
				writeJSON(w, http.StatusCreated, user)
			})))
			mux.Handle("PUT /v1/users/{userID}/role", RequirePermission(auth, "user.manage", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				userID, err := strconv.ParseInt(r.PathValue("userID"), 10, 64)
				if err != nil || userID <= 0 {
					writeError(w, http.StatusBadRequest, "invalid user id")
					return
				}
				var input struct {
					Role string `json:"role"`
				}
				if err := decodeJSON(w, r, &input); err != nil {
					writeError(w, http.StatusBadRequest, "invalid request")
					return
				}
				err = userManagement.SetRole(r.Context(), UserFromContext(r.Context()).ID, userID, input.Role)
				if errors.Is(err, application.ErrInvalidManagedRole) {
					writeError(w, http.StatusBadRequest, "invalid role")
					return
				}
				if errors.Is(err, application.ErrManagedUserNotFound) {
					writeError(w, http.StatusNotFound, "user not found")
					return
				}
				if errors.Is(err, application.ErrManagedAdminImmutable) {
					writeError(w, http.StatusConflict, "administrator role cannot be changed through this API")
					return
				}
				if err != nil {
					writeError(w, http.StatusInternalServerError, "could not change user role")
					return
				}
				w.WriteHeader(http.StatusNoContent)
			})))
			mux.Handle("DELETE /v1/users/{userID}", RequirePermission(auth, "user.manage", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				userID, err := strconv.ParseInt(r.PathValue("userID"), 10, 64)
				if err != nil || userID <= 0 {
					writeError(w, http.StatusBadRequest, "invalid user id")
					return
				}
				err = userManagement.Deactivate(r.Context(), UserFromContext(r.Context()).ID, userID)
				if errors.Is(err, application.ErrManagedUserNotFound) {
					writeError(w, http.StatusNotFound, "user not found")
					return
				}
				if errors.Is(err, application.ErrCannotDeactivateSelf) {
					writeError(w, http.StatusConflict, "cannot deactivate the current user")
					return
				}
				if errors.Is(err, application.ErrLastActiveAdmin) {
					writeError(w, http.StatusConflict, "cannot deactivate the last active administrator")
					return
				}
				if err != nil {
					writeError(w, http.StatusInternalServerError, "could not deactivate user")
					return
				}
				w.WriteHeader(http.StatusNoContent)
			})))
		}
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
		if cancelOrder != nil {
			mux.Handle("POST /v1/orders/{orderID}/cancel", RequirePermission(auth, "order.cancel", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				orderID, err := strconv.ParseInt(r.PathValue("orderID"), 10, 64)
				if err != nil || orderID <= 0 {
					writeError(w, http.StatusBadRequest, "invalid order id")
					return
				}
				err = cancelOrder.Execute(r.Context(), orderID, UserFromContext(r.Context()).ID)
				if errors.Is(err, application.ErrOrderNotFound) {
					writeError(w, http.StatusNotFound, "order not found")
					return
				}
				if errors.Is(err, application.ErrOrderPaymentBlocksCancellation) {
					writeError(w, http.StatusConflict, "payment must be resolved before cancelling")
					return
				}
				if errors.Is(err, domain.ErrInvalidOrderTransition) {
					writeError(w, http.StatusConflict, "order cannot be cancelled from its current state")
					return
				}
				if errors.Is(err, domain.ErrInvalidOrder) {
					writeError(w, http.StatusBadRequest, "invalid order")
					return
				}
				if err != nil {
					writeError(w, http.StatusInternalServerError, "order cancellation failed")
					return
				}
				w.WriteHeader(http.StatusNoContent)
			})))
		}
		if refunds != nil {
			mux.Handle("POST /v1/orders/{orderID}/refunds", RequirePermission(auth, "payment.refund", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				orderID, err := strconv.ParseInt(r.PathValue("orderID"), 10, 64)
				if err != nil || orderID <= 0 {
					writeError(w, http.StatusBadRequest, "invalid order id")
					return
				}
				key := r.Header.Get("Idempotency-Key")
				if key == "" {
					writeError(w, http.StatusBadRequest, "Idempotency-Key is required")
					return
				}
				var input application.RefundInput
				if err := decodeJSON(w, r, &input); err != nil {
					writeError(w, http.StatusBadRequest, "invalid request")
					return
				}
				refund, replayed, err := refunds.Execute(r.Context(), orderID, UserFromContext(r.Context()).ID, key, input)
				if errors.Is(err, application.ErrInvalidRefund) {
					writeError(w, http.StatusBadRequest, "invalid refund request")
					return
				}
				if errors.Is(err, application.ErrOrderNotFound) {
					writeError(w, http.StatusNotFound, "order not found")
					return
				}
				if errors.Is(err, application.ErrOrderNotRefundable) {
					writeError(w, http.StatusConflict, "only paid orders can be refunded")
					return
				}
				if errors.Is(err, application.ErrRefundInProgress) {
					writeError(w, http.StatusConflict, "another refund is already in progress")
					return
				}
				if errors.Is(err, application.ErrRefundConflict) {
					writeError(w, http.StatusConflict, "Idempotency-Key was used for a different refund request")
					return
				}
				if errors.Is(err, application.ErrRefundProviderUnavailable) {
					writeError(w, http.StatusBadGateway, "refund provider unavailable; retry with the same key")
					return
				}
				if err != nil {
					writeError(w, http.StatusInternalServerError, "refund failed")
					return
				}
				if replayed {
					w.Header().Set("Idempotent-Replay", "true")
				}
				writeJSON(w, http.StatusCreated, refund)
			})))
		}
		if salesReports != nil {
			mux.Handle("GET /v1/reports/sales", RequirePermission(auth, "report.read", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				report, err := salesReports.Execute(r.Context(), r.URL.Query().Get("from"), r.URL.Query().Get("to"))
				if errors.Is(err, application.ErrInvalidReportRange) {
					writeError(w, http.StatusBadRequest, "from and to must be valid dates (YYYY-MM-DD), no more than 366 days apart")
					return
				}
				if err != nil {
					writeError(w, http.StatusInternalServerError, "could not load sales report")
					return
				}
				writeJSON(w, http.StatusOK, report)
			})))
		}
		if orders != nil {
			mux.Handle("GET /v1/orders", RequirePermission(auth, "order.read", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
				items, err := orders.List(r.Context(), application.OrderListFilter{Status: domain.OrderStatus(r.URL.Query().Get("status")), Search: r.URL.Query().Get("search"), Limit: limit, Offset: offset})
				if errors.Is(err, application.ErrInvalidCheckout) {
					writeError(w, http.StatusBadRequest, "invalid order filters")
					return
				}
				if err != nil {
					writeError(w, http.StatusInternalServerError, "could not load orders")
					return
				}
				writeJSON(w, http.StatusOK, items)
			})))
			mux.Handle("GET /v1/orders/{orderID}", RequirePermission(auth, "order.read", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				id, err := strconv.ParseInt(r.PathValue("orderID"), 10, 64)
				if err != nil || id <= 0 {
					writeError(w, http.StatusBadRequest, "invalid order id")
					return
				}
				order, err := orders.Get(r.Context(), id)
				if errors.Is(err, application.ErrOrderNotFound) {
					writeError(w, http.StatusNotFound, "order not found")
					return
				}
				if err != nil {
					writeError(w, http.StatusInternalServerError, "could not load order")
					return
				}
				writeJSON(w, http.StatusOK, order)
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
			mux.Handle("PUT /v1/products/{productID}", RequirePermission(auth, "product.update", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				productID, err := strconv.ParseInt(r.PathValue("productID"), 10, 64)
				if err != nil || productID <= 0 {
					writeError(w, http.StatusBadRequest, "invalid product id")
					return
				}
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
				item, err := catalog.UpdateProduct(r.Context(), application.Product{ID: productID, CategoryID: input.CategoryID, SKU: input.SKU, Name: input.Name, Description: input.Description, PriceMinor: input.PriceMinor, Currency: input.Currency})
				if errors.Is(err, application.ErrInvalidCatalogItem) {
					writeError(w, http.StatusBadRequest, "invalid product")
					return
				}
				if errors.Is(err, application.ErrProductUnavailable) {
					writeError(w, http.StatusNotFound, "product unavailable")
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
					writeError(w, http.StatusInternalServerError, "could not update product")
					return
				}
				writeJSON(w, http.StatusOK, item)
			})))
			mux.Handle("DELETE /v1/products/{productID}", RequirePermission(auth, "product.delete", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				productID, err := strconv.ParseInt(r.PathValue("productID"), 10, 64)
				if err != nil || productID <= 0 {
					writeError(w, http.StatusBadRequest, "invalid product id")
					return
				}
				if err := catalog.DeactivateProduct(r.Context(), productID); errors.Is(err, application.ErrInvalidCatalogItem) {
					writeError(w, http.StatusBadRequest, "invalid product id")
					return
				} else if errors.Is(err, application.ErrProductUnavailable) {
					writeError(w, http.StatusNotFound, "product unavailable")
					return
				} else if err != nil {
					writeError(w, http.StatusInternalServerError, "could not deactivate product")
					return
				}
				w.WriteHeader(http.StatusNoContent)
			})))
		}
		if inventory != nil {
			mux.Handle("GET /v1/inventory/{productID}", RequirePermission(auth, "inventory.read", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				productID, err := strconv.ParseInt(r.PathValue("productID"), 10, 64)
				if err != nil || productID <= 0 {
					writeError(w, http.StatusBadRequest, "invalid product id")
					return
				}
				stock, err := inventory.Get(r.Context(), productID)
				if errors.Is(err, application.ErrProductUnavailable) {
					writeError(w, http.StatusNotFound, "inventory not found")
					return
				}
				if err != nil {
					writeError(w, http.StatusInternalServerError, "could not load inventory")
					return
				}
				writeJSON(w, http.StatusOK, stock)
			})))
			mux.Handle("POST /v1/inventory/{productID}/adjustments", RequirePermission(auth, "inventory.adjust", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				productID, err := strconv.ParseInt(r.PathValue("productID"), 10, 64)
				if err != nil || productID <= 0 {
					writeError(w, http.StatusBadRequest, "invalid product id")
					return
				}
				var input struct {
					Type   domain.MovementType `json:"type"`
					Delta  int64               `json:"quantity_delta"`
					Reason string              `json:"reason"`
				}
				if err := decodeJSON(w, r, &input); err != nil {
					writeError(w, http.StatusBadRequest, "invalid request")
					return
				}
				stock, err := inventory.Change(r.Context(), application.InventoryChangeRequest{ProductID: productID, ActorID: UserFromContext(r.Context()).ID, Type: input.Type, Delta: input.Delta, Reason: input.Reason})
				if errors.Is(err, application.ErrInvalidInventoryChange) || errors.Is(err, domain.ErrInvalidMovement) {
					writeError(w, http.StatusBadRequest, "invalid inventory adjustment")
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
					writeError(w, http.StatusInternalServerError, "inventory adjustment failed")
					return
				}
				writeJSON(w, http.StatusOK, stock)
			})))
		}
		if customers != nil {
			mux.Handle("GET /v1/customers", RequirePermission(auth, "customer.read", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
				items, err := customers.List(r.Context(), application.CustomerFilter{Search: r.URL.Query().Get("search"), Limit: limit, Offset: offset})
				if errors.Is(err, application.ErrInvalidCustomer) {
					writeError(w, http.StatusBadRequest, "invalid customer filters")
					return
				}
				if err != nil {
					writeError(w, http.StatusInternalServerError, "could not load customers")
					return
				}
				writeJSON(w, http.StatusOK, items)
			})))
			mux.Handle("POST /v1/customers", RequirePermission(auth, "customer.create", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var input struct {
					Name  string `json:"name"`
					Email string `json:"email"`
					Phone string `json:"phone"`
				}
				if err := decodeJSON(w, r, &input); err != nil {
					writeError(w, http.StatusBadRequest, "invalid request")
					return
				}
				customer, err := customers.Create(r.Context(), UserFromContext(r.Context()).ID, application.Customer{Name: input.Name, Email: input.Email, Phone: input.Phone})
				if errors.Is(err, application.ErrInvalidCustomer) {
					writeError(w, http.StatusBadRequest, "invalid customer")
					return
				}
				if errors.Is(err, application.ErrDuplicateCustomerEmail) {
					writeError(w, http.StatusConflict, "customer email already exists")
					return
				}
				if err != nil {
					writeError(w, http.StatusInternalServerError, "could not create customer")
					return
				}
				writeJSON(w, http.StatusCreated, customer)
			})))
			mux.Handle("PATCH /v1/customers/{customerID}", RequirePermission(auth, "customer.update", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				customerID, err := strconv.ParseInt(r.PathValue("customerID"), 10, 64)
				if err != nil || customerID <= 0 {
					writeError(w, http.StatusBadRequest, "invalid customer id")
					return
				}
				var input struct {
					Name  string `json:"name"`
					Email string `json:"email"`
					Phone string `json:"phone"`
				}
				if err := decodeJSON(w, r, &input); err != nil {
					writeError(w, http.StatusBadRequest, "invalid request")
					return
				}
				customer, err := customers.Update(r.Context(), UserFromContext(r.Context()).ID, customerID, application.Customer{Name: input.Name, Email: input.Email, Phone: input.Phone})
				if errors.Is(err, application.ErrInvalidCustomer) {
					writeError(w, http.StatusBadRequest, "invalid customer")
					return
				}
				if errors.Is(err, application.ErrCustomerNotFound) {
					writeError(w, http.StatusNotFound, "customer not found")
					return
				}
				if errors.Is(err, application.ErrDuplicateCustomerEmail) {
					writeError(w, http.StatusConflict, "customer email already exists")
					return
				}
				if err != nil {
					writeError(w, http.StatusInternalServerError, "could not update customer")
					return
				}
				writeJSON(w, http.StatusOK, customer)
			})))
			mux.Handle("DELETE /v1/customers/{customerID}", RequirePermission(auth, "customer.delete", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				customerID, err := strconv.ParseInt(r.PathValue("customerID"), 10, 64)
				if err != nil || customerID <= 0 {
					writeError(w, http.StatusBadRequest, "invalid customer id")
					return
				}
				err = customers.Deactivate(r.Context(), UserFromContext(r.Context()).ID, customerID)
				if errors.Is(err, application.ErrCustomerNotFound) {
					writeError(w, http.StatusNotFound, "customer not found")
					return
				}
				if err != nil {
					writeError(w, http.StatusInternalServerError, "could not deactivate customer")
					return
				}
				w.WriteHeader(http.StatusNoContent)
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
