# Options

## Option 1 - AccountImportExport CRD

Imports and exports are simply moved to a separate CRD, but they are still managed together. The new CRD has a reference
to the account. This option is in essence the same as the current state, but with imports and exports moved to a
separate CRD.

```yaml
# simplified example of the new CRD structure
AccountImportExport A:
    account: A
    imports:
        - foo.> from Account B
        - bar.> from Account C
    exports:
        - def.> to Account X

AccountImportExport B:
    account: B
    exports:
        - foo.> to Account A

AccountImportExport C:
    account: C
    exports:
        - bar.> to Account A

# alternative for A, can be used if it makes sense to split imports/exports
AccountImportExport A1:
    account: A
    imports:
        - foo.> from Account B

AccountImportExport A2:
    account: A
    imports:
        - bar.> from Account C

AccountImportExport A3:
    account: A
    exports:
        - def.> to Account X
```

### Benefits

- Allows for easiest migration of the existing imports and exports to the new CRD.

### Drawbacks

- Invites users to define both Export and Import in the same file, even if that's not desired.
- It's harder to present the data in a good way in `kubectl` and `k9s` as the columns might differ.
- Needs to handle partial failures.

## Option 2 - Separate AccountImport and AccountExport CRDs

Same as Option 1 but with separate CRDs for imports and exports.
Better for separating import and export specific fields and concerns.
For example `export` has `TokenReq (bool)` field, while `import` has `Token` field.

```yaml
# simplified example of the new CRD structure
AccountImport A:
    account: A
    imports:
        - foo.> from Account B
        - bar.> from Account C

AccountExport A:
    account: A
    exports:
        - def.> to Account X
```

### Benefits

- Allows for configuration that affects all listed subjects.
- Allows for configuration of individual subjects.
- Less verbose but still allows user to define one import or export per AccountXport if so desired.
- Allows grouping of related imports and exports, for example, all jetstream related API topics can be grouped together
  in one CRD.
    - And as a consequence, it is "all or nothing" for that group, which makes sense when it comes to jetstream.

### Drawbacks

- Needs to handle partial failures.

## Option 3 - One import/export per import/export CRD

Each import and export is represented as a separate CRD.
This allows for better granularity and easier management of individual imports and exports,
but it can lead to a large number of CRDs if there are many imports and exports.

```yaml
# simplified example of the new CRD structure
AccountImport A1:
    account: A
    subject: foo.>
    fromAccount: B

AccountImport A2:
    account: A
    subject: bar.>
    fromAccount: C

AccountExport A3:
    account: A
    subject: def.>
    toAccount: X
```

### Benefits:

- Very granular validation, i.e. an entire group of rules wouldn't be left out just because one of the subjects are
  conflicting or invalid.

### Drawbacks:

- Extremely verbose, at least if we think about import/export for JetStream stuff.

## Option 4 - Contract-based imports/exports

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

**Example**

```yaml
AccountExport:
    name: orders-and-stock-info     # unique name, referenced in imports
    namespace: namespace-a
    accountName: Account A          # parent account, expected to be in same namespace
    rules:
        -   name: "orders"          # human-readable name
            subject: "orders.>"     # example: orders.<region>.<id> (orders.eu-west-1.12345) 
            type: service           # service/stream
            responseType: Singleton # service response type
        -   name: "stock-info"
            subject: "stock-info"
            type: stream
```

```yaml

AccountImport:
    name: account-a-eu-orders         # unique name
    namespace: namespace-b
    accountName: Account B            # parent account, expected to be in same namespace
    exportRef: # reference to the export this import wants to consume
        name: orders-and-stock-info   # must match the export name
        namespace: namespace-a
    exportAccountName: Account A      # expected exporting account, used for validation
    ruleBindings:
        -   name: "orders eu-west-1"        # human-readable name
            subject: "orders.eu-west-1.>"   # must match or be a subset of an export rule subject
            type: service                   # must match the export rule type
            localSubject: "eu.orders.>"     # optional local subject 
```

In this example:

* Account A says: "I export these subjects and I allow any account to import them"
* Account B says: "I want to import only eu-west-1 related orders from the export, and rename the local subject."
* NAuth only renders final account JWT on the export account when `AccountExport` is ready
* NAuth only renders final account JWT on the import account when `AccountImport` is ready

### How validation and readiness work

This option introduces a fail-closed readiness model.

`AccountExport` validates the exporter-owned side of the contract.
`AccountImport` validates the importer-owned side of the contract.

Only ready `AccountExport` and ready `AccountImport` resources contribute to the final rendered account JWT.

This makes partial failures easier to reason about:

* if one import is invalid, that import becomes `Ready=False`
* any invalid rule makes the entire resource `Ready=False`
* other valid imports and exports can still remain usable
* the final account state only includes resources that are ready

For example, if an import references a subject that does not exist as whole or subset in the export, that import does
not become active.

### How conflicts are handled

Because multiple imports and exports may contribute to the final account state, the controller must resolve them
deterministically.

This includes:

* deterministic ordering of imports and exports
* conflict detection when subjects overlap
* clear reporting when two resources cannot be merged safely

To identify conflicts, we will use nats.io's JWT imports validation function.

### Benefits:

* Imports and Exports can be created in any order.
* Allows for exporting of group of related subjects, for example, all JetStream API subjects.
* Open to future improvements such as restricted exports (handling activation tokens) and approval workflows.

### Drawbacks:

* More complex controller logic to handle the relationships and validation
* Needs to handle partial failures.

### Working Notes

These documents are working documents and notes related to the options. They might be incomplete and are not meant to be
part of the final ADR, but they are included here for reference.

* [appendix: Reconcile flow](./option-4-appendix-reconcile-flow.md)
* [appendix: Resource relationships and reconcile triggers](./option-4-appendix-resource-relationships.md)
* [appendix: Future improvements and notes](./option-4-appendix-future-improvements.md)
