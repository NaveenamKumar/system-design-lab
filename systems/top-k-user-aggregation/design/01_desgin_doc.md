📄 Document 1
Design Document – Architecture & Invariants

(Vendor-agnostic, architecture-first)

Top-K Music Aggregation Platform (Pull-Based)

Author: Naveenam
Project: system-design-lab
Status: Design Complete
Scope: Architecture & System Invariants

1. Problem Overview

Music listening data today is siloed across providers (Spotify, Apple Music, YouTube Music). Each provider exposes limited, provider-specific views (e.g., “your top songs”), but there is no unified, cross-provider aggregation.

This system aggregates user listening history across multiple providers and exposes a Top-K listened songs view for the last 7 days, suitable for embedding into a social product (e.g., user profile view).

2. Goals
Functional Goals

Aggregate listening history across multiple providers

Compute Top-K songs per user for a rolling 7-day window

Support multiple providers per user

Serve results with low latency

Non-Functional Goals

Eventual consistency is acceptable

Up to 1 day staleness is acceptable

High availability > strict consistency

System must be horizontally scalable

Failures must not cause permanent data loss

3. Non-Goals

Real-time (< seconds) updates

Push-based ingestion (for now)

Music playback

Global analytics (e.g., global Top-K)

4. Core Architectural Principles (Invariants)

These principles must remain true, even as implementation details evolve.

4.1 Separation of Concerns

Ingestion: fetching data from providers

Processing: normalizing and aggregating data

Serving: responding to user-facing queries

These layers must not be coupled.

4.2 Event-Driven Core

All listen events flow through a durable event log that acts as the system’s backbone.

Invariant:

No component depends directly on another component’s in-memory state.

4.3 Raw Events Are the Source of Truth

Raw listen events are stored durably for a bounded time window.

Invariant:

Aggregated data must always be reproducible from raw events.

4.4 Aggregation Is Asynchronous

Aggregation is never performed on the user-facing read path.

Invariant:

User reads never scan raw listen events.

4.5 Time-Bounded Data

Listening history and aggregates are time-windowed.

Invariant:

All event and aggregate data must have explicit retention limits.

5. High-Level Architecture

```
User
  │
  ▼
OAuth Service ───► UserTokens (Control Plane DB)
                      │
                      ▼
               UserCrawlSchedule (Control Plane DB)
                      │
                      ▼
                    Scheduler
                      │
                      ▼
               Crawl Workers (Pull)
                      │
                      ▼
                 Event Log
                  ┌────┴────┐
                  ▼         ▼
       Raw Event Processor   Aggregation Service
              │                     │
              ▼                     ▼
     Raw Event Store        Aggregated Store
                                   │
                                   ▼
                           Top-K Read API
                                   │
                                   ▼
                                Cache
```

6. Data Flow (Conceptual)
6.1 OAuth & Setup

User authorizes a music provider

OAuth tokens are stored securely

A crawl schedule entry is created

6.2 Pull-Based Ingestion

Scheduler identifies users due for crawling

Crawl workers fetch listening history since last crawl

Workers normalize listen events

Events are appended to the event log

6.3 Processing & Aggregation

One consumer persists raw events

Another consumer aggregates daily counts per user/song

Aggregates are periodically flushed to durable storage

6.4 Serving

Read API fetches last 7 days of aggregates

Aggregation is performed in-memory on small datasets

Results are cached

7. Data Models (Logical)
7.1 Raw Listen Event
(user_id, provider, song_id, listened_at)

7.2 Daily Aggregate
(user_id, day, song_id) → listen_count

8. Consistency Model

Ingestion and aggregation are at-least-once

Small over-counts are acceptable

Aggregates are eventually consistent

Reads are consistent within a single request

9. Failure Model
Supported Failures

Worker crashes

Consumer restarts

Temporary provider outages

Partial data loss in cache

Guarantees

No permanent loss of raw events

Aggregates can be recomputed

User-facing APIs remain available

10. Why This Architecture Works

Scales horizontally

Is resilient to failures

Supports future push-based ingestion

Separates correctness from performance

Matches real-world production systems

11. Summary of Invariants

| Invariant | Description |
|-----------|-------------|
| Event log is the backbone | All data flows through it |
| Raw events are truth | Aggregates are derived |
| Async aggregation | Never on read path |
| Time-bounded storage | No infinite growth |
| Decoupled components | Easier evolution |
✅ Outcome of This Document

After this document, we are confident that:

The system architecture is correct, scalable, and evolvable, independent of specific technologies.