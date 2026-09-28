# Database design

PostgreSQL is the system of record. Planned core tables: `users, roles, permissions, user_roles, role_permissions, categories, products, inventory, stock_movements, customers, orders, order_items, payments, audit_logs`.

Schema work will add foreign keys, check constraints, uniqueness rules, and indexes alongside the migrations that introduce each feature. Money values use integer minor units plus an explicit currency. Timestamped records use UTC.

Critical invariants: stock changes are append-only movements; checkout is transactional and locks inventory; order item prices are snapshots; idempotency uniqueness is merchant-scoped; webhook event identifiers are deduplicated.
