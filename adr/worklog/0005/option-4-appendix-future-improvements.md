# Option 4 - Future improvements and Notes

## How updates and drift are handled

`AccountImport` binds to an `AccountExport` (`exportRef`) and asserts the expected exporting account
(`exporterAccountRef`).

`ruleBindings[].expected` declares optional invariants that the referenced export rule must satisfy.
If any expected field does not match the resolved export rule, the `AccountImport` is not ready.

`updatePolicy` does not affect validation semantics. It only controls whether the import automatically adopts upstream
export changes that continue to satisfy the declared expectations.

This means compatible upstream changes can be handled in two ways:

* `automatic`: the import adopts the updated export state automatically
* `manual`: drift is detected and surfaced, and the import stops advancing until the change is explicitly accepted

This gives a clearer operational model for upstream changes without weakening validation.

## Public and restricted exports and allow-lists

Introducing restricted import requires import/export activation tokens to be implemented. This has its own complexity,
and is therefore deferred to the future.

Things to consider:

* signing keys
* signed tokens, one for each subject
* token distribution and rotation

```yaml
AccountExport:
    allowedAccountRefs: # only these accounts are allowed to import this export
        -   name: Account B
            namespace: namespace-b
```

Public exports are the default and do not require any additional fields.
Any account can reference a public export.

## Full example

```yaml
AccountExport:
    name: jetstream-api
    namespace: namespace-a
    accountRef:
        name: Account A           # expected to be in same namespace
        namespace: namespace-a
    access:
        mode: restricted       # restricted = only allowed accounts can import, public = any account can import
        allowedAccountRefs: # only needed if mode is restricted, otherwise ignored
            -   name: Account B
                namespace: namespace-b
    rules:
        -   name: info              # unique name for reference in import
            subject: "$JS.API.INFO"
            type: service
            responseType: single
        -   name: stream-info
            subject: "$JS.API.STREAM.INFO.*"
            type: service
            responseType: single
        -   name: with-extra-options
            subject: "my.subject.>"
            type: stream
            options: # this is optional extra parameters (example struct below)
                advertise: true
                allowTrace: true  
```

`rules[].name` must be unique within the export and is used for reference in the import.

```go
package v1alpha2

type AccountExportRule struct {
    Name         string       `json:"name,omitempty"`
    Subject      Subject      `json:"subject,omitempty"`
    Type         ExportType   `json:"type,omitempty"`
    ResponseType ResponseType `json:"responseType,omitempty"`

    ExportOptions *ExportOptions `json:"options,omitempty"`
}

type ExportOptions struct {
    Revocations          RevocationList   `json:"revocations,omitempty"`
    AccountTokenPosition *uint            `json:"accountTokenPosition,omitempty"`
    Advertise            *bool            `json:"advertise,omitempty"`
    ResponseThreshold    *metav1.Duration `json:"responseThreshold,omitempty"`
    AllowTrace           *bool            `json:"allowTrace,omitempty"`
    Latency              *ServiceLatency  `json:"latency,omitempty"`
}

```

And the corresponding import:

```yaml
AccountImport:
    name: account-a-jetstream
    namespace: namespace-b
    accountRef:
        name: Account B
        namespace: namespace-b
    exporterAccountRef:
        name: Account A
        namespace: namespace-a
    exportRef:
        name: jetstream-api
        namespace: namespace-a
    updatePolicy: automatic             # automatic (default) = advance status when export changes compatibly, 
    # manual = detect drift and stop advancing until spec says to accept it
    localSubjectPrefix: ext.account-a   # optional prefix to add to all imported subjects, for example, to avoid conflicts with local subjects
    ruleBindings: # required, non-empty list
        -   name: info                  # export rule name
            expected: # validation only fields 
                subject: "$JS.API.INFO"     # validate remote exported subject
                type: service               # validate type   
            importOptions:
                localSubject: "ext.account-a.$JS.API.INFO"    # overrides local subject for this exported rule
                allowTrace: true            # optional override, allowing trace
        -   name: stream-info
        -   name: with-extra-options

```

**Imported subject resolution order**

* If localSubject is defined, the imported subject is the localSubject.
* If localSubjectPrefix is defined, the imported subject is localSubjectPrefix + "." + export subject.
* Otherwise, the imported subject is the same as the export subject.

