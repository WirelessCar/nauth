<!-- vale off -->
# ADR-6: TBD — NAuth interpolation syntax
<!-- Remove this and the above vale off command after creation, it's only to not get false vale errors from the template -->

Date: 2026-09-17

## Problem statement

NAuth manages NATS subject-bearing fields across resources and claims, but some values are only known during reconciliation. 
NAuth therefore needs a named, NAuth-wide interpolation syntax that survives Helm and is resolved before the affected resource or claim is emitted.

An unresolved marker must not be treated as a valid literal subject. 
NAuth must detect and reject it before considering the affected resource successfully reconciled.

## Status

In progress

## Context

NAuth applies to subject-bearing fields across resources and claims, including Account JWT imports and exports and User JWT permissions. 
Some values are unavailable when manifests are authored. 
JetStream Flow Control V2 is one example: its subject includes an account hash associated with the exporting account. 
This value is not generally available when the manifest is authored and must be resolved during reconciliation. 
Other values may only be available during a later NAuth-controlled rendering step, such as Auth Callout processing.

This ADR defines only the reserved marker syntax. 
Value resolution and the complete set of supported fields are outside its scope.

The evaluation criteria are:

1. **Named:** the marker must identify an allow-listed value or field path.
2. **Fail-closed during reconciliation:** an unresolved marker must be rejected before NAuth considers the affected resource reconciled. NATS rejection is preferred; validation during input processing or reconciliation is acceptable.
3. **Helm-compatible:** Helm must either pass the marker through unchanged or provide a deterministic, documented escape that emits it literally. The marker must be usable as a quoted YAML string value; unquoted use is not required.
4. **Practically safe:** the syntax must be intuitive and easy to type. Formatting mistakes must not silently turn an intended placeholder into a valid literal subject.
5. **Established:** prefer syntax used by an existing template or interpolation system; a new NAuth-specific pattern is a last resort.

NATS JWT and server subject validation reject ASCII whitespace, including space, while accepting printable punctuation as subject text. 
Of the ordinary printable characters, space is therefore the only practical character available for a strict-rejection pattern.

NATS also has feature-specific template-like syntax. User permission subjects support NATS-owned `{{...}}` operations, while subject mappings support `{{wildcard(...)}}` and legacy `$1`. These are not general NAuth value references; NAuth must distinguish them and resolve its own markers before NATS processes the resulting claim.

## Options

The following syntax families and syntaxes are candidates. 
Each option requires NAuth-side validation to reject unresolved placeholders before NAuth reports the affected resource or rendering operation as successful; unresolved markers otherwise remain valid NATS subjects. 
NAuth would support only allow-listed field paths inside the selected delimiters, not the source language's general expression semantics. 
This list is deliberately unresolved.

### Option 1: Require whitespace in placeholders

Whitespace-dependent placeholder syntax

Marker grammar: the selected delimiters require one or more ASCII space characters around or within the allow-listed field path. No concrete delimiter is selected by this option.

Used in: no established general interpolation system as a subject-placeholder convention.

**Pros**

- A correctly formed unresolved marker contains a space and is rejected by NATS subject validation.

**Cons**

- Whitespace is mandatory.
- A formatting mistake that removes the space is not substituted and remains a valid NATS subject.
- No concrete established syntax is identified; choosing one would make the pattern NAuth-specific.
- Making malformed variants fail closed would require NAuth to reserve and reject the surrounding delimiter family, so the safety would no longer come from NATS alone and otherwise valid literal subjects could be reserved.

### Option 2: Use Go-template field paths

`{{ .nauth.account.hash }}`

Used in: Go templates and Helm. Helm requires an escape when the marker must survive Helm rendering, for example:

```yaml
subject: '$JS.FC._.{{ `{{ .nauth.account.hash }}` }}.my-stream.>'
```

**Pros**

- Conventional spacing, as in `{{ .nauth.account.hash }}`, makes an unresolved marker contain ASCII spaces, so NATS rejects it.
- Familiar to Go and Helm users.
- Supports dotted field paths rooted in `.nauth`.
- Existing Go template tooling can support implementation.

**Cons**

- Requires explicit Helm escaping and has two rendering stages.
- Go also accepts whitespace-free forms such as `{{.nauth.account.hash}}`; those remain valid NATS subjects, so NAuth must validate unresolved markers independently and cannot rely on whitespace universally.
- Shares delimiters with NATS-owned templates. NAuth must preserve only recognized NATS expressions in fields where NATS supports them, treat only `{{ .nauth.<field> }}` as NAuth syntax, and reject all other `{{...}}` content.

### Option 3: Use Kubernetes-style variable references

`$(accountHash)`

Used in: Kustomize variable substitution and Kubernetes environment-variable expansion.

**Pros**

- Helm passes it through unchanged.
- Familiar to Kubernetes users.
- No whitespace-sensitive formatting.

**Cons**

- `$()` conventionally suggests shell command substitution.

### Option 4: Use braced variable interpolation

`${accountHash}`

Used in: Terraform/HCL string templates, shell-style expansion, and JavaScript and TypeScript template literals.

**Pros**

- Familiar and concise.
- Helm passes it through unchanged.
- Supports a clearly delimited name.

**Cons**

- May be interpreted by another string-rendering system before NAuth.

### Option 5: Use bare braced replacement fields

`{accountHash}`

Used in: [URI Templates](https://www.rfc-editor.org/rfc/rfc6570) and Python f-string replacement fields.

**Pros**

- Concise.
- Helm passes it through unchanged.

**Cons**

- Bare braces are weakly distinguished from literal or URI-template text.
- It is less familiar in Go and Kubernetes.

## Other syntaxes considered

- `${{accountHash}}`: used in GitHub Actions, but combines two interpolation conventions, conflicts with Helm's `{{...}}` actions, and adds no value compared with the Go-template option.
- `$accountHash`: used in shell-style variable expansion, but has ambiguous variable boundaries and does not naturally support dotted field paths.
- `#{accountHash}`: used in Ruby and Elixir, but is unfamiliar in the Go and Kubernetes ecosystem.
- `<%=accountHash%>`: used in Ruby ERB, but is verbose and tied to output-expression semantics.
- `@(accountHash)`: used in ASP.NET Core Razor, but is unfamiliar in the Go and Kubernetes ecosystem and associated with general expressions.

## Decision

_The proposed decision. If accepted, implementation begins._

## Consequences

_What becomes easier, more difficult and any risks introduced by the decision. Document risk mitigation strategies here._
