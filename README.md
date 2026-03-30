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
- `GET /v1/auth/security`: read current `totp_enabled` and `ip_allowlist`.
- `POST /v1/auth/2fa/enroll`: create a pending TOTP enrollment and return the `otpauth_url`.
- `POST /v1/auth/2fa/enable`: verify the authenticator code and return recovery codes once.
- `POST /v1/auth/2fa/disable`: disable 2FA with a valid TOTP code or recovery code.
- `PUT /v1/auth/ip-allowlist`: set or clear the single-IP dashboard allowlist.

## Store & Audit APIs

- `GET /v1/stores`: list stores by role scope.
- `POST /v1/stores`: owner-only store creation and returns one-time `api_token`.
- `PATCH /v1/stores/{storeID}`: update store name, status, and low balance threshold.
- `DELETE /v1/stores/{storeID}`: soft delete a store.
- `POST /v1/stores/{storeID}/token`: rotate store token and return the new plaintext token once.
- `PUT /v1/stores/{storeID}/callback-url`: set or clear the callback URL.
- `GET /v1/stores/{storeID}/staff`: list store staff for non-karyawan roles in scope.
- `GET /v1/staff/users`: owner-only employee list.
- `POST /v1/staff/users`: owner-only employee creation.
- `POST /v1/stores/{storeID}/staff`: owner-only staff assignment.
- `DELETE /v1/stores/{storeID}/staff/{userID}`: owner-only staff unassignment.
- `GET /v1/stores/{storeID}/members`: list store members in tenant scope.
- `POST /v1/stores/{storeID}/members`: create a member mapping with a generated immutable 12-char `upstream_user_code`.
- `GET /v1/audit/logs`: owner-scoped audit feed, or global for `dev` and `superadmin`.

## Bank Account APIs

- `GET /v1/banks?query=bca&limit=15`: search Bank RTOL directory by `bank_code` or `bank_name`.
- `GET /v1/stores/{storeID}/bank-accounts`: list masked bank accounts for the selected store.
- `POST /v1/stores/{storeID}/bank-accounts`: verify account with inquiry first, then store encrypted account number plus masked UI snapshot.
- `PATCH /v1/stores/{storeID}/bank-accounts/{bankAccountID}`: activate or deactivate a saved bank account.
- Local development falls back to a deterministic mock inquiry verifier whenever `QRIS_CLIENT`, `QRIS_CLIENT_KEY`, or `QRIS_GLOBAL_UUID` are still empty or `QRIS_BASE_URL` is still the placeholder value.

## Ledger & NexusGGR Foundation

- `ledger_accounts`, `ledger_entries`, and `ledger_reservations` now exist and keep `stores.current_balance` as a projection cache.
- `store_members` now exists with unique `(store_id, real_username)` plus globally unique immutable `upstream_user_code`.
- `internal/platform/nexusggr` wraps `provider_list`, `game_list`, `game_launch`, `money_info`, `user_create`, `user_deposit`, `user_withdraw`, `user_withdraw_reset`, and `transfer_status`.
- NexusGGR business failures are normalized even when upstream still returns HTTP `200`, request/response logs are masked, and `NEXUSGGR_TIMEOUT` controls the transport timeout.

## Notes

- `backend/` and `frontend/` are legacy placeholder directories; new work should go into `apps/`.
- Use `make hooks` after the repository is initialized with Git to enable the local hooks in `.githooks/`.
- API readiness is exposed at `/health/ready` and `/readyz`; liveness is exposed at `/health/live` and `/healthz`.
- Demo seed rows create one `dev` user, one `owner` user, one `karyawan` user, one store, one demo member, one store-staff relation, and one audit log entry for local development.
- Demo dashboard credentials after `./appctl migrate fresh --seed`:
- `dev@example.com` or `dev-demo` with password `DevDemo123!`
- `owner@example.com` or `owner-demo` with password `OwnerDemo123!`
- `staff@example.com` or `staff-demo` with password `StaffDemo123!`
- `apps/web` now contains a working login page plus `/app/stores`, `/app/members`, `/app/bank-accounts`, `/app/audit`, and `/app/security` for store ops, member mapping, verified bank accounts, scoped audit, TOTP enrollment, recovery code handoff, and dashboard IP allowlist management.
- Set `PUBLIC_API_BASE_URL` only when the web shell should talk to a different API origin; otherwise dev mode proxies `/v1` to `http://127.0.0.1:8080`.
