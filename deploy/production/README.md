# Production Runbook

Files in this directory provide the Hari 46 production checklist and deploy assets:

- `docker-compose.yml`: production stack with PostgreSQL, Redis, API, worker, scheduler, web runtime, and Caddy reverse proxy.
- `env.production.example`: placeholder contract. Copy it to `env.production`, then replace every placeholder before deploy.
- `deploy.sh`: preflight production values, boot infra, optionally take a pre-migration backup, run migrations, then bring the app stack up.
- `smoke-test.sh`: HTTPS smoke test against the public production origin.
- `backup-db.sh`: create a PostgreSQL custom-format dump and prune old dumps.
- `restore-db.sh`: restore a dump into a temporary verification database and report core row counts.
- `backup-cron.example`: example daily backup plus weekly restore drill schedule.
- `prometheus-scrape.example.yml`: minimal internal Prometheus scrape config for `api:9090`.
- `go-live-checklist.md`: operator checklist for secrets, domain, TLS, monitoring, retention, rollback, and access review.
- `rollback-plan.md`: rollback procedure for code-only failure and data rollback scenarios.

Typical production flow:

```bash
cp deploy/production/env.production.example deploy/production/env.production
# edit env.production with real values
./deploy/production/deploy.sh
./deploy/production/smoke-test.sh
./deploy/production/backup-db.sh
```

Production notes:

- `deploy/production/deploy.sh` is the single production entrypoint. Cloudflare Tunnel mode is chosen automatically when `PRODUCTION_HTTP_PORT` and `PRODUCTION_HTTPS_PORT` both bind to `127.0.0.1:*`.
- `deploy/cloudflare-tunnel/deploy.sh` and `deploy/cloudflare-tunnel/down.sh` are compatibility shims only.
- `deploy.sh` rejects placeholder or localhost-like values for core secrets, domain, and upstream URLs.
- `/metrics` is not proxied publicly. Prometheus should scrape `api:9090` on the internal Compose network.
- Run `restore-db.sh` during go-live rehearsal and then on a recurring drill cadence.
