#!/usr/bin/env bash
set -euo pipefail

CONNECT_URL="${CONNECT_URL:-http://localhost:8083}"

echo "Registering S3 sink connector (JSON) at $CONNECT_URL ..."
curl -sS -X PUT \
  -H 'Content-Type: application/json' \
  --data-binary @schemas/kafka-connect/s3-sink-json.json \
  "$CONNECT_URL/connectors/heatmap-s3-sink-json/config"

echo "Done."

