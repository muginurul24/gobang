# Tutorial Deployment Cloudflare Tunnel

Dokumen ini adalah panduan final untuk deploy repo ini dengan Cloudflare Tunnel, berdasarkan langkah yang benar-benar dipakai sampai server hidup untuk domain `app.bola788.store`.

Target akhir:

- user membuka `https://app.bola788.store`
- SSL publik ditangani oleh Cloudflare edge
- origin server tidak membuka `80/443` ke internet
- aplikasi tetap berjalan full stack: `postgres`, `redis`, `api`, `worker`, `scheduler`, `web`, `proxy`
- `cloudflared` meneruskan traffic publik ke origin private `http://127.0.0.1:18080`

## 1. Mode Yang Didukung

Ada dua mode deploy yang perlu dibedakan jelas:

| Mode | Tujuan | File env | Compose/script utama | Domain | Upstream |
|---|---|---|---|---|---|
| `staging` | verifikasi internal / RC | `deploy/staging/env.staging` | `deploy/staging/deploy.sh` | biasanya `staging.localhost` atau domain internal | boleh mock |
| `production` + Cloudflare Tunnel | trafik publik final | `deploy/production/env.production` | `deploy/cloudflare-tunnel/deploy.sh` | `app.bola788.store` | wajib real |

Rule penting:

- [`deploy/cloudflare-tunnel/deploy.sh`](/home/mugiew/project/onixggr/deploy/cloudflare-tunnel/deploy.sh) adalah helper untuk mode production via tunnel.
- untuk staging biasa, pakai [`deploy/staging/deploy.sh`](/home/mugiew/project/onixggr/deploy/staging/deploy.sh)
- jangan campur `env.staging` dengan `deploy/cloudflare-tunnel/deploy.sh`
- jangan pakai mock upstream di production tunnel

## 2. Arsitektur Yang Dipakai

Topologi yang dipakai di mode production Cloudflare Tunnel:

1. `cloudflared` berjalan di host server
2. `cloudflared` membuka koneksi outbound ke Cloudflare
3. public hostname `app.bola788.store` diarahkan ke `http://127.0.0.1:18080`
4. `proxy` Caddy hanya bind ke loopback host
5. Caddy internal meneruskan:
   - `/v1/*`, `/health/*`, `/readyz`, `/healthz` ke `api:8080`
   - sisanya ke `web:3000`

Alur request:

1. browser user ke `https://app.bola788.store`
2. TLS publik berhenti di Cloudflare edge
3. Cloudflare meneruskan ke `cloudflared`
4. `cloudflared` meneruskan ke `http://127.0.0.1:18080`
5. Caddy internal meneruskan request ke `api` atau `web`

Artinya:

- origin tidak perlu port publik `80/443`
- sertifikat publik tidak dikelola Caddy origin
- `TLS_EMAIL` tetap boleh ada di env, tapi pada mode tunnel tidak jadi jalur TLS utama

## 3. File Yang Dipakai Di Mode Production Tunnel

File yang harus dianggap sebagai satu paket:

- [`deploy/production/docker-compose.yml`](/home/mugiew/project/onixggr/deploy/production/docker-compose.yml)
- [`deploy/production/env.production.example`](/home/mugiew/project/onixggr/deploy/production/env.production.example)
- [`deploy/cloudflare-tunnel/docker-compose.override.yml`](/home/mugiew/project/onixggr/deploy/cloudflare-tunnel/docker-compose.override.yml)
- [`deploy/cloudflare-tunnel/Caddyfile`](/home/mugiew/project/onixggr/deploy/cloudflare-tunnel/Caddyfile)
- [`deploy/cloudflare-tunnel/deploy.sh`](/home/mugiew/project/onixggr/deploy/cloudflare-tunnel/deploy.sh)
- [`deploy/cloudflare-tunnel/down.sh`](/home/mugiew/project/onixggr/deploy/cloudflare-tunnel/down.sh)

Script `deploy.sh` sekarang mengikuti urutan ini:

1. load `deploy/production/env.production` ke shell dengan `set -a`
2. menyalin file itu ke `deploy/production/.env` sementara supaya `podman-compose 1.0.6` membaca env yang benar
3. validasi env penting dan menolak placeholder
4. start `postgres` dan `redis`
5. tunggu keduanya sehat
6. build image `manage`
7. create container `manage` satu kali
8. jalankan migration lewat `podman start -a ${COMPOSE_PROJECT_NAME}_manage_1`
9. start `api`, `worker`, `scheduler`, `web`, dan `proxy`
10. verifikasi `api`, `web`, `worker`, `scheduler`, dan `proxy` benar-benar running
11. verifikasi `http://127.0.0.1:18080/health/live`
12. verifikasi `http://127.0.0.1:18080/health/ready`
13. menghapus lagi `deploy/production/.env` sementara

Ini penting karena health API saja tidak cukup. Pada kasus nyata tadi, `worker` dan `scheduler` sempat mati walau `api` sudah hidup. Script sekarang menolak kondisi seperti itu.

## 4. Persiapan Server

Siapkan host Linux dengan:

- `git`
- `curl`
- `jq`
- `podman`
- `podman compose`
- `cloudflared`
- akses `sudo`

Port publik yang perlu dibuka:

- `22/tcp` untuk SSH

Port yang tidak perlu dibuka ke internet:

- `80`
- `443`
- `5432`
- `6379`
- `9090`
- `18080`
- `18443`

Contoh path repo yang benar-benar dipakai di server tadi:

```bash
cd /home/mugiew/projects/gobang
git pull origin main
```

Kalau Anda memakai path lain, sesuaikan semua command berikut.

## 5. Persiapan Cloudflare

Sebelum menyentuh server, siapkan dulu:

- domain `bola788.store` sudah aktif di akun Cloudflare
- Anda punya akses ke Cloudflare Zero Trust
- Anda akan membuat tunnel khusus, misalnya `onixggr-app-bola788-store`

Referensi resmi:

- <https://developers.cloudflare.com/tunnel/>
- <https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-tunnel/get-started/create-remote-tunnel/>
- <https://developers.cloudflare.com/tunnel/routing/>
- <https://developers.cloudflare.com/tunnel/advanced/local-management/as-a-service/linux/>

## 6. Siapkan Env Production Dengan Benar

Salin template:

```bash
cp deploy/production/env.production.example deploy/production/env.production
chmod 600 deploy/production/env.production
```

Isi minimal yang wajib benar:

```env
COMPOSE_PROJECT_NAME=onixggr-prod
PRODUCTION_DOMAIN=app.bola788.store
APP_URL=https://app.bola788.store
PRODUCTION_HTTP_PORT=127.0.0.1:18080
PRODUCTION_HTTPS_PORT=127.0.0.1:18443
APP_ENV=production
APP_NAME=onixggr
APP_LOG_LEVEL=info
POSTGRES_DB=onixggr
POSTGRES_USER=onixggr
POSTGRES_PASSWORD=<secret>
REDIS_PASSWORD=<secret>
JWT_ACCESS_SECRET=<secret>
AUTH_ENCRYPTION_KEY=<secret>
CALLBACK_SIGNING_SECRET=<secret>
QRIS_BASE_URL=<real-qris-base-url>
QRIS_CLIENT=<real-qris-client>
QRIS_CLIENT_KEY=<real-qris-client-key>
QRIS_GLOBAL_UUID=<real-qris-global-uuid>
QRIS_WEBHOOK_SHARED_SECRET=<real-qris-webhook-secret>
NEXUSGGR_BASE_URL=<real-nexus-base-url>
NEXUSGGR_AGENT_CODE=<real-agent-code>
NEXUSGGR_AGENT_TOKEN=<real-agent-token>
```

Rule yang wajib dipegang:

- `APP_ENV` harus `production`
- `APP_URL` harus HTTPS publik final
- `PRODUCTION_HTTP_PORT` harus loopback, misalnya `127.0.0.1:18080`
- `PRODUCTION_HTTPS_PORT` juga loopback, misalnya `127.0.0.1:18443`
- jangan sisakan dua definisi berbeda untuk key yang sama
  - contoh buruk: `PRODUCTION_HTTP_PORT=80` di atas lalu `PRODUCTION_HTTP_PORT=127.0.0.1:18080` di bawah
  - shell memang akan mengambil definisi terakhir, tetapi itu membuat debugging susah
- jangan sisakan placeholder seperti:
  - `change-me-*`
  - `*.example.com`
  - `*.provider.example`
  - `localhost`
  - `127.0.0.1` pada `APP_URL`

Verifikasi sinkronisasi nama env:

```bash
./scripts/check-env-sync.sh
```

## 7. Perbedaan Staging Dan Production Yang Harus Jelas

### Kalau target Anda `production` via tunnel

Gunakan:

- `deploy/production/env.production`
- `deploy/production/docker-compose.yml`
- `deploy/cloudflare-tunnel/docker-compose.override.yml`
- `deploy/cloudflare-tunnel/deploy.sh`

Dan pakai:

- domain publik final
- provider credential real
- no mock upstream

### Kalau target Anda `staging`

Gunakan:

- `deploy/staging/env.staging`
- `deploy/staging/docker-compose.yml`
- `deploy/staging/deploy.sh`

Staging repo ini didesain untuk:

- self-signed TLS internal
- domain internal / lokal
- mock upstream opsional
- demo seed opsional

Command staging standar:

```bash
./deploy/staging/deploy.sh
./deploy/staging/smoke-test.sh
```

### Kalau Anda ingin staging juga lewat Cloudflare Tunnel

Rekomendasi paling aman:

- tetap treat itu sebagai stack terpisah
- pakai subdomain berbeda, misalnya `staging.bola788.store`
- jangan campur database, secret, atau provider credential dengan production
- jangan pakai `deploy/cloudflare-tunnel/deploy.sh` untuk `env.staging`

Repo saat ini tidak mengirim helper `staging + tunnel` terpisah. Yang dikirim dan sudah dibuktikan hidup adalah `production + tunnel`.

## 8. Deploy Production Tunnel Dengan Helper

Ini jalur utama dan paling aman:

```bash
git pull origin main
./deploy/cloudflare-tunnel/deploy.sh
```

Kalau sukses, output akhirnya akan mencetak:

- `local_origin=http://127.0.0.1:18080`
- `health=http://127.0.0.1:18080/health/ready`

### Apa yang dikerjakan helper ini

Secara nyata, helper ini melakukan:

```bash
set -a
. deploy/production/env.production
set +a

cp deploy/production/env.production deploy/production/.env

cd deploy/production
podman compose -f docker-compose.yml -f ../cloudflare-tunnel/docker-compose.override.yml up -d --build postgres redis
podman compose -f docker-compose.yml -f ../cloudflare-tunnel/docker-compose.override.yml build manage
podman rm -f "${COMPOSE_PROJECT_NAME}_manage_1" || true
podman compose -f docker-compose.yml -f ../cloudflare-tunnel/docker-compose.override.yml create manage
podman start -a "${COMPOSE_PROJECT_NAME}_manage_1"
podman compose -f docker-compose.yml -f ../cloudflare-tunnel/docker-compose.override.yml up -d --build api worker scheduler web proxy

rm -f .env
```

Setelah itu helper memverifikasi:

- `api` running
- `web` running
- `worker` running
- `scheduler` running
- `proxy` running
- `http://127.0.0.1:18080/health/live`
- `http://127.0.0.1:18080/health/ready`

## 9. Verifikasi Setelah Deploy

Verifikasi langsung di host:

```bash
curl -fsS http://127.0.0.1:18080/health/live
curl -fsS http://127.0.0.1:18080/health/ready
podman ps --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}'
```

Output sehat minimal harus menunjukkan:

- `postgres` healthy
- `redis` healthy
- `api` healthy
- `web` healthy
- `proxy` up di `127.0.0.1:18080->80/tcp`
- `worker` up
- `scheduler` up

Kalau `health/live` atau `health/ready` belum hijau, jangan lanjut ke Cloudflare dulu.

## 10. Jalur Manual Kalau Anda Ingin Melihat Satu Per Satu

Helper di atas boleh dilewati kalau Anda ingin debug manual.

Yang penting: siapkan env Compose di direktori `deploy/production`, karena `podman-compose 1.0.6` pada host Ubuntu 24.04 tadi bisa membaca nilai yang salah jika hanya mengandalkan shell export.

```bash
cp deploy/production/env.production deploy/production/.env

set -a
. deploy/production/env.production
set +a

cd deploy/production
```

Lalu jalankan:

```bash
podman compose -f docker-compose.yml -f ../cloudflare-tunnel/docker-compose.override.yml up -d --build postgres redis
```

Tunggu infra sehat:

```bash
podman ps --format 'table {{.Names}}\t{{.Status}}'
```

Build migration image:

```bash
podman compose -f docker-compose.yml -f ../cloudflare-tunnel/docker-compose.override.yml build manage
```

Jalankan migration:

```bash
podman rm -f "${COMPOSE_PROJECT_NAME}_manage_1" || true
podman compose -f docker-compose.yml -f ../cloudflare-tunnel/docker-compose.override.yml create manage
podman start -a "${COMPOSE_PROJECT_NAME}_manage_1"
```

Naikkan app stack:

```bash
podman compose -f docker-compose.yml -f ../cloudflare-tunnel/docker-compose.override.yml up -d --build api worker scheduler web proxy
```

Verifikasi lagi:

```bash
curl -fsS http://127.0.0.1:18080/health/live
curl -fsS http://127.0.0.1:18080/health/ready
podman ps --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}'
```

Setelah selesai, bersihkan file env sementara:

```bash
rm -f .env
```

## 11. Recovery Kalau Stack Sempat Rusak

Kalau deploy sebelumnya setengah jadi, lakukan stop dulu:

```bash
./deploy/cloudflare-tunnel/down.sh
```

Catatan penting:

- jangan pakai `--remove-orphans` pada server yang memakai `podman-compose 1.0.6`
- bug itu nyata dan bisa membuat cleanup gagal
- `down.sh` sekarang akan mengabaikan flag itu kalau dikirim

Kalau masih ada container project yang nyangkut:

```bash
podman ps -a --format '{{.Names}}' | grep '^onixggr-prod_' | xargs -r podman rm -f
```

Command di atas hanya untuk recovery state project yang gagal. Jangan dipakai sembarangan pada host yang berbagi banyak stack lain.

## 12. Failure Yang Benar-Benar Terjadi Dan Sudah Ditutup

Berikut error nyata yang terjadi di server dan sekarang sudah diatasi di `main`:

### `manage migrate up` gagal karena config production

Root cause:

- `manage` belum menerima `APP_URL` dan `CALLBACK_SIGNING_SECRET`

Status sekarang:

- sudah fixed di [`deploy/production/docker-compose.yml`](/home/mugiew/project/onixggr/deploy/production/docker-compose.yml)

### `scheduler` mati dengan `JWT_ACCESS_SECRET cannot use placeholder value in production`

Root cause:

- `scheduler` belum menerima `APP_URL`, `JWT_ACCESS_SECRET`, `AUTH_ENCRYPTION_KEY`, dan `CALLBACK_SIGNING_SECRET`

Status sekarang:

- sudah fixed di compose produksi
- `deploy.sh` sekarang mengecek `scheduler` benar-benar running

### `worker` mati dengan error config production yang sama

Root cause:

- `worker` belum menerima `JWT_ACCESS_SECRET`

Status sekarang:

- sudah fixed di compose produksi
- `deploy.sh` sekarang mengecek `worker` benar-benar running

### `curl http://127.0.0.1:18080/health/live` mengembalikan 404 Svelte

Root cause:

- route API di Caddy internal belum di-handle secara eksplisit

Status sekarang:

- sudah fixed di [`deploy/cloudflare-tunnel/Caddyfile`](/home/mugiew/project/onixggr/deploy/cloudflare-tunnel/Caddyfile)

### `podman compose config --env-file ...` terlihat seperti env kosong

Catatan:

- pada `podman-compose 1.0.6`, output `config` kadang menyesatkan untuk interpolasi env
- pada host nyata, Compose juga bisa tetap mengambil nilai dari tempat yang salah kalau Anda hanya mengandalkan shell export dari repo root
- karena itu helper sekarang:
  - source `deploy/production/env.production`
  - membuat `deploy/production/.env` sementara
  - menjalankan compose dari direktori `deploy/production`

Jadi:

- pakai helper script
- atau pada jalur manual, buat `deploy/production/.env` sementara lalu jalankan compose dari direktori itu
- jangan terlalu percaya output `config` mentah kalau versi Podman Compose Anda tua

### `podman compose up manage` menggantung

Catatan:

- pada host nyata, `podman compose up manage` bisa tetap attach ke dependency yang long-running seperti `postgres` dan `redis`
- akibatnya command terlihat "hang" walaupun migration sebenarnya sudah selesai

Jadi jalur yang dipakai helper final adalah:

```bash
podman rm -f "${COMPOSE_PROJECT_NAME}_manage_1" || true
podman compose -f docker-compose.yml -f ../cloudflare-tunnel/docker-compose.override.yml create manage
podman start -a "${COMPOSE_PROJECT_NAME}_manage_1"
```

## 13. Install Cloudflare Tunnel

Contoh untuk Debian/Ubuntu:

```bash
curl -fsSL https://pkg.cloudflare.com/cloudflared-ascii.gpg | sudo gpg --dearmor -o /usr/share/keyrings/cloudflare-main.gpg
echo 'deb [signed-by=/usr/share/keyrings/cloudflare-main.gpg] https://pkg.cloudflare.com/cloudflared stable main' | sudo tee /etc/apt/sources.list.d/cloudflared.list
sudo apt update
sudo apt install -y cloudflared
```

## 14. Buat Tunnel Dan Public Hostname

Di Cloudflare Zero Trust:

1. buka `Networks > Connectors > Cloudflare Tunnels`
2. klik `Create a tunnel`
3. pilih `Cloudflared`
4. beri nama tunnel, misalnya `onixggr-app-bola788-store`
5. simpan tunnel

Lalu tambahkan public hostname:

- `Subdomain`: `app`
- `Domain`: `bola788.store`
- `Type`: `HTTP`
- `URL`: `127.0.0.1:18080`

Hasil final:

```text
app.bola788.store -> http://127.0.0.1:18080
```

## 15. Install cloudflared Service Di Host

Gunakan token tunnel dari dashboard:

```bash
sudo cloudflared service install <CLOUDFLARE_TUNNEL_TOKEN>
sudo systemctl enable cloudflared
sudo systemctl start cloudflared
sudo systemctl status cloudflared
```

Verifikasi log:

```bash
journalctl -u cloudflared -f
```

## 16. Verifikasi Domain Publik

Sesudah tunnel aktif:

```bash
curl -I https://app.bola788.store
curl https://app.bola788.store/health/live
curl https://app.bola788.store/health/ready
```

Yang diharapkan:

- HTTPS publik hidup
- `/health/live` sukses
- `/health/ready` sukses

## 17. Smoke Test Aplikasi

Isi dulu variabel smoke di `deploy/production/env.production`:

- `SMOKE_OWNER_LOGIN`
- `SMOKE_OWNER_PASSWORD`
- `SMOKE_STORE_ID`
- `SMOKE_STORE_TOKEN`
- `SMOKE_STORE_MEMBER`

Lalu jalankan:

```bash
SMOKE_BASE_URL=https://app.bola788.store ./deploy/production/smoke-test.sh
```

## 18. Endpoint Webhook Provider

Endpoint webhook inbound aplikasi:

```text
https://app.bola788.store/v1/webhooks/qris
```

Ini adalah endpoint global aplikasi, bukan `callback_url` store owner.

## 19. Monitoring Dan Log

Log stack aplikasi:

```bash
podman compose -f deploy/production/docker-compose.yml -f deploy/cloudflare-tunnel/docker-compose.override.yml logs -f api worker scheduler web proxy
```

Log tunnel:

```bash
journalctl -u cloudflared -f
```

Metrics internal tetap scrape ke:

```text
api:9090
```

## 20. Ringkasan Paling Singkat

Kalau Anda hanya butuh urutan yang benar:

1. `git pull origin main`
2. isi `deploy/production/env.production` dengan secret dan provider real
3. pastikan:
   - `APP_URL=https://app.bola788.store`
   - `PRODUCTION_HTTP_PORT=127.0.0.1:18080`
   - `PRODUCTION_HTTPS_PORT=127.0.0.1:18443`
4. jalankan `./scripts/check-env-sync.sh`
5. jalankan `./deploy/cloudflare-tunnel/deploy.sh`
6. pastikan `curl http://127.0.0.1:18080/health/live` sukses
7. pastikan `curl http://127.0.0.1:18080/health/ready` sukses
8. buat tunnel dan public hostname `app.bola788.store -> http://127.0.0.1:18080`
9. install `cloudflared` service
10. cek `https://app.bola788.store/health/live`
11. jalankan smoke test publik

Kalau langkah di atas hijau, origin dan tunnel Anda sudah sinkron dengan langkah yang memang terbukti berhasil di server.
