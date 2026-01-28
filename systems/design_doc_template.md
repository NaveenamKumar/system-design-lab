<System / Feature Name> — Design Document

Author: <Your Name>
Date: <YYYY-MM-DD>
Status: Draft / In Review / Approved
Reviewers: <Names>

1. Overview
1.1 Summary

Brief description of what is being built and why.

Example:
This document proposes a scalable email search architecture supporting 100M users with low-latency queries and efficient storage tiering.

1.2 Background / Context

What exists today? Why is change needed?

2. Problem Statement

Clearly define the problem.

What is broken or missing?

What user or business impact exists?

What constraints must be respected?

3. Goals and Non-Goals
3.1 Goals

Measurable and specific.

P99 latency < 200 ms

Support 2× data growth

Zero-downtime migration

3.2 Non-Goals

Explicitly state what is out of scope.

Not solving spam detection

No UI redesign in this phase

4. Requirements
4.1 Functional Requirements

What the system must do.

Search by keyword

Filter by date and sender

4.2 Non-Functional Requirements

System qualities.

Scalability

Reliability

Security

Cost constraints

5. High-Level Architecture
5.1 Architecture Diagram

(Insert block diagram here)

Components:

API Gateway

Search Service

Index Storage

Metadata Store

5.2 Data Flow

Step-by-step request flow.

Client sends query

API routes to Search Service

Cache lookup

Index query

Merge and return results

6. Detailed Component Design

Repeat this structure for each major component.

6.1 Component: <Name>
Responsibilities

What this component owns

What it does NOT own

Interfaces

APIs exposed

Events consumed/emitted

Schemas

Internal Design

Algorithms

Data structures

State management

Storage Model (if applicable)

Tables / indices

Partitioning strategy

Retention policy

Scaling Strategy

Horizontal scaling

Sharding keys

Backpressure handling

Failure Handling

Retries

Circuit breakers

Graceful degradation

Tradeoffs

Alternatives considered:

Option A

Option B

Chosen approach:

Explanation tied to requirements

Downsides:

What becomes worse or harder

Revisit if:

What future changes may require redesign

7. Data Model and Schema Design
7.1 Logical Data Model

Entities and relationships.

7.2 Physical Storage Layout

Tables

Indexes

TTLs

7.3 Schema Evolution

Backward compatibility

Versioning strategy

8. Consistency, Concurrency, and Ordering

Strong vs eventual consistency

Idempotency strategy

Conflict resolution

9. Scalability and Performance Analysis
9.1 Bottleneck Analysis

Potential hot spots.

9.2 Capacity Estimates

QPS

Storage size

Growth rate

9.3 Load Handling

Rate limiting

Queueing

Caching

10. Reliability and Fault Tolerance
10.1 Failure Scenarios

Node failures

Network partitions

Data corruption

10.2 Recovery Mechanisms

Replication

Reprocessing

Backups

10.3 Disaster Recovery

RPO / RTO targets

11. Security and Privacy

Authentication & authorization

Data encryption

PII handling

Compliance considerations

12. Observability

Metrics

Logs

Traces

Alerts

13. Testing Strategy
13.1 Unit Tests
13.2 Integration Tests
13.3 Load & Stress Testing
13.4 Failure Injection
14. Deployment and Rollout Plan

Feature flags

Canary release

Backward compatibility

15. Migration Plan (if replacing existing system)

Dual writes

Shadow reads

Cutover strategy

16. Risks and Mitigations
Risk	Impact	Mitigation
Index corruption	Data loss	Periodic snapshots
17. Open Questions

What still needs validation?

What depends on other teams?

18. Future Improvements

Phase 2 ideas

Deferred optimizations

Appendix A: Diagrams
Appendix B: Benchmarks
Appendix C: Glossary