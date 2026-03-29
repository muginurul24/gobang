# onixggr

Initial monorepo scaffold for the multi-tenant API bridge described in [`docs/blueprint.md`](docs/blueprint.md).

## Layout

- `apps/api`: Go HTTP API entrypoint
- `apps/manage`: migration and seeding CLI
- `apps/worker`: background worker entrypoint
- `apps/scheduler`: periodic job entrypoint
- `apps/web`: SvelteKit dashboard
- `internal/`: shared platform and domain packages
- `migrations/`: ordered SQL migrations named `000001_name.up.sql`
- `seeds/demo/`: optional demo seed SQL files
- `docs/`: architecture, database, and upstream API contracts

## Quick Start

1. Copy `.env.example` to `.env`.
2. Start local services with `./scripts/podman-up.sh` or any Compose-compatible runtime using `docker-compose.yml`. The default host ports are `15432` for PostgreSQL and `16379` for Redis to avoid common local conflicts.
3. Run `go run ./apps/manage migrate up` to apply SQL migrations.
4. Run `go test ./...` to verify the Go scaffold.
5. Run `go run ./apps/api` to start the API on `:8080`.
6. Run `npm install` to install the web workspace dependencies.
7. Run `npm run dev:web` to start the SvelteKit app.

## Notes

- `backend/` and `frontend/` are legacy placeholder directories; new work should go into `apps/`.
- Use `make hooks` after the repository is initialized with Git to enable the local hooks in `.githooks/`.
- API readiness is exposed at `/readyz`; liveness is exposed at `/healthz`.
- `go run ./apps/manage migrate fresh --seed` recreates the public schema, reapplies migrations, then runs any SQL files in `seeds/demo/`.
