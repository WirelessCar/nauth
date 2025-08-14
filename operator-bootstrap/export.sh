#!/usr/bin/env bash
set -e

STORE_PATH=_store
EXPORT_PATH=_export

echo "
Exporting config & secrets...
"

nsc generate config --nats-resolver > $EXPORT_PATH/nats-resolver.cfg

read -p "You will now need to enter a password for each of the exports in 7z format. Continue? (y/n)" CONFIRMATION
if [[ "$CONFIRMATION" != "y" ]]; then
    echo "Successfully updated operator WITHOUT exporting archives!"
    exit 0
fi


7z a -p nats-workload.7z $EXPORT_PATH
7z a -p nats-root-store.7z $STORE_PATH

echo "
Operator and system account are now updated!

********************** TODO ***********************************************************
*                                                                                     *
* - Store the 'nats-root-store.7z' securely for storage                               *
* - Update the NATS cluster and secret store with contents from 'nats-workload.7z'    *
* - 'RESET' this environment between usages                                           *
*                                                                                     *
***************************************************************************************

"
