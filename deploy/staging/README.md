# Staging RC

Files in this directory provide the Hari 45 staging release candidate:

- `docker-compose.yml`: full staging stack with PostgreSQL, Redis, API, worker, scheduler, Node web runtime, and Caddy reverse proxy.
- `env.staging.example`: placeholder runtime contract. Copy it to `env.staging` before deploy.
- `deploy.sh`: build images, start infra, run migrations, optionally seed demo data, then bring the app stack up.
- `smoke-test.sh`: HTTPS smoke test through the reverse proxy.
- `backup-db.sh`: create a PostgreSQL custom-format dump from the running staging stack.
- `restore-db.sh`: restore a backup into a temporary database and verify core row counts.
- `down.sh`: stop the staging stack.

Default local RC assumptions:

- domain: `staging.localhost`
- HTTPS port: `18443`
- PostgreSQL host port: `25432`
- Redis host port: `26379`
- TLS: `tls internal` from Caddy for self-signed staging certificates
- upstreams: mock provider profile enabled for local smoke runs

Typical local RC flow:

```bash
cp deploy/staging/env.staging.example deploy/staging/env.staging
./deploy/staging/deploy.sh
./deploy/staging/smoke-test.sh
BACKUP_FILE="$(./deploy/staging/backup-db.sh | sed 's/backup_file=//')"
./deploy/staging/restore-db.sh "$BACKUP_FILE"
./deploy/staging/down.sh
```
