# FIPS 140-3 Image Plan

Status: proposed
Branch: `feat/fips-image`

## Goal

Publish a FIPS variant of the nauth operator image (`<image>:<tag>-fips`) whose binary uses a
CMVP-validated cryptographic module, running in FIPS mode by default, alongside the existing
standard image.

## Decisions

| Topic | Decision | Rationale |
|---|---|---|
| Crypto provider | Go native FIPS 140-3 module (`GOFIPS140`) | Keeps `CGO_ENABLED=0`, the static distroless base and all target architectures. No toolchain fork. |
| Module version | `GOFIPS140=v1.0.0` (resolves to `v1.0.0-c2097c7c`) | Listed in `$GOROOT/lib/fips140/certified.txt` for Go 1.27.0. `v1.26.0` is still in `inprocess.txt` and must not be used for compliance claims. Re-evaluate when it moves to `certified.txt`. |
| MD5 usage | Replace with SHA-256 | MD5 is not FIPS-approved and fails under `GODEBUG=fips140=only`. |
| Runtime base image | Keep `gcr.io/distroless/static-debian12:nonroot`, pinned by digest | The binary is static; the base image ships no crypto that the operator uses. |
| Image layout | One `Dockerfile` with a `GOFIPS140` build arg | Avoids two Dockerfiles drifting apart. |

### Scope of the compliance claim

The FIPS image **uses a CMVP-validated module in approved mode**. That does not make the image or
nauth itself certified. The module's security policy lists the tested operating environments, and
operators remain responsible for their platform. `docs/fips.md` (Phase 5) must say this explicitly.

## Current state

| Location | Crypto use | FIPS status |
|---|---|---|
| `internal/core/{account,user,account_signing_key}.go`, `internal/adapter/outbound/k8s/cluster.go` | nkeys: Ed25519 key generation, `FromSeed`, signing | Approved (FIPS 186-5), inside the Go module |
| `github.com/nats-io/jwt/v2` | Ed25519 signatures, SHA-256/512 | Approved |
| `internal/domain/nats.go`, `internal/domain/nauth/account.go` | SHA-256 | Approved |
| `internal/core/secret.go:114` `mustGenerateShortHashFromID` | MD5 → 6-char Secret name suffix | **Not approved**. Replaced with SHA-256 (Phase 1.1). |
| `internal/testutil/scope.go:31` `ShortHash` | MD5 | **Not approved** (tests only, but breaks `fips140=only` test runs) |
| `cmd/main.go` metrics server, client-go, nats.go | `crypto/tls` | In FIPS mode Go restricts TLS to approved versions, cipher suites and groups. Peers must support them. |
| nkeys xkeys (`golang.org/x/crypto/curve25519`) | X25519 / XSalsa20 | Not approved. Linked through nkeys but not called by nauth. |
| nkeys `keypair.go` (v0.4.16) | Derives every key from its seed with `ed25519.GenerateKey(bytes.NewReader(seed))` | Approved algorithm, and correct under `fips140=on` (the stdlib reads the seed from the supplied reader). **Rejected under `fips140=only`**, which allows only `crypto/rand.Reader`. See "Strict mode". |

### Strict mode (`GODEBUG=fips140=only`) is not usable yet

Running the test suite with `GODEBUG=fips140=only` (Phase 1 work, 2026-11-17) fails in
`nkeys.FromSeed`, `PublicKey` and `Sign` with:

```
crypto/ed25519: only crypto/rand.Reader is allowed in FIPS 140-only mode
```

Cause: nkeys v0.4.16 derives Ed25519 keys via `ed25519.GenerateKey(bytes.NewReader(seed))` instead of
`ed25519.NewKeyFromSeed(seed)`. Both produce the same keys, but `only` mode rejects any reader other
than `crypto/rand.Reader`. Every signing path in nauth goes through this code.

Consequences:

- The FIPS image runs in `fips140=on` (the `GOFIPS140` default). This is unaffected: verified by running
  the full test suite built with `GOFIPS140=v1.0.0`.
- The planned `fips140=only` CI job and the `fips.strict` Helm option are dropped until nkeys changes.
- Regressions to non-approved algorithms are caught by a `depguard` lint rule instead (Phase 4.1).
- Follow-up: propose upstream to nats-io/nkeys switching to `ed25519.NewKeyFromSeed`. Once released,
  reinstate a `fips140=only` CI job.

Unrelated bug found during review: `Makefile` target `docker-buildx` references `Dockerfile.cross`,
which does not exist.

## Phase 1: Code changes

Status: 1.1, 1.2 and the CI/lint guards (4.1, 4.6) are implemented. 1.3 and 1.4 are next.

### 1.1 Replace MD5 in Secret naming (`internal/core/secret.go`)

```go
func mustGenerateShortHashFromID(id string) string {
	sum := sha256.Sum256([]byte(id))
	return hex.EncodeToString(sum[:])[:6]
}
```

The `must` prefix and panic path can go: `sha256.Sum256` cannot fail. Rename to
`shortHashFromID`.

**Migration impact (verified against the current code):**

- Account Secrets are **written only when an account is created** (`internal/core/account.go:123-143`,
  inside the `!found` branch).
- Existing Secrets are **read by label**, not by name (`getAccountSecretsByAccountID` /
  `getAccountSecretsByAccountName` filter on `account.nauth.io/id` / `account.nauth.io/name` and the
  managed label).
- Result: existing `<account>-ac-root-<md5>` / `<account>-ac-sign-<md5>` Secrets keep working
  unchanged. Only accounts created after the upgrade get SHA-256 suffixes. No rename or relabel job
  is required.

Risk to guard with tests: `getAccountSecretsFromK8sSecrets` requires exactly two Secrets. Because
nothing re-applies Secrets for an existing account, an MD5-named pair and a SHA-256-named pair should
never coexist. Add a regression test that reconciles an account with pre-existing MD5-named
Secrets and asserts that no new Secrets are created and the account keys are unchanged.

### 1.2 Replace MD5 in test helpers (`internal/testutil/scope.go`)

Same change for `ShortHash`. The output only names test resources, so there is nothing to migrate.

### 1.3 Startup FIPS self-check (`cmd/main.go`)

- Log `fips140.Enabled()` (package `crypto/fips140`) and the module version at startup.
- Add a `--require-fips` flag. When set and `fips140.Enabled()` is false, log an error and exit
  non-zero.
- The Helm chart sets `--require-fips` automatically for the FIPS variant (Phase 3.3).

### 1.4 Dependency audit

```sh
go list -deps ./cmd | grep -E 'golang.org/x/crypto/(curve25519|nacl|chacha20|salsa20)'
```

Record which non-approved packages are linked and confirm none are reachable from nauth code paths.
Direct imports are blocked by the `depguard` rule in Phase 4.1; transitive use through dependencies
still needs this manual audit while strict mode is unavailable.

## Phase 2: Dockerfile

```dockerfile
FROM golang:1.27.0-alpine@sha256:4c9fe60190a2a3350ddc51de80d0224b8a6698d12bdfc999fee45ea9d6c46dbc AS builder
ARG TARGETOS
ARG TARGETARCH
# off = standard image; v1.0.0 = FIPS image (CMVP-validated Go module snapshot)
ARG GOFIPS140=off

WORKDIR /workspace
COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download

COPY cmd/main.go cmd/main.go
COPY api/ api/
COPY internal/ internal/

RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} GOFIPS140=${GOFIPS140} \
    go build -trimpath -a -o manager cmd/main.go

# Fail the build if FIPS settings did not land in the binary
RUN if [ "${GOFIPS140}" != "off" ]; then \
      go version -m manager | grep -q 'GOFIPS140=v1.0.0' && \
      go version -m manager | grep -q 'fips140=on' \
      || { echo "FIPS build verification failed"; go version -m manager; exit 1; }; \
    fi

FROM gcr.io/distroless/static-debian12:nonroot@sha256:<pin>
ARG GOFIPS140=off
LABEL io.nauth.fips140="${GOFIPS140}"
WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532
ENTRYPOINT ["/manager"]
```

Notes:

- Any `GOFIPS140` value other than `off` makes `GODEBUG=fips140=on` the binary's default. No runtime
  env var is needed.
- `go.mod` has `godebug default=go1.25`. That does not override `fips140`; confirm with the build
  verification step above.
- Pin the distroless runtime image by digest (it currently floats on `:nonroot`). Renovate/Dependabot
  can keep the digest current.
- `-trimpath` is added for reproducibility in both variants.

## Phase 3: Build, release and Helm

### 3.1 Makefile

```make
GOFIPS140_VERSION ?= v1.0.0

.PHONY: docker-build-fips
docker-build-fips: ## Build the FIPS variant of the manager image
	$(CONTAINER_TOOL) build --build-arg GOFIPS140=$(GOFIPS140_VERSION) -t ${IMG}-fips .
```

Also add `docker-buildx-fips`, and fix `docker-buildx` to use `Dockerfile` instead of the missing
`Dockerfile.cross`.

### 3.2 GitHub workflows

- `release.yaml` / `prerelease.yml`: add a matrix `variant: [standard, fips]` to the `docker-build`
  job.
  - `fips` passes `build-args: GOFIPS140=v1.0.0`.
  - `docker/metadata-action` adds `suffix=-fips` for the fips variant.
  - Each variant gets its own SBOM and provenance attestation.
- `trivy.yaml`: scan both variants.

### 3.3 Helm chart (`charts/nauth`)

- New value `image.variant: ""` (`"fips"` appends `-fips` to the tag).
- When `image.variant == "fips"`, add the `--require-fips` arg.
- No `fips140=only` option for now (see "Strict mode").
- Update the chart README and `values.schema.json` if one exists.

## Phase 4: Verification

| # | Check | Where | Pass criteria |
|---|---|---|---|
| 4.1a | `make test` with `GOFIPS140=v1.0.0` | `test.yml` matrix leg `go test (fips)` (implemented) | All tests pass with the binary defaulting to `fips140=on`. |
| 4.1b | `depguard` denies `crypto/md5`, `crypto/sha1`, `crypto/des`, `crypto/rc4`, `golang.org/x/crypto` | `.golangci.yml` (implemented) | `make lint` reports 0 issues. Verified the rule fires on a probe MD5 import. |
| 4.2 | E2E against kind + NATS with the FIPS image | `test-e2e.yml` matrix | Accounts and users reconcile; TLS to kube-apiserver and NATS succeeds. |
| 4.3 | Binary metadata | Dockerfile build step | `go version -m` shows `GOFIPS140=v1.0.0` and `DefaultGODEBUG` contains `fips140=on`. |
| 4.4 | Startup log | E2E | Log line reports FIPS enabled and module `v1.0.0`. |
| 4.5 | Negative test | E2E or unit | Standard image with `--require-fips` exits non-zero. |
| 4.6 | Secret migration regression | Unit (`internal/core`) | Account with pre-existing MD5-named Secrets reconciles without creating new Secrets or keys (see 1.1). |

## Phase 5: Documentation

Add `docs/fips.md` covering:

- What the `-fips` image is, and the Go module version with its CMVP certificate reference.
- The image runs with `fips140=on`; `fips140=only` is not supported yet and why (nkeys).
- TLS peer requirements: TLS 1.2 or later, approved cipher suites, and key-exchange groups such as P-256
  or P-384. NATS servers configured for X25519-only key exchange will fail to connect.
- The Secret naming change: new accounts get SHA-256 suffixes, existing Secrets are untouched.
- Compliance scope and the operating-environment disclaimer (see "Scope of the compliance claim").

## Order of work

1. Phase 1.1, 1.2, the regression test in 4.6, and the CI job in 4.1. These carry most of the risk.
2. Phase 1.3 and 1.4.
3. Phase 2 and 3.1, then verify locally with 4.3.
4. Phase 3.2, 3.3, then 4.2, 4.4 and 4.5.
5. Phase 5.

## Open items

- Upstream change to nats-io/nkeys: use `ed25519.NewKeyFromSeed` so nauth can run under `fips140=only`.
- Get the CMVP certificate number for Go Cryptographic Module `v1.0.0` and reference it in `docs/fips.md`.
- Re-check `$GOROOT/lib/fips140/certified.txt` on each Go toolchain bump. Move to `v1.26.0` once it is
  certified.
- Confirm the TLS settings of the NATS clusters nauth targets in production (Phase 4.2 surfaces problems).
