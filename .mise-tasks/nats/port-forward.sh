#!/usr/bin/env zsh
#MISE description="Port forward to NATS server on cluster in current context"
#MISE alias="nats:pf"

kubectl port-forward -n nats --pod-running-timeout=15s service/nats "4222"

