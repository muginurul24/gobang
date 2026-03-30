# Failure Drill Baseline

Run the local failure drill with:

```bash
KEEP_STACK=1 ./scripts/run-failure-drill.sh
```

`KEEP_STACK=1` preserves the latest `.tmp/onixggr-failure.*` artifacts for log review. Without it, the script tears the stack down at exit.

## Scenarios

- Redis down: readiness must return `503`, `onixggr_dependency_up{dependency="redis"} 0`, then recover to healthy once Redis returns.
- DB slow or unavailable: the drill pauses PostgreSQL to force readiness `503` and `onixggr_dependency_up{dependency="postgres"} 0`, then verifies recovery.
- NexusGGR timeout plus worker down: five deposit requests are forced into `pending_reconcile`, stay pending while the worker is off, then finalize exactly once after the worker resumes.
- QRIS webhook missing: a `member_payment` stays pending until the worker resolves it through `checkstatus/v2`, then posts ledger credit once.
- Callback owner failing: the store callback URL is pointed at an unreachable endpoint until retries exhaust, then the worker marks rows `failed`, creates `callback.delivery_failed` notifications, and drives the callback failure metric.

## Latest Local Baseline

Run date: March 31, 2026

- `redis_down ready_status=503 metric=redis_down recovered=yes`
- `db_slow_sim ready_status=503 metric=postgres_down recovered=yes`
- `nexus_timeout pending_before=5 still_pending_while_worker_off=5 ledger_before=0 ledger_after=5 ledger_after_second_check=5 timeout_metric=5`
- `qris_webhook_missing ... ledger_before=0 ledger_after=1 reconciled=yes`
- `callback_owner_failing failed_callbacks=4 notifications=4 callback_metric=24`

## Alert Conditions

The drill drives exporter metrics past the alert thresholds defined in [`alerts.rules.yml`](/home/mugiew/project/onixggr/deploy/monitoring/alerts.rules.yml):

- `OnixggrRedisDown`
- `OnixggrDatabaseDown`
- `OnixggrNexusGGRErrorSpike`
- `OnixggrCallbackFailureSpike`

Prometheus still applies each rule’s configured `for` window. The local drill verifies that the metric conditions are reached and the system behavior stays safe while degraded.
