# Lessons Learned – Top-K User Aggregation

## What We Built (Lab Scope)

A functional end-to-end system that:
- Simulates crawling user listening history
- Publishes events to Kafka
- Persists raw events to Cassandra
- Aggregates daily counts using counter columns
- Serves Top-K via API with Redis caching

**Stack:** Go, Kafka, Cassandra, Redis, Asynq, Docker Compose

---

## Known Limitations (Lab Trade-offs)

| Area | Lab Implementation | Why It's OK for Lab |
|------|-------------------|---------------------|
| Crawl jobs | **DB-backed scheduler with reconciliation** | ✅ Production-ready pattern |
| Deduplication | None | Over-counting is acceptable per design doc |
| Partition affinity | Not enforced | Counter columns handle concurrent writes |
| Cache invalidation | TTL-only (5 min) | 1-day staleness is acceptable |
| OAuth tokens | No refresh rotation | GitHub tokens are long-lived for demo |
| Error handling | Log and continue | No production alerting needed |
| Metrics/Observability | None | Visual verification via logs |

---

## DB-Backed Scheduler Pattern (Implemented)

### The Problem: Redis Job Persistence

Originally, crawl jobs were scheduled using Asynq with `ProcessIn(24*time.Hour)`. The problem:
- Jobs stored in Redis (in-memory)
- Redis restart = all scheduled jobs lost
- 70M daily crawl jobs × 500 bytes = 35GB RAM (expensive at scale)

### The Solution: DB as Source of Truth

```
┌─────────────────────────────────────────────────────────────────┐
│   PostgreSQL: user_crawl_schedule                               │
│   ┌──────────┬──────────┬────────────────┬──────────────────┐  │
│   │ user_id  │ provider │ next_crawl_at  │ status           │  │
│   ├──────────┼──────────┼────────────────┼──────────────────┤  │
│   │ user_1   │ spotify  │ tomorrow 2AM   │ IDLE             │  │
│   │ user_2   │ spotify  │ now            │ ENQUEUED         │  │
│   └──────────┴──────────┴────────────────┴──────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
                              │
         Scheduler polls every 10 seconds
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│   Redis/Asynq: Ready Queue (only "execute NOW" jobs)            │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│   Crawl Workers → Update DB: status=IDLE, next_crawl_at=tomorrow│
└─────────────────────────────────────────────────────────────────┘
```

### Key Components

**1. Scheduler (crawl-scheduler)**
```sql
-- Query 1: Find ready jobs
SELECT * FROM user_crawl_schedule
WHERE next_crawl_at <= NOW() AND status = 'IDLE'
FOR UPDATE SKIP LOCKED;
→ Set status = 'ENQUEUED', enqueue to Asynq

-- Query 2: Reconciliation (find stuck jobs)
SELECT * FROM user_crawl_schedule
WHERE status = 'ENQUEUED' AND updated_at < NOW() - INTERVAL '1 hour';
→ Re-enqueue (job was lost in Redis)
```

**2. Worker (crawl-worker)**
```go
// On job start
updateStatus(userID, provider, "RUNNING")

// On success
UPDATE user_crawl_schedule
SET status = 'IDLE', next_crawl_at = NOW() + '24 hours'
WHERE user_id = ? AND provider = ?
```

### Why This Works

| Scenario | What Happens |
|----------|--------------|
| Normal flow | IDLE → ENQUEUED → RUNNING → IDLE (next_crawl_at = tomorrow) |
| Redis dies | DB has status='ENQUEUED', reconciliation re-enqueues |
| Worker crashes | Same as above |
| Scheduler restarts | Continues polling from DB |

### Trade-offs

| Aspect | DB-Backed | Redis-Only |
|--------|-----------|------------|
| Durability | ✅ Survives restart | ❌ Lost on restart |
| Cost at scale | ✅ Disk storage | ❌ RAM (expensive) |
| Latency | ~10s poll delay | Instant |
| Complexity | Medium | Low |
| Reconciliation | ✅ Built-in | ❌ Manual |

---

## Production Considerations

### 1. **Reliability & Durability**

| Concern | Lab | Production |
|---------|-----|------------|
| Kafka | Single broker, no replication | 3+ brokers, replication factor 3, ISR ≥ 2 |
| Cassandra | Single node | 3+ nodes, RF=3, LOCAL_QUORUM reads/writes |
| Redis | Single instance | Redis Cluster or Sentinel for HA |
| Job persistence | **DB-backed scheduler** ✅ | Same pattern scales to production |

### 2. **Scalability**

| Component | Lab | Production |
|-----------|-----|------------|
| Crawl workers | 1 instance | Auto-scaled based on queue depth |
| Aggregators | 1 instance | Partition-affine consumers (1 per Kafka partition) |
| API servers | 1 instance | Horizontally scaled behind load balancer |
| Kafka partitions | Default | Partition by `user_id` for ordering guarantees |

### 3. **Data Integrity**

```
Lab:      at-least-once delivery → possible over-counts
Production options:
  1. Exactly-once semantics (Kafka transactions)
  2. Idempotent writes (event_id as Cassandra clustering key)
  3. Deduplication window (Redis SET with TTL)
```

### 4. **Observability (Must-Have for Production)**

| Tool | Purpose |
|------|---------|
| Prometheus + Grafana | Metrics (lag, throughput, latencies) |
| Structured logging (JSON) | Queryable logs |
| Distributed tracing (Jaeger/Zipkin) | Request flow debugging |
| Alerting (PagerDuty/Opsgenie) | On-call notifications |

**Key metrics to track:**
- Kafka consumer lag per group
- Cassandra write latency (p99)
- API response time (p50, p95, p99)
- Cache hit rate
- Crawl job failure rate

### 5. **Security**

| Concern | Lab | Production |
|---------|-----|------------|
| OAuth tokens | SQLite, plaintext | Encrypted at rest (Vault, KMS) |
| API auth | None | JWT/OAuth2 bearer tokens |
| Network | Docker bridge | VPC, private subnets, TLS everywhere |
| Secrets | `.env` files | Secret manager (AWS Secrets Manager, Vault) |

### 6. **Deployment**

| Aspect | Lab | Production |
|--------|-----|------------|
| Orchestration | Docker Compose | Kubernetes (EKS/GKE) |
| CI/CD | Manual | GitHub Actions → ArgoCD |
| Config | Environment variables | ConfigMaps + Secrets |
| Rollbacks | `docker compose down` | Kubernetes rollback, blue-green |

### 7. **Data Management**

| Concern | Lab | Production |
|---------|-----|------------|
| Backups | None | Cassandra snapshots, Kafka MirrorMaker |
| TTL enforcement | Cassandra TTL | TTL + compaction tuning |
| Schema migrations | Manual CQL | Liquibase/Flyway or versioned CQL scripts |
| Data retention | 7 days | Compliance-driven (GDPR delete on request) |

---

## What Would Change at Scale

### 10K users → 10M users

1. **Kafka partitions**: Increase to 50-100, partition by `user_id % N`
2. **Aggregator scaling**: One consumer per partition (consumer group handles this)
3. **Cassandra**: 
   - Partition key tuning to avoid hot spots
   - Consider `(user_id, day)` vs `(user_id)` based on access patterns
4. **API caching**: 
   - Multi-tier: Local cache → Redis → Cassandra
   - Cache warming for active users

### 1M events/day → 1B events/day

1. **Kafka**: 
   - Multiple clusters (geo-distributed)
   - Tiered storage for cold data
2. **Aggregation**:
   - Pre-aggregation at crawl-worker level (micro-batching)
   - Stream processing (Flink/Spark Streaming) instead of simple consumers
3. **Storage**:
   - Time-series optimized store (ClickHouse, TimescaleDB) for analytics
   - Cassandra for serving, analytics DB for exploration

---

## Key Learnings from This Build

### Architecture
- **Event log as backbone works** — decouples ingestion from processing
- **Counter columns simplify concurrent writes** — no read-modify-write needed
- **Async aggregation is key** — never aggregate on read path

### Technology Choices
- **DB + Asynq for job scheduling** — DB for durability, Asynq for execution
- **Reconciliation pattern** — recovers from Redis failures automatically
- **Kafka for event log** — durable, replayable, multiple consumers
- **Cassandra counters have quirks** — no secondary indexes, no TTL on counters directly

### Development
- **Docker Compose is sufficient for labs** — fast iteration, easy cleanup
- **Bind mounts > named volumes for debugging** — can inspect data directly
- **Start with design doc** — prevents scope creep and rework

### What I'd Do Differently
1. **Add structured logging from day 1** — debugging distributed systems is hard
2. **Implement basic deduplication** — even a simple Redis SET helps
3. **Add health endpoints** — `/healthz` and `/readyz` for each service
4. **Schema versioning** — track CQL changes in migration files

---

## Interview Talking Points

When discussing this system in interviews:

1. **Why pull-based ingestion?**
   - Third-party APIs don't push; we control crawl rate and backoff

2. **Why Kafka + Cassandra?**
   - Kafka: durable log, replay capability, multiple consumers
   - Cassandra: high write throughput, time-series friendly, TTL built-in

3. **How do you handle failures?**
   - DB-backed scheduler with reconciliation for lost jobs
   - Kafka consumer offsets for replay
   - Cassandra counters for idempotent-ish increments
   - Cache fallback to DB

4. **How would you scale this?**
   - Horizontal: more partitions, more consumers, more API servers
   - Vertical: bigger Cassandra nodes for hot users

5. **What's the consistency model?**
   - Eventual consistency with ~1 day staleness acceptable
   - Strong consistency not needed for "top songs" use case

---

## Checklist: Before Going to Production

- [ ] Kafka replication configured (RF ≥ 3)
- [ ] Cassandra multi-node cluster (RF = 3)
- [ ] Redis HA (Sentinel or Cluster)
- [ ] All secrets in secret manager
- [ ] TLS enabled on all connections
- [ ] Prometheus metrics exposed
- [ ] Alerting configured (consumer lag, error rates)
- [ ] Health endpoints on all services
- [ ] CI/CD pipeline with automated tests
- [ ] Runbook for common failures
- [ ] Load tested to expected peak traffic
- [ ] GDPR/data deletion flow implemented

---

*Document created: 2026-01-30*
*System: Top-K User Aggregation (system-design-lab)*
