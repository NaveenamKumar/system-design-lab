# Heatmap SDK — Implementation (local)

Local infrastructure + services for the Heatmap SDK system.

## Architecture (Phase 1)

```
demo-client (mousemove) → ingestion-service → Kafka → Kafka Connect (S3 sink) → MinIO
                                                          ↓
                                                   Spark hourly job
                                                          ↓
                                                    ClickHouse (hourly)
```

## Quick start

```bash
# 0. Create runtime directories (one-time)
chmod +x init-runtime-dirs.sh
./init-runtime-dirs.sh

# 1. Start infra (Kafka, MinIO, Connect, ClickHouse, Spark + services)
docker compose up --build

# 2. Create Kafka topics (one-time, in another terminal)
chmod +x create-topics.sh
./create-topics.sh

# 3. Register Kafka Connect S3 sink (Kafka -> MinIO)
chmod +x register-connectors.sh
./register-connectors.sh

# 4. Open demo client and start generating events
# - demo client: http://localhost:8090
# - ingestion service: http://localhost:8088/healthz

# 5. Run Spark hourly job for a specific UTC hour (example)
# (Pick dt/hour that matches the files created in MinIO)
docker compose run --rm spark-job \
  /opt/spark-jobs/heatmap_hourly.py --dt 2026-02-03 --hour 00
```

## Connection points (from host)

| Service | Address |
|--------|---------|
| Kafka | `localhost:29092` |
| Kafka Connect | `http://localhost:8083` |
| MinIO | `http://localhost:9000` |
| MinIO Console | `http://localhost:9001` |
| ClickHouse HTTP | `http://localhost:8123` |
| ClickHouse Native | `localhost:9002` |
| Spark Master UI | `http://localhost:8080` |
| Query API | `http://localhost:8091` |
| Dashboard UI | `http://localhost:8092` |
| Demo Client | `http://localhost:8090` |
| Tabix (ClickHouse UI) | `http://localhost:8093` |

