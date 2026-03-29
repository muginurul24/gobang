# AGENTS.md

## Source of truth
Always treat these files as authoritative before making changes:

1. `docs/plan-execution.md` — execution order and milestone plan
2. `docs/blueprint.md` — final architecture and domain rules
3. `docs/database-final.md` — database contract
4. `docs/nexusggr-openapi-3.1.yaml` — game upstream contract
5. `docs/API Qris & VA V3.postman_collection.json` — QRIS/VA upstream contract
6. `docs/Bank RTOL.json` — valid bank_code reference

If code or assumptions conflict with these documents, follow the documents and update code accordingly.

## Repository shape
This project must follow this structure:

- `apps/web` — SvelteKit frontend
- `apps/api` — Go API
- `apps/worker` — Go worker
- `apps/scheduler` — Go scheduler
- `internal/platform/*` — shared infrastructure
- `internal/modules/*` — domain modules
- `migrations/` — PostgreSQL migrations
- `seeds/` — seed scripts
- `scripts/` — developer tooling
- `deploy/` — deployment assets

Do not introduce alternative top-level layouts without updating the docs first.

## Architecture rules
- PostgreSQL is the source of truth.
- Redis is never the source of truth for balances or transaction finality.
- All store balance mutations must go through ledger posting.
- Timeout or ambiguous upstream responses must go to pending/reconcile flow.
- All money-moving flows must be idempotent.
- All owner/karyawan data access must be tenant-scoped by `store_id`.
- Never bypass domain services from HTTP handlers.

## Domain rules that must not be broken
- 1 store = 1 active token.
- Store token is used for all store API endpoints.
- `stores.owner_user_id` defines store ownership.
- `(store_id, real_username)` is unique for store members.
- `upstream_user_code` is immutable, unique globally, and never exposed as a replacement for the real username in owner-facing UI.
- Game deposit success debits store balance.
- Game withdraw success credits store balance.
- `store_topup` and `member_payment` are separate transaction domains.
- `member_payment` credits store balance after deducting the platform fee.
- Store withdraw uses inquiry first, then reserve, then transfer, then webhook/check-status finalization.

## Security rules
- Never commit real provider credentials, callback secrets, production URLs, or bank account data.
- Never store raw secrets or full sensitive upstream payloads in logs or audit rows.
- Mask sensitive values before persistence when full storage is not explicitly required.
- Owner and superadmin may view full token and full callback URL.
- Karyawan must never view full token, callback URL, or withdraw bank details.
- Dev has full platform visibility.

## Build and validation
When scaffolding lands, prefer these commands and keep them working:

- `migrate up`
- `migrate fresh`
- `migrate fresh --seed`
- `seed demo`
- `worker run`
- `scheduler run`
- `sync providers`

After each milestone:
1. run formatting
2. run tests relevant to the changed module
3. fix failures before expanding scope

## Coding conventions
- Use lower-case module directories, for example `internal/modules/ledger`.
- Use `snake_case` for SQL tables, columns, enums, and transaction identifiers.
- Use tool-enforced formatting only:
  - `gofmt` for Go
  - Prettier for frontend files
- Keep handlers thin, services explicit, and repositories isolated.

## Testing priorities
Prioritize tests in this order:

1. ledger and money domain services
2. persistence/repository correctness
3. upstream adapter integration behavior
4. reconcile and duplicate-webhook paths
5. RBAC and tenant-boundary tests

Place tests near the code they verify.

## Execution behavior for Codex
When implementing:
- follow `docs/plan-execution.md` milestone order
- keep diffs scoped to the current milestone
- do not silently redesign documented business rules
- do not invent missing provider behavior; use the upstream docs
- update docs if implementation changes a documented contract
- if a required rule is ambiguous, stop and ask rather than guessing

## Pull request expectations
Use Conventional Commits.

PRs should include:
- scope summary
- affected modules
- migrations added/changed
- API contract changes
- test evidence
- screenshots for dashboard/UI changes