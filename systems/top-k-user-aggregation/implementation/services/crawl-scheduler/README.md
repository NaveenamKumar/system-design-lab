# Crawl Scheduler

DB-backed scheduler that polls PostgreSQL for ready crawl jobs and enqueues them to Asynq/Redis.

## How It Works

1. **Ready Jobs**: Finds `status='IDLE'` and `next_crawl_at <= NOW()`
2. **Stuck Jobs (Reconciliation)**: Finds `status='ENQUEUED'` and `updated_at` older than threshold
3. **Enqueue**: Pushes jobs to Asynq for crawl-worker to process

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `POSTGRES_URL` | `postgres://topk:topk@localhost:5432/topk?sslmode=disable` | PostgreSQL connection string |
| `REDIS_ADDR` | `localhost:6379` | Redis address for Asynq |
| `POLL_INTERVAL` | `10s` | How often to poll DB |
| `STUCK_THRESHOLD` | `1h` | How long before ENQUEUED is considered stuck |

## Why This Design?

- **DB is source of truth**: Schedule survives Redis restart
- **Reconciliation**: Automatically recovers lost jobs
- **No Redis persistence needed**: DB handles durability
