# Tutorial Deployment Cloudflare Tunnel

Dokumen ini adalah panduan deploy produksi repo ini jika akses publik memakai Cloudflare Tunnel untuk domain `app.bola788.store`.

Target akhirnya:

- user membuka `https://app.bola788.store`
- SSL publik ditangani Cloudflare edge
- origin tidak membuka port publik `80/443`
- aplikasi tetap memakai stack repo ini: `postgres`, `redis`, `api`, `worker`, `scheduler`, `web`, dan `proxy`
- `cloudflared` berjalan di host sebagai service dan meneruskan traffic ke reverse proxy lokal `127.0.0.1:18080`

## 1. Arsitektur yang dipakai

Untuk skenario Cloudflare Tunnel, best practice yang dipakai di repo ini adalah:

1. `cloudflared` berjalan di host server
2. `cloudflared` membuat koneksi outbound-only ke Cloudflare
3. public hostname `app.bola788.store` diarahkan ke `http://127.0.0.1:18080`
4. port `18080` hanya bind ke loopback host, bukan ke internet
5. `Caddy` di origin dipakai hanya sebagai reverse proxy internal, tanpa ACME publik

Alur request:

1. browser user ke `https://app.bola788.store`
2. TLS publik berhenti di Cloudflare
3. Cloudflare meneruskan traffic ke `cloudflared`
4. `cloudflared` meneruskan traffic ke `http://127.0.0.1:18080`
5. Caddy internal meneruskan `/v1/*` ke `api:8080` dan sisanya ke `web:3000`

Catatan:

- ini sengaja berbeda dari [`TUTORIAL_DEPLOYMENT.md`](/home/mugiew/project/onixggr/TUTORIAL_DEPLOYMENT.md), karena deploy biasa di sana mengandalkan Caddy publik + ACME langsung
- untuk tunnel, origin tetap private; Cloudflare edge yang menyediakan HTTPS publik

## 2. Referensi resmi Cloudflare

Saya menyusun panduan ini mengikuti dokumentasi resmi Cloudflare:

- Cloudflare Tunnel overview: <https://developers.cloudflare.com/tunnel/>
- Create a tunnel (dashboard): <https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-tunnel/get-started/create-remote-tunnel/>
- Tunnel tokens: <https://developers.cloudflare.com/tunnel/advanced/tunnel-tokens/>
- Tunnel routing/public hostname: <https://developers.cloudflare.com/tunnel/routing/>
- Run `cloudflared` as a service on Linux: <https://developers.cloudflare.com/tunnel/advanced/local-management/as-a-service/linux/>

Cloudflare saat ini merekomendasikan remotely-managed tunnel untuk kebanyakan use case. Panduan ini mengikuti rekomendasi itu.

## 3. Kebutuhan server

Siapkan 1 host Linux dengan:

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

Karena Cloudflare Tunnel bersifat outbound-only, origin tidak perlu membuka port web publik.

## 4. Siapkan repo di server

Contoh:

```bash
sudo mkdir -p /srv/onixggr
sudo chown -R "$USER":"$USER" /srv/onixggr
cd /srv/onixggr
git clone git@github.com:muginurul24/gobang.git .
git checkout main
```

## 5. Siapkan env produksi

Salin env produksi biasa:

```bash
cp deploy/production/env.production.example deploy/production/env.production
chmod 600 deploy/production/env.production
```

Isi minimal ini dengan nilai real:

```env
COMPOSE_PROJECT_NAME=onixggr-prod
PRODUCTION_DOMAIN=app.bola788.store
APP_URL=https://app.bola788.store
PRODUCTION_HTTP_PORT=127.0.0.1:18080
PRODUCTION_HTTPS_PORT=127.0.0.1:18443
APP_ENV=production
APP_LOG_LEVEL=info
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

Untuk mode tunnel ini:

- `PRODUCTION_DOMAIN` tetap isi `app.bola788.store`
- `APP_URL` tetap isi `https://app.bola788.store`
- `PRODUCTION_HTTP_PORT` wajib diubah ke `127.0.0.1:18080` agar origin hanya listen di loopback
- `PRODUCTION_HTTPS_PORT` set ke loopback juga, misalnya `127.0.0.1:18443`, supaya tidak ada bind `443` ke internet
- `TLS_EMAIL` boleh tetap diisi, tetapi tidak dipakai oleh Caddy internal-only override ini

Verifikasi sinkronisasi key:

```bash
./scripts/check-env-sync.sh
```

## 6. Jalankan stack aplikasi dalam mode tunnel

Untuk mode Cloudflare Tunnel, gunakan:

- base file: [`deploy/production/docker-compose.yml`](/home/mugiew/project/onixggr/deploy/production/docker-compose.yml)
- override file: [`deploy/cloudflare-tunnel/docker-compose.override.yml`](/home/mugiew/project/onixggr/deploy/cloudflare-tunnel/docker-compose.override.yml)
- internal proxy config: [`deploy/cloudflare-tunnel/Caddyfile`](/home/mugiew/project/onixggr/deploy/cloudflare-tunnel/Caddyfile)
- helper deploy: [`deploy/cloudflare-tunnel/deploy.sh`](/home/mugiew/project/onixggr/deploy/cloudflare-tunnel/deploy.sh)

Override ini melakukan dua hal:

- mengganti Caddy publik menjadi Caddy internal-only pada `:80`
- mempertahankan route `/v1/*` ke `api` dan sisanya ke `web`

Binding loopback host dikontrol oleh `deploy/production/env.production`:

- `PRODUCTION_HTTP_PORT=127.0.0.1:18080`
- `PRODUCTION_HTTPS_PORT=127.0.0.1:18443`

Dengan begitu, file compose dasar tetap bisa dipakai, tetapi origin tidak membuka `80/443` ke internet.

Cara yang direkomendasikan:

```bash
./deploy/cloudflare-tunnel/deploy.sh
```

Script ini akan:

- me-load `deploy/production/env.production` ke shell lebih dulu
- menghindari bug `podman-compose 1.0.6` yang sering mengabaikan interpolasi `--env-file`
- menyalakan `postgres` dan `redis`
- build image `manage`
- menjalankan `migrate up`
- menyalakan `api`, `worker`, `scheduler`, `web`, dan `proxy`
- menunggu `http://127.0.0.1:18080/health/live` benar-benar hijau

Kalau Anda tetap ingin jalan manual, wajib export env dulu:

```bash
set -a
. deploy/production/env.production
set +a
```

Baru setelah itu jalankan command Compose berikut.

Start infra manual:

```bash
podman compose \
  -f deploy/production/docker-compose.yml \
  -f deploy/cloudflare-tunnel/docker-compose.override.yml \
  up -d --build postgres redis
```

Jalankan migration:

```bash
podman compose \
  -f deploy/production/docker-compose.yml \
  -f deploy/cloudflare-tunnel/docker-compose.override.yml \
  build manage
```

Lalu jalankan migration:

```bash
podman compose \
  -f deploy/production/docker-compose.yml \
  -f deploy/cloudflare-tunnel/docker-compose.override.yml \
  run --rm -T manage migrate up
```

Naikkan app stack:

```bash
podman compose \
  -f deploy/production/docker-compose.yml \
  -f deploy/cloudflare-tunnel/docker-compose.override.yml \
  up -d --build api worker scheduler web proxy
```

Verifikasi local origin:

```bash
curl -fsS http://127.0.0.1:18080/health/live
curl -fsS http://127.0.0.1:18080/health/ready
curl -fsS http://127.0.0.1:18080/
```

Kalau ini belum hijau, jangan lanjut ke Cloudflare Tunnel dulu.

## 7. Buat Tunnel di Cloudflare Dashboard

Masuk ke Cloudflare Zero Trust:

1. buka `Networks > Connectors > Cloudflare Tunnels`
2. klik `Create a tunnel`
3. pilih `Cloudflared`
4. beri nama, misalnya `onixggr-app-bola788-store`
5. simpan tunnel

Setelah tunnel dibuat, Cloudflare akan menampilkan install command untuk connector baru.

Jangan langsung copy-paste keseluruhan command ke shell sebelum Anda pahami token-nya. Yang Anda perlukan adalah tunnel token.

Menurut dokumentasi Cloudflare, remotely-managed tunnel hanya butuh token untuk berjalan, dan siapa pun yang memegang token itu bisa menjalankan tunnel. Jadi simpan token ini seperti secret produksi.

## 8. Tambahkan Public Hostname

Masih di tunnel yang sama:

1. pilih `Add a public hostname`
2. isi:
   - `Subdomain`: `app`
   - `Domain`: `bola788.store`
   - `Type`: `HTTP`
   - `URL`: `127.0.0.1:18080`
3. simpan

Hasil akhirnya harus memetakan:

```text
app.bola788.store -> http://127.0.0.1:18080
```

Catatan penting:

- jika `app.bola788.store` sudah punya record `A`, `AAAA`, atau `CNAME`, hapus record lama dulu bila bertabrakan
- tunnel public hostname akan mengelola routing DNS untuk hostname tersebut

## 9. Install dan jalankan cloudflared di server

Contoh untuk Debian/Ubuntu, sesuaikan dengan OS Anda:

```bash
curl -fsSL https://pkg.cloudflare.com/cloudflared-ascii.gpg | sudo gpg --dearmor -o /usr/share/keyrings/cloudflare-main.gpg
echo 'deb [signed-by=/usr/share/keyrings/cloudflare-main.gpg] https://pkg.cloudflare.com/cloudflared stable main' | sudo tee /etc/apt/sources.list.d/cloudflared.list
sudo apt update
sudo apt install -y cloudflared
```

Setelah itu install service menggunakan token tunnel dari dashboard:

```bash
sudo cloudflared service install <CLOUDFLARE_TUNNEL_TOKEN>
sudo systemctl enable cloudflared
sudo systemctl start cloudflared
sudo systemctl status cloudflared
```

Kalau service sudah aktif, `cloudflared` akan menjaga koneksi outbound ke Cloudflare dan tidak memerlukan inbound port `80/443`.

## 10. Verifikasi domain publik

Dari laptop atau dari server:

```bash
curl -I https://app.bola788.store
curl https://app.bola788.store/health/live
curl https://app.bola788.store/health/ready
```

Yang diharapkan:

- domain resolve dan bisa diakses via HTTPS
- `/health/live` mengembalikan sukses
- `/health/ready` mengembalikan sukses atau `degraded` bila upstream provider belum lengkap

## 11. Jalankan smoke test aplikasi

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

Script ini akan memverifikasi:

- login dashboard
- list stores
- provider catalog
- members
- satu panggilan store API balance

## 12. Pasang webhook provider ke domain final

Setelah domain tunnel stabil, arahkan provider QRIS/disbursement ke:

```text
https://app.bola788.store/v1/webhooks/qris
```

Ini adalah endpoint global inbound webhook aplikasi.

Catatan:

- ini bukan `callback_url` merchant
- `callback_url` merchant tetap milik store masing-masing
- webhook provider selalu masuk ke endpoint global aplikasi

## 13. Monitoring dan logs

Lihat logs app stack:

```bash
podman compose \
  -f deploy/production/docker-compose.yml \
  -f deploy/cloudflare-tunnel/docker-compose.override.yml \
  logs -f api worker scheduler web proxy
```

Lihat logs tunnel:

```bash
journalctl -u cloudflared -f
```

Prometheus tetap scrape internal:

```text
api:9090
```

Referensi:

- [`deploy/production/prometheus-scrape.example.yml`](/home/mugiew/project/onixggr/deploy/production/prometheus-scrape.example.yml)
- [`deploy/monitoring/alerts.rules.yml`](/home/mugiew/project/onixggr/deploy/monitoring/alerts.rules.yml)

## 14. Backup dan restore

Walau akses publik memakai tunnel, backup-restore tetap pakai script produksi yang sama:

```bash
./deploy/production/backup-db.sh
./deploy/production/restore-db.sh deploy/production/backups/<dump-file>
```

Lakukan restore drill minimal sekali sebelum aplikasi dibuka penuh.

## 15. Troubleshooting cepat

### Domain tidak bisa dibuka

Periksa:

- tunnel status di dashboard Cloudflare
- `systemctl status cloudflared`
- `journalctl -u cloudflared -f`
- public hostname benar-benar `app.bola788.store -> http://127.0.0.1:18080`

### Tunnel aktif tetapi app tetap 502/503

Periksa local origin:

```bash
curl -fsS http://127.0.0.1:18080/health/live
curl -fsS http://127.0.0.1:18080/health/ready
```

Kalau ini gagal, masalah ada di stack aplikasi, bukan di Cloudflare.

### Login atau API error setelah domain hidup

Periksa:

- `APP_URL=https://app.bola788.store`
- tidak ada placeholder secret di `env.production`
- `api`, `redis`, `postgres`, `worker`, `scheduler` sehat

### Webhook provider tidak masuk

Periksa:

- provider benar-benar mengarah ke `https://app.bola788.store/v1/webhooks/qris`
- domain tunnel aktif
- API log tidak menunjukkan error parsing atau reject

## 16. Best practice yang dipakai di panduan ini

- gunakan remotely-managed tunnel, bukan quick tunnel
- jalankan `cloudflared` sebagai service, bukan process manual di terminal
- jangan buka port publik `80/443` pada origin
- pastikan `PRODUCTION_HTTP_PORT` dan `PRODUCTION_HTTPS_PORT` di mode tunnel sama-sama bind ke `127.0.0.1`
- jangan pakai ACME publik di origin untuk mode ini
- pertahankan origin private dan bind reverse proxy hanya ke `127.0.0.1`
- simpan tunnel token seperti secret produksi
- jalankan smoke test setelah tunnel hidup
- lakukan backup dan restore drill sebelum go-live

## 17. Ringkasan langkah tercepat

Kalau Anda ingin alur singkat untuk `app.bola788.store`, urutannya:

1. isi `deploy/production/env.production`
2. jalankan stack dengan `deploy/production/docker-compose.yml` + [`deploy/cloudflare-tunnel/docker-compose.override.yml`](/home/mugiew/project/onixggr/deploy/cloudflare-tunnel/docker-compose.override.yml)
   atau langsung `./deploy/cloudflare-tunnel/deploy.sh`
3. pastikan `curl http://127.0.0.1:18080/health/live` sukses
4. buat remotely-managed tunnel di Cloudflare
5. buat public hostname `app.bola788.store -> http://127.0.0.1:18080`
6. install `cloudflared` service dengan token tunnel
7. cek `https://app.bola788.store/health/live`
8. jalankan `SMOKE_BASE_URL=https://app.bola788.store ./deploy/production/smoke-test.sh`

Kalau semua langkah itu hijau, domain `app.bola788.store` sudah siap dipakai lewat Cloudflare Tunnel.
