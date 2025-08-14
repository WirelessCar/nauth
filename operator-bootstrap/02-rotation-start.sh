#!/usr/bin/env bash
#
# Run this to start the process to rotate:
# - Operator signing key
# - System account signing key
# - System account user (with new signing key)
#
# This will KEEP the current signing keys present.
#
# In order to complete the rotation, run `03-rotation-end.sh`
# It is recommended that these operations are done separately in order for existing accounts to have been re-signed.

set -e

OPERATOR_NAME=nauth-operator
SYS_ACCOUNT=SYS
STORE_PATH=_store
EXPORT_PATH=_export
SYS_SK_REMOVAL_FILE_PATH=$STORE_PATH/$OPERATOR_NAME/accounts/$SYS_ACCOUNT/SYS_SK_AWAITING_REMOVAL.nsc
OP_SK_REMOVAL_FILE_PATH=$STORE_PATH/$OPERATOR_NAME/OP_SK_AWAITING_REMOVAL.nsc

nsc env --store $STORE_PATH
nsc select operator $OPERATOR_NAME || echo -e "!!!!!!!!\n  HINT : Make sure to run the command directly in it's directory\n!!!!!!!!\n"
nsc select account $SYS_ACCOUNT || echo -e "!!!!!!!!\n  HINT : Make sure to run the command directly in it's directory\n!!!!!!!!\n"

nsc describe operator $OPERATOR_NAME

echo -e "
- A new signing key will be added and existing keys will be marked for deletion on next rotation.
- The same procedure will be done to the system account.
- A new sys user will be created and the old removed from the root store.

"

read -p "Continue? (y/n): " CONFIRMATION
if [[ "$CONFIRMATION" != "y" ]]; then
    echo "Operation aborted by the user."
    exit 1
fi

nsc describe operator $OPERATOR_NAME -J | jq '.nats.signing_keys[]' -r > $OP_SK_REMOVAL_FILE_PATH
nsc describe account $SYS_ACCOUNT -J | jq '.nats.signing_keys[]' -r > $SYS_SK_REMOVAL_FILE_PATH

echo "
Generate a new OPERATOR signing key and add it to the operator...
"

NEW_OP_SK=$(nsc generate nkey -o -S --all-dirs $STORE_PATH | head -n 1)
nsc edit operator --sk $NEW_OP_SK --all-dirs $STORE_PATH
cp $STORE_PATH/keys/O/*/$NEW_OP_SK.nk $EXPORT_PATH/operator-sk_$NEW_OP_SK.nk

echo "
Generate a new ACCOUNT signing key and add it to the SYS account...
"

NEW_SYS_SK=$(nsc generate nkey -a -S --all-dirs $STORE_PATH | head -n 1)
nsc edit account --sk $NEW_SYS_SK --all-dirs $STORE_PATH

echo "
Re-signing an account with 'nsc' requires the current key to be invalid.
Temporarily remove the old key while re-signing the account...
"

while read -r SK; do
    nsc edit operator --rm-sk $SK --all-dirs $STORE_PATH
done < $OP_SK_REMOVAL_FILE_PATH

echo "
Re-importing the account JWT and signing it with the new operator signing key...
"

nsc import account -K $NEW_OP_SK --file $STORE_PATH/$OPERATOR_NAME/accounts/$SYS_ACCOUNT/$SYS_ACCOUNT.jwt --all-dirs $STORE_PATH --overwrite --force

echo "
Re-adding the current signing key again after system account was re-signed...
"

while read -r SK; do
    nsc edit operator --sk $SK --all-dirs $STORE_PATH
done < $OP_SK_REMOVAL_FILE_PATH

echo "
Recreating sys user with new signing key...
"

nsc delete user sys
nsc add user --name sys -K $NEW_SYS_SK --all-dirs $STORE_PATH
cp $STORE_PATH/creds/nauth-operator/SYS/sys.creds $EXPORT_PATH/sys-user.creds

./export.sh
