Top K Multi-tenant music app Aggregation

Author: Naveenam
Date: 2026-01-22
Status: Draft (my scribbles)
Reviewers: TBD

1. Overview
1.1 Summary

Brief description of what is being built and why.

This document proposes a scalable top k multi-provider music app with oauth resource access.
Currently no of the music provider has API exposed to access user data. So we are going with simulated data and possible load testing scenarios to get as real as possible

1.2 Background / Context

What exists today? Why is change needed?
Currently each music app provide top k listens of the user self contained. This application is aimed to bring a unified topk account across mutiple user's music apps. This application/feature can be independently host and can be part of instagram under each user profile which is visible to other followers of that user. Think of this feature where we see what Taylor swift topk in the past 7 days. This itself will boom the music industry.

2. Problem Statement
Currently all the data in the instagram platform is self contained. We are accessing other information of users outside the social media. By integration with third party app we can understand the user behaviour better and may be use this data to while generating recommendation for users. Either feeds/reels.

The business impact would increased user engagement with user metrics alignment from songs angle.
The constraints to be respected is third party integration mechanisms to be considered.

Possible expansion: are we planning to expand meta music as a future product? meaning venturing into music domain also?

3. Goals and Non-Goals
3.1 Goals

Measurable and specific.

P99 latency < 200 ms

Support 2× data growth


3.2 Non-Goals

Explicitly state what is out of scope.
Not solving listening to topk music inside the current app like instagram. This is just listing the topk listens under each user's profile and visible to the followers.


4. Requirements
4.1 Functional Requirements

User should be able to intergrate with multiple third party music providers apps using oauth. No sso is expected.
User should be able to see the topk listens for the past 7 days as a list.
User should be able the topk of the users that they are following.
#  - song: title, description, singers, albums, listen_count, url, thumbnail_image, (fingerprint - internal uniq identifier of the song)


4.2 Non-Functional Requirements
500M active users in instagram
1/10 if the user does integration - 
500 * 10 ^ 6
-                - 50 Million users * 3 (3 music provider integration - spoitfy, apple, youtube)
10

Scalability - scalable to handle the scale of the users.

Reliability - the user data for the past listens has to be stored and results has to be accurate

Security - should safely integrate with third party app. No leaking of user data.

Consistency -  eventaul consistency
Availability - highly available (availabiltiy > consistency)
Durability - replica for the listens to handle physical failure scenario

Freshness - 1 day of stale data is fine. (1-7) - 8th day showing the result is fine
Datatype - we might be storing the thumbnail of the image - lets see


4.3 Entities

Song
Artist
Albumn
User
UserTokens (outh)
UserListenHistory


#Flow
1. User Outh

user -> thirparty provider -> callback (with access_token and access_token)

UserTokens
id
user_id
third_party_type: spotify/apple/youtube
access_token:
expires_at:
refresh_token:


UserListenHistory
Flow:
Discussion strategies:
pull/push model

Pull Model - /third_party/{user_id}/listen?since=epoch_second
pros:
simple
no additional integration efforts from third party system.
batch processing
third party rate limits can be an issue which might impact the data accuracy
cons:
data staleness depends on the pull schedule

Push Model - ingestion 
pros:
data freshness is near realtime - stream processing
no rate limit issue
cons:
third patry system has to build the integration efforts


tradeoff - if the scale is not much, then pull model is fine. We can start with pull model first and slowly evolve to push model.


User -> OuthService  -> Thirdparty
                         System
        /callback
        UserTokens table entry is created

Pull Model for UserListenHistory:

UserCrawlSchedule
user_id
crawl_window: 7
last_crawled_at: #now - 7 days in epoch
next_crawl_at: 
on create: next_crawl_at: now, last_crawled_at: now - ($crawl_window) days
after crawl: next_crawl_at: now + 1.day, last_crawled_at: now
user_token_id:  (linking to the third party provider)

After create of UserToken entry: Create UserCrawlInfo entry: with default

-> cron ->
user_crawl_info where next_crawl_at in the past:
enqueue to a queue

queue and worker we can scale based on the number of users/per third party

cron -> ---------{crawl_info_id}------ -> Worker

Worker Job:
Worker pickups the schedule entry:
access user token and fetch the api
the api can be scrolling api with
/third_party/{user_id}/listen?since=epoch?per_page=10andpage=1
/third_party/{user_id}/listen?since=epoch?cursor=history_id

edgecase: 
user currently listening - since one day staleness is fine,we can till=today.beginning_of_day_epoch-1.second
rate limiting - backoff strategy has to be implemeneted - user level or client level

UserListenHistory

{
meta_data: {
artist:
albumn:
thumbnail_image:
}
finger_print:
listen_at:
uploaded_at:
}

High write DB: we can write since the write is high, and maybe the data response might not be of the same structure:
dump this history to NoSQL db: (mango/cassandra): based on scale
add: user_id to the payload
add: song_id to the payload


UserListenHistory
song_id:
user_id:
finger_print:
listened_at:
uploaded_at:
third_patry_id: 

select song_id, count(*) user_listen_history
group by song_id where user_id = 1 order by count(*) desc limit 10;
similar equivalent query in NoSQL space:

Run and cache this query per day for celebrity users - FIFO access stragtey, edgecache per day

edge_case: user listening offline
that is where the source of truth should be on listened_at and not uploaded_at; this should handle the edge case.

Scale:
500M DAU
10% - integrate with third party - 50M - is this valid? considering the oauth integration ansd stuff, this shpould be reasonable
difference could be - out of 50M, 70% are actively listening to songs - 35M users
Assume avg song in 3min with 1.5hr listening per day - 90 minutes/day - 5400 seconds 
5400/210 - 25 songs/day
Add power users ~ 200 songs/day
35M/day - 30 listen/day - 1.05B events/day
writes/sec - 12,150 writes/sec NoSQL + batching

My calc:
Assuming each song in 5minute(avg)
86400/300- 288 songs max can be listened in a day by a person. * 3 if they are listening magically - 864 (~1000 songs) - high
initial the 7 days  this could be 7k entries. Then only incremental listens. Not worried about the write storage. Since we can cleanup/archive old data and not infinite growth.

Crawlers needed?
35M * 2 - 70M crawl job/day ( for 2 providers)
Each crawl give 50 listens. ~1 API call per user per provider
worst case - 5 calls (pagnition)
take avg 2 calls
70M * 2 - 140M crawl job/day
140m/86400 - 1,620 calls/sec - 540 RPS/provider

Assume one worker can do 10 calls/sec
each crawl ~ 2 API calls
1 worker per sec - 5 crawls/sec
70M/ 86400 - 810 crawls/sec
810/5 - 162 workers

Add retries, spikes, safety margin - 300-350 wokers total - 100 -120 workers per provider
“Yes, we can crawl all users daily.
With ~300 workers and controlled rate limiting per provider, the system can ingest ~1B listen events per day with acceptable staleness.”

If we can't
1. Prioritize active users/celebrity users
2. Increase crawl freq for low-activity users
3. Event-based refresh when profile is viewed.
4. Gradually migrate hot providers to push models

Need to understand the worker scale configuration needed to crawl for all the users and whether we can crawl for all the users
    

5. High-Level Architecture
5.1 Architecture Diagram

(Insert block diagram here)

Components:

API Gateway


5.2 Data Flow

Step-by-step request flow.


6. Detailed Component Design

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


#simulation in real world.
redis instance needed calc:
1 worker - 5crawl/sec (10 API calls/sec)
300 workers - 1500 crawls/sec

How heavy is one worker:
CPU - 0.1 - 0.2 vCPU
memory - approx 300MB
network-bound 
10-20 workers per vCPU is realistic
EC2:
c6i.4xlarge
16 vCPU
32 GB RAM

16 vCPU × 10 workers/vCPU = 160 workers/node
160 × 300MB ≈ 48GB ❌ (too much) memory constraint
safe 100 workers/node
300/100  - 3 EC2 instances
Add redundacny - (4-5 instances)
