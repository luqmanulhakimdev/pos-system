# API

Base URL: `http://localhost:8080`.

- `GET /healthz` returns `200 OK` with `ok` when the HTTP process is running.
- `GET /readyz` returns `200 OK` when PostgreSQL is reachable, otherwise `503 Service Unavailable`.
- `POST /v1/auth/login` accepts an email and password and returns a short-lived bearer token. Invalid credentials return `401`.
- `GET /v1/me` requires a bearer token and returns the account email and its current permission names.
- `POST /v1/auth/logout` requires a bearer token and revokes that session. A revoked token returns `401` on subsequent requests.

Authenticated routes load current role permissions from PostgreSQL on each request. The `RequirePermission` middleware is available for business handlers; permission names are seeded in the initial migration (for example `order.read` and `inventory.adjust`).

The machine-readable contract is [OpenAPI](openapi.yaml). Business API routes will be added alongside implemented use cases. Error responses will use a consistent JSON envelope and request validation will happen at the transport boundary.
