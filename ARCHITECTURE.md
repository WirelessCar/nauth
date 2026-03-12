# Architecture

This project follows a hexagonal architecture design.

## Goals

- Keep domain and application logic independent from adapter implementations.
- Let controllers drive use-cases only through inbound ports.
- Let core logic depend on outbound ports, not concrete Kubernetes/NATS clients.
- Keep wiring in one place (`cmd/main.go`).

## Current Decisions

- `api/v1alpha1` is treated as part of the domain contract for now.
- Core logic currently lives in one package: `internal/core`.
  - We may split by bounded context later if needed.

## Target Structure

```text
cmd/
  main.go

api/
  v1alpha1/

internal/
  domain/
    account/
      claims.go                    [package account]
    user/
      claims.go                    [package user]
    cluster/
      target.go                    [package cluster]

  ports/
    inbound/
      account.go                   [package inbound]
      user.go                      [package inbound]
    outbound/
      nats.go                      [package outbound]
      k8s.go                       [package outbound]
      signing.go                   [package outbound]

  core/
    account_service.go             [package core]
    user_service.go                [package core]
    signing_service.go             [package core]

  adapters/
    inbound/
      controller/
        account.go                 [package controller]
        user.go                    [package controller]
        status.go                  [package controller]
    outbound/
      k8s/
        account_reader.go          [package k8s]
        secret_store.go            [package k8s]
        natscluster_reader.go      [package k8s]
      nats/
        client.go                  [package nats]
```

## Package Responsibilities

### `internal/domain/*`

- Pure business rules, claim transformation, value validation.
- No adapter dependencies.

### `internal/ports/inbound` (`package inbound`)

- Use-case interfaces that inbound adapters call.
- Example: `AccountUseCase`, `UserUseCase`.

### `internal/ports/outbound` (`package outbound`)

- Interfaces for infrastructure interactions used by core.
- Example: `NatsClient`, `SecretStore`, `NatsClusterReader`, `SigningKeyResolver`.

### `internal/core` (`package core`)

- Use-case implementations.
- Implements inbound ports.
- Uses outbound ports.
- Must not import adapter packages.

### `internal/adapters/inbound/controller` (`package controller`)

- Kubernetes reconcilers.
- Should depend on inbound ports only (not concrete core types where avoidable).

### `internal/adapters/outbound/*`

- Concrete implementations of outbound ports.
- Kubernetes/NATS integration details.

## Dependency Rules

Allowed direction:

```text
domain <- ports <- core <- adapters(inbound)
                  <- adapters(outbound)

cmd/main.go wires all concrete implementations.
```

More concretely:

- `domain` imports: standard library (and `api/v1alpha1` when required by current domain decision).
- `ports/*` imports: `domain` and `api/v1alpha1` types as needed.
- `core` imports: `domain`, `ports/inbound`, `ports/outbound`, `api/v1alpha1`.
- `adapters/inbound` imports: `ports/inbound`, `domain`, `api/v1alpha1`.
- `adapters/outbound` imports: `ports/outbound`, `domain`, `api/v1alpha1`, external libs.
- `adapters/*` must not import `internal/core` implementation details.

## Wiring Pattern

`cmd/main.go` composes the application:

1. Build outbound adapters (`k8s`, `nats`).
2. Build core services with outbound ports.
3. Pass inbound port implementations to controllers.
4. Register controllers with controller-runtime manager.

## Notes for Upcoming Refactor

- Keep `core` as a single package initially to reduce churn.
- Extract shared account-signing behavior into outbound port(s) or core service(s), not manager-to-manager dependencies.
- Avoid direct `User -> Account manager` concrete coupling; share behavior via ports/services.
