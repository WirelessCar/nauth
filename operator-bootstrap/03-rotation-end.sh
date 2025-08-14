#!/usr/bin/env bash
#
# Run this to finalize the rotation process.
#
# It will:
# - Remove the old signing key(s) from both operator AND system account
# - Export the updated root store to export archive

set -e

OPERATOR_NAME=nauth-operator
SYS_ACCOUNT=SYS
STORE_PATH=_store
EXPORT_PATH=_export
SYS_SK_REMOVAL_FILE_PATH=$STORE_PATH/$OPERATOR_NAME/accounts/$SYS_ACCOUNT/SYS_SK_AWAITING_REMOVAL.nsc
OP_SK_REMOVAL_FILE_PATH=$STORE_PATH/$OPERATOR_NAME/OP_SK_AWAITING_REMOVAL.nsc

nsc env --store $STORE_PATH
nsc select operator $OPERATOR_NAME || echo "
!!!!!!!!
HINT : Make sure to run the command directly in it's directory
!!!!!!!!"
nsc describe operator $OPERATOR_NAME

echo "
!!!!! NOTE !!!!!

- This script is intended to be used after all valid accounts have been signed with the new signing key.
- Do not run this script in direct succession with the 'rotation-start' script.

This will remove the following signing keys from the operator:
"

cat $OP_SK_REMOVAL_FILE_PATH
echo "
...and the following signing keys from the system account:
"
cat $SYS_SK_REMOVAL_FILE_PATH

read -p "Continue? (y/n): " CONFIRMATION
if [[ "$CONFIRMATION" != "y" ]]; then
    echo "Operation aborted by the user."
    exit 1
fi

while read -r SK; do
    nsc edit operator --rm-sk $SK --all-dirs $STORE_PATH
    rm $STORE_PATH/keys/O/*/$SK.nk
done < $OP_SK_REMOVAL_FILE_PATH

while read -r SK; do
    nsc edit account $SYS_ACCOUNT --rm-sk $SK --all-dirs $STORE_PATH
    rm $STORE_PATH/keys/A/*/$SK.nk
done < $SYS_SK_REMOVAL_FILE_PATH

rm $OP_SK_REMOVAL_FILE_PATH || echo "No operator signing keys to remove in store!"
rm $SYS_SK_REMOVAL_FILE_PATH || echo "No sys account signing keys to remove in store!"

./export.sh
