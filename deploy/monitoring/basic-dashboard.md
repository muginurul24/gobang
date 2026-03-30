# Basic Dashboard

Gunakan query Prometheus berikut untuk dashboard baseline Hari 37.

## Request Rate

```promql
sum(rate(onixggr_http_requests_total[5m])) by (method, status)
```

## Request Latency P95

```promql
histogram_quantile(0.95, sum(rate(onixggr_http_request_duration_seconds_bucket[5m])) by (le, method, status))
```

## Provider Latency P95

```promql
histogram_quantile(0.95, sum(rate(onixggr_upstream_request_duration_seconds_bucket[5m])) by (le, provider, operation, result))
```

## Game Balance Cache Hit Ratio

```promql
sum(rate(onixggr_cache_lookups_total{cache="game_balance",result="hit"}[5m]))
/
sum(rate(onixggr_cache_lookups_total{cache="game_balance"}[5m]))
```

## Callback Queue Depth

```promql
onixggr_callback_queue_depth
```

## Reconcile Backlog

```promql
onixggr_reconcile_backlog
```

## WebSocket Connections

```promql
onixggr_websocket_connections
```
