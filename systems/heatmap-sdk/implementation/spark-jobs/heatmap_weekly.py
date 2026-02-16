import argparse
import time
import urllib.request
import urllib.parse
from datetime import datetime, timedelta, timezone

from pyspark.sql import SparkSession, functions as F


def _ch_execute(url, user, password, sql):
    """Execute a SQL statement against ClickHouse HTTP API."""
    data = sql.encode("utf-8")
    req = urllib.request.Request(url, data=data, method="POST")
    req.add_header("X-ClickHouse-User", user)
    req.add_header("X-ClickHouse-Key", password)
    with urllib.request.urlopen(req) as resp:
        return resp.read().decode("utf-8").strip()


def delete_bucket_sync(url, user, password, table, where, timeout=60):
    """Delete rows matching `where` and wait for the mutation to complete."""
    _ch_execute(url, user, password, f"ALTER TABLE {table} DELETE WHERE {where}")
    deadline = time.time() + timeout
    while time.time() < deadline:
        result = _ch_execute(
            url, user, password,
            f"SELECT count() FROM system.mutations WHERE database='heatmap' AND table='{table.split('.')[-1]}' AND is_done=0",
        )
        if result == "0":
            return
        time.sleep(1)
    raise TimeoutError(f"Mutation on {table} not done within {timeout}s")


def main():
    parser = argparse.ArgumentParser(description="Weekly heatmap rollup (daily -> weekly via ClickHouse).")
    parser.add_argument("--week-start", required=True, help="Monday date YYYY-MM-DD (ISO week start)")
    parser.add_argument("--clickhouse-jdbc", default="jdbc:clickhouse://clickhouse:8123/heatmap")
    parser.add_argument("--clickhouse-http", default="http://clickhouse:8123")
    parser.add_argument("--clickhouse-user", default="heatmap")
    parser.add_argument("--clickhouse-password", default="heatmap")
    parser.add_argument("--source-table", default="heatmap_daily")
    parser.add_argument("--target-table", default="heatmap_weekly")
    args = parser.parse_args()

    # Validate that --week-start is a Monday
    week_start = datetime.strptime(args.week_start, "%Y-%m-%d").date()
    if week_start.weekday() != 0:
        raise ValueError(f"--week-start must be a Monday, got {week_start} ({week_start.strftime('%A')})")

    week_end = week_start + timedelta(days=6)

    spark = SparkSession.builder.appName("heatmap-weekly").getOrCreate()

    # Read daily rows for the 7-day range from ClickHouse via JDBC
    query = f"""
    (SELECT client_id, screen_id, cell_x, cell_y, count
     FROM heatmap.{args.source_table}
     WHERE toDate(bucket_start) BETWEEN '{week_start.isoformat()}' AND '{week_end.isoformat()}'
    ) AS sub
    """

    df = (
        spark.read.format("jdbc")
        .option("url", args.clickhouse_jdbc)
        .option("dbtable", query)
        .option("user", args.clickhouse_user)
        .option("password", args.clickhouse_password)
        .option("driver", "com.clickhouse.jdbc.ClickHouseDriver")
        .load()
    )

    # Group by week start (Monday) and sum counts
    out = (
        df.withColumn("bucket_start", F.lit(week_start.strftime("%Y-%m-%d 00:00:00")).cast("timestamp"))
        .groupBy("client_id", "screen_id", "bucket_start", "cell_x", "cell_y")
        .agg(F.sum("count").alias("count"))
        .select(
            F.col("client_id").cast("string"),
            F.col("screen_id").cast("string"),
            F.col("bucket_start").cast("timestamp"),
            F.col("cell_x").cast("int"),
            F.col("cell_y").cast("int"),
            F.col("count").cast("long"),
        )
    )

    # Delete existing rows for this bucket (idempotency: delete-then-insert)
    bucket_str = f"{week_start.isoformat()} 00:00:00"
    delete_bucket_sync(
        args.clickhouse_http, args.clickhouse_user, args.clickhouse_password,
        f"heatmap.{args.target_table}",
        f"bucket_start = toDateTime('{bucket_str}')",
    )

    (
        out.write.format("jdbc")
        .mode("append")
        .option("url", args.clickhouse_jdbc)
        .option("dbtable", f"heatmap.{args.target_table}")
        .option("user", args.clickhouse_user)
        .option("password", args.clickhouse_password)
        .option("driver", "com.clickhouse.jdbc.ClickHouseDriver")
        .save()
    )

    spark.stop()


if __name__ == "__main__":
    main()
