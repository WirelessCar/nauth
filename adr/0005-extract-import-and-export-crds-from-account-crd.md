# ADR-5: Extract Import- and Export CRDs from Account CRD

Date: 2026-03-26

## Problem statement

The `Account` CRD is currently used to manage everything related to the account, which includes limits, imports and
exports. If any of these settings are broken, especially imports and exports, it can affect the entire account.

Splitting the import and export CRDs from the Account CRD would allow for better separation of concerns.

## Status

Accepted

## Context

There exists several identified issues or improvements related to imports and exports, such as: validation of imports
and matching exports, import and export using activation tokens, support for grouping of related imports and exports (
for example, JetStream related API subjects).

All these are concerns of imports and exports, and might benefit from the split as well as make the account resources
more robust and easier to manage.

It is still the responsibility of the `Account` CRD, to reconcile all the imports and exports that belong to the
account.

This ADR describes some options for splitting import / export into separate CRDs. It aims to scope the split into
features that exist today and only introduce new features if they are necessary or small enough. Other features are
deferred.

### Concerns

These concerns have been lifted during the writing of the ADR. They are not necessarily all solved by the solutions, but
they are important to keep in mind when evaluating the options.

- How to handle partial failures, for example, if one of the imports or exports is invalid, should the entire account
  be left out / invalidated or should the valid imports and exports still be applied?
- Account CRD must be reconciled before imports and exports, otherwise the account might be left in a "broken" state.
- Changes in imports and exports must be reflected in the account JWT, which means that the account JWT must be
  reconciled after any change in imports and exports.
- How to present reconciliation state in a good way in tools such as `kubectl`, `k9s` and `argoCD`?
- How to handle overlapping subjects for same target account, from one or more CRs?
- Can an account in one namespace reference exports/imports in another namespace?
- Will import/export activation tokens work with these options?
- Must use deterministic ordering of imports/exports, so that reconciliation does not go into loop.

### Out of Scope - Deferred to future ADRs

* Cross-cluster imports/exports
* import and export activation tokens
* import and export drift detection and approval workflows

## Options

See [work log](worklog/0005/options.md) for details about options.

### Option 1 - AccountImportExport CRD

Imports and exports are simply moved to a separate CRD, but they are still managed together. The new CRD has a reference
to the account. This option is in essence the same as the current state, but with imports and exports moved to a
separate CRD.

### Option 2 - Separate AccountImport and AccountExport CRDs

Same as Option 1 but with separate CRDs for imports and exports.
Better for separating import and export specific fields and concerns.

### Option 3 - One import/export per import/export CRD

Each import and export is represented as a separate CRD.
This allows for better granularity and easier management of individual imports and exports,
but it can lead to a large number of CRDs if there are many imports and exports.

### Option 4 - Contract-based imports/exports

`AccountExport` represents an explicit contract owned by the exporting account.
`AccountImport` represents an import binding owned by the importing account and references a specific `AccountExport`.

Instead of only defining raw import/export fragments, this option models the relationship between importer and exporter
as first-class resources. The `Account` CRD remains responsible for reconciling the final account JWT from all ready
import/export resources belonging to the account.

The key idea is that both sides describe their own part of the relationship:

* the exporter publishes what it is willing to expose
* the importer declares which export it wants to consume

This makes the relationship explicit, easier to validate, and easier to reason about than embedding raw import/export
fragments directly in `Account`.

## Decision

* Extract import/export to separate CRDs
* First release must support what is supported today by inline imports and exports
* Deprecate the import and export fields in `Account` CRD.
* Use `Option 4 - Contract-based imports/exports` as a target vision
* `tokenReq` and `revocations` in export will be deferred, as these are not implemented today

## Consequences

* Import and export CRDs can reconcile by themselves.
* Failing import and export CRDs will not block account reconciliation.
