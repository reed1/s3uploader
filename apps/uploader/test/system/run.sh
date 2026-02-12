#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Load .env if present
if [ -f "$SCRIPT_DIR/.env" ]; then
    set -a
    source "$SCRIPT_DIR/.env"
    set +a
fi

# Validate required env vars
for var in S3_ACCESS_KEY_ID S3_SECRET_ACCESS_KEY S3_REGION S3_ENDPOINT S3_BUCKET; do
    if [ -z "${!var:-}" ]; then
        echo "ERROR: $var is not set. Copy .env.example to .env and fill in credentials."
        exit 1
    fi
done

if [ "$S3_BUCKET" != "r-testing" ]; then
    echo "ERROR: S3_BUCKET must be 'r-testing', got '$S3_BUCKET'. Refusing to run against a non-testing bucket."
    exit 1
fi

cleanup() {
    echo "==> Cleaning up containers..."
    docker compose -f "$SCRIPT_DIR/docker-compose.yml" down -v --remove-orphans 2>/dev/null || true
}
trap cleanup EXIT

echo "==> Generating server.yaml..."
cat <<EOF > "$SCRIPT_DIR/server.yaml"
server:
  host: "0.0.0.0"
  port: 8080

s3:
  endpoint: "$S3_ENDPOINT"
  region: "$S3_REGION"
  bucket: "$S3_BUCKET"
  path_prefix: "system-test/"
  access_key_id: "$S3_ACCESS_KEY_ID"
  secret_access_key: "$S3_SECRET_ACCESS_KEY"

database:
  path: "/data/server.db"

clients_config: "/etc/s3uploader/clients.yaml"
EOF

echo "==> Building Linux binaries..."
cd "$REPO_ROOT"
just build-linux

echo "==> Building and starting containers..."
docker compose -f "$SCRIPT_DIR/docker-compose.yml" build
docker compose -f "$SCRIPT_DIR/docker-compose.yml" up -d server

echo "==> Waiting for server health..."
for i in $(seq 1 30); do
    if docker compose -f "$SCRIPT_DIR/docker-compose.yml" exec -T server wget -q --spider http://localhost:8080/health 2>/dev/null; then
        echo "    Server healthy"
        break
    fi
    if [ "$i" -eq 30 ]; then
        echo "ERROR: Server failed to become healthy"
        docker compose -f "$SCRIPT_DIR/docker-compose.yml" logs server
        exit 1
    fi
    sleep 1
done

echo "==> Starting client..."
docker compose -f "$SCRIPT_DIR/docker-compose.yml" up -d client

# Brief pause to let client initialize watcher
sleep 2

echo "==> Running validation..."
docker compose -f "$SCRIPT_DIR/docker-compose.yml" run --rm test-runner /validate.sh
RESULT=$?

if [ $RESULT -eq 0 ]; then
    echo ""
    echo "==> SYSTEM TEST PASSED"
else
    echo ""
    echo "==> SYSTEM TEST FAILED"
    echo "==> Server logs:"
    docker compose -f "$SCRIPT_DIR/docker-compose.yml" logs server
    echo "==> Client logs:"
    docker compose -f "$SCRIPT_DIR/docker-compose.yml" logs client
fi

exit $RESULT
