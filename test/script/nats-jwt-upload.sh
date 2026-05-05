#!/usr/bin/env bash
# Usage: nats-jwt-upload.sh <jwt-file> [nats-cluster-name] [nats-cluster-namespace]
# Example: ./nats-jwt-upload.sh /tmp/account.jwt local-nats nats
#
# Uploads an account JWT to NATS using system account credentials resolved from
# a NatsCluster resource.

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
source "$SCRIPT_DIR/e2e-debug.sh"

if [ "$#" -lt 1 ] || [ "$#" -gt 3 ]; then
  echo "[$0] Usage: $0 <jwt-file> [nats-cluster-name] [nats-cluster-namespace]" >&2
  exit 1
fi

JWT_FILE="$1"
NATS_CLUSTER_NAME="${2:-local-nats}"
NATS_CLUSTER_NAMESPACE="${3:-nats}"

log "validate JWT upload inputs"
if [ ! -f "$JWT_FILE" ]; then
  echo "[$0] ERROR: JWT file not found: $JWT_FILE" >&2
  echo "[$0] Absolute path would be: $(cd "$(dirname "$JWT_FILE")" 2>/dev/null && printf '%s/%s' "$(pwd -P)" "$(basename "$JWT_FILE")" || echo 'cannot resolve')" >&2
  exit 2
fi

# Keep a short filename for unique remote temp paths in the nats-box pod.
JWT_FILENAME=$(basename "$JWT_FILE")

# Fail before resolving credentials if the target NatsCluster does not exist.
if ! kubectl get natsclusters.nauth.io "$NATS_CLUSTER_NAME" -n "$NATS_CLUSTER_NAMESPACE" >/dev/null 2>&1; then
  echo "[$0] ERROR: NatsCluster $NATS_CLUSTER_NAMESPACE/$NATS_CLUSTER_NAME not found" >&2
  exit 3
fi

log "resolve NATS URL from NatsCluster $NATS_CLUSTER_NAMESPACE/$NATS_CLUSTER_NAME"
# Prefer a direct spec.url value.
NATS_URL="$(kubectl get natsclusters.nauth.io "$NATS_CLUSTER_NAME" -n "$NATS_CLUSTER_NAMESPACE" -o jsonpath='{.spec.url}')"

if [ -z "$NATS_URL" ]; then
  log "resolve NATS URL from spec.urlFrom"
  # Fall back to spec.urlFrom, which can point to a ConfigMap or Secret key.
  URL_FROM_KIND="$(kubectl get natsclusters.nauth.io "$NATS_CLUSTER_NAME" -n "$NATS_CLUSTER_NAMESPACE" -o jsonpath='{.spec.urlFrom.kind}')"
  URL_FROM_NAME="$(kubectl get natsclusters.nauth.io "$NATS_CLUSTER_NAME" -n "$NATS_CLUSTER_NAMESPACE" -o jsonpath='{.spec.urlFrom.name}')"
  URL_FROM_KEY="$(kubectl get natsclusters.nauth.io "$NATS_CLUSTER_NAME" -n "$NATS_CLUSTER_NAMESPACE" -o jsonpath='{.spec.urlFrom.key}')"
  URL_FROM_NAMESPACE="$(kubectl get natsclusters.nauth.io "$NATS_CLUSTER_NAME" -n "$NATS_CLUSTER_NAMESPACE" -o jsonpath='{.spec.urlFrom.namespace}')"

  if [ -z "$URL_FROM_NAMESPACE" ]; then
    URL_FROM_NAMESPACE="$NATS_CLUSTER_NAMESPACE"
  fi

  case "$URL_FROM_KIND" in
    ConfigMap|configmap)
      # ConfigMap values are stored as plain data.
      NATS_URL="$(kubectl get configmap "$URL_FROM_NAME" -n "$URL_FROM_NAMESPACE" -o "go-template={{ index .data \"$URL_FROM_KEY\" }}" 2>/dev/null || true)"
      ;;
    Secret|secret)
      # Secret values are base64 encoded by Kubernetes.
      URL_B64="$(kubectl get secret "$URL_FROM_NAME" -n "$URL_FROM_NAMESPACE" -o "go-template={{ index .data \"$URL_FROM_KEY\" }}" 2>/dev/null || true)"
      if [ -n "$URL_B64" ] && [ "$URL_B64" != "<no value>" ]; then
        NATS_URL="$(printf '%s' "$URL_B64" | base64 -d)"
      fi
      ;;
    "")
      ;;
    *)
      echo "[$0] ERROR: Unsupported spec.urlFrom.kind '$URL_FROM_KIND' for NatsCluster $NATS_CLUSTER_NAMESPACE/$NATS_CLUSTER_NAME" >&2
      exit 4
      ;;
  esac
fi

if [ -z "$NATS_URL" ]; then
  echo "[$0] ERROR: Unable to resolve NATS URL from NatsCluster $NATS_CLUSTER_NAMESPACE/$NATS_CLUSTER_NAME" >&2
  exit 4
fi

log "resolve system account credentials"
# Read the Secret reference that contains system account user creds for account JWT upload.
CREDS_SECRET="$(kubectl get natsclusters.nauth.io "$NATS_CLUSTER_NAME" -n "$NATS_CLUSTER_NAMESPACE" -o jsonpath='{.spec.systemAccountUserCredsSecretRef.name}')"
CREDS_KEY="$(kubectl get natsclusters.nauth.io "$NATS_CLUSTER_NAME" -n "$NATS_CLUSTER_NAMESPACE" -o jsonpath='{.spec.systemAccountUserCredsSecretRef.key}')"

if [ -z "$CREDS_SECRET" ]; then
  echo "[$0] ERROR: NatsCluster $NATS_CLUSTER_NAMESPACE/$NATS_CLUSTER_NAME does not define spec.systemAccountUserCredsSecretRef.name" >&2
  exit 5
fi

if [ -z "$CREDS_KEY" ]; then
  CREDS_KEY="default"
fi

# Extract the referenced creds key without logging the secret contents.
CREDS_B64="$(kubectl get secret "$CREDS_SECRET" -n "$NATS_CLUSTER_NAMESPACE" -o "go-template={{ index .data \"$CREDS_KEY\" }}" 2>/dev/null || true)"
if [ -z "$CREDS_B64" ] || [ "$CREDS_B64" = "<no value>" ]; then
  echo "[$0] ERROR: Secret $NATS_CLUSTER_NAMESPACE/$CREDS_SECRET does not contain key '$CREDS_KEY'" >&2
  exit 5
fi

log "prepare temporary system account creds file"
CREDS_FILE=$(mktemp)
trap 'rm -f "$CREDS_FILE"' EXIT
# Decode creds to a temp file for kubectl cp; keep secret material out of logs.
printf '%s' "$CREDS_B64" | base64 -d > "$CREDS_FILE"

log "resolve nats-box pod"
# Pick the nats-box pod because it has the nats CLI needed for system account requests.
EXEC_POD=$(kubectl get pods -n "$NATS_CLUSTER_NAMESPACE" --field-selector=status.phase=Running -o name | awk -F/ '/^pod\/nats-box/ {print $2; exit}')

if [ -z "$EXEC_POD" ]; then
  echo "[$0] ERROR: nats-box pod not found in namespace $NATS_CLUSTER_NAMESPACE" >&2
  echo "[$0] Available pods:" >&2
  kubectl get pods -n "$NATS_CLUSTER_NAMESPACE" -o wide 2>&1 >&2
  echo "[$0] Note: nats-box pod is required (has nats CLI installed)" >&2
  exit 6
fi

log "copy upload files into nats-box pod"
# Use unique remote paths so repeated KUTTL retries do not collide.
REMOTE_CREDS_PATH="/tmp/nauth-upload-creds-$$-${JWT_FILENAME%.jwt}.default"
kubectl cp "$CREDS_FILE" "$NATS_CLUSTER_NAMESPACE/$EXEC_POD:$REMOTE_CREDS_PATH" 2>&1 | grep -v "tar:" || true

REMOTE_JWT_PATH="/tmp/nauth-upload-$$-${JWT_FILENAME%.jwt}.jwt"
kubectl cp "$JWT_FILE" "$NATS_CLUSTER_NAMESPACE/$EXEC_POD:$REMOTE_JWT_PATH" 2>&1 | grep -v "tar:" || true

log "upload account JWT through NATS system account"
# Send the account JWT to the NATS system account claims update subject.
set +e
RESPONSE=$(kubectl exec -i -n "$NATS_CLUSTER_NAMESPACE" "$EXEC_POD" -- sh -c \
  "nats req --creds='$REMOTE_CREDS_PATH' --server='$NATS_URL' '\$SYS.REQ.CLAIMS.UPDATE' \"\$(cat '$REMOTE_JWT_PATH')\"" 2>&1)
NATS_EXIT_CODE=$?
set -e

log "remove remote upload files from nats-box pod"
kubectl exec -n "$NATS_CLUSTER_NAMESPACE" "$EXEC_POD" -- rm -f "$REMOTE_CREDS_PATH" "$REMOTE_JWT_PATH" 2>/dev/null || true

if [ $NATS_EXIT_CODE -ne 0 ]; then
  echo "[$0] ERROR: nats CLI command failed" >&2
  echo "[$0] Response: $RESPONSE" >&2
  exit 7
fi

if echo "$RESPONSE" | grep -q '"code":200'; then
  exit 0
fi

echo "[$0] ERROR: Failed to upload account JWT" >&2
echo "[$0] Response: $RESPONSE" >&2
exit 7
