# Lessons Learned -- Heatmap Analytics Platform

## What We Built (Lab Scope)

A functional end-to-end heatmap analytics platform that:
- Captures high-volume mouse interaction events from client applications via SDK
- Ingests events through an HTTP service into Kafka
- Lands raw events to object storage (MinIO) via Kafka Connect
- Aggregates events into heatmap tiles using Spark batch jobs (hourly -> daily -> weekly)
- Serves aggregates from ClickHouse via a query API + dashboard

**Stack:** Go (ingestion + query API), Kafka, Kafka Connect, MinIO (S3), Apache Spark (PySpark), ClickHouse, Redis, Docker Compose

**Data Flow:**
```
Client (mousemove events)
  |
  v
Ingestion Service (Go, HTTP)
  |-- Redis SETNX batch dedup (Phase 2)
  v
Kafka (heatmap.events.raw)
  |
  v
Kafka Connect S3 Sink --> MinIO (raw events, Parquet/JSON)
  |
  v
Spark Batch Jobs
  |-- hourly: raw S3 files -> grid bucketing -> heatmap_hourly
  |-- daily:  heatmap_hourly rollup -> heatmap_daily
  |-- weekly: heatmap_daily rollup -> heatmap_weekly
  |
  v
ClickHouse (OLAP)  <--  Query API (Go)  <--  Dashboard (HTML/JS)
```

---

## Known Limitations (Lab Trade-offs)

| Area | Lab Implementation | Why It's OK for Lab |
|------|-------------------|---------------------|
| Batch dedup | Redis SETNX per `(client_id, batch_id)` with TTL | Catches client retries; not per-event dedup |
| Spark job idempotency | Delete-then-insert via ClickHouse HTTP API | Simple, no schema changes; mutation latency acceptable for batch |
| Scheduling | Manual Spark job invocation | No orchestrator (Airflow/Dagster); sufficient for demo |
| Grid resolution | Fixed 100x200 grid | Hardcoded; production would be configurable per screen |
| Auth | None | Lab is single-tenant, no multi-tenant isolation |
| Observability | Logs only | No Prometheus/Grafana; visual verification via dashboard |
| Raw format | JSON or Parquet (selectable) | Parquet preferred for production; JSON easier to debug |

---

## Key Design Patterns Implemented

### 1. Event Log as Buffer & Fan-out Boundary

Kafka decouples ingestion from all downstream consumers. Ingestion writes once; Connect, debug consumers, and future stream processors can each read independently.

**Why it matters:** Ingestion latency is unaffected by downstream slowness. Adding a new consumer (e.g., real-time anomaly detector) requires zero changes to the ingestion service.

### 2. Raw Events as Source of Truth (MinIO)

Object storage holds append-only raw events for the retention window. All aggregates are derived data.

**Why it matters:** Aggregates can be recomputed from scratch if a bug is found in the Spark job. Schema evolution is safe -- reprocess raw files with new logic.

### 3. Async Bucketed Aggregation (Hourly -> Daily -> Weekly Rollups)

Aggregation is periodic batch, never on the read path. Each tier reads from the tier below:
- Hourly: reads raw S3 files, buckets into 100x200 grid cells
- Daily: `SUM(count) GROUP BY day` from hourly
- Weekly: `SUM(count) GROUP BY week` from daily

**Why it matters:** Dashboard queries hit pre-aggregated ClickHouse tables -- O(cells) not O(events). Rollups reduce storage: 24 hourly rows collapse to 1 daily row per cell.

### 4. Batch-Level Idempotency (Redis SETNX)

Ingestion checks `SETNX dedup:{client_id}:{batch_id}` with a 24h TTL before writing to Kafka. Duplicate batches (client retries) are dropped.

**Why it matters:** Prevents double-counting from client-side retries without requiring per-event dedup state for billions of events. Cheap and effective for telemetry.

### 5. Delete-Then-Insert for Spark Job Idempotency

Before inserting aggregates, each Spark job deletes existing rows for the target time bucket via `ALTER TABLE DELETE` on ClickHouse's HTTP API, polls `system.mutations` until complete, then inserts via JDBC.

**Why it matters:** Re-running a job (retry, backfill, bug fix) produces the same result. No schema changes required (vs. ReplacingMergeTree or version columns).

### 6. Grid Bucketing for Spatial Aggregation

Normalized coordinates `(x_norm, y_norm)` in `[0,1)` are mapped to discrete grid cells:
```
cell_x = floor(x_norm * GRID_W)   -- [0, 99]
cell_y = floor(y_norm * GRID_H)   -- [0, 199]
```

**Why it matters:** Reduces cardinality from infinite float pairs to 20,000 cells. Enables efficient `GROUP BY` and compact storage. Grid resolution is a tunable knob for precision vs. storage trade-off.

---

## Production Considerations

### 1. Reliability & Durability

| Concern | Lab | Production |
|---------|-----|------------|
| Kafka | Single broker, no replication | 3+ brokers, RF=3, ISR >= 2, `acks=all` |
| ClickHouse | Single node | Replicated cluster (ReplicatedMergeTree) |
| MinIO | Single node | Multi-node erasure coding or AWS S3 |
| Redis | Single instance | Redis Cluster or Sentinel for HA |
| Spark | Local mode | YARN/K8s with job retry and checkpointing |

### 2. Scalability

| Component | Lab | Production |
|-----------|-----|------------|
| Ingestion | 1 instance | Horizontally scaled behind LB; stateless |
| Kafka partitions | Default | Partition by `client_id` for ordering |
| Kafka Connect | 1 task | Multiple tasks, one per partition |
| Spark jobs | Manual | Orchestrated (Airflow/Dagster), parallelized per client |
| ClickHouse | Single node | Sharded by `client_id`, replicated |
| Query API | 1 instance | Horizontally scaled, add Redis cache layer |

### 3. Observability (Must-Have)

| Tool | Purpose |
|------|---------|
| Prometheus + Grafana | Metrics: ingestion throughput, Kafka lag, Spark job duration, query latency |
| Structured logging (JSON) | Queryable logs across all services |
| Alerting (PagerDuty) | Kafka consumer lag > threshold, Spark job failure, ClickHouse replication lag |

**Key metrics to track:**
- Ingestion: requests/sec, batch size, dedup hit rate, p99 latency
- Kafka: consumer lag per connector/group
- Spark: job duration, rows processed, failure count
- ClickHouse: query latency (p50/p95/p99), mutation queue depth
- Dashboard: time-to-first-paint, cache hit rate

### 4. Security

| Concern | Lab | Production |
|---------|-----|------------|
| API auth | None | API key per client_id, rate limiting |
| Network | Docker bridge | VPC, private subnets, TLS everywhere |
| Secrets | Hardcoded | Secret manager (Vault, AWS Secrets Manager) |
| Data privacy | None | PII audit, GDPR delete-by-client flow |
| ClickHouse access | Open | RBAC, read-only query API user |

---

## What Would Change at Scale

### Streaming vs. Batch

At ~1B events/day, hourly batch aggregation may be too slow for some use cases:
- **Option A:** Keep batch but increase frequency (every 15 min)
- **Option B:** Hybrid -- Flink/Spark Streaming for near-real-time hourly, batch for daily/weekly rollups
- **Option C:** ClickHouse materialized views for real-time aggregation (skip Spark for hourly)

### Partitioning Strategies

- **Kafka:** Partition by `client_id` to keep per-tenant events together
- **MinIO/S3:** Current `dt=/hour=` partitioning works well; add `client_id=` prefix for multi-tenant isolation
- **ClickHouse:** Shard by `client_id` for query isolation; partition by `toYYYYMMDD(bucket_start)` (already done)

### Multi-Tenant Isolation

- Separate Kafka topics per client (or at least separate consumer groups)
- Per-client rate limiting at ingestion
- ClickHouse row-level security or separate databases per client
- Per-client Spark job scheduling (avoid noisy neighbor)

---

## Talking Points

When discussing this system in interviews:

1. **Why batch instead of streaming?**
   - Heatmaps are inherently "look at yesterday's data" -- ~1h staleness is acceptable
   - Batch is simpler to debug, replay, and make idempotent
   - Raw events in S3 enable full reprocessing if aggregation logic changes

2. **How do you handle duplicate events?**
   - Two layers: batch-level dedup at ingestion (Redis SETNX), and delete-then-insert idempotency in Spark jobs
   - Per-event dedup is too expensive for mousemove telemetry (~1B events/day)
   - Small over-counts from at-least-once delivery are acceptable for heatmaps

3. **Why ClickHouse for aggregates?**
   - Columnar storage is ideal for `SUM(count) WHERE client_id = X AND bucket_start BETWEEN ...`
   - Built-in TTL for automatic data lifecycle management
   - MergeTree engine handles time-series partitioning natively

4. **How would you make Spark jobs reliable?**
   - Orchestrator (Airflow) with retry + alerting on failure
   - Delete-then-insert idempotency means retries are safe
   - Backfill: re-run for any historical hour from raw S3 data

5. **What's the consistency model?**
   - Eventual consistency with ~1 hour staleness for hourly data
   - Dashboard reads pre-aggregated tables; never scans raw events
   - Aggregates are derived data -- always reproducible from raw events

6. **How do you handle late-arriving events?**
   - Kafka Connect partitions by `event_time`, so late events land in the correct hour partition
   - Re-running the Spark job for that hour (with delete-then-insert) picks them up
   - Could add a "reprocess last N hours" policy in the orchestrator

---

## Checklist: Before Going to Production

- [x] **Batch dedup at ingestion** (Redis SETNX)
- [x] **Spark job idempotency** (delete-then-insert)
- [x] **ClickHouse TTLs configured** (90d hourly, 2y daily, 5y weekly)
- [x] **Timezone consistency** (UTC everywhere)
- [ ] Kafka replication configured (RF >= 3)
- [ ] ClickHouse replicated cluster (ReplicatedMergeTree)
- [ ] Redis HA (Sentinel or Cluster)
- [ ] All secrets in secret manager
- [ ] TLS enabled on all connections
- [ ] API authentication per client_id
- [ ] Rate limiting at ingestion
- [ ] Prometheus metrics exposed (all services)
- [ ] Alerting configured (Kafka lag, Spark failures, query latency)
- [ ] Spark job orchestrator (Airflow/Dagster)
- [ ] Health endpoints on all services (`/healthz`, `/readyz`)
- [ ] CI/CD pipeline with automated tests
- [ ] Load tested to expected peak traffic
- [ ] GDPR/data deletion flow implemented
- [ ] Runbook for common failures

---

*Document created: 2026-02-16*
*System: Heatmap Analytics Platform (system-design-lab)*
