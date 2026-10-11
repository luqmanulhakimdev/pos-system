# Database design

PostgreSQL is the system of record. Ordered migrations create users, roles, permissions, user-role and role-permission assignments, categories, products, inventory, customers, orders, order items, stock movements, payments, refunds, and audit logs. They seed the four standard roles and granular permissions; no default user or password is created. See the `migrations/` directory for exact columns, constraints, and indexes.

Important constraints include unique user email and product SKU, nonnegative prices and stock, positive order-item quantities, explicit order and payment state checks, order item price snapshots, and foreign keys that prevent deleting records referenced by financial history. Partial indexes support optional customer email/phone and audit/order lookup patterns.

Money is stored as integer minor units with an explicit ISO currency code. Timestamps are `TIMESTAMPTZ`. Checkout and order cancellation lock inventory rows and write stock movements with order state in one transaction. Refund reservations use an order-scoped idempotency key and a stable provider key; financial refunds do not automatically restock goods. The application computes totals from persisted prices; the database also checks each line total against quantity × snapshot unit price.

The API applies all pending migrations at startup under a PostgreSQL advisory lock. See [ERD](erd.md) and [migration instructions](../migrations/README.md).
