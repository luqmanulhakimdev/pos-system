# ADR 002: Transactional checkout

- Status: Accepted
- Date: 2026-09-28

## Decision

Checkout will create order records and update inventory under one PostgreSQL transaction. Inventory rows are locked while availability is checked and stock movement entries are inserted in the same transaction. The server calculates totals from current product prices and snapshots each unit price into the order item.

## Consequences

Concurrent checkout cannot oversell when all stock writes follow this path. External payment calls must not hold the database transaction open; payment orchestration will use explicit states and compensating actions.
