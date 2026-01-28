# Top-K infra (local)

This folder contains the local infrastructure needed to run the system:
- Kafka + Zookeeper
- Postgres
- Cassandra
- Redis

## Start infra
```bash
cd /home/naveenam.k/personal/prep/system-design-lab/systems/top-k-user-aggregation/implementation
docker compose up
```

## Connection points
- Kafka: `localhost:29092`
- Postgres: `postgresql://topk:topk@localhost:5432/topk`
- Cassandra: `localhost:9042`
- Redis: `localhost:6379`

## Create Kafka topics
```bash
./create-topics.sh
```

