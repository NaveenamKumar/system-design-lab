import argparse
from datetime import datetime, timezone

from pyspark.sql import SparkSession, functions as F, types as T


GRID_W = 100
GRID_H = 200


def clamp_col(col, lo, hi):
    return F.when(col < lo, lo).when(col > hi, hi).otherwise(col)


def main():
    parser = argparse.ArgumentParser(description="Hourly heatmap aggregation (raw -> ClickHouse hourly).")
    parser.add_argument("--s3-endpoint", default="http://minio:9000")
    parser.add_argument("--s3-bucket", default="heatmap-raw")
    parser.add_argument("--topics-dir", default="topics", help="Kafka Connect topics.dir (default: topics)")
    parser.add_argument("--topic", default="heatmap.events.raw", help="Kafka topic name as landed by Connect")
    parser.add_argument("--dt", required=True, help="UTC date YYYY-MM-DD")
    parser.add_argument("--hour", required=True, help="UTC hour HH (00-23)")
    parser.add_argument("--clickhouse-url", default="jdbc:clickhouse://clickhouse:8123/heatmap")
    parser.add_argument("--clickhouse-user", default="heatmap")
    parser.add_argument("--clickhouse-password", default="heatmap")
    parser.add_argument("--target-table", default="heatmap_hourly")
    args = parser.parse_args()

    # Validate bucket start
    bucket_start = datetime.strptime(f"{args.dt}T{args.hour}:00:00", "%Y-%m-%dT%H:%M:%S").replace(tzinfo=timezone.utc)

    spark = (
        SparkSession.builder.appName("heatmap-hourly")
        # S3A settings for MinIO
        .config("spark.hadoop.fs.s3a.endpoint", args.s3_endpoint)
        .config("spark.hadoop.fs.s3a.path.style.access", "true")
        .config("spark.hadoop.fs.s3a.connection.ssl.enabled", "false")
        .config("spark.hadoop.fs.s3a.access.key", "minio")
        .config("spark.hadoop.fs.s3a.secret.key", "minio123")
        .getOrCreate()
    )

    # Raw event schema as landed (JSONLines values from Kafka)
    schema = T.StructType(
        [
            T.StructField("client_id", T.StringType(), False),
            T.StructField("screen_id", T.StringType(), False),
            T.StructField("batch_id", T.StringType(), True),
            T.StructField("event_time", T.StringType(), False),
            T.StructField("x_norm", T.DoubleType(), False),
            T.StructField("y_norm", T.DoubleType(), False),
            T.StructField("event_type", T.StringType(), False),
            T.StructField("received_at", T.StringType(), True),
        ]
    )

    base = f"s3a://{args.s3_bucket}/{args.topics_dir.strip('/')}/{args.topic}"

    # Kafka Connect TimeBasedPartitioner writes keys under dt=.../hour=...
    input_path = f"{base}/dt={args.dt}/hour={args.hour}"

    df = spark.read.schema(schema).json(input_path)

    # Future: if we include client_id/screen_id at record-level, use them directly.
    # For now, Spark job will attach placeholders (lab), and we'll fix this after we verify Connect pathing.
    df2 = (
        df.filter(F.col("event_type") == F.lit("mousemove"))
        .withColumn("cell_x_raw", F.floor(F.col("x_norm") * F.lit(GRID_W)).cast("int"))
        .withColumn("cell_y_raw", F.floor(F.col("y_norm") * F.lit(GRID_H)).cast("int"))
        .withColumn("cell_x", clamp_col(F.col("cell_x_raw"), F.lit(0), F.lit(GRID_W - 1)).cast("int"))
        .withColumn("cell_y", clamp_col(F.col("cell_y_raw"), F.lit(0), F.lit(GRID_H - 1)).cast("int"))
        .withColumn("bucket_start", F.lit(bucket_start.strftime("%Y-%m-%d %H:%M:%S")).cast("timestamp"))
    )

    out = (
        df2.groupBy("client_id", "screen_id", "bucket_start", "cell_x", "cell_y")
        .agg(F.count(F.lit(1)).alias("count"))
        .select(
            F.col("client_id").cast("string"),
            F.col("screen_id").cast("string"),
            F.col("bucket_start").cast("timestamp"),
            F.col("cell_x").cast("int"),
            F.col("cell_y").cast("int"),
            F.col("count").cast("long"),
        )
    )

    (
        out.write.format("jdbc")
        .mode("append")
        .option("url", args.clickhouse_url)
        .option("dbtable", f"heatmap.{args.target_table}")
        .option("user", args.clickhouse_user)
        .option("password", args.clickhouse_password)
        .option("driver", "com.clickhouse.jdbc.ClickHouseDriver")
        .save()
    )

    spark.stop()


if __name__ == "__main__":
    main()

