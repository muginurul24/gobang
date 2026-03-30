# Hari 43 K6 Baseline

Baseline ini dijalankan pada 31 Maret 2026 dengan:

- PostgreSQL + Redis lokal via Podman
- API lokal di `http://127.0.0.1:18080`
- mock upstream lokal di `http://127.0.0.1:18081`
- demo seed dari `./appctl migrate fresh --seed`
- runner `npm run perf:k6`

## Hasil

| Scenario | Catatan | p95 | p99 |
| --- | --- | ---: | ---: |
| Login | 10 VU, success penuh | 1.25s | 1.46s |
| Game deposit | 2 VU, store balance cepat habis sesudah 500 success | 19.35ms | 22.93ms |
| Game withdraw | 2 VU, member balance cepat habis sesudah 500 success | 11.22ms | 15.21ms |
| Game balance read | 20 VU, success penuh | 18.90ms | 27.20ms |
| QRIS generate | 6 VU, success penuh | 18.42ms | 24.74ms |
| Webhook burst | 24 VU, 0.17% non-200 saat burst duplicate/simultan | 149.58ms | 234.97ms |
| WebSocket concurrency | 50 koneksi, hold 10s | connect 137.15ms | connect 154.01ms |
| WebSocket session | 50 koneksi, hold 10s | session 10150.25ms | session 10160ms |

## Bottleneck Awal

1. Login adalah jalur paling lambat. p95 `1.25s` dan p99 `1.46s` konsisten menunjukkan bottleneck utama ada di verifikasi bcrypt dan rotasi session satu-device.
2. Finalisasi webhook adalah write path paling berat sesudah login. p95 `149.58ms` dan p99 `234.97ms` datang dari kombinasi update transaksi, ledger, audit, notif, dan duplicate guard dalam satu request.
3. Harness deposit dan withdraw saat ini masih dibatasi fixture demo, bukan kapasitas API. Dengan seed default `2_500_000.00`, flow deposit berhenti pada 500 success lalu sisa request berubah jadi rejection cepat; withdraw juga berhenti saat saldo member mock habis. Untuk baseline success-path yang lebih panjang, fixture perf perlu store balance dan upstream member balance yang lebih besar atau reset berkala.
