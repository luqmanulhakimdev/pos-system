# API

Base URL: `http://localhost:8080`.

- `GET /healthz` returns `200 OK` with `ok` when the HTTP process is running.
- `GET /readyz` returns `200 OK` when PostgreSQL is reachable, otherwise `503 Service Unavailable`.
- `POST /v1/auth/login` accepts an email and password and returns a short-lived bearer token. Invalid credentials return `401`.
- `GET /v1/me` requires a bearer token and returns the account email and its current permission names.
- `POST /v1/auth/logout` requires a bearer token and revokes that session. A revoked token returns `401` on subsequent requests.
- `POST /v1/orders/{orderID}/payments` charges a payable order through the configured mock provider; it requires `payment.create` and uses a stable provider idempotency key for retries. It returns `409` for orders that cannot be paid and `502` when the provider is unavailable.
- `POST /v1/orders/checkout` creates a pending order for the authenticated cashier, derives prices from PostgreSQL, and atomically records order items, stock movements, inventory updates, and audit data. It requires `order.create`.
- `GET /v1/orders` and `GET /v1/orders/{orderID}` require `order.read`; the detail response uses checkout-time item name, SKU, and unit-price snapshots.
- `POST /v1/orders/{orderID}/cancel` requires `order.cancel`, cancels only pending or confirmed unpaid orders, and returns stock with cancellation movements in the same audited transaction. An order with a pending or authorized payment must be reconciled before it can be cancelled.
- `POST /v1/orders/{orderID}/refunds` requires `payment.refund` and an `Idempotency-Key`; it refunds the full paid amount, then marks the payment and order `REFUNDED`. Retries reuse the same provider idempotency key. Stock is not automatically changed by a financial refund; record an inventory return when goods are physically received.
- `GET /v1/reports/sales?from=YYYY-MM-DD&to=YYYY-MM-DD` requires `report.read` and returns paid-order count, gross, refunded, and net amounts grouped by currency for an inclusive range of up to 366 days.
- `GET /v1/categories` and `GET /v1/products` require `product.read`; product listing supports `search`, `category_id`, `limit`, and `offset` filters.
- `POST /v1/categories` and `POST /v1/products` require `product.create`. Product prices use integer minor units and default to IDR if currency is omitted.
- `GET /v1/customers` requires `customer.read` and supports bounded search and pagination. `POST /v1/customers` requires `customer.create`, a name, and at least one of email or phone. Email addresses are normalized and unique; creation and its audit record commit together.
- Creating a product also creates a zero-quantity inventory record. `GET /v1/inventory/{productID}` requires `inventory.read`; `POST /v1/inventory/{productID}/adjustments` requires `inventory.adjust` and records a stock movement and audit event in the same transaction.

Authenticated routes load current role permissions from PostgreSQL on each request. The `RequirePermission` middleware is available for business handlers; permission names are seeded in the initial migration (for example `order.read` and `inventory.adjust`).

The local payment adapter always returns a deterministic successful mock result and accepts no card credentials. A real provider must honor the payment attempt's stable idempotency key so a retry after a timeout does not create a second charge.

Checkout request example: `{"items":[{"product_id":12,"quantity":2}]}`. Product example: `{"sku":"SKU-001","name":"Coffee","price_minor":25000,"currency":"IDR"}`.

The machine-readable contract is [OpenAPI](openapi.yaml). Business API routes will be added alongside implemented use cases. Error responses will use a consistent JSON envelope and request validation will happen at the transport boundary.
