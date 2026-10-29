# User limit defaulting regression

This scenario checks that reconciliation preserves a partial User NATS-limit
spec and sparse status claims. A minimal Account with JetStream disabled is a
prerequisite fixture and must become ready before the User is created.

API-server tests cover the omitted, empty, partial, and explicit-value matrix.
This scenario covers the User controller and NATS integration. On failure,
collectors print User and prerequisite Account readiness reasons/messages,
followed by namespace events.
