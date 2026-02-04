FROM apache/spark:3.5.8-scala2.12-java11-python3-ubuntu

USER root

# ClickHouse JDBC driver (needed on executors too for DataFrameWriter.jdbc)
ADD https://repo1.maven.org/maven2/com/clickhouse/clickhouse-jdbc/0.9.5/clickhouse-jdbc-0.9.5-all.jar /opt/spark/jars/clickhouse-jdbc-0.9.5-all.jar

# Hadoop AWS + AWS SDK bundle for S3A access (MinIO)
ADD https://repo1.maven.org/maven2/org/apache/hadoop/hadoop-aws/3.3.4/hadoop-aws-3.3.4.jar /opt/spark/jars/hadoop-aws-3.3.4.jar
ADD https://repo1.maven.org/maven2/com/amazonaws/aws-java-sdk-bundle/1.12.262/aws-java-sdk-bundle-1.12.262.jar /opt/spark/jars/aws-java-sdk-bundle-1.12.262.jar

RUN chmod 644 /opt/spark/jars/hadoop-aws-3.3.4.jar /opt/spark/jars/aws-java-sdk-bundle-1.12.262.jar /opt/spark/jars/clickhouse-jdbc-0.9.5-all.jar

USER 185

