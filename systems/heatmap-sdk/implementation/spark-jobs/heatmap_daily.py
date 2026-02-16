import argparse
import time
import urllib.request
import urllib.parse
from datetime import datetime, timezone

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
    parser = argparse.ArgumentParser(description="Daily heatmap rollup (hourly -> daily via ClickHouse).")
    parser.add_argument("--dt", required=True, help="UTC date YYYY-MM-DD to aggregate")
    parser.add_argument("--clickhouse-jdbc", default="jdbc:clickhouse://clickhouse:8123/heatmap")
    parser.add_argument("--clickhouse-http", default="http://clickhouse:8123")
    parser.add_argument("--clickhouse-user", default="heatmap")
    parser.add_argument("--clickhouse-password", default="heatmap")
    parser.add_argument("--source-table", default="heatmap_hourly")
    parser.add_argument("--target-table", default="heatmap_daily")
    args = parser.parse_args()

    # Validate date
    dt = datetime.strptime(args.dt, "%Y-%m-%d").date()

    spark = SparkSession.builder.appName("heatmap-daily").getOrCreate()

    # Read hourly rows for the given date from ClickHouse via JDBC
    query = f"""
    (SELECT client_id, screen_id,
            toStartOfDay(bucket_start) AS bucket_start,
            cell_x, cell_y, count
     FROM heatmap.{args.source_table}
     WHERE toDate(bucket_start) = '{dt.isoformat()}'
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

    # Group by day bucket and sum counts
    out = (
        df.groupBy("client_id", "screen_id", "bucket_start", "cell_x", "cell_y")
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
    bucket_str = f"{dt.isoformat()} 00:00:00"
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
