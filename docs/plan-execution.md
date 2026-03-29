# plan-execution.md — Execution Plan A–Z (Step-by-Step Build Roadmap)

> Dokumen ini adalah **rencana perjalanan implementasi** dari nol sampai project siap staging dan production.
>
> Fokus dokumen ini:
> - urutan kerja yang jelas
> - target harian
> - hasil akhir per hari
> - checklist implementasi
> - anti-pattern yang harus dihindari
>
> Ini **bukan** sekadar blueprint arsitektur. Ini adalah **langkah bangun project**.

---

# 1. Target akhir project

Project akhir harus punya:

- landing page SEO
- login dashboard
- auth + session + TOTP 2FA + recovery code
- RBAC: dev / superadmin / owner / karyawan
- store management
- 1 toko = 1 token
- callback URL per toko
- bank account per toko
- member toko + upstream user mapping 12 char immutable
- game create / deposit / withdraw / launch / balance
- QRIS member payment
- QRIS store topup
- withdraw balance toko ke bank
- audit log
- realtime notifications
- realtime dashboard cards
- global chat
- worker + scheduler
- observability
- Docker Compose deploy

---

# 2. Struktur repo final yang harus dipakai dari awal

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
    /queue
    /realtime
    /observability
    /clock
    /id
    /crypto
    /errors

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
    /notifications
    /chat
    /providercatalog
    /ops

/migrations
/seeds
/scripts
/deploy
/docs
```

---

# 3. Aturan implementasi

1. PostgreSQL adalah **source of truth**
2. Redis hanya akselerator
3. Semua mutasi uang wajib lewat ledger
4. Timeout upstream tidak boleh langsung dianggap gagal final
5. Semua transaksi uang wajib idempotent
6. Semua data sensitif wajib masked saat log
7. Semua query owner/karyawan wajib tenant-aware
8. Jangan pindah ke fitur berikutnya sebelum domain sebelumnya stabil

---

# 4. Planning harian dari awal sampai akhir

---

## Hari 1 — Inisialisasi repo dan workspace

### Target
Menyiapkan fondasi workspace yang tidak berubah-ubah lagi.

### Kerjakan
- Buat repo
- Buat folder utama
- Buat `.editorconfig`, `.gitignore`, `.env.example`
- Inisialisasi:
  - SvelteKit di `apps/web`
  - Go API di `apps/api`
- Buat `README.md`
- Buat `Makefile` atau `Taskfile`

### Deliverable
- repo bersih
- frontend jalan
- backend jalan
- struktur folder final terkunci

### Checklist
- [ ] `apps/web` bisa start
- [ ] `apps/api` bisa start
- [ ] `.env.example` ada
- [ ] command dasar ada

---

## Hari 2 — Setup frontend dasar

### Target
Membuat shell UI final.

### Kerjakan
- Install Tailwind CSS v4
- Install shadcn-svelte
- Buat theme token
- Buat layout:
  - public
  - auth
  - app
- Buat loading bar navigation
- Buat halaman placeholder:
  - `/`
  - `/login`
  - `/app`
  - `/app/chat`

### Deliverable
- app shell frontend siap
- arah visual sudah terkunci

### Checklist
- [ ] Tailwind aktif
- [ ] shadcn-svelte aktif
- [ ] layout terpisah
- [ ] loading bar ada

---

## Hari 3 — Setup backend Go dasar

### Target
Menyiapkan runtime API yang benar dari awal.

### Kerjakan
- HTTP server
- graceful shutdown
- request ID middleware
- config loader
- logger
- route:
  - `/health/live`
  - `/health/ready`

### Deliverable
- API server siap jadi fondasi

### Checklist
- [ ] health endpoints
- [ ] structured logging
- [ ] config from env
- [ ] request id

---

## Hari 4 — Docker Compose, PostgreSQL, Redis, migration

### Target
Menyatukan dependency lokal.

### Kerjakan
- Setup Docker Compose:
  - postgres
  - redis
  - api
  - web
- Tambah migration tool
- Buat script:
  - migrate up
  - migrate down
  - migrate fresh
  - seed
- Buat CLI helper ala artisan:
  - `appctl migrate up`
  - `appctl migrate fresh`
  - `appctl seed demo`

### Deliverable
- environment dev lokal stabil

### Checklist
- [ ] postgres hidup
- [ ] redis hidup
- [ ] migration jalan
- [ ] seed jalan

---

## Hari 5 — Migration batch 1: akun, toko, audit

### Target
Membuat fondasi database utama.

### Kerjakan
Buat tabel:
- `users`
- `user_recovery_codes`
- `user_sessions`
- `stores`
- `store_staff`
- `store_bank_accounts`
- `audit_logs`

### Deliverable
- fondasi auth/store/audit siap

### Checklist
- [ ] unique email global
- [ ] unique username global
- [ ] FK benar
- [ ] soft delete store siap

---

## Hari 6 — Login dasar dashboard

### Target
Menyelesaikan login tanpa 2FA dulu.

### Kerjakan
- login email/username + password
- password hashing
- access token
- refresh/session record di Redis
- one account one device
- forced invalidate session lama saat login baru

### Deliverable
- login dasar jalan end-to-end

### Checklist
- [ ] login via email
- [ ] login via username
- [ ] session Redis jalan
- [ ] login baru kill session lama

---

## Hari 7 — 2FA TOTP + recovery code

### Target
Menutup keamanan auth.

### Kerjakan
- enrollment TOTP
- verify TOTP
- secret encrypted
- generate recovery code
- disable 2FA flow
- UX rekomendasi aktifkan 2FA

### Deliverable
- 2FA usable

### Checklist
- [ ] QR / secret enrollment
- [ ] verify code
- [ ] recovery code sekali pakai
- [ ] disable flow

---

## Hari 8 — IP allowlist + login rate limit

### Target
Menutup hardening login.

### Kerjakan
- allowlist single IP per user dashboard
- rate limit login:
  - per IP
  - per identifier
- audit login success/fail

### Deliverable
- login hardening baseline siap

### Checklist
- [ ] IP allowlist jalan
- [ ] rate limit jalan
- [ ] login audit jalan

---

## Hari 9 — Frontend auth, guards, session UX

### Target
Menutup UX auth di frontend.

### Kerjakan
- route guard
- protected layout
- session bootstrap
- logout
- expired session handling
- unauthorized states

### Deliverable
- dashboard auth usable

### Checklist
- [ ] guard jalan
- [ ] logout jalan
- [ ] expired state jelas

---

## Hari 10 — Store CRUD

### Target
Menyelesaikan manajemen toko.

### Kerjakan
- create store
- update store
- soft delete store
- owner lihat tokonya sendiri
- dev/superadmin lihat semua
- audit create/update/delete

### Deliverable
- store domain dasar selesai

### Checklist
- [ ] scoped list benar
- [ ] soft delete benar
- [ ] audit ada

---

## Hari 11 — Token toko + callback URL

### Target
Menyelesaikan integration settings toko.

### Kerjakan
- 1 toko = 1 token
- generate token
- rotate token -> token lama mati
- simpan hash token
- callback URL update
- full token visible untuk owner/superadmin
- full callback URL visible untuk owner/superadmin
- audit token & callback change

### Deliverable
- store integration ready

### Checklist
- [ ] token hash
- [ ] rotate works
- [ ] callback validated
- [ ] visibility sesuai role

---

## Hari 12 — Akun karyawan dan relasi toko

### Target
Menyelesaikan model tim owner.

### Kerjakan
- owner create karyawan
- pivot many-to-many ke toko
- pastikan satu karyawan hanya dalam scope owner yang sama
- assign/unassign toko
- audit relation change

### Deliverable
- model owner → karyawan → toko selesai

### Checklist
- [ ] many-to-many jalan
- [ ] cross-owner relation ditolak
- [ ] audit assign ada

---

## Hari 13 — Ledger engine

### Target
Membuat engine saldo toko final.

### Kerjakan
Buat tabel:
- `ledger_accounts`
- `ledger_entries`
- `ledger_reservations`

Buat service:
- credit
- debit
- reserve
- commit reserve
- release reserve
- current balance read

### Deliverable
- semua mutasi uang nanti sudah punya fondasi benar

### Checklist
- [ ] no negative balance
- [ ] reserve/commit/release
- [ ] balance_after benar

---

## Hari 14 — Dashboard balance + audit viewer

### Target
Menampilkan data toko yang benar.

### Kerjakan
- card balance toko
- histori ledger dasar
- audit log viewer:
  - owner scoped
  - superadmin/dev global
  - karyawan tidak boleh lihat audit

### Deliverable
- dashboard mulai bernilai

### Checklist
- [ ] owner scoped audit
- [ ] superadmin/dev full audit
- [ ] karyawan blocked

---

## Hari 15 — Store members + upstream mapping

### Target
Menyelesaikan member toko.

### Kerjakan
- tabel `store_members`
- `(store_id, real_username)` unique
- generate `upstream_user_code` 12 char immutable
- create member
- duplicate handling

### Deliverable
- member domain siap dihubungkan ke game

### Checklist
- [ ] username unique per store
- [ ] upstream user code unique global
- [ ] immutable

---

## Hari 16 — Wrapper NexusGGR

### Target
Membungkus API game upstream dengan benar.

### Kerjakan
Buat wrapper:
- `user_create`
- `user_deposit`
- `user_withdraw`
- `user_withdraw_reset`
- `money_info`
- `provider_list`
- `game_list`
- `game_launch`
- `transfer_status`

Tambahkan:
- normalize `msg` / `error`
- normalize business failure pada HTTP 200
- timeout strategy
- masked logging

### Deliverable
- adapter NexusGGR final

### Checklist
- [ ] semua method siap
- [ ] error normalized
- [ ] timeout handled

---

## Hari 17 — Game create user

### Target
Menyelesaikan create user game flow.

### Kerjakan
- endpoint create user
- kalau duplicate -> error
- kalau sukses -> simpan mapping
- audit/event basic

### Deliverable
- create user usable

### Checklist
- [ ] duplicate reject
- [ ] upstream mapping saved

---

## Hari 18 — Game deposit

### Target
Menyelesaikan deposit game.

### Kerjakan
- endpoint store API deposit:
  - token
  - username
  - amount
  - trx_id
- validasi:
  - amount >= minimum env
  - balance toko cukup
  - trx_id unique global per toko
- generate `agent_sign` internal
- call upstream deposit
- success -> ledger debit
- fail -> no ledger change
- timeout -> pending reconcile

### Deliverable
- game deposit end-to-end selesai

### Checklist
- [ ] insufficient balance reject
- [ ] duplicate trx_id reject
- [ ] success debit ledger
- [ ] timeout pending

---

## Hari 19 — Game withdraw

### Target
Menyelesaikan withdraw game.

### Kerjakan
- endpoint store API withdraw:
  - token
  - username
  - amount
  - trx_id
- generate `agent_sign` internal
- success -> ledger credit
- fail -> no ledger change
- timeout -> pending reconcile

### Deliverable
- game withdraw selesai

### Checklist
- [ ] success credit ledger
- [ ] pending reconcile jalan
- [ ] retry same trx_id returns old result

---

## Hari 20 — Game balance + launch

### Target
Menyelesaikan read flow game.

### Kerjakan
- endpoint get balance
- cache Redis 5 detik
- request coalescing
- endpoint launch
- auto create jika belum ada
- validasi provider/game harus ada di DB sync
- `lang` optional default `id`

### Deliverable
- game integration usable untuk owner website

### Checklist
- [ ] balance cache 5 detik
- [ ] launch no idempotency
- [ ] launch without deposit allowed
- [ ] auto create on launch

---

## Hari 21 — Provider sync dan game catalog

### Target
Menyediakan catalog lokal untuk validasi.

### Kerjakan
- sync provider list
- sync game list
- simpan ke DB
- scheduler sync berkala
- dashboard browse/search

### Deliverable
- provider/game source lokal siap

### Checklist
- [ ] provider sync
- [ ] game sync
- [ ] search/filter ada

---

## Hari 22 — Reconcile worker game

### Target
Menutup timeout ambigu untuk transaksi game.

### Kerjakan
- scan transaksi pending
- call `transfer_status`
- finalisasi success/fail
- idempotent finalize
- emit notification

### Deliverable
- game timeout aman

### Checklist
- [ ] reconcile scan
- [ ] finalize safe
- [ ] duplicate finalize blocked

---

## Hari 23 — Wrapper QRIS / VA provider

### Target
Membungkus API QRIS/VA secara rapi.

### Kerjakan
- wrapper:
  - generate
  - check-status
  - inquiry
  - transfer
- webhook parser
- global UUID config
- default expire 300 detik
- masked payload logging

### Deliverable
- adapter QRIS/VA siap

### Checklist
- [ ] generate wrapper
- [ ] check-status wrapper
- [ ] inquiry wrapper
- [ ] transfer wrapper

---

## Hari 24 — Store topup QRIS

### Target
Menyelesaikan topup balance toko via dashboard.

### Kerjakan
- owner input nominal
- create pending topup
- generate QRIS:
  - username = owner username
  - custom_ref = internal topup ref
  - uuid = global uuid
- simpan provider trx_id
- render QR image di UI
- histori pending/success/failed/expired

### Deliverable
- owner bisa topup store balance

### Checklist
- [ ] multiple pending allowed
- [ ] qris image tampil
- [ ] list status benar

---

## Hari 25 — Member payment QRIS

### Target
Menyelesaikan QRIS untuk member payment.

### Kerjakan
- store API generate member payment
- username external = unique internal member
- custom_ref internal
- store pending transaction
- return QR data

### Deliverable
- member payment QRIS siap dipakai

### Checklist
- [ ] dipisah dari store_topup
- [ ] fee belum diterapkan sampai success

---

## Hari 26 — Webhook inbound global QRIS

### Target
Menyelesaikan webhook global untuk semua transaksi QRIS/withdraw status.

### Kerjakan
- endpoint webhook global
- dispatcher:
  - store_topup
  - member_payment
  - withdrawal status
- duplicate-safe
- finalisasi status
- ledger:
  - store_topup success -> credit full
  - member_payment success -> credit after 3% fee

### Deliverable
- inbound webhook final

### Checklist
- [ ] one endpoint global
- [ ] duplicate safe
- [ ] idempotent ledger posting

---

## Hari 27 — Callback outbound ke website owner

### Target
Menyelesaikan callback ke callback_url toko.

### Kerjakan
- payload event member_payment
- HMAC signature
- callback attempt log
- retry 5x exponential backoff
- failure notification

### Deliverable
- callback outbound reliable

### Checklist
- [ ] HMAC active
- [ ] retry worker active
- [ ] failed attempt logged

---

## Hari 28 — Reconcile worker QRIS

### Target
Menutup kasus webhook QRIS tidak datang.

### Kerjakan
- scan pending transactions
- check-status dengan backoff:
  - 30 detik
  - 60 detik
  - 120 detik
  - lalu 5 menit
- finalisasi status final

### Deliverable
- QRIS reconcile aman

### Checklist
- [ ] check-status worker
- [ ] no double credit
- [ ] expire final

---

## Hari 29 — Verifikasi rekening tujuan withdraw

### Target
Menyelesaikan setup bank account per store.

### Kerjakan
- load bank list dari JSON
- searchable bank select
- input account number
- inquiry nominal minimum
- dapatkan `account_name`
- jika valid -> simpan rekening
- simpan:
  - full encrypted account number
  - masked account number
- history account options per store

### Deliverable
- rekening tujuan withdraw valid

### Checklist
- [ ] bank_code valid
- [ ] inquiry verification works
- [ ] account saved
- [ ] visibility role correct

---

## Hari 30 — Withdraw balance toko

### Target
Menyelesaikan request withdraw ke bank.

### Kerjakan
- owner pilih rekening
- owner input **net amount**
- inquiry dulu
- hitung:
  - platform fee = 12% x net
  - external fee
  - total_store_debit = net + platform fee + external fee
- jika balance kurang -> tolak
- jika cukup:
  - create pending
  - reserve total_store_debit
  - transfer

### Deliverable
- withdraw request end-to-end selesai

### Checklist
- [ ] fee formula benar
- [ ] reserve sebelum transfer
- [ ] balance cukup dicek setelah inquiry

---

## Hari 31 — Withdraw webhook + status checker

### Target
Menutup finalisasi withdraw.

### Kerjakan
- webhook inbound global tangani withdraw
- check-status tiap 30 detik
- success -> commit reserve
- failed -> release reserve penuh
- check-status success walau webhook tidak datang -> tetap final success

### Deliverable
- withdraw domain final

### Checklist
- [ ] pending success failed works
- [ ] reserve released on fail
- [ ] final success idempotent

---

## Hari 32 — WebSocket realtime foundation

### Target
Menyiapkan transport realtime tunggal.

### Kerjakan
- websocket endpoint
- auth websocket
- channel:
  - user
  - store
  - role
  - global_chat
- Redis pub/sub fanout

### Deliverable
- realtime backbone siap

### Checklist
- [ ] websocket auth
- [ ] pub/sub works
- [ ] reconnect works

---

## Hari 33 — Notification stream realtime

### Target
Menyelesaikan stream event realtime.

### Kerjakan
- event model
- emit untuk:
  - member_payment success
  - store_topup success
  - withdraw success
  - withdraw failed
  - callback delivery failed
  - game deposit success
  - game withdraw success
  - low balance
- store in DB
- push via websocket

### Deliverable
- notification stream usable

### Checklist
- [ ] owner scoped
- [ ] karyawan scoped
- [ ] dev/superadmin global

---

## Hari 34 — Realtime dashboard cards

### Target
Menyelesaikan angka dashboard.

### Kerjakan
Owner/karyawan:
- balance toko
- pending QRIS
- transaksi success hari ini
- transaksi expired hari ini
- pendapatan toko bulan ini

Dev:
- income platform hari ini/bulan ini
- total semua toko
- total pending withdraw
- upstream error rate
- callback failure rate

### Deliverable
- dashboard cards realtime final

### Checklist
- [ ] owner/karyawan cards benar
- [ ] dev cards benar
- [ ] dev-only metrics hidden dari role lain

---

## Hari 35 — Chat global

### Target
Menyelesaikan chat global.

### Kerjakan
- one global room
- send/receive realtime
- retention 7 hari
- no DM
- no edit
- no delete user biasa
- dev moderation delete

### Deliverable
- global chat usable

### Checklist
- [ ] one room only
- [ ] retention cleanup
- [ ] dev moderation works

---

## Hari 36 — Audit log final

### Target
Menutup audit coverage penuh.

### Kerjakan
Pastikan event berikut masuk audit:
- login success/fail
- create/update/delete store
- token create/rotate/revoke
- callback_url change
- withdraw request/result
- topup success/fail
- manual actions dev/superadmin

Tambahkan:
- audit filter UI
- retention 90 hari

### Deliverable
- audit log siap operasional

### Checklist
- [ ] audit coverage penuh
- [ ] owner scoped viewer
- [ ] superadmin/dev full viewer

---

## Hari 37 — Observability dan metrics

### Target
Menyiapkan visibilitas sistem.

### Kerjakan
- request count
- latency
- upstream latency
- cache hit rate
- queue depth
- websocket connection count
- health endpoints final
- metrics export

### Deliverable
- observability dasar siap

### Checklist
- [ ] metrics exported
- [ ] dashboards basic
- [ ] provider latency visible

---

## Hari 38 — Alerts

### Target
Membuat sistem tidak buta saat error.

### Kerjakan
Buat alert:
- webhook failure spike
- callback failure spike
- Redis down
- DB down
- NexusGGR error spike
- QRIS provider error spike

### Deliverable
- alerting baseline siap

### Checklist
- [ ] alert rules dibuat
- [ ] test alert minimal

---

## Hari 39 — UX hardening

### Target
Merapikan dashboard dari sisi UX.

### Kerjakan
- better loading states
- better error messages
- form validation UX
- empty states
- table filters
- low-balance alert UX
- store switch UX

### Deliverable
- UI terasa matang

### Checklist
- [ ] loading jelas
- [ ] error jelas
- [ ] empty state jelas

---

## Hari 40 — Security hardening

### Target
Melakukan hardening akhir.

### Kerjakan
- CSRF untuk browser mutation
- cookie hardening
- secret review
- rate limit review
- masking review
- permission audit review
- sensitive field access review

### Deliverable
- baseline security final

### Checklist
- [ ] CSRF aktif
- [ ] secret aman
- [ ] masking aman
- [ ] permissions audited

---

## Hari 41 — Test suite kritikal

### Target
Menutup flow uang dengan test.

### Kerjakan
Test:
- ledger reserve/commit/release
- game deposit success/fail/pending
- game withdraw success/fail/pending
- QRIS duplicate webhook
- member payment fee 3%
- withdraw formula 12% + external fee
- token rotate
- one-session-only auth

### Deliverable
- safety net domain uang siap

### Checklist
- [ ] all money flows tested
- [ ] timeout tested
- [ ] duplicate tested

---

## Hari 42 — Seed demo dan staging bootstrap

### Target
Membuat project gampang didemokan dan dites.

### Kerjakan
- seed dev
- seed superadmin
- seed owner
- seed karyawan
- seed toko
- seed rekening
- seed members
- seed catalog provider/game

### Deliverable
- staging/demo mudah dinyalakan

### Checklist
- [ ] seed usable
- [ ] demo accounts ready

---

## Hari 43 — Performance test k6

### Target
Mengukur bottleneck awal.

### Kerjakan
- login load test
- game deposit load test
- game withdraw load test
- game balance read load test
- QRIS generate test
- webhook burst test
- websocket concurrency test

### Deliverable
- bottleneck awal ketahuan

### Checklist
- [ ] p95 noted
- [ ] p99 noted
- [ ] bottleneck documented

---

## Hari 44 — Failure drill

### Target
Menguji sistem saat gangguan.

### Kerjakan
- Redis down
- DB slow
- NexusGGR timeout
- QRIS webhook missing
- callback owner failing
- worker down

### Deliverable
- perilaku sistem saat gagal sudah diketahui

### Checklist
- [ ] degraded mode reviewed
- [ ] no double ledger mutation
- [ ] alerts fire

---

## Hari 45 — Staging release

### Target
Membuat staging release candidate.

### Kerjakan
- Docker Compose final
- env staging
- reverse proxy
- TLS
- health checks
- backup DB
- restore test basic
- smoke test penuh

### Deliverable
- staging RC siap

### Checklist
- [ ] deploy success
- [ ] smoke test pass
- [ ] backup restore pass

---

## Hari 46 — Production checklist

### Target
Siap go-live.

### Kerjakan
- secret review
- domain review
- TLS review
- alerts review
- backup schedule
- retention jobs review
- rollback plan
- access review final

### Deliverable
- project siap production

### Checklist
- [ ] secrets rotated
- [ ] monitoring active
- [ ] rollback ready
- [ ] backup ready

---

# 5. Urutan prioritas kalau ingin lebih cepat

Kalau ingin build cepat tapi tetap aman, urutannya harus:

1. auth
2. stores + token
3. ledger
4. members
5. game flows
6. QRIS flows
7. withdraw bank
8. audit
9. realtime notif
10. dashboard cards
11. chat
12. observability
13. perf + hardening

---

# 6. Anti-pattern yang wajib dihindari

1. Jangan ubah balance toko langsung tanpa ledger
2. Jangan jadikan Redis source of truth saldo
3. Jangan anggap timeout upstream = gagal final
4. Jangan proses webhook tanpa idempotency
5. Jangan logging raw secret/token
6. Jangan gabungkan `store_topup` dan `member_payment`
7. Jangan beri owner replay callback manual jika policy bisnis melarang
8. Jangan optimistic-update angka uang di frontend

---

# 7. Definition of done untuk flow uang

Sebuah flow uang dianggap selesai hanya jika:
- [ ] business row tersimpan
- [ ] ledger mutation/reserve benar
- [ ] timeout strategy ada
- [ ] duplicate strategy ada
- [ ] audit ada
- [ ] notification ada jika relevan
- [ ] masked logging ada
- [ ] test ada

---

# 8. Penutup

Dokumen ini adalah **panduan build**, bukan sekadar dokumen ide.
Kalau Anda mengikuti urutannya, project akan dibangun:
- lebih teratur
- lebih aman
- lebih mudah di-debug
- dan tidak melompat-lompat antar domain.
