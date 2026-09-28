# ADR 001: Architecture and persistence boundaries

- Status: Accepted
- Date: 2026-09-28

## Context

The project should demonstrate maintainable Go backend design and preserve business invariants under concurrent requests.

## Decision

Adopt pragmatic hexagonal architecture with explicit domain, application, infrastructure, and interface layers. PostgreSQL is the source of truth. Checkout will reserve/decrement stock and persist order, items, payment state, stock movements, and audit records in one transaction, locking inventory rows to prevent overselling. Order item prices are server-side snapshots; client totals are never authoritative.

## Consequences

Use cases can be tested without PostgreSQL or external providers. Adapter and transaction integration tests will verify database behavior as these flows are implemented.
