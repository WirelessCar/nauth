# ADR-7: Default explicitly enabled Account JetStream quotas to unlimited

Date: 2026-11-03

## Problem statement

When an Account explicitly enables JetStream but omits its quotas, NAuth must choose between unlimited quotas and the JWT library's zero defaults. Static-config NATS behavior provides a useful comparison, not a separate NAuth option.

## Status

Accepted

## Context

The JWT constructor's zero memory and disk limits disable JetStream, while `JetStreamLimits.IsUnlimited` identifies explicit `NoLimit` values as unlimited. The constructor default and the unlimited policy are therefore distinct. This behavior was present in JWT v2.8.1 and remains in the latest stable GitHub release, v2.8.2, as of 2026-11-03; the v2.8.2 constructor explicitly documents the zero-value behavior.

An enabled static-config account with omitted quotas has unlimited account-level limits; server-level JetStream capacity still constrains storage. This is a behavioral comparison only: with decentralized auth, the Account JWT is the account configuration. The storage, stream, and consumer defaults date to NATS Server v2.2.0; `max_bytes_required: false` dates to v2.7.0, and unlimited ack-pending and per-stream byte quotas date to v2.8.0. Latest stable GitHub release v2.15.2 retains these defaults as of 2026-11-03. Since v2.15.0, a global default of 1,000 consumers per stream applies only when neither stream nor account limits specify a value; the enabled static account's omitted `max_consumers` is unlimited.

## Options

### Option 1 - Default explicit enablement to unlimited quotas

For `jetStreamEnabled: true`, default omitted quota fields to the JWT library's unlimited representation: `NoLimit` for memory, disk, streams, consumers, and ack pending; zero for per-stream byte caps; and `false` for `max_bytes_required`.

- **Pros:** JetStream is enabled by the explicit request; matches the JWT library's unlimited representation and the static-config NATS defaults for an enabled account with omitted quotas.
- **Cons:** An Account has no per-account storage or count cap. Its use is limited by server capacity and any server-level limits, so one Account can consume a disproportionate share.

### Option 2 - Use the JWT constructor's zero defaults

Leave the JetStream limits at zero. JetStream remains disabled unless memory or disk storage is explicitly allowed.

- **Pros:** Fails closed and requires an explicit resource allowance; avoids silently granting an unbounded per-Account quota.
- **Cons:** `jetStreamEnabled: true` alone cannot enable JetStream and must fail validation. Callers must supply storage limits, including `NoLimit` when unlimited use is intended.

## Decision

Use Option 1. When JetStream is explicitly enabled and a quota is omitted, default it to unlimited. This matches static-config NATS behavior. NAuth encodes the policy in the Account JWT; server-level capacity remains the resource boundary. Callers that need per-Account isolation must set explicit limits.

## Consequences

- An explicitly enabled Account can use JetStream without also specifying quotas.
- NAuth does not promise per-Account storage isolation by default; operators must set server limits and callers can configure Account limits.
- The JWT constructor's zero defaults remain appropriate when JetStream was not requested; they are not the defaults for an explicitly enabled Account.
