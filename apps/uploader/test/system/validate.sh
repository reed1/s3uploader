#!/bin/sh
set -eu

API_KEY="sk_test_system_integration"
SERVER="http://server:8080"
S3_PREFIX="system-test/test-client"
FAILED=0

export AWS_ACCESS_KEY_ID="$S3_ACCESS_KEY_ID"
export AWS_SECRET_ACCESS_KEY="$S3_SECRET_ACCESS_KEY"

cleanup_s3() {
    echo "==> Cleaning up S3 objects..."
    aws s3 rm "s3://${S3_BUCKET}/${S3_PREFIX}/" --recursive \
        --endpoint-url "$S3_ENDPOINT" --region "$S3_REGION" 2>/dev/null || true
}
trap cleanup_s3 EXIT

assert_hash() {
    local label="$1" expected="$2" actual="$3"
    if [ "$expected" = "$actual" ]; then
        echo "    PASS: $label hash matches"
    else
        echo "    FAIL: $label hash mismatch (expected $expected, got $actual)"
        FAILED=1
    fi
}

check_exists() {
    local filepath="$1"
    BODY=$(curl -s -H "Authorization: Bearer $API_KEY" "${SERVER}/exists?path=${filepath}")
    echo "$BODY" | grep -q '"exists":true'
}

# --- Step 0: Clean any leftover S3 objects from previous runs ---
echo "==> Cleaning any leftover S3 objects..."
aws s3 rm "s3://${S3_BUCKET}/${S3_PREFIX}/" --recursive \
    --endpoint-url "$S3_ENDPOINT" --region "$S3_REGION" 2>/dev/null || true

# --- Step 1: Create test files ---
echo "==> Creating test files..."

echo "hello from system test" > /watch/hello.txt
HASH_HELLO=$(sha256sum /watch/hello.txt | cut -d' ' -f1)

mkdir -p /watch/subdir
echo "nested file content" > /watch/subdir/nested.txt
HASH_NESTED=$(sha256sum /watch/subdir/nested.txt | cut -d' ' -f1)

dd if=/dev/urandom of=/watch/binary.bin bs=1024 count=1 2>/dev/null
HASH_BINARY=$(sha256sum /watch/binary.bin | cut -d' ' -f1)

echo "    Created 3 test files"

# --- Step 2: Wait for uploads via server API ---
echo "==> Waiting for uploads to complete..."

FILES="hello.txt subdir/nested.txt binary.bin"
TIMEOUT=60
ELAPSED=0

while true; do
    ALL_EXIST=1
    for f in $FILES; do
        if ! check_exists "$f"; then
            ALL_EXIST=0
            break
        fi
    done

    if [ "$ALL_EXIST" -eq 1 ]; then
        echo "    All files reported as uploaded (${ELAPSED}s)"
        break
    fi

    ELAPSED=$((ELAPSED + 2))
    if [ "$ELAPSED" -ge "$TIMEOUT" ]; then
        echo "    FAIL: Timed out waiting for uploads after ${TIMEOUT}s"
        for f in $FILES; do
            if ! check_exists "$f"; then
                echo "      Missing: $f"
            fi
        done
        FAILED=1
        break
    fi
    sleep 2
done

# --- Step 3: Verify via server /download ---
echo "==> Verifying downloads through server API..."

if [ "$FAILED" -eq 0 ]; then
    HASH=$(curl -sf -H "Authorization: Bearer $API_KEY" "${SERVER}/download?path=hello.txt" | sha256sum | cut -d' ' -f1)
    assert_hash "hello.txt (server)" "$HASH_HELLO" "$HASH"

    HASH=$(curl -sf -H "Authorization: Bearer $API_KEY" "${SERVER}/download?path=subdir/nested.txt" | sha256sum | cut -d' ' -f1)
    assert_hash "subdir/nested.txt (server)" "$HASH_NESTED" "$HASH"

    HASH=$(curl -sf -H "Authorization: Bearer $API_KEY" "${SERVER}/download?path=binary.bin" | sha256sum | cut -d' ' -f1)
    assert_hash "binary.bin (server)" "$HASH_BINARY" "$HASH"
fi

# --- Step 4: Verify directly from S3 ---
echo "==> Verifying files directly from S3..."

if [ "$FAILED" -eq 0 ]; then
    HASH=$(aws s3 cp "s3://${S3_BUCKET}/${S3_PREFIX}/hello.txt" - \
        --endpoint-url "$S3_ENDPOINT" --region "$S3_REGION" | sha256sum | cut -d' ' -f1)
    assert_hash "hello.txt (S3 direct)" "$HASH_HELLO" "$HASH"

    HASH=$(aws s3 cp "s3://${S3_BUCKET}/${S3_PREFIX}/subdir/nested.txt" - \
        --endpoint-url "$S3_ENDPOINT" --region "$S3_REGION" | sha256sum | cut -d' ' -f1)
    assert_hash "subdir/nested.txt (S3 direct)" "$HASH_NESTED" "$HASH"

    HASH=$(aws s3 cp "s3://${S3_BUCKET}/${S3_PREFIX}/binary.bin" - \
        --endpoint-url "$S3_ENDPOINT" --region "$S3_REGION" | sha256sum | cut -d' ' -f1)
    assert_hash "binary.bin (S3 direct)" "$HASH_BINARY" "$HASH"
fi

# --- Result ---
if [ "$FAILED" -eq 0 ]; then
    echo ""
    echo "All checks passed."
    exit 0
else
    echo ""
    echo "Some checks failed."
    exit 1
fi
