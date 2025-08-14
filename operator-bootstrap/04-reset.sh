#!/usr/bin/env bash
#
# REMOVES ALL SENSITIVE DATA FROM THE DIRECTORY

set -e

echo -e "Make sure that you have moved the archives for offline storage and the cluster!\n"
read -p "This will reset the operator management environment - REMOVING ALL SENSITIVE DATA. Continue? (y/n): " CONFIRMATION
if [[ "$CONFIRMATION" != "y" ]]; then
    echo "Operation aborted by the user."
    exit 1
fi

rm -rf _store/* || echo "No store to remove..."
rm -rf _export/* || echo "No exports to remove..."
rm *.7z || echo "No archives to remove..."
