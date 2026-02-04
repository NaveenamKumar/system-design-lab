#!/usr/bin/env bash
set -euo pipefail

# Create runtime directories for Docker bind mounts.
# Keep it local to avoid sudo requirements.
RUNTIME_BASE="$(pwd)/.runtime"

echo "Creating runtime directories at $RUNTIME_BASE ..."

mkdir -p "$RUNTIME_BASE/zookeeper/data"
mkdir -p "$RUNTIME_BASE/zookeeper/log"
mkdir -p "$RUNTIME_BASE/kafka"
mkdir -p "$RUNTIME_BASE/kafka-connect"
mkdir -p "$RUNTIME_BASE/minio"
mkdir -p "$RUNTIME_BASE/clickhouse"

# Set permissions (Docker containers often run as specific users)
chmod -R 777 "$RUNTIME_BASE"

echo "Runtime directories created:"
ls -la "$RUNTIME_BASE"

