📄 Document 1
Design Document – Architecture & Invariants

(Vendor-agnostic, architecture-first)

Heatmap Analytics Platform (SDK-style, event-driven, batch aggregates)

Author: Naveenam
Project: system-design-lab
Status: Draft
Scope: Architecture & System Invariants

1. Problem Overview

Third-party applications want to understand user interaction patterns (e.g., mouse movement hotspots) on specific screens/pages.
We provide:

- An ingestion endpoint that receives high-volume interaction events from client apps
- A pipeline to store raw events durably for replay and debugging
- Periodic aggregation to produce heatmap tiles for hourly/daily/weekly buckets
- A query surface for dashboards to visualize heatmaps with acceptable staleness

2. Goals

Functional Goals

- Collect high-volume interaction events (mousemove) from multiple client applications (multi-tenant)
- Normalize coordinates so events are comparable across device/screen sizes
- Produce heatmap aggregates for:
  - Hourly buckets
  - Daily buckets (rollup from hourly)
  - Weekly buckets (rollup from daily)
- Serve aggregates to a dashboard (or API clients) efficiently

Non-Functional Goals

- Eventual consistency is acceptable
- Freshness: up to ~1 hour staleness is acceptable for hourly aggregates
- High availability > strict consistency (analytics pipeline)
- Horizontally scalable ingestion (burst handling)
- Failures must not cause permanent loss of raw events within retention window
- Aggregates must be reproducible from raw events

Scale Assumption (Interview-style)

- ~1B events/day (average ~11.6k events/sec), with bursty peaks

3. Non-Goals

- Real-time heatmap updates (< seconds)
- Strong exactly-once semantics end-to-end (at-least-once is acceptable)
- Per-event idempotency / strict deduplication for high-volume telemetry (deferred; see Phase 2)
- User authentication / OAuth flows (explicitly out of scope for this system)
- Privacy/legal/compliance deep dive (PII handling is acknowledged, not implemented)

4. Core Architectural Principles (Invariants)

These principles must remain true, even as implementation details evolve.

4.1 Separation of Concerns

- Ingestion: accept events, validate, append to log
- Landing: convert stream to durable “raw files” for batch processing
- Aggregation: compute heatmap tiles asynchronously
- Serving: query aggregates only (never raw scans on dashboard path)

Invariant:

User-facing reads must never scan the raw event store.

4.2 Event Log as the Buffer & Fan-out Boundary

All events flow through a durable event log before landing into object storage.

Invariant:

Producers (ingestion) are decoupled from consumers (landing/aggregation/QA tools).

4.3 Raw Events Are the Source of Truth

Object storage holds append-only raw events for a bounded retention window.

Invariant:

Aggregates are derived data and must be reproducible from raw events.

4.4 Aggregation Is Asynchronous and Bucketed

Heatmap aggregation is performed on a periodic schedule (hourly/daily/weekly).

Invariant:

Aggregation never blocks ingestion and never runs on the dashboard read path.

4.5 Time-Bounded Storage & Rollups

We keep finer-grained data for a shorter period and coarser rollups for longer.

Invariant:

Each store/table has explicit retention/TTL.

5. High-Level Architecture

```
Client (web demo / SDK)
  │
  ▼
Ingestion Service (HTTP)
  │
  ▼
Event Log (Kafka)  ──────────────► (optional) Debug Consumers
  │
  ▼
Landing to Object Storage (S3/MinIO)
  │
  ▼
Spark Batch Jobs
  │
  ├─ hourly aggregates  ─► heatmap_hourly (OLAP)
  ├─ daily rollup       ─► heatmap_daily  (OLAP)
  └─ weekly rollup      ─► heatmap_weekly (OLAP)
  │
  ▼
Query API / Dashboard
```

6. Data Flow (Conceptual)

6.1 Event Capture (Client)

- Client captures mousemove events on a defined “screen region”
- Client normalizes coordinates:
  - x_norm = x_px / width_px
  - y_norm = y_px / height_px
- Client batches/throttles events to reduce request overhead

6.2 Ingestion

- Ingestion service validates payloads (bounds checks, required fields)
- Ingestion service appends events to Kafka topic(s)
- At-least-once write to Kafka is acceptable

6.3 Landing to Object Storage

- A landing component reads Kafka and writes immutable files to object storage
- Files are partitioned by event time (and optionally client) to enable selective reads

6.4 Aggregation (Spark Batch)

- Hourly job reads the hour’s raw files, buckets normalized coords into a fixed grid, and writes `heatmap_hourly`
- Daily job reads `heatmap_hourly` and rolls up to `heatmap_daily`
- Weekly job reads `heatmap_daily` and rolls up to `heatmap_weekly`

6.5 Serving

- Dashboard/API queries aggregates from OLAP store with low latency
- Visualization renders a heatmap overlay from cell counts (no raw points)

7. Data Models (Logical)

7.1 Raw Interaction Event

(client_id, screen_id, event_time, x_norm, y_norm, event_type=mousemove)

7.2 Heatmap Cell Count (Hourly/Daily/Weekly)

(client_id, screen_id, bucket_start, cell_x, cell_y) → count

8. Consistency Model

- Ingestion and landing are at-least-once
- Aggregation is eventually consistent
- Small over-counts are acceptable (duplicates can be mitigated later; see Phase 2 idempotency)

Phase 2: Idempotency / Deduplication (Optional)

For mousemove-style telemetry, strict per-event dedup is usually too expensive at scale.
If duplicates become a practical problem (e.g., client retries batching aggressively), we can add batch-level idempotency:

- Client attaches a `batch_id` (UUID) to each `POST /events/batch`
- Ingestion performs a best-effort Redis `SETNX` on `(client_id, batch_id)` with a short TTL (e.g., 1–24 hours)
- Duplicate batches are dropped at ingestion

This improves correctness without requiring per-event dedup state for billions of events.

9. Failure Model

Supported Failures

- Ingestion node crash/restart
- Kafka consumer restarts
- Temporary object storage unavailability
- Spark job failure and retry
- OLAP store restart

Guarantees

- No permanent loss of raw events once they have been successfully appended to Kafka and landed to object storage
- Aggregates can be recomputed from raw object storage data (within retention window)
- Dashboard can tolerate stale data (up to ~1h for hourly)

10. Why This Architecture Works

- Scales ingestion independently from aggregation
- Uses durable boundaries (Kafka and object storage) to simplify failure recovery
- Enables reprocessing and schema evolution (raw events retained)
- Produces fast-to-query aggregates for visualization

11. Summary of Invariants

| Invariant | Description |
|-----------|-------------|
| Event log is the buffer boundary | Ingestion is decoupled from landing/aggregation |
| Raw events are truth | Aggregates are always reproducible |
| Async aggregation | Never on the read path |
| Time-bounded retention | Prevents unbounded growth |
| Rollups by bucket | Hourly → daily → weekly |

✅ Outcome of This Document

After this document, we are confident that:

- The architecture is correct, scalable, and evolvable without committing to specific tools.
