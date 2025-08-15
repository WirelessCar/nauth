#!/usr/bin/env zsh
#MISE description="Subscribe using NATS creds - Example: mise nats:sub -- example-user foo.>"

set -e

user_name=$1
subject=$2

if [[ -z "$user_name" ]]; then
  echo "Usage: $0 <user-name> <subject>"
  exit 1
fi

creds=$(kubectl get secret "${user_name}-nats-user-creds" -o jsonpath="{.data['user\.creds']}" | base64 --decode)

creds_file=$(mktemp)
echo "$creds" > "$creds_file"

# Subscribe to test.> using NATS with creds
nats sub "$subject" --creds "$creds_file" --server nats://localhost:4222
