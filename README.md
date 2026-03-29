# onixggr

Initial monorepo scaffold for the multi-tenant API bridge described in [`docs/blueprint.md`](docs/blueprint.md).

## Layout

- `apps/api`: Go HTTP API entrypoint
- `apps/manage`: migration and seeding CLI
- `apps/worker`: background worker entrypoint
- `apps/scheduler`: periodic job entrypoint
- `apps/web`: SvelteKit dashboard
- `appctl`: repo-local helper for migration and seed commands
- `internal/`: shared platform and domain packages
- `migrations/`: ordered SQL migrations named `000001_name.up.sql`
- `seeds/demo/`: demo seed SQL files
- `docs/`: architecture, database, and upstream API contracts

## Quick Start

1. Copy `.env.example` to `.env`.
2. Start the local stack with `./scripts/podman-up.sh` or any Compose-compatible runtime using `docker-compose.yml`. The default host ports are `15432` for PostgreSQL, `16379` for Redis, `8080` for the API, and `4173` for the web shell.
3. Run `./appctl migrate up` to apply SQL migrations.
4. Run `./appctl seed demo` to insert the demo rows.
5. Run `go test ./...` to verify the Go scaffold.
6. Run `go run ./apps/api` to start the API on `:8080`.
7. Run `npm install` to install the web workspace dependencies.
8. Run `npm run dev:web` to start the SvelteKit app.

## Local Commands

- `./appctl migrate up`: apply pending migrations.
- `./appctl migrate down`: roll back the last applied migration.
- `./appctl migrate fresh --seed`: recreate the public schema, apply migrations, then run demo seeds.
- `./appctl seed demo`: apply SQL seed files from `seeds/demo/`.
- `./scripts/podman-up.sh`: start PostgreSQL, Redis, API, and web in one command via Podman Compose.
- `go run ./apps/api`: starts the API and exposes `/health/live` plus `/health/ready`.
- `npm run dev:web`: starts the SvelteKit shell with public, auth, and app layouts.

## Auth Core

- `POST /v1/auth/login`: login with `{"login":"dev@example.com","password":"DevDemo123!"}` or `{"login":"owner-demo","password":"OwnerDemo123!"}`.
- `POST /v1/auth/refresh`: rotate the refresh token with `{"refresh_token":"..."}`.
- `GET /v1/auth/me`: read the current dashboard user with `Authorization: Bearer <access_token>`.
- `POST /v1/auth/logout`: revoke the current session.
- `POST /v1/auth/logout-all`: revoke every active session for the current account.

## Notes

- `backend/` and `frontend/` are legacy placeholder directories; new work should go into `apps/`.
- Use `make hooks` after the repository is initialized with Git to enable the local hooks in `.githooks/`.
- API readiness is exposed at `/health/ready` and `/readyz`; liveness is exposed at `/health/live` and `/healthz`.
- Demo seed rows create one `dev` user, one `owner` user, one store, and one audit log entry for local development.
- Demo dashboard credentials after `./appctl migrate fresh --seed`:
- `dev@example.com` or `dev-demo` with password `DevDemo123!`
- `owner@example.com` or `owner-demo` with password `OwnerDemo123!`
