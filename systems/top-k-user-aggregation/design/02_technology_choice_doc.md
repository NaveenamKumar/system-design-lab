Document 2
Technology Choices Document – Concrete Tools & Tradeoffs

(Opinionated, context-aware, but still reversible)

Top-K Music Aggregation Platform

Author: Naveenam
Project: system-design-lab
Scope: Technology Selection & Tradeoffs
Audience: Builders / Reviewers

1. Purpose of This Document

This document answers:

Given the architecture and invariants, what concrete technologies do we choose to build this system today?

This is not about “best in general”, but about:

building a real system

running it locally

learning from production-like behavior

keeping the design evolvable

All choices here are explicitly allowed to change later.

2. Constraints & Context

Single-developer / lab project

Must run locally (Docker / Compose)

Must simulate production-scale patterns

Operational simplicity > theoretical purity

Cloud neutrality (not AWS-only)

3. Technology Decisions (By Layer)
3.1 Event Log (Durability Boundary)
Requirement

Append-only log

Replayable

Multiple independent consumers

Ordering per partition

Backpressure support

Options Considered
Option	Pros	Cons
Redis Streams	Simple, fast	Memory-bound, weak replay
SQS	Managed, simple	No replay, poor fan-out
Kafka	Durable, replayable	Ops overhead
✅ Decision

Kafka

Rationale

Central to event-driven architecture

Enables replay and fan-out

Widely used in real systems

Excellent learning value

Tradeoffs

More operational overhead

Requires Docker setup locally

3.2 Raw Event Storage (UserListenHistory)
Requirement

Very high write throughput

Append-only

TTL-based retention

Partitioned access (user, day)

Not queried by user-facing APIs

Options Considered
Option	Pros	Cons
Relational DB	Familiar	Poor write scalability
MongoDB	Flexible	Harder TTL semantics
Wide-column NoSQL	Scales writes	Operational complexity
✅ Decision (Abstract)

Wide-column NoSQL

Local Development Choice

Apache Cassandra (Docker)

Rationale

Write-optimized

Natural TTL support

Matches production patterns

Tradeoffs

More complex than SQL

Requires careful schema design

3.3 Aggregated Storage (UserDailyTopK)
Requirement

Small, bounded datasets

Read-heavy

Low latency

TTL-based sliding window

Decision

Same wide-column NoSQL technology as raw events

Rationale

Operational simplicity

Identical access patterns

Predictable performance

Why NOT OLAP

OLAP optimized for analytics, not serving

Higher latency

Overkill for Top-K reads

3.4 Control Plane Storage (OAuth, Scheduling)
Requirement

Strong consistency

Atomic updates

Low write volume

Clear relational semantics

Decision

Relational Database (PostgreSQL)

Rationale

Simple transactional semantics

Easy to reason about state transitions

Excellent local dev support

3.5 Cache (Read Optimization)
Requirement

Fast reads

TTL support

Best-effort consistency

Decision

Redis

Rationale

Simple

Widely used

Perfect for caching Top-K results

3.6 Programming Language
Options Considered
Language	Pros	Cons
Java	Mature, performant	Verbose
Go	Simple, fast, great concurrency	Smaller ecosystem
Python	Fast iteration	Slower, memory-heavy
✅ Decision

Go

Rationale

Excellent concurrency model

Lightweight services

Great Kafka clients

Simple deployment

3.7 Service Communication
Decision

HTTP/JSON for APIs

Kafka for async communication

Rationale

Simplicity

Debuggability

Interoperability

4. Summary of Technology Stack
Layer	Technology
Event Log	Kafka
Raw Event Store	Cassandra
Aggregate Store	Cassandra
Control Plane DB	PostgreSQL
Cache	Redis
Language	Go
Orchestration (local)	Docker Compose
5. Explicit Non-Decisions (Intentional)

The following are explicitly deferred:

Cloud provider (AWS/GCP/Azure)

Managed vs self-hosted Kafka

Exactly-once semantics

Push-based ingestion

These are future evolution decisions, not MVP blockers.

6. Why This Stack Is “Correct” for a Lab

Mirrors real production systems

Forces correct architectural thinking

Supports failure testing & replay

Teachable and debuggable

Evolves cleanly to push-based ingestion

7. Key Takeaways

Architecture drives technology, not vice versa

Kafka is the backbone, not a convenience

NoSQL is chosen by access pattern

SQL is reserved for control-plane state

Simplicity beats premature optimization

✅ Outcome of This Document

After this document, we know:

Exactly what technologies we will use and why — without compromising architectural integrity.