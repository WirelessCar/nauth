#!/bin/bash
# Usage: nats-jwt-assert.sh <account-id> <expect-present>
#    OR: nats-jwt-assert.sh <namespace> <account-name> <expect-present>
# Example: ./nats-jwt-assert.sh ADDS6I3G7LBIBNDMZ5Q32VUJN2XNSW2QK4HY2SY5ZA7I2JLBFYF4KJDO TRUE
# Example: ./nats-jwt-assert.sh my-namespace example-account FALSE
#
# This script asserts whether an account JWT is present or absent on the NATS server.
# It queries the NATS server using $SYS.REQ.CLAIMS.LIST to look up the account.
#
# If 2 parameters: <account-id> <expect-present>
# If 3 parameters: <namespace> <account-name> <expect-present>
#   - The Account ID will be extracted from the Account resource's label
#
# Note: NATS server and system account credentials are always in the 'nats' namespace.

set -euo pipefail

if [ "$#" -ne 2 ] && [ "$#" -ne 3 ]; then
  echo "[$0] Usage: $0 <account-id> <expect-present>" >&2
  echo "[$0]    OR: $0 <namespace> <account-name> <expect-present>" >&2
  echo "[$0] Example: $0 ADDS6I3G7LBIBNDMZ5Q32VUJN2XNSW2QK4HY2SY5ZA7I2JLBFYF4KJDO TRUE" >&2
  echo "[$0] Example: $0 my-namespace example-account FALSE" >&2
  exit 1
fi

# Determine if we have 2 or 3 parameters
if [ "$#" -eq 2 ]; then
  # Direct Account ID provided
  ACCOUNT_ID="$1"
  EXPECT_PRESENT="$2"
else
  # Namespace and account name provided, extract Account ID
  NAMESPACE="$1"
  ACCOUNT_NAME="$2"
  EXPECT_PRESENT="$3"

  # Get the directory of this script to find nauth-account-id.sh
  SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

  # Try to resolve from Account resource first, fall back to temp file if resource doesn't exist
  # Redirect stderr to /dev/null for RESOLVE, but preserve it for READ for debugging
  ACCOUNT_ID=$("$SCRIPT_DIR/nauth-account-id.sh" RESOLVE "$NAMESPACE" "$ACCOUNT_NAME" 2>/dev/null || \
                "$SCRIPT_DIR/nauth-account-id.sh" READ "$NAMESPACE" "$ACCOUNT_NAME")

  if [ -z "$ACCOUNT_ID" ]; then
    echo "[$0] ERROR: Account ID not found for account '$ACCOUNT_NAME' in namespace '$NAMESPACE'" >&2
    echo "[$0] Neither Account resource nor temp file found" >&2
    exit 2
  fi
fi

# Normalize expect-present to uppercase for comparison
EXPECT_PRESENT_UPPER=$(echo "$EXPECT_PRESENT" | tr '[:lower:]' '[:upper:]')

# Validate the expect-present parameter
if [ "$EXPECT_PRESENT_UPPER" != "TRUE" ] && [ "$EXPECT_PRESENT_UPPER" != "FALSE" ]; then
  echo "[$0] ERROR: expect-present parameter must be 'TRUE' or 'FALSE' (case insensitive), got: $EXPECT_PRESENT" >&2
  exit 1
fi

# NATS server and operator are always deployed in the 'nats' namespace
NATS_NAMESPACE="nats"

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

# Find nats-box pod by name pattern (it has nats CLI installed)
EXEC_POD=$(kubectl get pods -n "$NATS_NAMESPACE" --field-selector=status.phase=Running -o name 2>&1 | grep "pod/nats-box" | head -1 | cut -d'/' -f2)

if [ -z "$EXEC_POD" ]; then
  echo "[$0] ERROR: nats-box pod not found in namespace $NATS_NAMESPACE" >&2
  echo "[$0] Available pods:" >&2
  kubectl get pods -n "$NATS_NAMESPACE" -o wide 2>&1 >&2
  echo "[$0] Note: nats-box pod is required (has nats CLI installed)" >&2
  exit 6
fi

# Copy credentials to the pod
REMOTE_CREDS_PATH="/tmp/nauth-assert-creds-$$-${ACCOUNT_ID}.default"
kubectl cp "$CREDS_FILE" "$NATS_NAMESPACE/$EXEC_POD:$REMOTE_CREDS_PATH" 2>&1 | grep -v "tar:" || true

# Use the cluster-internal DNS name
NATS_URL="nats://nats.${NATS_NAMESPACE}.svc.cluster.local:4222"

# Use nats CLI inside the pod to query for the account JWT
set +e  # Don't exit on error, capture it
RESPONSE=$(kubectl exec -i -n "$NATS_NAMESPACE" "$EXEC_POD" -- sh -c \
  "nats req --creds='$REMOTE_CREDS_PATH' --server='$NATS_URL' '\$SYS.REQ.CLAIMS.LIST' '$ACCOUNT_ID'" 2>&1)
NATS_EXIT_CODE=$?
set -e

# Cleanup remote files
kubectl exec -n "$NATS_NAMESPACE" "$EXEC_POD" -- rm -f "$REMOTE_CREDS_PATH" 2>/dev/null || true

if [ $NATS_EXIT_CODE -ne 0 ]; then
  echo "[$0] ERROR: nats CLI command failed" >&2
  echo "[$0] Response: $RESPONSE" >&2
  exit 4
fi

# Check if the account ID is present in the response
# $SYS.REQ.CLAIMS.LIST returns: {"data":["ACCOUNT_ID_1","ACCOUNT_ID_2",...]}
JWT_IS_PRESENT=false
if echo "$RESPONSE" | grep -q "\"$ACCOUNT_ID\""; then
  JWT_IS_PRESENT=true
fi

# Evaluate the assertion based on expectation
if [ "$EXPECT_PRESENT_UPPER" == "TRUE" ]; then
  if [ "$JWT_IS_PRESENT" == "true" ]; then
    exit 0
  else
    echo "[$0] ERROR: Account ($ACCOUNT_ID) JWT NOT found on NATS server (expected it to be present)" >&2
    echo "[$0] Response: $RESPONSE" >&2
    exit 1
  fi
else
  # EXPECT_PRESENT_UPPER == "FALSE"
  if [ "$JWT_IS_PRESENT" == "false" ]; then
    exit 0
  else
    echo "[$0] ERROR: Account ($ACCOUNT_ID) JWT found on NATS server (expected it to be absent)" >&2
    echo "[$0] Response: $RESPONSE" >&2
    exit 1
  fi
fi

