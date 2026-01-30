-- User Crawl Schedule Table
-- Source of truth for scheduling crawl jobs

CREATE TABLE IF NOT EXISTS user_crawl_schedule (
    user_id       TEXT NOT NULL,
    provider      TEXT NOT NULL,
    next_crawl_at TIMESTAMP NOT NULL DEFAULT NOW(),
    status        TEXT NOT NULL DEFAULT 'IDLE',  -- IDLE, ENQUEUED, RUNNING
    updated_at    TIMESTAMP NOT NULL DEFAULT NOW(),
    created_at    TIMESTAMP NOT NULL DEFAULT NOW(),
    last_error    TEXT,
    
    PRIMARY KEY (user_id, provider)
);

-- Index for scheduler: find ready jobs
CREATE INDEX IF NOT EXISTS idx_crawl_ready 
ON user_crawl_schedule (next_crawl_at, status)
WHERE status = 'IDLE';

-- Index for reconciliation: find stuck jobs
CREATE INDEX IF NOT EXISTS idx_crawl_stuck 
ON user_crawl_schedule (status, updated_at)
WHERE status = 'ENQUEUED';

-- Trigger to auto-update updated_at
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

DROP TRIGGER IF EXISTS update_user_crawl_schedule_updated_at ON user_crawl_schedule;
CREATE TRIGGER update_user_crawl_schedule_updated_at
    BEFORE UPDATE ON user_crawl_schedule
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Insert some test users for demo
INSERT INTO user_crawl_schedule (user_id, provider, next_crawl_at, status)
VALUES 
    ('user-001', 'spotify', NOW(), 'IDLE'),
    ('user-001', 'youtube', NOW(), 'IDLE'),
    ('user-002', 'spotify', NOW(), 'IDLE'),
    ('user-003', 'spotify', NOW() + INTERVAL '1 day', 'IDLE')  -- scheduled for tomorrow
ON CONFLICT (user_id, provider) DO NOTHING;
