# POS System

A Go point-of-sale backend project focused on reliable inventory and checkout workflows.

## Architecture

Pragmatic hexagonal architecture separates domain rules, application use cases, infrastructure adapters, and HTTP interfaces. PostgreSQL is the system of record; `pgx` manages the connection pool. See [architecture](docs/architecture.md), [database design](docs/database.md), and [architecture decisions](docs/adr/).

## Implemented baseline

- Go HTTP service with process health and database readiness endpoints
- PostgreSQL schema migrations applied at startup under an advisory lock
- Seeded roles and granular permissions
- Transactional checkout and stock adjustment use cases, with stock movements, price snapshots, server-calculated totals, and audit records
- Docker Compose local environment and GitHub Actions CI with a PostgreSQL integration-test service

The HTTP business endpoints are still in progress; checkout and inventory change use cases use PostgreSQL transactions; concurrent oversell protection is covered by a CI integration test.

## Tech stack

Go 1.23, PostgreSQL 16, Docker Compose, GitHub Actions.

## ERD

See [docs/erd.md](docs/erd.md) for the Mermaid entity relationship diagram.

## Local setup

```sh
cp .env.example .env
docker compose up --build
# in another terminal
curl -i http://localhost:8080/healthz
curl -i http://localhost:8080/readyz
```

The Compose database credentials are for local development only. Do not reuse them outside local development.

## Environment variables

| Variable | Purpose | Default |
| --- | --- | --- |
| `HTTP_ADDR` | HTTP listen address in the container | `:8080` |
| `HTTP_PORT` | Host port for the API | `8080` |
| `POSTGRES_PORT` | Host port for PostgreSQL | `5432` |
| `DATABASE_URL` | Required PostgreSQL connection | Local Compose database |

## Migrations

Ordered up/down SQL migrations live in `migrations/`. Pending up migrations run at service startup. See [migration instructions](migrations/README.md) and [database design](docs/database.md).

## API and OpenAPI

```sh
curl -i http://localhost:8080/healthz
curl -i http://localhost:8080/readyz
```

See [docs/api.md](docs/api.md) and the [OpenAPI contract](docs/openapi.yaml). Business routes will be added with their use cases.

## Testing

```sh
go test ./...
go vet ./...
go build ./...
```

Unit tests run without a database. To include the PostgreSQL migration integration test, start a clean local database and set `TEST_DATABASE_URL` before `go test ./...`. CI runs both unit and integration tests.

### Create the first administrator

Copy `.env.example` to `.env`, start PostgreSQL, and set `ADMIN_EMAIL` and `ADMIN_PASSWORD` in the local `.env` file. Use a password between 12 and 72 bytes. Then run `docker compose --profile tools run --rm bootstrap-admin`. The command applies pending migrations and creates one active administrator only when no active admin exists; it refuses subsequent bootstrap attempts. Keep `.env` out of version control. Login at `POST /v1/auth/login`, then use the returned bearer token with `GET /v1/me` and `POST /v1/auth/logout`.

## Design decisions

- Stock changes use stock movements; checkout will apply inventory balance changes and movements in the same transaction while locking inventory rows.
- The server calculates order totals from persisted product prices and stores unit price snapshots on order items.
- Explicit order states are `PENDING`, `CONFIRMED`, `PAID`, `CANCELLED`, and `REFUNDED`; valid transitions are documented in [architecture](docs/architecture.md).

## Future improvements

Implement catalog and user APIs, transactional checkout and cancellation, authentication and RBAC enforcement, payment provider integration, audit event writing, and operational metrics and tracing.
