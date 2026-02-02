Document 3
Solution / Implementation Document – Services, Schemas, and Execution Plan

(Concrete, actionable, build-ready)

Heatmap Analytics Platform

Author: Naveenam
Project: system-design-lab
Status: Draft
Scope: Service Boundaries, APIs, Events, Storage Layout, Batch Jobs

1. Purpose of This Document

This document answers:

How exactly do we implement the heatmap system described in the design doc using the chosen technologies?

It defines:

- service boundaries
- repo structure
- APIs and events
- object storage layout
- ClickHouse tables
- Spark batch jobs (hourly → daily → weekly)
- build order

After this document, coding can begin immediately.

2. Repository Structure (proposed)

```
systems/heatmap-sdk/
  design/
    01_design_doc.md
    02_technology_choice_doc.md
    03_solution_doc.md
    hld.png (to be added/updated)
  implementation/
    docker-compose.yml
    create-topics.sh
    init-runtime-dirs.sh
    schemas/
      clickhouse/
        01_init.sql
      kafka-connect/
        s3-sink-json.properties
        s3-sink-parquet.properties (Phase 2)
    services/
      ingestion-service/         (Go)
      query-api/                (Go)
      demo-client/              (static HTML)
      dashboard/                (static HTML)
    spark-jobs/
      heatmap_hourly.py
      heatmap_daily.py
      heatmap_weekly.py
      requirements.txt (if needed)
```

3. Services & Responsibilities

3.1 Demo Client (web)

Responsibilities

- Capture `mousemove` events within a fixed “screen” container
- Normalize coordinates to [0,1]
- Throttle and batch events (e.g., flush every 1s or 200 events)
- Send batches to ingestion API

Example normalization

- x_norm = (clientX - rect.left) / rect.width
- y_norm = (clientY - rect.top) / rect.height

3.2 Ingestion Service (HTTP → Kafka)

Responsibilities

- Receive batches from clients
- Validate event schema (required fields, x_norm/y_norm bounds)
- Enrich (received_at)
- Publish to Kafka topic `heatmap.events.raw`

APIs

POST /events/batch

Request body (Phase 1, counts-only):

```json
{
  "client_id": "acme",
  "screen_id": "home_v1",
  "batch_id": "uuid-optional",
  "events": [
    {
      "event_time": "2026-02-02T10:15:00Z",
      "x_norm": 0.25,
      "y_norm": 0.20,
      "event_type": "mousemove"
    }
  ]
}
```

Notes

- In production, we would include an event_id for dedup and optional session_id/user_id.
- For lab MVP, counts-only is sufficient.
- `batch_id` is optional for MVP; in Phase 2 it enables best-effort batch idempotency.

3.3 Event Log (Kafka)

Topic(s)

| Topic | Purpose | Key |
|-------|---------|-----|
| heatmap.events.raw | append-only raw interaction stream | client_id|screen_id (or client_id) |

Rationale for key choice

- Key by (client_id, screen_id) improves locality for downstream consumers and reduces cross-partition shuffles for per-screen aggregates.

3.4 Landing (Kafka → MinIO)

Responsibilities

- Consume `heatmap.events.raw`
- Buffer and flush to MinIO as immutable files
- Partition by event time to enable efficient Spark reads

Implementation Choice

- Kafka Connect S3 Sink to MinIO

Output File Layout (stable contract)

We use event-time based partitioning:

```
s3://heatmap-raw/
  dt=YYYY-MM-DD/
    hour=HH/
      client_id=<client_id>/
        part-*.json   (Phase 1)
        part-*.parquet (Phase 2)
```

This layout is stable across JSON → Parquet upgrades.

Why include client_id in the path?

- Reduces scan for a single tenant
- Avoids mixing tenants in the same files for simpler governance

3.5 Spark Batch Jobs

We run three jobs:

- Hourly: raw events → `heatmap_hourly`
- Daily: `heatmap_hourly` → `heatmap_daily`
- Weekly: `heatmap_daily` → `heatmap_weekly`

Grid bucketing

- GRID_W = 100
- GRID_H = 200
- cell_x = floor(x_norm * GRID_W) clamped to [0, GRID_W-1]
- cell_y = floor(y_norm * GRID_H) clamped to [0, GRID_H-1]

Job 1: Hourly Aggregation (raw → hourly)

Input

- MinIO path for a given (dt, hour)

Output

- ClickHouse table `heatmap_hourly` rows:
  (client_id, screen_id, bucket_start, cell_x, cell_y, count)

Job 2: Daily Rollup (hourly → daily)

Input

- ClickHouse `heatmap_hourly` rows for a given day

Transform

- bucket_start_day = startOfDay(bucket_start)
- group by (client_id, screen_id, bucket_start_day, cell_x, cell_y)
- sum(count)

Output

- ClickHouse `heatmap_daily`

Job 3: Weekly Rollup (daily → weekly)

Input

- ClickHouse `heatmap_daily` rows for a given week

Transform

- bucket_start_week = toStartOfWeek(bucket_start_day)
- group by (client_id, screen_id, bucket_start_week, cell_x, cell_y)
- sum(count)

Output

- ClickHouse `heatmap_weekly`

Scheduling

- Hourly job runs every hour for the previous hour (or “current hour with delay”)
- Daily job runs once per day (after hourly completion)
- Weekly job runs once per week (after daily completion)

3.6 OLAP Storage (ClickHouse)

We store aggregates in three tables.

Table: heatmap_hourly

- Granularity: 1 hour
- Retention: shorter (e.g., 90 days)

Table: heatmap_daily

- Granularity: 1 day
- Retention: longer (e.g., 2 years)

Table: heatmap_weekly

- Granularity: 1 week
- Retention: longest (e.g., 5 years)

Table engine and keys (baseline)

- Engine: MergeTree
- PARTITION BY: toYYYYMMDD(bucket_start)
- ORDER BY: (client_id, screen_id, bucket_start, cell_x, cell_y)

Why these matter (OLAP mental model)

- PARTITION BY enables pruning entire day partitions quickly.
- ORDER BY defines on-disk sort order; queries filtering on client_id/screen_id/bucket_start read fewer marks and are much faster.
- Inserts create parts; ClickHouse merges parts in the background.

3.7 Query API (ClickHouse → JSON)

Responsibilities

- Provide endpoints for the dashboard
- Translate query parameters into ClickHouse queries
- Return cell counts in a format suitable for visualization

API (initial)

GET /heatmap/hourly?client_id=...&screen_id=...&dt=YYYY-MM-DD&hour=HH
GET /heatmap/daily?client_id=...&screen_id=...&dt=YYYY-MM-DD
GET /heatmap/weekly?client_id=...&screen_id=...&week_start=YYYY-MM-DD

Response (sparse)

```json
{
  "grid_w": 100,
  "grid_h": 200,
  "cells": [
    { "cell_x": 25, "cell_y": 40, "count": 12345 }
  ]
}
```

3.8 Dashboard (Visualization)

Responsibilities

- Fetch aggregates from Query API
- Render heatmap overlay using `heatmap.js` on top of:
  - a screenshot image OR
  - a blank canvas matching the “screen” size

Flow

- User selects client/screen and time bucket
- Dashboard fetches corresponding aggregate cells
- Dashboard renders intensity map

4. Data Format Notes: JSON vs Parquet

Phase 1 (JSONLines)

- Easy to produce and inspect
- Spark reads with `spark.read.json(...)`

Phase 2 (Parquet)

- Enforce schema & types
- Faster reads via column pruning and predicate pushdown
- Spark reads with `spark.read.parquet(...)`

Migration strategy

- Run JSON landing first
- Add a parallel Parquet landing (new prefix or bucket), validate outputs
- Switch Spark hourly job input to Parquet once validated

5. Failure Handling (Implementation-Level)

- Ingestion crash: Kafka producer retries; at-least-once acceptable
- Landing restart: Kafka Connect resumes from offsets
- Spark job failure: rerun the job for the bucket (idempotency discussed below)
- ClickHouse: prefer insert patterns that avoid partial duplicates

Idempotency strategy (practical)

For the lab MVP, easiest:

- Make Spark jobs write into a staging table for that bucket and then swap/replace
- OR include a `run_id`/`version` column and query the latest version

We can choose a simple approach once implementation begins.

Phase 2: Batch-level idempotency at ingestion (optional)

If duplicate batches become a real issue (e.g., clients retry on slow networks), implement:

- Client supplies `batch_id` per request
- Ingestion checks Redis `SETNX(client_id:batch_id)` with TTL (e.g., 1–24 hours)
- If already seen, drop the batch

Avoid per-event dedup for mousemove telemetry; it requires too much state at scale.

6. Build Order (Critical)

Recommended implementation sequence:

- Infra: Kafka + MinIO + ClickHouse + Spark (compose)
- Kafka topic creation script
- Ingestion service (HTTP → Kafka)
- Demo client (mousemove → ingestion)
- Kafka Connect S3 sink (Kafka → MinIO JSON)
- Spark hourly job (MinIO JSON → ClickHouse hourly)
- Spark daily job (hourly → daily)
- Spark weekly job (daily → weekly)
- Query API
- Dashboard view

✅ Outcome of This Document

After this document:

- We know the exact APIs, storage layouts, and job boundaries.
- The system can be implemented incrementally and tested end-to-end.
