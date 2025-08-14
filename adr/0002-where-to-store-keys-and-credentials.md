# ADR-2: Store keys and credentials as secrets in Kubernetes

Date: 2025-05-09

## Problem statement
The different seeds & credentials that are required for the NATS JWT AuthN & AuthZ all have different requirements for how they are used and how sensitive they are - and need to be stored accordingly.

How should keys & credentials be stored to meet needs of security & usability?

## Status

**Accepted**

## Context

### Operator root key
The operator root key is essential for the operation of a NATS cluster (or supercluster). It is the base for the trust chain and without it, the whole cluster would need to be rebuilt.

Nauth needs to be able to create the operator key from the `Operator` custom resource and use the key to sign new JWTs whenever a signing key needs to be rotated.

### Operator signing keys
The primary operator signing key is used to sign every account which is minted, so it is needed frequently. If a signing key is compromised, it can be rotated using the operator root key. The operator JWT can have multiple keys, so if an operator signing key is lost but not compromised - the accounts signed by it are still valid.

### Account Root key
Account seed keys are the identities of accounts and these should be secured to avoid having to recreate export/imports as well as all persistent data. The root key does not need to be used for anything but provide the identity - it is not needed for minting users.

### Account signing keys
Used when minting new users. Can also be rotated more frequently in order to keep the validity of user credentials to an acceptable duration.

### User credentials
User credentials are a combination of a private key and a, by the account signing key, signed JWT. Theses live their own life and as long as the identity is not on the revocation list or if the signing key has been removed from the account JWT on the server, it will be valid according to the user JWT.

New user credentials can be minted easily and there is no consequence from the credentials being deleted other than the need of getting new ones.

## Options

### All keys in external secret store
If all keys and credentials are stored in an external store, these can all be restored in the case of a cluster upgrade or recovery in a seamless way.

The drawbacks are: 

* increased risk of rate limiting when scaling up the system
* increased maintenance burden of cleaning up old keys
* increased cost of managed secret stores for every key type regardless of requirements

### All keys are stored in Kubernetes secrets
No external dependency and it would be possible to use tools such as external-secrets operator to push secrets to an external store if wanted. It would not be part of Nauth to handle this.

This would give a good responsibility boundary, since it would not require Nauth to have any cloud provider specific implementations.

Could however be impractical to pull the secret back from external-secret in the event of a cluster recovery since the name of the secret would likely need to be the same as the pushed secret.

Nauth would not incur any rate limiting towards a secret service, since this would be handled outside in a fashion which is not likely to cause rate limiting.

During a disaster recovery or during an upgrade using a new cluster, all credentials would need to be restored from backup or re-created. It also means that backup of credentials increases in importance and that the backups themselves contains more sensitive data.

### Operator root keys in external secret store - others as Kubernetes secrets
The root operator needs to be stored securely in a way that it does not get removed in the case of a cluster failure. It also needs to be able to be fetched when the Operator CR is updated and the JWT needs to be updated in order to sign it.

#### User credentials
Since the user credentials can be lost and simply recreated, there is no need for the credentials to be backed up. Nauth can create new ones from the `User` CRs. 
This allows for quick distribution without the risk of rate limiting services which hold secrets.

#### Signing keys
Since the signing keys themselves are not required during runtime, but only for creating new accounts & users it is compelling to only store these as Kubernetes secrets and not store these in an external secret.
As long as the public key is still known and can be part of the list of valid signing keys in the JWT, all previously signed JWTs are still valid.

If a signing key is lost, a new key is minted and added to the trusted JWT by the using the root key. Accounts and users can be re-issued without interruption.

## Decision
**All keys are stored in Kubernetes secrets**

## Consequences
The nauth implementation will not have any external dependencies in the form of services for secrets more than Kubernetes native secrets.

### Advantages
- Developer experience is improved as development & testing is easy to do in a local environment without dependencies
- Responsibility boundary is clear
- Any interaction with external secret services can be achieved by other solutions which specialize in secret management

### Disadvantages
- Sensitive keys like operator root key are being accessed by the nauth controller even if not always needed
- Still need to handle isolation of operator root key in other solution (external-secrets)
