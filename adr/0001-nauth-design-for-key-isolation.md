# ADR-1: Deploy controller handling NATS operator as separate Kubernetes deployment
Date: 2025-05-09

## Problem statement
The different seeds & user credentials that are required for the NATS JWT AuthN & AuthZ all have different requirements for how they are used and how sensitive they are - and need to be stored accordingly.

How should Nauth be designed in order to achieve isolation between the different types of keys?

## Status

**Accepted**

Superceded by [ADR-4: Break out Operator management from Nauth](0004-break-out-operator-management-from-nauth.md)

## Context

### Secret overview

The keys below are the complete key pair, although sensitivity is only concerning the private part of the key pair.

| Secret | Sensitivity | Usage | Number of secrets created |
|--|--|--|--|
| Operator root key | ⚠️ Very high | During vertical creation and signing key rotation | 1 |
| Operator signing key |  🔴 High | Every time account is created | 1-2 |
| Account root key | 🔵 n/a | Only the public key is used as account identifier | 0 |
| Account signing key | 🟡 Medium | Every time a user is created | 1-2 per account |
| User credentials | 🟢 Low | Never by Nauth. Used by clients | 0-N per account |

### Details about different secrets handled
#### Operator root key
The operator root key is essential for the operation of a NATS cluster (or supercluster). It is the base for the trust chain and without it, the whole cluster would need to be rebuilt.

Nauth needs to be able to create the operator key from the `Operator` custom resource and use the key to sign new JWTs whenever a signing key needs to be rotated.

#### Operator signing keys
The primary operator signing key is used to sign every account which is minted, so it is needed frequently. If a signing key is compromised, it can be rotated using the operator root key. The operator JWT can have multiple keys, so if an operator signing key is lost but not compromised - the accounts signed by it are still valid.

#### Account Root key
Account seed keys are the identities of accounts and these should be secured to avoid having to recreate export/imports as well as all persistent data. The root key does not need to be used for anything but provide the identity - it is not needed for minting users.

#### Account signing keys
Used when minting new users. Can also be rotated more frequently in order to keep the validity of user credentials to an acceptable duration.

#### User credentials
User credentials are a combination of a private key and a, by the account signing key, signed JWT. Theses live their own life and as long as the identity is not on the revocation list or if the signing key has been removed from the account JWT on the server, it will be valid according to the user JWT.

New user credentials can be minted easily and there is no consequence from the user credentials being deleted other than the need of getting new ones.


## Options

### Operator minting & updating broken out of Nauth
Instead of allowing Nauth to handle a Operator CRD and creating the JWT, the JWT could be created by another tool such as `nsc` and the seed could be stored offline or very restricted & operator signing keys could be stored in an external secret.

This since the update would still require the update of the NATS configuration and it is updated so seldom.
The key could be retrieved in those cases where it is required to update the signing keys.
This would be a manual operation which would be time consuming, but could also increase the security to avoid the risk that the entire NATS cluster is compromised.

The operator signing key would need to be made available for Nauth for account minting, hence Nauth needs to be able to reach it during runtime.

### All controllers act under the same deployment
Since the controllers are quite similar and the application itself is not very big, it is redundant to split the controllers into multiple pods with their own access.

It is not sure how much more security is gained by splitting the controllers into separate pods, even if it gives the possibility to use separate roles for the external secrets in that case. For an attacker to be able to exploit the permissions `nauth` has, the attack vector is either via the Kubernetes API and the events listened to, or a breach that enables the attacker to start up new containers that acts as nauth. This as nauth doesn't expose is own API nor makes it possible to initiate shell access to nauth.

### Split controllers into separate deployments
By splitting the account & user related CRD controllers to a separate deployment from the operator controller, the different pods can assume different roles, which have different policies.

This would restrict the access to only the relevant secrets if needed. All operator-related operations would only have access to operator root key location & signing key location, whereas the account- & user-related controller would have access to the operator signing key, account keys & user credentials.

## Decision
**Split controllers into separate deployments**

## Consequences
Splitting the controllers into multiple deployments give security benefits as the life cycle and sensitivity of the produced secrets differ vastly between the controllers.
The controllers can be deployed in different ways in different clusters, allowing nauth to be deployed without any operator capability in clusters which don't need it.

This requires the operator controller to have guardrails from creating multiple operator config and only allow the creation in allowed locations.
The manual process of moving the resolver config to the NATS server after creation needs to be managed.
The Operator CRD might become awkward since there can be only one for every NATS (super)cluster.
