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

