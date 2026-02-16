-- Heatmap aggregates
-- Notes:
-- - Separate tables for hourly/daily/weekly to allow different TTLs and clear rollup boundaries.
-- - TTLs: hourly 90 days, daily 2 years (730d), weekly 5 years (1825d).

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
ORDER BY (client_id, screen_id, bucket_start, cell_x, cell_y)
TTL bucket_start + INTERVAL 90 DAY;

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
ORDER BY (client_id, screen_id, bucket_start, cell_x, cell_y)
TTL bucket_start + INTERVAL 730 DAY;

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
ORDER BY (client_id, screen_id, bucket_start, cell_x, cell_y)
TTL bucket_start + INTERVAL 1825 DAY;

