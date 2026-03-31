# Go-Live Checklist

Use this checklist with `deploy/production/env.production`, not with the example file.

## Secrets

- Rotate `POSTGRES_PASSWORD`, `REDIS_PASSWORD`, `JWT_ACCESS_SECRET`, `AUTH_ENCRYPTION_KEY`, `CALLBACK_SIGNING_SECRET`, `QRIS_CLIENT_KEY`, `QRIS_WEBHOOK_SHARED_SECRET`, and `NEXUSGGR_AGENT_TOKEN`.
- Keep `env.production` outside Git and restrict read access to the deploy operator group only.
- Confirm provider dashboards, callback endpoints, and bank-account ops use production values, not demo or mock credentials.

## Domain and TLS

- Point `PRODUCTION_DOMAIN` DNS to the reverse-proxy host before deploy.
- Set `APP_URL` to the exact public HTTPS origin.
- Open ports `80` and `443` so Caddy can issue and renew certificates.
- Set `TLS_EMAIL` to a monitored inbox for ACME notices.

## Monitoring and Alerts

- Keep `METRICS_ENABLED=true` and expose `api:9090` only on the internal Compose network.
- Load [`alerts.rules.yml`](../monitoring/alerts.rules.yml) into Prometheus and scrape `api:9090` using [`prometheus-scrape.example.yml`](./prometheus-scrape.example.yml).
- Review the starter dashboard queries in [`basic-dashboard.md`](../monitoring/basic-dashboard.md).

## Backup and Retention

- Install [`backup-cron.example`](./backup-cron.example) or an equivalent scheduler before go-live.
- Run `./deploy/production/backup-db.sh` and `./deploy/production/restore-db.sh <dump>` once on the production host before opening traffic.
- Confirm `AUDIT_RETENTION_PERIOD`, `AUDIT_PRUNE_INTERVAL`, `CHAT_RETENTION_PERIOD`, and `CHAT_PRUNE_INTERVAL` match policy.

## Rollback and Access

- Read [`rollback-plan.md`](./rollback-plan.md) and record the previous release SHA before each deploy.
- Limit shell and `env.production` access to the deploy operator group. DB superuser and provider dashboard access should not be shared with dashboard operators.
- Limit who can rotate store callback URLs, store tokens, and withdraw destination accounts in production.
