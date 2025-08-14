# ADR-3: Handle JetStream controller credentials by automatically creating NACK user per Account

Date: 2025-05-15

## Problem statement
Enabling Infrastructure-as-Code (IaC) for JetStream resources such as Streams, Consumers, Key-Value bucket or Object store requires the ability to orchestrate credentials for managing these for all accounts created by NAuth. [NACK](https://github.com/nats-io/nack) is the only available project which supports this, making this an appealing candidate to integrate with.

## Status

Accepted

## Context

NACK has the ability to read a secret to reconcile JetStream resources for specific Account by referencing these in manifests. NACK uses an `Account` CRD to reference a secret in the same namespace. This allows for creating the NACK CR in the owning namespace and simply provide the credentials within the same namespace. Allowing NAuth to create credentials that integrates with NACK enables existing users of NACK to use NAuth without having to change their existing setup. 

A NACK user requires a set of permissions to use the JetStream API to manage subjects which are not obvious. Developers would benefit from having a pre-configured user with the correct permissions to use NACK.

## Options

### Global configuration to automatically create pre-defined user per Account
Adding the opt-in feature to integrate with NACK on a global configuration level where an additional pre-configured user is created in the NACK namespace upon Account creation. The Account controller would need to also produce a child resource which can reconcile the JetStreams controller user. The child resource can be a `User` CR, just that it is created when reconciling the account with predefined permissions.

#### Advantage
- Repeated permission settings are set once
- No need for all teams to explicitly set NACK users
- Seamless integration with JetStream operator
- Enables a operator patterns and is not bound to specific integration
- Utilizes existing CRDs

#### Disadvantages
- Account reconciliation needs to also reconcile the resulting user permissions

### User CRD is used as-is
The `User` CRD already supports the use case and can be used to create the required credentials.

#### Advantages
- No NACK specific solution
- No additional effort

#### Disadvantages
- Users must know of JetStreams API permissions or it needs to be packaged
- Mixing concerns for teams utilizing platform provided NATS

## Decision
**Global configuration to automatically create pre-defined user per Account**

## Consequences
NAuth & NACK work well side by side and adding NACK integration to NAuth is a natural way to go. The solution with global configuration is generic enough to be used by any other operator if this would be the case.
The reconciliation would need to handle updates to global configuration and push the changed permissions to the underlying child resources.

There is nothing blocking from using the CRDs as-is now and build out the feature in a later stage since the `User` CRD is re-used for the functionality.
