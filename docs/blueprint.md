# PLAN.md — Blueprint A–Z

Dokumen ini adalah planning implementasi end-to-end untuk platform API bridge multi-tenant dengan:

- **Backend**: Go
- **Frontend**: SvelteKit + Svelte 5
- **Database**: PostgreSQL
- **Cache / Session / Rate limit / Queue assist / Realtime fanout**: Redis
- **Styling**: Tailwind CSS v4 + shadcn-svelte (custom design system)
- **Integrasi upstream game**: NexusGGR
- **Integrasi upstream payment/disbursement**: QRIS / VA provider

Dokumen ini mengikuti keputusan final yang sudah ditetapkan:

- **1 toko = 1 token API**
- **Balance toko = ledger internal project**
- **Game deposit success => balance toko berkurang**
- **Game withdraw success => balance toko bertambah**
- **QRIS store_topup success => balance toko bertambah**
- **QRIS member_payment success => balance toko bertambah setelah fee platform 3%**
- **Withdraw store balance** memakai model **net requested amount**, fee platform **12%** dari net, ditambah fee external hasil inquiry
- **Timeout ambigu** tidak langsung mengubah saldo final; masuk jalur **pending + reconcile**
- **Realtime** memakai WebSocket
- **Audit log** wajib untuk semua role
- **Chat global** hanya 1 room, retensi 7 hari

---

# 1. Tujuan Sistem

## 1.1 Tujuan bisnis

Membangun platform API bridge multi-tenant di mana:

1. **Owner toko** memiliki toko di dashboard.
2. **Website owner** mengintegrasikan website mereka ke platform menggunakan **token toko** sebagai Bearer token.
3. Platform menjadi **jembatan** antara website owner dan external API:
   - game provider (NexusGGR)
   - QRIS/payment provider
4. Platform memiliki **ledger internal** per toko sebagai balance operasional.
5. Semua alur transaksi penting diaudit, bisa di-reconcile, dan aman terhadap retry/duplicate.

## 1.2 Tujuan teknis

1. Aman terhadap duplicate request, webhook duplicate, retry, timeout, dan race condition.
2. Mudah di-debug dan di-maintain.
3. Bisa di-scale tanpa berpindah ke microservices terlalu dini.
4. Memiliki boundary role yang jelas:
   - `dev`
   - `superadmin`
   - `owner`
   - `karyawan`
5. Memiliki DX yang nyaman untuk pengembangan jangka panjang.

## 1.3 Non-goals awal

1. Tidak memecah jadi microservices dulu.
2. Tidak membuat multi-region atau distributed database.
3. Tidak menambah fitur file upload/attachment chat.
4. Tidak membuat DM/private chat.
5. Tidak membuat multi-token per toko.

---

# 2. Prinsip Arsitektur

## 2.1 Bentuk sistem

Gunakan **modular monolith + worker + scheduler** dalam satu repo:

- **apps/web**: SvelteKit frontend
- **apps/api**: Go HTTP API utama
- **apps/worker**: Go worker untuk retry/reconcile/callback processing
- **apps/scheduler**: Go scheduler untuk job periodik

## 2.2 Prinsip sumber kebenaran

- **PostgreSQL** = source of truth
- **Redis** = cache / session / limiter / pubsub / lock / queue support
- **Jangan** menjadikan Redis sebagai source of truth saldo atau histori transaksi

## 2.3 Prinsip uang/ledger

- Semua perubahan saldo toko **harus lewat ledger**
- Tidak boleh ada update saldo toko secara langsung tanpa ledger posting
- Tidak boleh ada saldo toko minus
- Semua transaksi ambigu harus masuk status `pending` lalu di-reconcile

## 2.4 Prinsip integrasi external API

- Semua integrasi upstream dibungkus dalam **adapter/service** internal
- Handler/controller **tidak boleh** memanggil upstream langsung
- Upstream response harus dinormalisasi ke kontrak internal
- Raw payload upstream disimpan **masked/sanitized**

---

# 3. Domain Sistem

## 3.1 Auth & Security
- login email/username + password
- JWT access token + session record di Redis
- one account one device
- TOTP 2FA optional
- recovery codes
- IP allowlist per user dashboard

## 3.2 Stores
- pembuatan toko
- token toko
- callback URL
- threshold low balance
- bank account tujuan withdraw

## 3.3 Store Members
- member/end-user milik toko
- `(store_id, real_username)` unik
- mapping ke `upstream_user_code` 12 karakter immutable

## 3.4 Game
- create user upstream
- deposit
- withdraw
- read balance
- launch game
- provider/game sync
- timeout reconcile

## 3.5 QRIS / Payments
- `store_topup`
- `member_payment`
- QR generation
- webhook inbound
- status reconcile
- callback outbound ke callback URL toko

## 3.6 Withdrawals
- inquiry rekening
- reserve balance toko
- transfer bank
- webhook/check-status
- fee platform + fee external

## 3.7 Ledger
- immutable ledger entries
- reserve / commit / release
- current balance snapshot

## 3.8 Audit
- audit semua role
- 90 hari retention

## 3.9 Realtime
- dashboard realtime
- notification stream
- chat global

## 3.10 Ops
- metrics
- alerting
- health checks
- scheduler
- worker
- backup/restore drill

---

# 4. Alur Inti Bisnis

## 4.1 Game — create user

Flow:

1. Website owner request create user ke project.
2. Project cari `(store_id, username)`.
3. Jika sudah ada => error duplicate.
4. Jika belum ada:
   - generate `upstream_user_code` 12 karakter alfanumerik, immutable, unik global
   - kirim `user_create` ke NexusGGR
   - jika success => simpan mapping member
   - return response ke website owner

Catatan:
- username asli member **tidak** dikirim ke upstream
- yang dikirim ke upstream adalah `upstream_user_code`

## 4.2 Game — deposit

Flow:

1. Website owner kirim request dengan:
   - Bearer token toko
   - `username`
   - `amount`
   - `trx_id`
2. Project validasi:
   - token valid
   - store aktif
   - member ada / auto-create jika kebijakan endpoint mengizinkan
   - `amount >= GAME_MIN_AMOUNT`
   - `(store_id, trx_id)` belum pernah dipakai
   - balance toko cukup
3. Project buat transaction row `pending`
4. Project call `user_deposit` ke NexusGGR
5. Jika success:
   - post ledger debit `game_deposit`
   - update tx `success`
6. Jika fail:
   - update tx `failed`
7. Jika timeout/ambigu:
   - update tx `pending_reconcile`
   - worker cek `transfer_status`

## 4.3 Game — withdraw

Flow:

1. Website owner kirim:
   - Bearer token toko
   - `username`
   - `amount`
   - `trx_id`
2. Project validasi:
   - token valid
   - store aktif
   - member ada / auto-create jika dibolehkan
   - `amount >= GAME_MIN_AMOUNT`
   - `(store_id, trx_id)` belum pernah dipakai
3. Project buat transaction row `pending`
4. Project call `user_withdraw`
5. Jika success:
   - post ledger credit `game_withdraw`
   - update tx `success`
6. Jika fail:
   - update tx `failed`
7. Jika timeout/ambigu:
   - `pending_reconcile`
   - worker cek `transfer_status`

## 4.4 Game — launch

Flow:

1. Website owner kirim:
   - Bearer token toko
   - `username`
   - `provider_code`
   - `game_code`
   - `lang` optional default `id`
2. Project validasi provider/game dari DB sync
3. Jika member belum ada => auto-create
4. Project call `game_launch`
5. Return URL/token launch dari upstream hampir tanpa perubahan

## 4.5 QRIS — store_topup

Flow:

1. Owner klik top up di dashboard
2. Project langsung create transaksi `store_topup` status `pending`
3. Project call generate QRIS external dengan:
   - `username = owner.username`
   - `amount`
   - `uuid = GLOBAL_QRIS_UUID`
   - `expire = 300`
   - `custom_ref = internal_topup_ref`
4. Provider mengembalikan:
   - QR string
   - `trx_id`
5. Project simpan `provider_trx_id`
6. Frontend render QR image dari QR string
7. Saat webhook inbound menyatakan `success`:
   - idempotent check by `provider_trx_id`
   - post ledger credit `store_topup`
8. Jika `expired` / `failed`:
   - finalkan tanpa ledger credit
9. Jika webhook hilang:
   - worker/scheduler reconcile via check-status

## 4.6 QRIS — member_payment

Flow:

1. Website owner request generate QRIS via Store API
2. Project validasi store + member
3. Project call generate QRIS external dengan:
   - `username = upstream_user_code`
   - `amount`
   - `uuid = GLOBAL_QRIS_UUID`
   - `expire = 300`
   - `custom_ref = internal_member_payment_ref`
4. Project simpan transaksi `member_payment` status `pending`
5. Saat webhook inbound `success`:
   - hitung `platform_fee = 3% × gross_amount`
   - `store_credit_amount = gross_amount - platform_fee`
   - post ledger credit `member_payment_credit`
   - post ledger debit/fee record `member_payment_fee`
   - kirim event realtime
   - enqueue outbound callback ke callback URL toko
6. Callback outbound ke toko owner:
   - signed HMAC header
   - retry 5x exponential backoff jika gagal

## 4.7 Withdraw balance toko

Flow:

1. Owner pilih rekening tujuan tersimpan
2. Owner input **net amount** yang ingin diterima
3. Project call **inquiry** external lebih dulu
4. Provider mengembalikan:
   - `account_name`
   - `fee` external
   - `inquiry_id`
5. Project hitung:
   - `platform_fee = 12% × net_requested`
   - `external_fee = inquiry.fee`
   - `total_store_debit = net_requested + platform_fee + external_fee`
6. Jika balance toko kurang dari `total_store_debit` => tolak sebelum transfer
7. Jika cukup:
   - create withdrawal row `pending`
   - create ledger reservation `withdraw_reserve`
   - call transfer
8. Final state datang dari:
   - webhook inbound global
   - dan/atau check-status setiap 30 detik
9. Jika `success`:
   - commit reservation menjadi debit final
10. Jika `failed`:
   - release reservation penuh kembali ke balance toko

---

# 5. Response Contract Internal

## 5.1 Success

```json
{
  "status": true,
  "message": "SUCCESS",
  "data": {}
}
```

## 5.2 Validation / business fail

```json
{
  "status": false,
  "message": "INSUFFICIENT_STORE_BALANCE",
  "data": null
}
```

## 5.3 Pending reconcile

```json
{
  "status": false,
  "message": "PENDING_RECONCILE",
  "data": {
    "trx_id": "...",
    "state": "pending"
  }
}
```

## 5.4 HTTP status semantics

- `200` success final
- `202` accepted / pending reconcile
- `400` malformed request
- `401` unauthorized
- `403` forbidden
- `404` not found
- `409` conflict / duplicate
- `422` validation / business rule fail
- `429` rate limited
- `502` upstream error
- `503` service unavailable / degraded mode

---

# 6. Environment Variables

Gunakan `.env` terstruktur per concern.

## 6.1 App
- `APP_NAME`
- `APP_ENV`
- `APP_URL`
- `APP_TIMEZONE`
- `APP_LOG_LEVEL`

## 6.2 Database
- `DATABASE_URL`
- `DATABASE_MAX_OPEN_CONNS`
- `DATABASE_MAX_IDLE_CONNS`
- `DATABASE_CONN_MAX_LIFETIME`

## 6.3 Redis
- `REDIS_URL`
- `REDIS_PASSWORD`
- `REDIS_DB`

## 6.4 Auth
- `JWT_ACCESS_SECRET`
- `JWT_ACCESS_TTL=1h`
- `SESSION_TTL=168h`
- `PASSWORD_BCRYPT_COST` atau alternatif modern sesuai implementasi

## 6.5 Business values
- `MIN_TRANSACTION_AMOUNT=5000`
- `STORE_LOW_BALANCE_THRESHOLD`
- `MEMBER_PAYMENT_PLATFORM_FEE_PERCENT=3`
- `STORE_WITHDRAW_PLATFORM_FEE_PERCENT=12`

## 6.6 QRIS provider
- `QRIS_BASE_URL`
- `QRIS_CLIENT`
- `QRIS_CLIENT_KEY`
- `QRIS_GLOBAL_UUID`
- `QRIS_DEFAULT_EXPIRE_SECONDS=300`
- `QRIS_WEBHOOK_SHARED_SECRET` jika nantinya ada dukungan signature eksternal

## 6.7 NexusGGR
- `NEXUSGGR_BASE_URL`
- `NEXUSGGR_AGENT_CODE`
- `NEXUSGGR_AGENT_TOKEN`

## 6.8 Realtime
- `WS_HEARTBEAT_SECONDS`

## 6.9 Observability
- `METRICS_ENABLED`
- `PROMETHEUS_PORT`

---

# 7. Struktur Repo

```txt
/apps
  /web
  /api
  /worker
  /scheduler

/internal
  /platform
    /config
    /db
    /redis
    /httpserver
    /middleware
    /security
    /observability
    /queue
    /realtime
    /clock
    /id
    /hash
    /signing
  /modules
    /auth
    /users
    /stores
    /storemembers
    /ledger
    /game
    /paymentsqris
    /withdrawals
    /callbacks
    /audit
    /chat
    /providercatalog
    /notifications
    /ops
/pkg
/migrations
/seeds
/scripts
/docs
/deploy
```

---

# 8. Planning A–Z

## Phase A — Inception & Ground Rules
- finalisasi scope V1
- finalisasi naming conventions
- finalisasi env contract
- finalisasi coding standards
- finalisasi API envelope

## Phase B — Repository Setup
- init monorepo structure
- setup Go workspace / module
- setup SvelteKit app
- setup linting, formatting, pre-commit hooks
- setup conventional commits

## Phase C — Developer Tooling
- buat CLI internal bergaya artisan
- command minimal:
  - `migrate up`
  - `migrate fresh`
  - `migrate fresh --seed`
  - `seed demo`
  - `worker run`
  - `scheduler run`
  - `sync providers`

## Phase D — Core Platform Layer
- config loader
- structured logger
- HTTP server bootstrap
- PostgreSQL bootstrap
- Redis bootstrap
- health checks
- request ID middleware

## Phase E — Security Foundation
- password hashing
- JWT access token
- session store Redis
- refresh/session rotation
- one-account-one-device enforcement
- login throttling
- store API rate limiting

## Phase F — Auth Module
- login by email/username
- logout
- forced logout all devices
- TOTP enable/verify/disable
- recovery codes
- IP allowlist validation

## Phase G — User & RBAC Module
- users CRUD internal/dev flows
- role enforcement
- authorization middleware
- owner creates employee user
- employee-store relation rules

## Phase H — Store Module
- create store
- soft delete store
- rotate store token
- callback URL update
- threshold config
- list/filter stores by role

## Phase I — Bank Account Module
- search bank by `bank_code` / `bank_name`
- inquiry verification before save
- store multiple bank accounts
- mark active / inactive
- masked account storage for UI
- encrypted account number at rest

## Phase J — Audit Module Base
- audit middleware
- audit event contract
- persistence
- query/filter endpoint
- role-based visibility

## Phase K — Ledger Engine
- ledger accounts
- immutable ledger entries
- reservation system
- current balance projection
- transaction wrapper helpers

## Phase L — Store Members Module
- create member
- `(store_id, real_username)` uniqueness
- generate `upstream_user_code`
- immutable upstream mapping

## Phase M — NexusGGR Adapter
- low-level client
- request normalizer
- response normalizer
- single endpoint dispatcher by `method`
- error mapping
- timeout mapping

## Phase N — Provider/Game Catalog Sync
- sync provider list
- sync game list
- schedule sync
- diff + upsert strategy
- cache warmup

## Phase O — Game Flows
- create user upstream
- deposit
- withdraw
- launch
- read balance cache 5 detik
- pending reconcile worker
- transfer status checker

## Phase P — QRIS Generate Flows
- generate `store_topup`
- generate `member_payment`
- QR string response mapping
- QR image rendering helper
- expiry handling

## Phase Q — QRIS Webhook Global
- single webhook endpoint
- transaction lookup by provider `trx_id`
- idempotent success handling
- expired/failed handling
- dedup lock

## Phase R — Callback Outbound Module
- callback payload builder
- HMAC signer
- retry engine 5x exponential backoff
- attempt log
- failure notifications

## Phase S — Withdrawal Engine
- inquiry first
- fee calculation
- reservation hold
- transfer execution
- webhook/check-status finalization
- release on failed

## Phase T — Reconcile & Recovery Jobs
- game pending reconcile
- qris pending reconcile
- withdrawal pending check-status every 30 detik
- callback retry queue
- dead-letter/reporting

## Phase U — Realtime Layer
- WebSocket auth
- room routing
- user/store/global channels
- event broadcaster
- dashboard counter updates

## Phase V — Dashboard Analytics
- owner cards
- karyawan cards
- dev cards
- pending counters
- monthly income calculations
- low balance alert emitter

## Phase W — Chat Global
- global room
- send/read stream
- dev moderation delete
- retention prune 7 hari

## Phase X — Frontend UX & Design System
- custom shadcn-svelte theme
- cyberpunk enterprise dashboard tokens
- loading bar SPA transitions
- table/filter/search patterns
- empty states
- clear error states
- optimistic UX hanya untuk state aman non-uang

## Phase Y — Testing & Hardening
- unit tests domain services
- repository tests
- integration tests adapters
- webhook duplicate tests
- timeout/reconcile tests
- ledger consistency tests
- role/tenant boundary tests
- k6 performance tests

## Phase Z — Deployment & Operability
- Docker Compose production stack
- reverse proxy
- TLS
- metrics
- logs
- alerting
- backup schedule
- restore drill
- runbook operasional

---

# 9. Realtime Blueprint

## 9.1 Transport
Gunakan **WebSocket** untuk:
- notification stream
- dashboard realtime
- chat global

## 9.2 Channel model
- `user:{userId}`
- `store:{storeId}`
- `role:dev`
- `role:superadmin`
- `global_chat`

## 9.3 Event minimal

### owner/karyawan
- `member_payment.success`
- `store_topup.success`
- `withdraw.success`
- `withdraw.failed`
- `callback.delivery_failed`
- `game.deposit.success`
- `game.withdraw.success`
- `store.low_balance`

### dev/superadmin
- semua event owner/karyawan sesuai scope
- plus platform-wide error notifications

---

# 10. Security Blueprint

## 10.1 Dashboard auth
- login email/username + password
- access token TTL 1 jam
- refresh/session TTL 7 hari
- session record di Redis
- one account one device
- login baru invalid session lama

## 10.2 2FA
- TOTP only
- self-enable per user
- recovery code wajib
- tidak mandatory, tapi strongly recommended via UX

## 10.3 IP allowlist
- hanya untuk login dashboard
- per user
- single IP only

## 10.4 Store token
- 1 toko = 1 token
- token disimpan hashed
- owner dan superadmin bisa melihat full token
- rotate token mematikan token lama langsung

## 10.5 Callback security
- outbound callback ke toko owner signed dengan HMAC
- inbound provider webhook diverifikasi lewat korelasi transaksi internal
- raw payload disimpan masked

---

# 11. Observability & Ops Blueprint

## 11.1 Metrics
- request count
- request latency
- upstream latency
- cache hit rate
- queue depth
- websocket connection count
- webhook success/failure rate
- callback success/failure rate
- reconcile backlog

## 11.2 Alerts
- webhook failure spike
- callback failure spike
- Redis down
- DB down
- NexusGGR error spike
- QRIS provider error spike

## 11.3 Health checks
- live
- ready
- DB connectivity
- Redis connectivity
- degraded upstream status

## 11.4 Retention jobs
- audit log prune 90 hari
- chat prune 7 hari
- stale sessions cleanup
- old callback attempts cleanup policy

---

# 12. Desain Database Final

Di bawah ini adalah desain database final yang direkomendasikan untuk PostgreSQL.

## 12.1 Konvensi umum

- semua primary key gunakan `uuid` atau `bigint` konsisten; rekomendasi: `uuid`
- semua tabel punya:
  - `created_at timestamptz not null default now()`
  - `updated_at timestamptz not null default now()` jika mutable
- untuk soft delete gunakan:
  - `deleted_at timestamptz null`
- nilai sensitif simpan encrypted atau hashed
- semua query tenant-aware harus punya index `store_id`

---

## 12.2 Enumerations (disarankan sebagai text + check atau PostgreSQL enum)

### user_role
- `dev`
- `superadmin`
- `owner`
- `karyawan`

### store_status
- `active`
- `inactive`
- `banned`
- `deleted`

### transaction_status_basic
- `pending`
- `success`
- `failed`

### qris_status
- `pending`
- `success`
- `expired`
- `failed`

### game_action
- `deposit`
- `withdraw`

### qris_transaction_type
- `store_topup`
- `member_payment`

### callback_status
- `pending`
- `success`
- `failed`
- `retrying`

### reservation_status
- `pending`
- `committed`
- `released`

### ledger_direction
- `debit`
- `credit`

### ledger_entry_type
- `game_deposit`
- `game_withdraw`
- `store_topup`
- `member_payment_credit`
- `member_payment_fee`
- `withdraw_reserve`
- `withdraw_commit`
- `withdraw_release`
- `withdraw_platform_fee`
- `withdraw_external_fee`

---

## 12.3 Tables

### A. `users`

Menyimpan akun dashboard.

| column | type | notes |
|---|---|---|
| id | uuid pk | |
| email | citext unique | unique global |
| username | citext unique | unique global, owner username immutable |
| password_hash | text | |
| role | text | `dev/superadmin/owner/karyawan` |
| is_active | boolean | default true |
| totp_enabled | boolean | default false |
| totp_secret_encrypted | text null | encrypted |
| ip_allowlist | inet null | single IP only |
| created_by_user_id | uuid null fk users(id) | owner membuat karyawan |
| last_login_at | timestamptz null | |
| created_at | timestamptz | |
| updated_at | timestamptz | |

Index:
- unique(email)
- unique(username)
- index(role)
- index(created_by_user_id)

### B. `user_recovery_codes`

| column | type | notes |
|---|---|---|
| id | uuid pk | |
| user_id | uuid fk users(id) | |
| code_hash | text | jangan simpan plain |
| used_at | timestamptz null | |
| created_at | timestamptz | |

Index:
- index(user_id)

### C. `user_sessions`

| column | type | notes |
|---|---|---|
| id | uuid pk | |
| user_id | uuid fk users(id) | |
| session_jti | text unique | |
| refresh_hash | text | hashed |
| ip_address | inet | |
| user_agent | text | |
| expires_at | timestamptz | |
| revoked_at | timestamptz null | |
| created_at | timestamptz | |
| updated_at | timestamptz | |

Index:
- unique(session_jti)
- index(user_id)
- index(expires_at)

### D. `stores`

| column | type | notes |
|---|---|---|
| id | uuid pk | |
| owner_user_id | uuid fk users(id) | role owner |
| name | text | |
| slug | citext unique | |
| status | text | active/inactive/banned/deleted |
| api_token_hash | text | 1 token per store |
| callback_url | text | full visible to owner/superadmin/dev |
| current_balance | numeric(20,2) | projection/cache, source of truth tetap ledger |
| low_balance_threshold | numeric(20,2) | bisa override env bila suatu saat dibutuhkan; jika tidak, nullable |
| created_at | timestamptz | |
| updated_at | timestamptz | |
| deleted_at | timestamptz null | soft delete |

Index:
- unique(slug)
- index(owner_user_id)
- index(status)
- partial index(deleted_at is null)

### E. `store_staff`

Pivot karyawan ke toko.

| column | type | notes |
|---|---|---|
| id | uuid pk | |
| store_id | uuid fk stores(id) | |
| user_id | uuid fk users(id) | karyawan |
| created_by_owner_id | uuid fk users(id) | owner pembuat |
| created_at | timestamptz | |

Constraint:
- unique(store_id, user_id)

Index:
- index(user_id)
- index(store_id)

### F. `store_bank_accounts`

| column | type | notes |
|---|---|---|
| id | uuid pk | |
| store_id | uuid fk stores(id) | |
| bank_code | text | harus valid terhadap Bank RTOL |
| bank_name | text | snapshot dari hasil inquiry / mapping json |
| account_number_encrypted | text | encrypted |
| account_number_masked | text | untuk UI |
| account_name | text | hasil inquiry |
| verified_at | timestamptz | |
| is_active | boolean | default true |
| created_at | timestamptz | |
| updated_at | timestamptz | |

Index:
- index(store_id)
- index(bank_code)
- partial index(store_id, is_active)

### G. `store_members`

| column | type | notes |
|---|---|---|
| id | uuid pk | |
| store_id | uuid fk stores(id) | |
| real_username | citext | unik per toko |
| upstream_user_code | text unique | 12-char immutable unique global |
| status | text | active/inactive |
| created_at | timestamptz | |
| updated_at | timestamptz | |

Constraint:
- unique(store_id, real_username)
- unique(upstream_user_code)

Index:
- index(store_id)
- index(real_username)

### H. `provider_catalogs`

| column | type | notes |
|---|---|---|
| id | uuid pk | |
| provider_code | text unique | |
| provider_name | text | |
| status | integer | open/maintenance snapshot |
| synced_at | timestamptz | |
| created_at | timestamptz | |
| updated_at | timestamptz | |

### I. `provider_games`

| column | type | notes |
|---|---|---|
| id | uuid pk | |
| provider_code | text | fk logical to provider_catalogs.provider_code |
| game_code | text | |
| game_name | jsonb or text | untuk v2 bisa simpan localized jsonb |
| banner_url | text null | |
| status | integer | |
| synced_at | timestamptz | |
| created_at | timestamptz | |
| updated_at | timestamptz | |

Constraint:
- unique(provider_code, game_code)

Index:
- index(provider_code)
- index(status)

### J. `ledger_accounts`

Sederhanakan ke 1 account utama per store untuk balance operasional.

| column | type | notes |
|---|---|---|
| id | uuid pk | |
| store_id | uuid fk stores(id) unique | 1 account operasional per store |
| currency | text | default IDR |
| created_at | timestamptz | |

Constraint:
- unique(store_id)

### K. `ledger_entries`

| column | type | notes |
|---|---|---|
| id | uuid pk | |
| store_id | uuid fk stores(id) | |
| ledger_account_id | uuid fk ledger_accounts(id) | |
| direction | text | debit/credit |
| entry_type | text | lihat enum |
| amount | numeric(20,2) | > 0 |
| balance_after | numeric(20,2) | snapshot hasil posting |
| reference_type | text | game_transaction/qris_transaction/store_withdrawal dll |
| reference_id | uuid | |
| metadata_json | jsonb | |
| created_at | timestamptz | |

Index:
- index(store_id, created_at desc)
- index(reference_type, reference_id)
- index(entry_type)

### L. `ledger_reservations`

| column | type | notes |
|---|---|---|
| id | uuid pk | |
| store_id | uuid fk stores(id) | |
| reference_type | text | store_withdrawal dll |
| reference_id | uuid | |
| amount | numeric(20,2) | |
| status | text | pending/committed/released |
| created_at | timestamptz | |
| updated_at | timestamptz | |

Constraint:
- unique(reference_type, reference_id)

Index:
- index(store_id)
- index(status)

### M. `game_transactions`

| column | type | notes |
|---|---|---|
| id | uuid pk | |
| store_id | uuid fk stores(id) | |
| store_member_id | uuid fk store_members(id) | |
| action | text | deposit/withdraw |
| trx_id | text | unique global per toko |
| upstream_user_code | text | snapshot |
| amount | numeric(20,2) | |
| agent_sign | text unique | generated internal |
| status | text | pending/success/failed |
| reconcile_status | text null | pending_reconcile/resolved |
| upstream_error_code | text null | |
| upstream_response_masked | jsonb | |
| created_at | timestamptz | |
| updated_at | timestamptz | |

Constraint:
- unique(store_id, trx_id)
- unique(agent_sign)

Index:
- index(store_id, created_at desc)
- index(store_member_id, created_at desc)
- index(status)
- index(reconcile_status)

### N. `game_launch_logs`

| column | type | notes |
|---|---|---|
| id | uuid pk | |
| store_id | uuid fk stores(id) | |
| store_member_id | uuid fk store_members(id) | |
| provider_code | text | |
| game_code | text | |
| lang | text | default id |
| status | text | success/failed |
| upstream_payload_masked | jsonb | |
| created_at | timestamptz | |

Index:
- index(store_id, created_at desc)
- index(store_member_id, created_at desc)

### O. `qris_transactions`

Gabungkan `store_topup` dan `member_payment` dalam satu tabel dengan discriminator `type`.

| column | type | notes |
|---|---|---|
| id | uuid pk | |
| store_id | uuid fk stores(id) | |
| store_member_id | uuid null fk store_members(id) | null untuk store_topup |
| type | text | store_topup/member_payment |
| provider_trx_id | text unique | kunci unik utama provider |
| custom_ref | text | internal unique ref |
| external_username | text | owner username / upstream_user_code |
| amount_gross | numeric(20,2) | nominal original |
| platform_fee_amount | numeric(20,2) | 0 untuk store_topup, 3% untuk member_payment |
| store_credit_amount | numeric(20,2) | nominal yang masuk ke balance toko |
| status | text | pending/success/expired/failed |
| expires_at | timestamptz | |
| provider_payload_masked | jsonb | |
| created_at | timestamptz | |
| updated_at | timestamptz | |

Constraint:
- unique(provider_trx_id)
- unique(type, custom_ref)

Index:
- index(store_id, created_at desc)
- index(store_member_id, created_at desc)
- index(status)
- index(type)

### P. `qris_reconcile_attempts`

| column | type | notes |
|---|---|---|
| id | uuid pk | |
| qris_transaction_id | uuid fk qris_transactions(id) | |
| attempt_no | integer | |
| status | text | |
| response_masked | jsonb | |
| created_at | timestamptz | |

Index:
- index(qris_transaction_id, attempt_no)

### Q. `outbound_callbacks`

| column | type | notes |
|---|---|---|
| id | uuid pk | |
| store_id | uuid fk stores(id) | |
| event_type | text | member_payment.success dll |
| reference_type | text | qris_transaction |
| reference_id | uuid | |
| payload_json | jsonb | body yang akan dikirim |
| signature | text | HMAC signature snapshot |
| status | text | pending/success/failed/retrying |
| created_at | timestamptz | |
| updated_at | timestamptz | |

Index:
- index(store_id, created_at desc)
- index(status)
- index(reference_type, reference_id)

### R. `outbound_callback_attempts`

| column | type | notes |
|---|---|---|
| id | uuid pk | |
| outbound_callback_id | uuid fk outbound_callbacks(id) | |
| attempt_no | integer | |
| http_status | integer null | |
| status | text | success/failed |
| response_body_masked | text | |
| next_retry_at | timestamptz null | |
| created_at | timestamptz | |

Index:
- index(outbound_callback_id, attempt_no)
- index(next_retry_at)

### S. `store_withdrawals`

| column | type | notes |
|---|---|---|
| id | uuid pk | |
| store_id | uuid fk stores(id) | |
| store_bank_account_id | uuid fk store_bank_accounts(id) | |
| net_requested_amount | numeric(20,2) | owner input |
| platform_fee_amount | numeric(20,2) | 12% × net |
| external_fee_amount | numeric(20,2) | hasil inquiry |
| total_store_debit | numeric(20,2) | net + platform_fee + external_fee |
| provider_partner_ref_no | text unique | identifier transfer |
| provider_inquiry_id | text | |
| status | text | pending/success/failed |
| request_payload_masked | jsonb | |
| provider_payload_masked | jsonb | |
| created_at | timestamptz | |
| updated_at | timestamptz | |

Index:
- index(store_id, created_at desc)
- index(status)
- unique(provider_partner_ref_no)

### T. `withdrawal_status_checks`

| column | type | notes |
|---|---|---|
| id | uuid pk | |
| store_withdrawal_id | uuid fk store_withdrawals(id) | |
| attempt_no | integer | |
| status | text | |
| response_masked | jsonb | |
| created_at | timestamptz | |

Index:
- index(store_withdrawal_id, attempt_no)

### U. `audit_logs`

| column | type | notes |
|---|---|---|
| id | uuid pk | |
| actor_user_id | uuid null fk users(id) | null untuk system |
| actor_role | text | snapshot |
| store_id | uuid null fk stores(id) | null jika global |
| action | text | |
| target_type | text | |
| target_id | uuid null | |
| payload_masked | jsonb | |
| ip_address | inet null | |
| user_agent | text null | |
| created_at | timestamptz | |

Index:
- index(store_id, created_at desc)
- index(actor_user_id, created_at desc)
- index(action)

### V. `notifications`

| column | type | notes |
|---|---|---|
| id | uuid pk | |
| scope_type | text | user/store/role/global |
| scope_id | text | string agar fleksibel |
| event_type | text | |
| title | text | |
| body | text | |
| read_at | timestamptz null | |
| created_at | timestamptz | |

Index:
- index(scope_type, scope_id, created_at desc)
- index(read_at)

### W. `chat_messages`

| column | type | notes |
|---|---|---|
| id | uuid pk | |
| sender_user_id | uuid fk users(id) | |
| body | text | no attachment |
| deleted_by_dev_user_id | uuid null fk users(id) | hanya dev bisa moderasi |
| deleted_at | timestamptz null | |
| created_at | timestamptz | |

Index:
- index(created_at desc)
- index(sender_user_id)

### X. `system_jobs`

Opsional untuk tracking job penting.

| column | type | notes |
|---|---|---|
| id | uuid pk | |
| job_type | text | |
| reference_type | text null | |
| reference_id | uuid null | |
| status | text | queued/running/success/failed |
| payload | jsonb | |
| error_message | text null | |
| run_at | timestamptz | |
| created_at | timestamptz | |
| updated_at | timestamptz | |

Index:
- index(job_type, status)
- index(run_at)

---

# 13. Relasi Kunci yang Harus Dijaga

## 13.1 Users ↔ Stores
- `stores.owner_user_id -> users.id`
- owner bisa punya banyak stores

## 13.2 Stores ↔ Staff
- many-to-many via `store_staff`
- karyawan bisa banyak store, tapi tetap dalam boundary owner pembuat

## 13.3 Stores ↔ Members
- satu store punya banyak member
- member username hanya unik **dalam store itu**

## 13.4 Stores ↔ Ledger
- satu store punya satu ledger account utama
- satu store punya banyak ledger entries

## 13.5 Stores ↔ QRIS / Game / Withdraw
- semua transaksi uang harus punya `store_id`
- ini wajib untuk tenant isolation dan reporting

---

# 14. Index Strategy

Index yang wajib diperhatikan dari awal:

1. `(store_id, created_at desc)` pada tabel transaksi besar
2. `(store_id, trx_id)` pada `game_transactions`
3. `provider_trx_id` pada `qris_transactions`
4. `provider_partner_ref_no` pada `store_withdrawals`
5. `(store_id, real_username)` pada `store_members`
6. `upstream_user_code` unique global
7. `status` indexes untuk job/reconcile tables
8. `scope_type, scope_id, created_at desc` untuk notifications

Untuk volume tinggi, pertimbangkan di tahap berikutnya:
- partition by month pada `ledger_entries`, `audit_logs`, `game_transactions`, `qris_transactions`

---

# 15. Data Sensitivity Rules

## 15.1 Wajib hashed
- `stores.api_token_hash`
- `user_sessions.refresh_hash`
- `user_recovery_codes.code_hash`

## 15.2 Wajib encrypted
- `users.totp_secret_encrypted`
- `store_bank_accounts.account_number_encrypted`

## 15.3 Wajib masked in logs
- callback payload sensitif
- upstream raw payload
- nomor rekening penuh
- token sensitif

---

# 16. Query / Reporting Views yang Direkomendasikan

Buat materialized view atau SQL view untuk kebutuhan dashboard agar query tidak liar.

## 16.1 `store_balance_view`
- store_id
- current_balance
- reserved_amount
- available_balance

## 16.2 `owner_monthly_income_view`
- store_id
- month
- total_member_payment_gross
- total_member_payment_fee
- total_store_credit

## 16.3 `dev_platform_income_view`
- month
- total_member_payment_fee
- total_withdraw_platform_fee
- total_qris_volume
- total_game_volume

## 16.4 `callback_failure_view`
- store_id
- failed_count
- latest_failed_at

---

# 17. Testing Checklist

## 17.1 Ledger
- game deposit success
- game deposit fail
- game deposit timeout reconcile success
- game withdraw success
- member_payment success with 3% fee
- store_topup success
- withdrawal success reserve commit
- withdrawal failed reserve release

## 17.2 Idempotency
- duplicate webhook
- duplicate `trx_id`
- retry callback
- transfer status reconcile

## 17.3 Role boundary
- owner tidak bisa lihat store lain
- karyawan hanya store relasinya
- karyawan tidak bisa lihat withdraw/history sensitif
- only dev can moderate chat

## 17.4 Security
- one-session-only enforcement
- IP allowlist
- TOTP verification
- store token rotate invalidates old token
- HMAC callback verification

---

# 18. Definition of Done

Project dianggap layak produksi bila minimal:

1. Semua flow uang utama lolos integration test.
2. Ledger balance konsisten pada seluruh skenario sukses/gagal/timeout.
3. Role boundary tervalidasi.
4. Webhook duplicate aman.
5. Callback outbound retry aman.
6. Reconcile worker terbukti menyelesaikan transaksi ambigu.
7. Metrics dan alerts aktif.
8. Backup dan restore drill sudah diuji.
9. k6 baseline test sudah dijalankan.
10. Dokumentasi operasional dan API docs internal sudah tersedia.

---

# 19. Rekomendasi Langkah Berikutnya

Urutan kerja paling tepat setelah dokumen ini:

1. buat **ERD final** dari desain database ini
2. tulis **SQL migration awal**
3. definisikan **contract endpoint internal**
4. implement **ledger engine terlebih dahulu**
5. implement **game flow**
6. implement **QRIS flow**
7. implement **withdraw store balance**
8. implement **realtime + dashboard + chat**
9. implement **ops & observability**

