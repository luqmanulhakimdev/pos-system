# ADR 003: PostgreSQL migrations at startup

- Status: Accepted
- Date: 2026-09-28

## Context

Local Compose startup and CI should use the same schema, and concurrently starting service instances must not apply a migration twice.

## Decision

Embed ordered up migrations in the Go binary, apply each unapplied version in a transaction, and serialize runners with a transaction-scoped PostgreSQL advisory lock. Record applied versions in `schema_migrations`. Keep down migrations for controlled operator use; startup never rolls back schema.

## Consequences

The application role needs DDL privileges during startup. Production deployments can later separate migration credentials from the runtime role if operational policy requires it. Migrations must avoid non-transactional statements.
