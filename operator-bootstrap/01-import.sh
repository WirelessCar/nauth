#!/usr/bin/env bash
#
# Use to import existing root store
#
# Add the `nats-root-store.7z` in the directory to import
#
set -e

OPERATOR_NAME=nauth-operator
STORE_PATH=_store

echo "
Importing root store...
"

7z x nats-root-store.7z
nsc env --store $STORE_PATH
nsc describe operator $OPERATOR_NAME && echo "Operator imported successfully!"
