# Database migrations

Migrations are ordered SQL files with `.up.sql` and `.down.sql` suffixes. The Go service embeds the up files, records applied versions in `schema_migrations`, and applies pending migrations at startup inside PostgreSQL transactions. A PostgreSQL advisory lock serializes migration startup across instances.

For manual local inspection, start PostgreSQL with `docker compose up -d postgres` and inspect the schema with `docker compose exec postgres psql -U app -d pos_system`. Apply an up script manually only when running migrations outside the application. Rollback scripts are intended for controlled operator use; the service never runs down migrations automatically.
