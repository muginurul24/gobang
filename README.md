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
- `./appctl sync providers`: pull provider list and game list from NexusGGR, then upsert the local catalog tables.
- `./appctl worker run`: start the background worker and process game reconcile backlog, QRIS check-status reconcile, withdraw status checks, and outbound callback retries.
- `./appctl scheduler run`: start the scheduler and periodically refresh the provider catalog.
- `./scripts/podman-up.sh`: start PostgreSQL, Redis, API, and web in one command via Podman Compose.
- `go run ./apps/api`: starts the API and exposes `/health/live` plus `/health/ready`.
- `go run ./apps/worker`: starts the background worker and periodically resolves game transactions in `pending_reconcile`, QRIS pending transactions, store withdraw status checks, plus outbound callback retries.
- `go run ./apps/scheduler`: starts the scheduler and periodically refreshes the local provider catalog.
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

## Provider Catalog

- `GET /v1/catalog/providers`: browse and search synced providers with optional `query`, `status`, and `limit`.
- `GET /v1/catalog/games`: browse and search synced games with optional `provider_code`, `query`, `status`, and `limit`.
- `apps/scheduler` now runs periodic provider catalog syncs using `PROVIDER_CATALOG_SYNC_INTERVAL`.
- The dashboard now includes `/app/catalog` for provider/game browse and filter against the local PostgreSQL catalog.

## QRIS / VA Wrapper

- `internal/platform/qris` now wraps provider calls for `generate`, QRIS `checkstatus/v2`, bank `inquiry`, bank `transfer`, and disbursement `check-status`.
- The wrapper follows [`docs/API Qris & VA V3.postman_collection.json`](docs/API%20Qris%20%26%20VA%20V3.postman_collection.json), applies `QRIS_DEFAULT_EXPIRE_SECONDS` as the generate fallback, and masks sensitive request/response fields in logs.
- `ParsePaymentWebhook` and `ParseTransferWebhook` now back the single inbound webhook endpoint at `POST /v1/webhooks/qris`.
- Saved bank-account verification now reuses the shared QRIS wrapper instead of a second bespoke HTTP client.

## Store Topup QRIS

- `qris_transactions` now exists for both `store_topup` and the later `member_payment` milestone, with `provider_trx_id`, `custom_ref`, `status`, expiry, and masked provider payload persistence.
- `GET /v1/stores/{storeID}/topups/qris`: list QRIS store topup history for the selected store in dashboard scope.
- `POST /v1/stores/{storeID}/topups/qris`: create a pending `store_topup`, call provider `generate` with the owner username plus internal `custom_ref`, then persist `provider_trx_id` and QR payload on success.
- `GET /v1/stores/{storeID}/withdrawals`: list dashboard store-withdraw requests for the selected store.
- `POST /v1/stores/{storeID}/withdrawals`: create a dashboard withdraw intent with body `{"bank_account_id":"uuid","amount":1000000,"idempotency_key":"uuid-or-ulid"}`.
- Ambiguous generate responses currently stay in `pending` with `provider_state=pending_provider_response`; hard provider/config errors mark the topup `failed` without touching the ledger.
- `apps/web` now includes `/app/topups` so owners, `dev`, and `superadmin` can generate QRIS topups, render the QR image, and inspect pending, success, failed, or expired history per store.

## Member Payment QRIS

- `POST /v1/store-api/qris/member-payments`: generate QRIS for one existing active store member via Bearer `store_token` with body `{"username":"member-alpha","amount":25000}`.
- Member-payment QRIS is deliberately separated from dashboard `store_topup`: it authenticates with the store token, validates `real_username`, then sends provider `generate` using the member `upstream_user_code`.
- The pending row is stored as `type=member_payment` in `qris_transactions`; once the QRIS webhook reports `success`, the API finalizes it with a 3% platform fee and credits only the net amount to the store ledger.
- Ambiguous provider generate responses still create a `pending` row with `provider_state=pending_provider_response`; hard provider/config failures do not post ledger entries and do not finalize success early.

## QRIS Webhook Finalization

- `POST /v1/webhooks/qris`: single inbound endpoint for QRIS payment callbacks and later disbursement-status callbacks.
- Payment webhooks correlate by `provider_trx_id` first and fall back to `custom_ref`, so pending rows created before provider confirmation can still be finalized safely.
- `store_topup` success credits the full gross amount to the store ledger; `member_payment` success credits the net amount after the configured 3% platform fee and persists both `platform_fee_amount` and `store_credit_amount`.
- Duplicate webhooks are safe: ledger posting uses the `qris_transaction` reference plus a unique ledger-entry guard, so retries do not double-credit the store balance.

## QRIS Reconcile

- Pending QRIS rows with a stored `provider_trx_id` are now scanned by the worker and checked through provider `checkstatus/v2`.
- Reconcile backoff follows the execution plan: first retry after 30 seconds, then 60 seconds, then 120 seconds, then every 5 minutes, with attempts logged in `qris_reconcile_attempts`.
- Provider `success` finalizes through the same idempotent payment finalizer used by webhook handling, so store credit cannot be posted twice.
- Provider `pending` becomes `expired` once the local `expires_at` has passed; transient upstream errors only record the attempt and keep the transaction pending for the next retry.

## Store Withdrawals

- Dashboard withdraw now uses one `idempotency_key` per intent. The first request is persisted in `store_withdrawals`, and duplicate requests with the same key return the existing row instead of making a second inquiry or transfer.
- The request path follows the blueprint: inquiry first, compute `platform_fee_amount` at 12% of the requested net amount, add provider `external_fee_amount`, check available balance, reserve `total_store_debit`, then call transfer.
- `POST /v1/webhooks/qris` now also finalizes withdraw transfer callbacks by `provider_partner_ref_no`, commits the reservation on `success`, and releases it on `failed`.
- Pending withdraw rows with `provider_partner_ref_no` are scanned by the worker every 30 seconds through provider disbursement `check-status`, with every attempt recorded in `withdrawal_status_checks`.
- Final withdraw success is idempotent: duplicate webhook/check-status callbacks reuse the same `store_withdrawal` reference, so the ledger reservation cannot be committed twice.

## Outbound Callbacks

- `member_payment.success` now enqueues one durable callback row in `outbound_callbacks`, keyed by `event_type + reference_type + reference_id` so duplicate webhooks cannot duplicate callback delivery intent.
- The worker reads due callback rows, signs the stored payload with `X-Onixggr-Signature`, then POSTs it to the store `callback_url` with `X-Onixggr-Event`, `X-Onixggr-Delivery-ID`, and reference headers.
- Failed callback deliveries are logged in `outbound_callback_attempts` with masked response bodies and exponential backoff for 5 retries; the last failure creates a `callback.delivery_failed` notification for the store.
- Local tuning uses `CALLBACK_SIGNING_SECRET`, `CALLBACK_DELIVERY_TIMEOUT`, `CALLBACK_RETRY_INTERVAL`, and `CALLBACK_RETRY_BATCH_SIZE`.

## Realtime Backbone

- `GET /v1/realtime/ws`: authenticated websocket endpoint for dashboard sessions. Browser clients pass the dashboard access token as `?access_token=...`, and the API validates it against the same Redis-backed session state used by HTTP auth.
- Current channel routing follows the blueprint backbone: `user:{userId}`, owner or karyawan `store:{storeId}` scopes, `role:dev` or `role:superadmin`, plus `global_chat`.
- The API now keeps one Redis pub/sub subscription fanout in `internal/platform/realtime`; every websocket connection gets a local subscription set, a `hello` frame, periodic heartbeat frames, and a Redis-delivered `realtime.connection.ready` event.
- `/app/chat` now hosts the one-room global chat required by the blueprint. Messages are created over HTTP and fanned out through the `global_chat` websocket channel in realtime.
- Local tuning uses `WS_HEARTBEAT_SECONDS`. The Vite dev proxy now forwards websocket upgrades on `/v1`, so local web sessions can connect through the same origin in development.

## Chat Global

- `GET /v1/chat/messages`: list active messages in the single global room, capped by the 7-day retention window.
- `POST /v1/chat/messages`: send one message to the global room with body `{"body":"halo semua"}`.
- `DELETE /v1/chat/messages/{messageID}`: dev-only moderation delete. Regular users cannot edit or delete messages.
- `apps/scheduler` prunes `chat_messages` older than 7 days on a configurable interval via `CHAT_RETENTION_PERIOD` and `CHAT_PRUNE_INTERVAL`.
- There is no DM flow and no multi-room support. All realtime chat events publish through `global_chat`.

## Dashboard Cards

- `GET /v1/dashboard/cards`: role-aware dashboard aggregate endpoint. Owner or karyawan receives store-scope cards only, while dev or superadmin receives platform-scope cards only.
- Store cards include total balance, pending QRIS, success today, expired today, and monthly store income from `member_payment.success`.
- Platform cards include income today or month, total stores, pending withdraw, upstream error rate 24h, and callback failure rate 24h.

## Audit Log

- `GET /v1/audit/logs`: role-aware audit endpoint. Owner stays scoped to owned stores and owned staff domain, while dev or superadmin can query the global audit stream.
- Supported filters now include `store_id`, `action`, `actor_role`, `target_type`, and `limit`.
- Coverage now includes login success or fail, store lifecycle changes, token create or rotate or revoke-via-rotation, callback URL updates, withdraw request and result, QRIS topup result, and other manual dashboard actions that already flow through domain services.
- `apps/scheduler` prunes `audit_logs` older than 90 days via `AUDIT_RETENTION_PERIOD` and `AUDIT_PRUNE_INTERVAL`.

## Observability

- When `METRICS_ENABLED=true`, the API also starts a Prometheus exporter on `:${PROMETHEUS_PORT}` with `GET /metrics`.
- Exported basics now include request count and latency, upstream latency, webhook processing results, game balance cache hit or miss, callback queue depth, recent callback failures, dependency health, reconcile backlog, and websocket connection count.
- Health readiness now distinguishes degraded upstream configuration from hard dependency failure: PostgreSQL or Redis failure keeps `/health/ready` as `503`, while missing QRIS or NexusGGR credentials marks readiness payload as `degraded` but keeps the API bootable.
- Starter PromQL panels live in [`basic-dashboard.md`](/home/mugiew/project/onixggr/deploy/monitoring/basic-dashboard.md).
- Starter alert rules for Hari 38 live in [`alerts.rules.yml`](/home/mugiew/project/onixggr/deploy/monitoring/alerts.rules.yml) and cover webhook failure spike, callback failure spike, Redis down, DB down, NexusGGR error spike, and QRIS error spike.

## Store API Game Flows

- `POST /v1/store-api/game/users`: create a game user via Bearer `store_token` with body `{"username":"member-alpha"}`.
- `GET /v1/store-api/game/balance?username=member-alpha`: read one member balance via Bearer `store_token`.
- Balance reads cache the upstream `money_info` result in Redis for 5 seconds and coalesce concurrent requests per store/member key inside the API process.
- `POST /v1/store-api/game/launch`: launch a game via Bearer `store_token` with body `{"username":"member-alpha","provider_code":"PRAGMATIC","game_code":"vs20doghouse","lang":"id"}`.
- Launch validates `provider_code` and `game_code` against the synced catalog tables, auto-creates the member mapping if it does not exist yet, and logs every launch attempt without idempotency.
- The flow follows `docs/blueprint.md`: reject duplicate `(store_id, username)`, generate a 12-char immutable `upstream_user_code`, call NexusGGR `user_create`, then persist the mapping only on upstream success.
- Store API token auth is scoped by `stores.api_token_hash`, and inactive or deleted stores are blocked before upstream calls.
- `POST /v1/store-api/game/deposits`: create a game deposit via Bearer `store_token` with body `{"username":"member-alpha","amount":5000,"trx_id":"trx-001"}`.
- Deposit currently requires an existing active member mapping, rejects duplicate `trx_id`, rejects insufficient balance, reserves balance before the upstream call, commits ledger debit on success, and returns `202 PENDING_RECONCILE` on timeout or other ambiguous upstream failures.
- `POST /v1/store-api/game/withdrawals`: create a game withdraw via Bearer `store_token` with body `{"username":"member-alpha","amount":5000,"trx_id":"trx-002"}`.
- Withdraw credits store balance only after upstream `user_withdraw` succeeds, marks ambiguous upstream responses as `pending_reconcile`, and returns the existing transaction on an idempotent retry with the same `trx_id`.
- `apps/worker` now scans `game_transactions` with `status=pending` plus `reconcile_status=pending_reconcile`, calls NexusGGR `transfer_status`, finalizes ledger success/fail safely, and emits store-scope notifications when reconcile closes.

## Notes

- `backend/` and `frontend/` are legacy placeholder directories; new work should go into `apps/`.
- Use `make hooks` after the repository is initialized with Git to enable the local hooks in `.githooks/`.
- API readiness is exposed at `/health/ready` and `/readyz`; liveness is exposed at `/health/live` and `/healthz`.
- `notifications` now backs the realtime notification stream. Use `GET /v1/notifications`, `GET /v1/notifications/unread-count`, and `POST /v1/notifications/{id}/read` with the same scope rules as the dashboard role.
- Demo seed rows create one `dev` user, one `owner` user, one `karyawan` user, one store, one demo member, one sample provider/game catalog row, one store-staff relation, and one audit log entry for local development.
- Demo dashboard credentials after `./appctl migrate fresh --seed`:
- `dev@example.com` or `dev-demo` with password `DevDemo123!`
- `owner@example.com` or `owner-demo` with password `OwnerDemo123!`
- `staff@example.com` or `staff-demo` with password `StaffDemo123!`
- `apps/web` now contains a working login page plus `/app/stores`, `/app/topups`, `/app/withdrawals`, `/app/members`, `/app/bank-accounts`, `/app/audit`, and `/app/security` for store ops, QRIS topup generation, store withdraw requests, member mapping, verified bank accounts, scoped audit, TOTP enrollment, recovery code handoff, and dashboard IP allowlist management.
- Set `PUBLIC_API_BASE_URL` only when the web shell should talk to a different API origin; otherwise dev mode proxies `/v1` to `http://127.0.0.1:8080`.
