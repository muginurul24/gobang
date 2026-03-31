# Rollback Plan

## Before Deploy

1. Record the current Git SHA deployed on the host.
2. Run `./deploy/production/backup-db.sh` and keep the dump path.
3. Confirm `git status --short` is clean on the deploy host.

## Standard Rollback

1. Stop traffic changes and note the failing SHA.
2. Check out the previous known-good SHA on the deploy host.
3. Re-run `./deploy/production/deploy.sh`.
4. Run `./deploy/production/smoke-test.sh` before reopening traffic.

## Data Rollback

Use data rollback only if the release introduced an incompatible migration or data corruption.

1. Stop API, worker, and scheduler writes.
2. Restore the last known-good backup with `./deploy/production/restore-db.sh <dump>` into a temporary verification database first.
3. If the verification counts and critical rows look correct, restore the same dump into the live database during the maintenance window.
4. Re-deploy the previous known-good SHA and rerun smoke checks.

## Notes

- Prefer backward-compatible migrations so standard code rollback stays possible.
- Keep at least one successful restore drill result from the last 7 days before a production release.
