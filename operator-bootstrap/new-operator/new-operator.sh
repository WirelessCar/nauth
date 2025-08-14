#!/usr/bin/env bash
OPERATOR_NAME=nauth-operator

nsc env --store ../_store
nsc add operator --generate-signing-key -n $OPERATOR_NAME -s --all-dirs ../_store
