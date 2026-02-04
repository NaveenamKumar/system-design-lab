-- Heatmap aggregates (Phase 1)
-- Notes:
-- - Separate tables for hourly/daily/weekly to allow different TTLs and clear rollup boundaries.
-- - For the lab, keep TTLs commented out initially (optional).

CREATE DATABASE IF NOT EXISTS heatmap;

CREATE TABLE IF NOT EXISTS heatmap.heatmap_hourly
(
  client_id String,
  screen_id String,
  bucket_start DateTime,
  cell_x Int32,
  cell_y Int32,
  count Int64
)
ENGINE = MergeTree
PARTITION BY toYYYYMMDD(bucket_start)
ORDER BY (client_id, screen_id, bucket_start, cell_x, cell_y);

CREATE TABLE IF NOT EXISTS heatmap.heatmap_daily
(
  client_id String,
  screen_id String,
  bucket_start DateTime,
  cell_x Int32,
  cell_y Int32,
  count Int64
)
ENGINE = MergeTree
PARTITION BY toYYYYMMDD(bucket_start)
ORDER BY (client_id, screen_id, bucket_start, cell_x, cell_y);

CREATE TABLE IF NOT EXISTS heatmap.heatmap_weekly
(
  client_id String,
  screen_id String,
  bucket_start DateTime,
  cell_x Int32,
  cell_y Int32,
  count Int64
)
ENGINE = MergeTree
PARTITION BY toYYYYMMDD(bucket_start)
ORDER BY (client_id, screen_id, bucket_start, cell_x, cell_y);

