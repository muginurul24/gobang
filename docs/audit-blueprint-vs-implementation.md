# Blueprint vs Implementation Audit

## Summary

- Audit basis: `docs/blueprint.md`, `docs/database-final.md`, `docs/plan-execution.md`, dan implementasi saat ini di `main`.
- Audit order mengikuti prioritas production review: money flow, RBAC, idempotency/reconcile, persistence, worker/scheduler, frontend parity, ops, lalu docs drift.
- Total checks: `28`
- `PASS`: `23`
- `PARTIAL`: `1`
- `FAIL`: `0`
- `NOT_IMPLEMENTED`: `3`
- `INTENTIONAL_DEVIATION`: `1`

Status rubric:

- `PASS`
- `PARTIAL`
- `FAIL`
- `NOT_IMPLEMENTED`
- `INTENTIONAL_DEVIATION`

Severity rubric:

- `Critical`: salah saldo, double post, data sensitif bocor, auth bypass
- `High`: reconcile/realtime/permission penting tidak sesuai kontrak
- `Medium`: retention, visibility operasional, atau parity contract belum lengkap
- `Low`: docs drift, optimization, atau deviance yang masih aman

## Domain: Ledger & Money

| Check | Blueprint rule | Current implementation | Status | Severity | Action |
|---|---|---|---|---|---|
| Store balance source of truth | Semua mutasi saldo toko harus lewat ledger | Pencarian `current_balance` write menunjukkan update projection hanya ada di `internal/modules/ledger/repository.go`; modul lain membaca saja | PASS | Critical | Pertahankan invariant ini |
| Game deposit flow | Deposit sukses harus reserve lalu commit debit `game_deposit`; ambigu harus `pending_reconcile` | `internal/modules/game/service.go` reserve dulu, commit `game_deposit` saat success, lalu worker reconcile di `internal/modules/game/reconcile.go` | PASS | High | Tidak ada aksi |
| Game withdraw flow | Withdraw sukses harus credit `game_withdraw`; ambigu harus `pending_reconcile` | `internal/modules/game/service.go` post credit `game_withdraw` saat success; reconcile worker menutup path ambigu | PASS | High | Tidak ada aksi |
| QRIS `store_topup` | Webhook success harus credit ledger `store_topup` dan duplicate-safe | `internal/modules/paymentsqris/service.go` melakukan `ledger.Credit` sekali dengan guard `HasReferenceEntries` | PASS | High | Tidak ada aksi |
| QRIS `member_payment` fee posting | `member_payment success` harus post `member_payment_credit` dan `member_payment_fee` | `internal/modules/paymentsqris/service.go` sekarang mem-post batch ledger atomik: credit gross `member_payment_credit` lalu debit `member_payment_fee`, sehingga saldo akhir tetap net dan jejak fee masuk ke ledger | PASS | High | Tidak ada aksi |
| Store withdraw flow | Inquiry -> hitung fee -> cek balance -> reserve -> transfer -> webhook/check-status finalization | `internal/modules/withdrawals/service.go` dan `internal/modules/withdrawals/reconcile.go` sudah mengikuti urutan ini, termasuk split commit `withdraw_commit`, `withdraw_platform_fee`, `withdraw_external_fee` | PASS | High | Tidak ada aksi |

## Domain: RBAC & Sensitive Visibility

| Check | Blueprint rule | Current implementation | Status | Severity | Action |
|---|---|---|---|---|---|
| Tenant scoping owner/karyawan | Owner hanya store sendiri; karyawan hanya store relation | Store, member, notification, dashboard, dan audit access memakai owner/store_staff scope; handler tidak bypass service | PASS | Critical | Pertahankan test boundary ini |
| Karyawan tidak boleh lihat data sensitif withdraw/bank/audit | Karyawan tidak boleh lihat callback URL penuh, token penuh, rekening withdraw, atau audit sensitif | `stores.sanitizeStore` mengosongkan `callback_url`; `api_token` disembunyikan untuk non-owner/superadmin; bank accounts dan withdrawals diblok untuk `karyawan`; audit handler menolak `karyawan` | PASS | High | Tidak ada aksi |
| Callback URL visibility | Full callback URL visible untuk owner/superadmin/dev; karyawan masked | `internal/modules/stores/service.go:631-638` hanya mask untuk `karyawan`; UI `apps/web/src/routes/(app)/app/stores/+page.svelte` mengikuti scope itu | PASS | Medium | Tidak ada aksi |
| Store token reveal model | Kontrak final: token disimpan hashed, plaintext hanya one-time reveal saat create/rotate | Implementasi stores dan UI sekarang sudah sesuai: token hanya muncul saat create/rotate, lalu hilang dari listing berikutnya | PASS | Medium | Tidak ada aksi |

## Domain: Idempotency & Reconcile

| Check | Blueprint rule | Current implementation | Status | Severity | Action |
|---|---|---|---|---|---|
| Game deposit duplicate strategy | Duplicate `trx_id` untuk deposit harus aman terhadap retry/duplicate | `internal/modules/game/service.go` menolak duplicate `trx_id` sebelum create row; sesuai checklist Hari 18 | PASS | High | Tidak ada aksi |
| Game withdraw duplicate strategy | Retry `trx_id` yang sama harus mengembalikan transaksi lama | `internal/modules/game/service.go:549-555` mengembalikan row existing untuk duplicate withdraw | PASS | High | Tidak ada aksi |
| QRIS duplicate webhook/check-status | Duplicate webhook dan reconcile tidak boleh double credit | `internal/modules/paymentsqris/service.go` pakai `HasReferenceEntries`; reconcile worker memanggil finalizer yang sama | PASS | Critical | Tidak ada aksi |
| Withdraw duplicate webhook/check-status | Duplicate webhook/status check tidak boleh double commit | `internal/modules/withdrawals/reconcile.go` cek `HasReferenceEntries`, reservation state, dan advisory lock sebelum final commit/release | PASS | Critical | Tidak ada aksi |
| Platform-wide realtime error notifications | Blueprint dev/superadmin: semua event owner/karyawan sesuai scope plus platform-wide error notifications | Emitter domain saat ini selalu store-scoped via `internal/modules/notifications/store_emitter.go:15-22`; tidak ada producer `ScopeRole` atau `ScopeGlobal` untuk error platform | NOT_IMPLEMENTED | High | Tambahkan producer role/global untuk error platform, atau ubah blueprint bila role-wide notification tidak jadi target |

## Domain: Persistence & Schema

| Check | Blueprint rule | Current implementation | Status | Severity | Action |
|---|---|---|---|---|---|
| Core constraints and indexes | Unique/FK/index utama harus ada dari awal | Migration saat ini sudah punya unique `(store_id, trx_id)`, `(store_id, real_username)`, `provider_trx_id`, `(store_id, idempotency_key)`, notifications scope index, dan ledger idempotency index | PASS | High | Tidak ada aksi |
| Monthly partitioning | Docs menyarankan partition by month untuk tabel volume tinggi di tahap berikutnya | Tidak ada partition di migrations saat ini; semua tabel masih plain heap | INTENTIONAL_DEVIATION | Low | Aman ditunda, tetapi tetap catat sebagai scaling backlog |
| Stale session cleanup | Blueprint ops meminta stale sessions cleanup | Tidak ada config, scheduler, atau worker job untuk prune `user_sessions`; expired session hanya ditangani via TTL/expiry path auth, bukan cleanup persistence | NOT_IMPLEMENTED | Medium | Tambahkan prune job atau dokumentasikan retention policy untuk `user_sessions` |
| Callback attempt cleanup policy | Blueprint ops meminta old callback attempts cleanup policy | Tidak ada retention config atau cleanup job untuk `outbound_callback_attempts` di worker/scheduler/config | NOT_IMPLEMENTED | Medium | Tambahkan retention policy + cleanup job agar tabel attempt tidak tumbuh tanpa batas |

## Domain: Worker & Scheduler

| Check | Blueprint rule | Current implementation | Status | Severity | Action |
|---|---|---|---|---|---|
| Background job coverage | QRIS reconcile, game reconcile, withdraw checker, callback retry, provider sync, audit prune, chat prune harus ada | `apps/worker/main.go` dan `apps/scheduler/main.go` sudah menjalankan seluruh job tersebut | PASS | High | Tidak ada aksi |
| Low balance alerts coverage | Low balance alerts harus operasional, bukan hanya best effort | Event `store.low_balance` hanya di-emit inline setelah `game.deposit.success` dan `withdraw.success`; tidak ada periodic catch-up scan untuk store yang sudah low balance dari awal atau missed event | PARTIAL | Medium | Tambahkan scheduler scan low balance, atau dokumentasikan bahwa alert hanya fire on downward balance mutations |

## Domain: Frontend Parity

| Check | Blueprint rule | Current implementation | Status | Severity | Action |
|---|---|---|---|---|---|
| Dashboard scope and masking | Cards owner/karyawan vs dev harus sesuai scope; masked fields tetap aman | Dashboard cards, stores, bank accounts, withdrawals, dan audit UI sudah memisahkan role scope dan masking sesuai backend | PASS | Medium | Tidak ada aksi |
| Dev/superadmin realtime parity | Dev/superadmin harus menerima event owner/karyawan sesuai scope untuk notification stream/dashboard realtime | `internal/modules/realtime/service.go` sekarang subscribe `dev` dan `superadmin` ke semua `store:{storeId}` aktif plus `role:*`, sehingga event store-scope masuk ke stream role platform juga | PASS | High | Tidak ada aksi |
| Notification surface in dashboard | Deliverable Hari 33 menyebut notification stream usable | Dashboard sekarang punya unread badge di shell dan feed `/app/notifications` yang memakai `GET /v1/notifications`, `GET /v1/notifications/unread-count`, `POST /v1/notifications/{id}/read`, serta refresh via WebSocket sesuai scope aktif | PASS | Low | Tidak ada aksi |

## Domain: Ops & Production

| Check | Blueprint rule | Current implementation | Status | Severity | Action |
|---|---|---|---|---|---|
| Production preflight and internal metrics | Secret placeholders harus dipaksa diganti; metrics internal only | `deploy/production/deploy.sh` memblok placeholder dan localhost/mock URLs; `deploy/production/go-live-checklist.md` mewajibkan scrape `api:9090` hanya di jaringan internal | PASS | Medium | Tidak ada aksi |
| Backup, restore, and smoke readiness | Backup/restore dan smoke test harus usable sebelum go-live | `deploy/production/backup-db.sh`, `restore-db.sh`, `smoke-test.sh`, dan runbook production sudah tersedia | PASS | Medium | Tidak ada aksi |

## Domain: Docs Drift

| Check | Blueprint rule | Current implementation | Status | Severity | Action |
|---|---|---|---|---|---|
| Hari 33 checklist accuracy | Checklist yang sudah dicentang harus benar-benar tercapai | Deliverable `notification stream usable` sekarang sudah terpenuhi oleh unread badge shell dan halaman `/app/notifications` yang memakai scope backend asli | PASS | Medium | Tidak ada aksi |
| Callback queue contract in docs | Database/blueprint harus memuat constraint penting yang dipakai produksi | `docs/database-final.md` sekarang mencatat unique `event_type + reference_type + reference_id`, sesuai migration `000010_outbound_callbacks.up.sql` | PASS | Low | Tidak ada aksi |

## Must Fix Before Production

- Tidak ada temuan `Must Fix Before Production` yang masih terbuka pada audit saat ini.

## Should Fix Soon After Launch

- Tambahkan stale session cleanup untuk `user_sessions`.
- Tambahkan retention cleanup untuk `outbound_callback_attempts`.
- Tambahkan periodic low-balance sweep agar store yang sudah low balance sejak awal tetap menghasilkan alert operasional.

## Intentional Deviations from Blueprint

- Partition bulanan untuk `ledger_entries`, `audit_logs`, `game_transactions`, dan `qris_transactions` belum diimplementasikan. Ini masih aman sebagai scaling backlog karena docs sendiri menaruhnya sebagai tahap berikutnya, bukan hard requirement tahap sekarang.

## Docs That Must Be Updated

- Tidak ada drift docs kontraktual ber-severity rendah yang masih terbuka pada audit saat ini.
