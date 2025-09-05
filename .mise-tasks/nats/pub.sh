#!/usr/bin/env zsh
#MISE description="Publish using NATS creds from namespace - Example: mise nats:pub -- example-user foo.test 'hello there'"

# Usage: <user-name> <subject> [message]
# Example: mise nats:pub -- example-account foo "my message"
set -e

user_name=$1
subject=$2
message=${3:-"hello from $user_name"}

if [[ -z "$user_name" ]]; then
  echo "Usage: $0 <user-name> <subject> [message]"
  exit 1
fi

creds=$(kubectl get secret "${user_name}-nats-user-creds" -o jsonpath="{.data['user\.creds']}" | base64 --decode)

creds_file=$(mktemp)
echo "$creds" > "$creds_file"

nats pub $subject "$message" --creds "$creds_file" --server nats://localhost:4222
