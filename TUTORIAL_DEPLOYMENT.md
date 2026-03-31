# Tutorial Deployment

Dokumen ini adalah panduan deploy produksi final untuk repo ini, dengan fokus supaya pemasangan domain berjalan mulus dari nol sampai aplikasi bisa diakses lewat HTTPS.

Dokumen ini mengikuti aset produksi yang sudah ada di:

- [`deploy/production/docker-compose.yml`](/home/mugiew/project/onixggr/deploy/production/docker-compose.yml)
- [`deploy/production/env.production.example`](/home/mugiew/project/onixggr/deploy/production/env.production.example)
- [`deploy/production/deploy.sh`](/home/mugiew/project/onixggr/deploy/production/deploy.sh)
- [`deploy/production/smoke-test.sh`](/home/mugiew/project/onixggr/deploy/production/smoke-test.sh)
- [`deploy/production/backup-db.sh`](/home/mugiew/project/onixggr/deploy/production/backup-db.sh)
- [`deploy/production/restore-db.sh`](/home/mugiew/project/onixggr/deploy/production/restore-db.sh)
- [`deploy/production/Caddyfile`](/home/mugiew/project/onixggr/deploy/production/Caddyfile)

## 1. Arsitektur Deploy

Stack produksi terdiri dari:

- `postgres`: database utama
- `redis`: session, cache, limiter, realtime backbone
- `api`: Go API
- `worker`: reconcile dan callback retry
- `scheduler`: retention, provider sync, low-balance sweep, stale session cleanup
- `web`: SvelteKit dashboard
- `proxy`: Caddy reverse proxy + TLS otomatis

Alur publik:

1. domain publik mengarah ke server
2. traffic `80/443` masuk ke `Caddy`
3. `Caddy` meneruskan `/v1/*`, `/health/*`, `/readyz`, `/healthz` ke `api`
4. route selain itu diteruskan ke `web`

## 2. Kebutuhan Server

Siapkan 1 host Linux dengan:

- akses `sudo`
- domain publik, misalnya `app.domainanda.com`
- port `80` dan `443` terbuka dari internet
- `git`
- `curl`
- `jq`
- `podman`
- `podman compose`

Direktori contoh deploy:

```bash
sudo mkdir -p /srv/onixggr
sudo chown -R "$USER":"$USER" /srv/onixggr
cd /srv/onixggr
git clone git@github.com:muginurul24/gobang.git .
git checkout main
```

## 3. Pasang Domain

Pilih satu host final, misalnya:

- `PRODUCTION_DOMAIN=app.domainanda.com`
- `APP_URL=https://app.domainanda.com`

DNS minimal:

- buat `A record` untuk `app.domainanda.com` ke IP publik server
- jika server punya IPv6 publik, tambahkan `AAAA record`

Verifikasi dari laptop Anda:

```bash
dig +short app.domainanda.com
```

Hasilnya harus IP server produksi.

Catatan penting:

- jangan pakai `localhost`, `127.0.0.1`, atau domain internal untuk `APP_URL`
- untuk issuance TLS pertama, jalur paling aman adalah DNS langsung ke server tanpa proxy CDN di depan
- Caddy butuh port `80` dan `443` benar-benar reachable dari internet

## 4. Buka Firewall

Minimal buka:

- `22/tcp` untuk SSH
- `80/tcp` untuk HTTP ACME challenge
- `443/tcp` untuk HTTPS publik

Jangan expose publik:

- `5432`
- `6379`
- `9090`

`/metrics` sengaja tidak diproxy publik. Prometheus harus scrape `api:9090` dari network internal.

## 5. Buat File Env Produksi

Salin template:

```bash
cp deploy/production/env.production.example deploy/production/env.production
chmod 600 deploy/production/env.production
```

Lalu isi nilai final.

### Nilai yang wajib benar untuk domain

```env
PRODUCTION_DOMAIN=app.domainanda.com
APP_URL=https://app.domainanda.com
TLS_EMAIL=ops@domainanda.com
PRODUCTION_HTTP_PORT=80
PRODUCTION_HTTPS_PORT=443
APP_ENV=production
```

Rule:

- `PRODUCTION_DOMAIN` harus sama dengan host publik
- `APP_URL` harus full URL HTTPS dari host yang sama
- `TLS_EMAIL` harus email aktif untuk notifikasi ACME
- biarkan port `80/443` default kecuali benar-benar tahu alasan menggantinya

### Nilai secret yang wajib dirotasi

Minimal ganti semua ini:

- `POSTGRES_PASSWORD`
- `REDIS_PASSWORD`
- `JWT_ACCESS_SECRET`
- `AUTH_ENCRYPTION_KEY`
- `CALLBACK_SIGNING_SECRET`
- `QRIS_CLIENT_KEY`
- `QRIS_WEBHOOK_SHARED_SECRET`
- `NEXUSGGR_AGENT_TOKEN`

Contoh generate secret:

```bash
openssl rand -hex 32
```

### Nilai provider yang wajib benar

Isi dari environment provider asli:

- `QRIS_BASE_URL`
- `QRIS_CLIENT`
- `QRIS_CLIENT_KEY`
- `QRIS_GLOBAL_UUID`
- `QRIS_WEBHOOK_SHARED_SECRET`
- `NEXUSGGR_BASE_URL`
- `NEXUSGGR_AGENT_CODE`
- `NEXUSGGR_AGENT_TOKEN`

Jangan pakai nilai demo, mock, `example`, atau sandbox jika targetnya production.

### Nilai smoke test

Isi juga variabel ini setelah Anda punya akun owner dan store yang valid:

- `SMOKE_OWNER_LOGIN`
- `SMOKE_OWNER_PASSWORD`
- `SMOKE_STORE_ID`
- `SMOKE_STORE_TOKEN`
- `SMOKE_STORE_MEMBER`

Tanpa ini, `smoke-test.sh` tidak bisa memverifikasi login dashboard dan store API.

## 6. Cek Env Sebelum Deploy

Jalankan:

```bash
./scripts/check-env-sync.sh
```

Script ini memastikan key runtime tetap sinkron dengan `.env.example`.

Lalu cek file env produksi Anda secara manual:

- tidak ada `change-me-*`
- tidak ada `example.com`
- tidak ada `provider.example`
- tidak ada `localhost`
- tidak ada `127.0.0.1`

## 7. Deploy Pertama

Jalankan dari root repo:

```bash
./deploy/production/deploy.sh
```

Script ini akan:

1. membaca `deploy/production/env.production`
2. menolak placeholder atau URL tidak valid
3. menyalakan `postgres` dan `redis`
4. menunggu database dan Redis sehat
5. menjalankan backup pre-migration jika stack lama sudah ada
6. menjalankan migration
7. menyalakan `api`, `worker`, `scheduler`, `web`, dan `proxy`
8. menunggu proxy sehat

Kalau sukses, output akhir akan menampilkan origin dan health URL.

## 8. Verifikasi Setelah Deploy

### Cek service container

```bash
podman compose --env-file deploy/production/env.production -f deploy/production/docker-compose.yml ps
```

### Cek health publik

```bash
curl -k https://app.domainanda.com/health/live
curl -k https://app.domainanda.com/health/ready
```

### Jalankan smoke test

```bash
./deploy/production/smoke-test.sh
```

Script ini akan mengecek:

- `/health/live`
- `/health/ready`
- `/`
- login owner
- list stores
- list catalog providers
- list members
- satu panggilan store API balance

### Backup dan restore drill

```bash
./deploy/production/backup-db.sh
./deploy/production/restore-db.sh deploy/production/backups/<file.dump>
```

Ini wajib dilakukan minimal sekali sebelum trafik dibuka penuh.

## 9. Hubungkan Provider ke Domain Produksi

Endpoint inbound QRIS/disbursement app ini adalah:

```text
https://app.domainanda.com/v1/webhooks/qris
```

Saat mengisi dashboard provider:

- pakai domain final, bukan IP
- pakai HTTPS
- kalau provider mendukung IP allowlist, tambahkan IP publik server
- kalau provider perlu secret webhook, samakan dengan `QRIS_WEBHOOK_SHARED_SECRET`

Catatan:

- `callback_url` store milik merchant berbeda dengan webhook provider
- webhook provider masuk ke endpoint global aplikasi
- callback merchant keluar dari aplikasi menuju URL toko masing-masing

## 10. Jika Sertifikat TLS Tidak Terbit

Periksa urutan ini:

1. `dig +short app.domainanda.com` sudah mengarah ke IP server
2. port `80` dan `443` benar-benar terbuka
3. tidak ada reverse proxy lain yang sudah memakai port `80/443`
4. `PRODUCTION_DOMAIN` dan `APP_URL` benar-benar sama host-nya
5. `TLS_EMAIL` valid

Cek log Caddy:

```bash
podman compose --env-file deploy/production/env.production -f deploy/production/docker-compose.yml logs -f proxy
```

## 11. Jika Domain Sudah Hidup Tapi App Error

Cek API:

```bash
podman compose --env-file deploy/production/env.production -f deploy/production/docker-compose.yml logs -f api
```

Cek web:

```bash
podman compose --env-file deploy/production/env.production -f deploy/production/docker-compose.yml logs -f web
```

Cek worker:

```bash
podman compose --env-file deploy/production/env.production -f deploy/production/docker-compose.yml logs -f worker
```

Cek scheduler:

```bash
podman compose --env-file deploy/production/env.production -f deploy/production/docker-compose.yml logs -f scheduler
```

Masalah umum:

- `health/ready` `degraded`: biasanya credential provider belum lengkap
- `deploy.sh` berhenti saat preflight: masih ada placeholder atau URL lokal
- login gagal: secret auth salah atau seed/smoke credential belum benar
- websocket tidak jalan: Redis belum sehat atau proxy belum stabil

## 12. Monitoring Produksi

Prometheus tidak scrape domain publik. Gunakan network internal Compose.

Referensi:

- [`deploy/production/prometheus-scrape.example.yml`](/home/mugiew/project/onixggr/deploy/production/prometheus-scrape.example.yml)
- [`deploy/monitoring/alerts.rules.yml`](/home/mugiew/project/onixggr/deploy/monitoring/alerts.rules.yml)
- [`deploy/monitoring/basic-dashboard.md`](/home/mugiew/project/onixggr/deploy/monitoring/basic-dashboard.md)

Target scrape:

```text
api:9090
```

## 13. Backup Rutin

Contoh cron sudah ada di:

- [`deploy/production/backup-cron.example`](/home/mugiew/project/onixggr/deploy/production/backup-cron.example)

Minimal:

- backup DB harian
- restore drill mingguan

## 14. Update Rilis Berikutnya

Untuk update code:

```bash
git fetch origin
git checkout main
git pull --ff-only origin main
./deploy/production/deploy.sh
./deploy/production/smoke-test.sh
```

Sebelum tiap deploy:

- catat SHA saat ini
- jalankan backup
- pastikan `git status --short` bersih

## 15. Rollback

Referensi resmi:

- [`deploy/production/rollback-plan.md`](/home/mugiew/project/onixggr/deploy/production/rollback-plan.md)

Rollback standar:

1. checkout SHA terakhir yang sehat
2. jalankan `./deploy/production/deploy.sh`
3. jalankan `./deploy/production/smoke-test.sh`

Rollback data:

1. lakukan restore drill ke DB sementara dulu
2. validasi count dan row penting
3. baru restore ke DB live saat maintenance window

## 16. Checklist Singkat Sebelum Domain Dibuka

- DNS domain sudah mengarah ke server
- port `80` dan `443` terbuka
- `env.production` sudah final dan bukan placeholder
- `deploy.sh` sukses
- `smoke-test.sh` sukses
- `backup-db.sh` sukses
- `restore-db.sh` sukses
- webhook provider diarahkan ke `https://<domain>/v1/webhooks/qris`
- Prometheus scrape internal `api:9090`

Kalau semua poin ini sudah hijau, domain sudah siap dipakai untuk traffic produksi.
