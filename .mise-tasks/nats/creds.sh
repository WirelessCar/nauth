#!/usr/bin/env zsh
#MISE description="Get user-creds file for use with nats locally. Use namespace of user as current context"

# Usage: <user-name>
# Example: mise nats:creds -- example-user
set -e

user_name=$1

if [[ -z "$user_name" ]]; then
  echo "Usage: $0 <user-name>"
  exit 1
fi

creds=$(kubectl get secret "${user_name}-nats-user-creds" -o jsonpath="{.data['user\.creds']}" | base64 --decode)

creds_file=$(mktemp)
echo "$creds" > "$creds_file"

echo "credentials retrieved in:
$creds_file

Usage:

nats pub hello.there \"this is a message\" --creds $creds_file --server nats://localhost:4222
"
