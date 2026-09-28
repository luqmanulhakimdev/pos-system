# Database design

PostgreSQL is the system of record. The first migration creates users, roles, permissions, user-role and role-permission assignments, categories, products, inventory, customers, orders, order items, stock movements, payments, and audit logs. See `migrations/000001_initial_schema.up.sql` for exact columns, constraints, and indexes.

Important constraints include unique user email and product SKU, nonnegative prices and stock, positive order-item quantities, explicit order and payment state checks, order item price snapshots, and foreign keys that prevent deleting records referenced by financial history. Partial indexes support optional customer email/phone and audit/order lookup patterns.

Money is stored as integer minor units with an explicit ISO currency code. Timestamps are `TIMESTAMPTZ`. Checkout must lock inventory rows and write stock movements with order state in one transaction. The application computes totals from persisted prices; the database also checks each line total against quantity × snapshot unit price.

Apply locally using `docker compose exec -T postgres psql -U app -d pos_system < migrations/000001_initial_schema.up.sql`. See [ERD](erd.md).
