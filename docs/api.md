# API

Base URL: `http://localhost:8080`.

- `GET /healthz` returns `200 OK` with `ok` when the HTTP process is running.
- `GET /readyz` returns `200 OK` when PostgreSQL is reachable, otherwise `503 Service Unavailable`.

The machine-readable contract is [OpenAPI](openapi.yaml). Business API routes will be added alongside implemented use cases. Error responses will use a consistent JSON envelope and request validation will happen at the transport boundary.
