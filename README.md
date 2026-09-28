# POS System

A production-oriented point-of-sale backend focused on reliable inventory and checkout workflows.

> Portfolio and educational project. The payment gateway does not process real card data. Never submit real card numbers, CVV, or credentials.

## Architecture

Pragmatic hexagonal architecture separates domain rules, application use cases, infrastructure adapters, and HTTP interfaces. See [architecture](docs/architecture.md) and the [architecture decision record](docs/adr/001-architecture.md).

## Features

- Go HTTP service with health endpoint
- PostgreSQL local development environment
- Domain and persistence structure prepared for the planned capabilities: Authentication and RBAC; products and categories; inventory and stock movements; customers; orders and order items; checkout and payment integration; cancellation, refunds, and audit logs.

Business flows are added incrementally; the current baseline does not claim these features are implemented.

## Tech stack

Go 1.23, PostgreSQL 16, Docker Compose, GitHub Actions.

## ERD

See [docs/erd.md](docs/erd.md) for the Mermaid diagram.

## Local setup

```sh
cp .env.example .env
docker compose up --build
# in another terminal
curl -i http://localhost:8080/healthz
```

The Compose database credentials are for local development only. Do not reuse them outside local development.

## Environment variables

| Variable | Purpose | Default |
| --- | --- | --- |
| `APP_ENV` | Runtime environment | `development` |
| `HTTP_ADDR` | HTTP listen address | `:8080` |
| `DATABASE_URL` | PostgreSQL connection | local Compose database |
| `LOG_LEVEL` | Log verbosity | `debug` |

## Migrations

Ordered up/down SQL migrations live in `migrations/`. Apply the initial schema using the command in [migration instructions](migrations/README.md). See [database design](docs/database.md).

## API example

```sh
curl -i http://localhost:8080/healthz
```

Planned routes are documented in [docs/api.md](docs/api.md); the baseline OpenAPI contract is [docs/openapi.yaml](docs/openapi.yaml).

## Testing

```sh
go test ./...
go vet ./...
go build ./...
```

Database integration tests will be added with persistence flows. CI currently runs formatting, vet, tests, and build.

## Design decisions

- Adopt pragmatic hexagonal architecture with explicit domain, application, infrastructure, and interface layers. PostgreSQL is the source of truth. Checkout will reserve/decrement stock and persist order, items, payment state, stock movements, and audit records in one transaction, locking inventory rows to prevent overselling. Order item prices are server-side snapshots; client totals are never authoritative.
- Detailed state transitions: `PENDING → CONFIRMED → PAID → REFUNDED; PENDING or CONFIRMED → CANCELLED`.

## Future improvements

Implement schema migrations and use cases incrementally, publish an OpenAPI contract, add unit and PostgreSQL integration tests, and add operational metrics and tracing.
