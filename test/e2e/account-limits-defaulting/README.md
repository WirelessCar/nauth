# Account limit defaulting regression

This scenario checks that reconciliation preserves partial Account limit specs
and sparse status claims. The JetStream-disabled Account verifies that omitted
storage limits do not become unlimited and invalidate the request.

API-server tests cover the omitted, empty, partial, and explicit-value matrix.
This scenario covers the Account controller and NATS integration. On failure,
collectors print each Account's name and readiness reason/message, followed by
namespace events.
