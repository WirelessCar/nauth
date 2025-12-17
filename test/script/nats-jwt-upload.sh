#!/bin/bash
# Usage: nats-jwt-upload.sh <jwt-file>
# Example: ./nats-jwt-upload.sh /tmp/account.jwt
#
# This script uploads an account JWT to the NATS server using the system account credentials.
# It mimics what the nauth operator does when it calls UploadAccountJWT().
# Note: NATS server and system account credentials are always in the 'nats' namespace.

set -euo pipefail

if [ "$#" -ne 1 ]; then
  echo "[$0] Usage: $0 <jwt-file>" >&2
  exit 1
fi

JWT_FILE="$1"

# NATS server and operator are always deployed in the 'nats' namespace
NATS_NAMESPACE="nats"

if [ ! -f "$JWT_FILE" ]; then
  echo "[$0] ERROR: JWT file not found: $JWT_FILE" >&2
  echo "[$0] Absolute path would be: $(realpath "$JWT_FILE" 2>&1 || echo 'cannot resolve')" >&2
  exit 2
fi

# Extract JWT filename for use in temporary file names
JWT_FILENAME=$(basename "$JWT_FILE")

# Get the system account credentials from the secret in the nats namespace
CREDS_SECRET=$(kubectl get secret -n "$NATS_NAMESPACE" \
  -l "nauth.io/secret-type=system-account-user-creds" \
  -o jsonpath='{.items[0].metadata.name}' 2>&1)

CREDS_SECRET_EXIT_CODE=$?

if [ $CREDS_SECRET_EXIT_CODE -ne 0 ] || [ -z "$CREDS_SECRET" ] || echo "$CREDS_SECRET" | grep -q "error:"; then
  echo "[$0] ERROR: System account credentials secret not found in namespace $NATS_NAMESPACE" >&2
  kubectl get secrets -n "$NATS_NAMESPACE" 2>&1 | head -20 >&2
  exit 3
fi

# Extract the credentials to a temp file
CREDS_FILE=$(mktemp)
trap 'rm -f "$CREDS_FILE"' EXIT

kubectl get secret -n "$NATS_NAMESPACE" "$CREDS_SECRET" -o jsonpath='{.data.default}' | base64 -d > "$CREDS_FILE"


# Instead of running nats CLI locally, we'll use kubectl exec to run it inside a pod
# This avoids DNS and networking issues when running outside the cluster

# Find nats-box pod by name pattern (it has nats CLI installed)
EXEC_POD=$(kubectl get pods -n "$NATS_NAMESPACE" --field-selector=status.phase=Running -o name 2>&1 | grep "pod/nats-box" | head -1 | cut -d'/' -f2)

if [ -z "$EXEC_POD" ]; then
  echo "[$0] ERROR: nats-box pod not found in namespace $NATS_NAMESPACE" >&2
  echo "[$0] Available pods:" >&2
  kubectl get pods -n "$NATS_NAMESPACE" -o wide 2>&1 >&2
  echo "[$0] Note: nats-box pod is required (has nats CLI installed)" >&2
  exit 6
fi

# Copy credentials to the pod (using PID and JWT filename for unique identification)
REMOTE_CREDS_PATH="/tmp/nauth-upload-creds-$$-${JWT_FILENAME%.jwt}.default"
kubectl cp "$CREDS_FILE" "$NATS_NAMESPACE/$EXEC_POD:$REMOTE_CREDS_PATH" 2>&1 | grep -v "tar:" || true

# Copy JWT to the pod (using PID and JWT filename for unique identification)
REMOTE_JWT_PATH="/tmp/nauth-upload-$$-${JWT_FILENAME%.jwt}.jwt"
kubectl cp "$JWT_FILE" "$NATS_NAMESPACE/$EXEC_POD:$REMOTE_JWT_PATH" 2>&1 | grep -v "tar:" || true

# Use the cluster-internal DNS name
NATS_URL="nats://nats.${NATS_NAMESPACE}.svc.cluster.local:4222"

# Use nats CLI inside the pod to publish the JWT to $SYS.REQ.CLAIMS.UPDATE
# Note: JWT must be passed as an argument, not via stdin
set +e  # Don't exit on error, capture it
RESPONSE=$(kubectl exec -i -n "$NATS_NAMESPACE" "$EXEC_POD" -- sh -c \
  "nats req --creds='$REMOTE_CREDS_PATH' --server='$NATS_URL' '\$SYS.REQ.CLAIMS.UPDATE' \"\$(cat '$REMOTE_JWT_PATH')\"" 2>&1)
NATS_EXIT_CODE=$?
set -e

# Cleanup remote files
kubectl exec -n "$NATS_NAMESPACE" "$EXEC_POD" -- rm -f "$REMOTE_CREDS_PATH" "$REMOTE_JWT_PATH" 2>/dev/null || true

if [ $NATS_EXIT_CODE -ne 0 ]; then
  echo "[$0] ERROR: nats CLI command failed" >&2
  echo "[$0] Response: $RESPONSE" >&2
  exit 4
fi

# Check if the response indicates success (code 200)
if echo "$RESPONSE" | grep -q '"code":200'; then
  exit 0
else
  echo "[$0] ERROR: Failed to upload account JWT" >&2
  echo "[$0] Response: $RESPONSE" >&2
  exit 4
fi

