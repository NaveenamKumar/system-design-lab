Document 2
Technology Choices Document – Concrete Tools & Tradeoffs

(Opinionated, context-aware, but still reversible)

Heatmap Analytics Platform

Author: Naveenam
Project: system-design-lab
Scope: Technology Selection & Tradeoffs
Audience: Builders / Reviewers

1. Purpose of This Document

This document answers:

Given the architecture and invariants, what concrete technologies do we choose to build this system in the lab?

This is not about “best in general”, but about:

- building a real end-to-end system locally
- learning production-like behaviors (partitioning, batching, OLAP modeling, batch jobs)
- keeping the design evolvable (especially JSON → Parquet)

All choices here are explicitly allowed to change later.

2. Constraints & Context

- Single developer / lab project
- Must run locally (Docker / Compose)
- Wants exposure to:
  - Kafka → object storage landing
  - Spark batch job execution (hourly/daily/weekly rollups)
  - OLAP data modeling & query behavior (ClickHouse)
- Freshness requirement: ~1 hour is acceptable
- Scale assumption: ~1B events/day (bursty), but we simulate locally

3. Technology Decisions (By Layer)

3.1 Event Log (Durability Boundary)

Requirement

- Append-only log
- Replayable
- Fan-out to multiple consumers (landing + debug tools)
- Backpressure handling for bursty ingestion

Options Considered

| Option | Pros | Cons |
|-------|------|------|
| Direct to S3 | Simplest components | Harder to absorb spikes; batching/flush logic moves into ingestion; replay/debug tooling more manual |
| Redis Streams | Simple for labs | Memory-bound, weaker replay/retention story |
| Kafka | Durable, replayable, fan-out, common | Operational overhead |

✅ Decision

Kafka

Rationale

- Matches event-driven invariant (log as backbone)
- Simplifies failure recovery (replay from offsets)
- Decouples ingestion from landing/aggregation
- Good learning value and reusable across systems in this repo

3.2 Object Storage (Raw Event Truth)

Requirement

- Cheap durable storage for raw events
- Works with Spark
- Locally runnable

✅ Decision

MinIO (S3-compatible) for local development

Rationale

- Mimics S3 semantics closely enough for learning
- Works with Spark’s S3A connector and ClickHouse S3 features (optional)

3.3 Kafka → S3 Landing (Stream-to-Files)

Requirement

- Convert Kafka stream into immutable, time-partitioned files
- Efficient batching (avoid one object per event)
- Extensible to Parquet

Options Considered

| Option | Pros | Cons |
|-------|------|------|
| Kafka Connect S3 Sink | Standard pattern; handles batching/retries; configurable | Config-heavy; Parquet often implies schema registry/converters |
| Custom “landed-writer” consumer | Full control; easy to teach file layout; can write Parquet via libraries | You own correctness (flush, retries, idempotency); more code |

✅ Decision (Lab)

Kafka Connect + S3 Sink (to MinIO)

Idempotency / Dedup (Phase 2)

For high-volume telemetry (mousemove), strict per-event deduplication is usually not worth the cost.
If client retries cause many duplicate batches, prefer batch-level idempotency at ingestion:

- Add `batch_id` to ingestion requests
- Use Redis `SETNX(client_id:batch_id)` with a short TTL to drop duplicates

This is optional and intentionally deferred until there is evidence of an actual duplicate problem.

Format Strategy (Extensible)

- Phase 1: JSONLines (fastest to bootstrap)
- Phase 2: Parquet (upgrade to learn data-lake best practices)

Why JSON first

- Avoids schema registry and converter complexity in the first iteration
- Lets us focus on Spark batch + ClickHouse OLAP modeling quickly

Why Parquet later (what you’ll learn)

- Columnar layout, schema enforcement, compression
- Faster Spark scans (column pruning, predicate pushdown)
- Better long-term storage and query economics

3.4 Batch Processing (MapReduce-style)

Requirement

- Hourly job over raw files
- Daily rollup from hourly
- Weekly rollup from daily
- Locally runnable and teachable

Options Considered

| Option | Pros | Cons |
|-------|------|------|
| Apache Spark | Industry standard; great for batch; rich ecosystem | Heavier local footprint |
| Hadoop MapReduce | “Classic MapReduce” | Operationally heavy; less common today |
| Flink batch/streaming | You already know it | Less aligned to learning Spark batch specifically |
| DuckDB batch scripts | Very easy locally | Less industry-aligned for “batch cluster” mental model |

✅ Decision

Apache Spark (standalone master/worker in Compose)

Rationale

- Strong learning value for batch execution model
- Clear separation between “landing files” and “compute job”

3.5 OLAP Serving Store (Aggregates)

Requirement

- Fast group-by queries for dashboards
- Columnar storage, efficient scans
- Handles large aggregate tables (hourly + rollups)
- Locally runnable

Options Considered

| Option | Pros | Cons |
|-------|------|------|
| ClickHouse | Excellent performance; common; great learning tool | Must learn partition/order-by/insert patterns |
| Druid / Pinot | Strong OLAP too | Heavier infra; more moving parts |
| Postgres | Familiar | Not ideal for large OLAP scans |

✅ Decision

ClickHouse

Rationale

- Great for teaching OLAP internals (parts, merges, partition pruning, sort keys)
- Great performance for heatmap queries

3.6 Aggregation Storage Model (Hourly/Daily/Weekly)

Decision

Separate tables:

- `heatmap_hourly`
- `heatmap_daily`
- `heatmap_weekly`

Rationale

- Different retention horizons per granularity are common in production
- Clear mental model for rollups and OLAP table design

Rollup plan:

- Daily derived from hourly (fast)
- Weekly derived from daily (fast)

3.7 Client Simulator & Visualization

Decision

- Client simulator: simple web page capturing `mousemove`, throttled + batched
- Visualization: `heatmap.js` overlay driven by cell counts

Rationale

- No mobile device needed for lab
- Mousemove generates volume; batching mimics SDK behavior

4. Explicit Upgrade Plan: JSON → Parquet

We design the landing layout and Spark code so that format upgrades are localized.

4.1 Stable “data contract” that must not change

- Partitioning scheme (by event time, optionally by client)
- Field names and semantics (client_id, screen_id, event_time, x_norm, y_norm)

4.2 What changes in the upgrade

- Kafka Connect S3 sink config: JSON → Parquet (and any schema converter changes)
- Spark read path: `spark.read.json(...)` → `spark.read.parquet(...)`

4.3 What does NOT change

- Ingestion HTTP API
- Kafka event schema at the logical level
- Aggregation logic (cell bucketing, group-by)
- ClickHouse output schema

5. Summary of Technology Stack (Lab)

| Layer | Technology |
|------|------------|
| Client simulator | Static web page (mousemove + batching) |
| Ingestion API | Go (HTTP → Kafka) |
| Event log | Kafka |
| Object storage | MinIO (S3 compatible) |
| Kafka → S3 landing | Kafka Connect + S3 sink |
| Batch compute | Apache Spark (standalone) |
| OLAP serving | ClickHouse |
| Local orchestration | Docker Compose |

✅ Outcome of This Document

After this document, we know:

- Exactly what technologies we will use in the lab and why.
- How we will upgrade from JSON to Parquet without redesigning the system.
