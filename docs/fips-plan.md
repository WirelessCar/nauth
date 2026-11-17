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
operators remain responsible for their platform. The user guide (Phase 5) says this explicitly.

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

## Phase 1: Code changes

Status: complete. 1.1 and 1.2 (with the CI/lint guards 4.1 and 4.6) landed in `9a9b28f`; 1.3 and 1.4
are done as described below.

### 1.1 Replace MD5 in Secret naming (`internal/core/secret.go`)

```go
func shortHashFromID(id string) string {
	sum := sha256.Sum256([]byte(id))
	return hex.EncodeToString(sum[:])[:6]
}
```

Replaces `mustGenerateShortHashFromID`. The panic path is gone: `sha256.Sum256` cannot fail.

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

### 1.3 Startup FIPS self-check (`cmd/fips.go`, `cmd/main.go`)

- At startup the manager logs `FIPS 140-3 status` with `enabled` (`crypto/fips140.Enabled()`),
  `moduleVersion` (`crypto/fips140.Version()`) and `required`.
- New flag `--require-fips` (default `false`). When set, the manager exits non-zero unless FIPS mode
  is enabled **and** the module is a frozen snapshot. A binary built with `GOFIPS140=latest` reports
  version `latest` and is rejected, because only frozen snapshots are CMVP-validated.
- The Helm chart sets `--require-fips` automatically for the FIPS variant (Phase 3.3).
- `Dockerfile` and `Makefile` (`build`, `run`) now build the `./cmd` package instead of the single
  file `cmd/main.go`, so `cmd/fips.go` is included.

Verified with real binaries:

| Build | `--require-fips` result |
|---|---|
| standard (`GOFIPS140=off`), also the current `Dockerfile` image | exits 1: "FIPS 140-3 mode is required but not enabled" |
| `GOFIPS140=latest` | exits 1: module "latest" is not a frozen, validated module |
| `GOFIPS140=v1.0.0` | check passes (`enabled: true`, `moduleVersion: v1.0.0`) |

### 1.4 Dependency audit (2026-11-17)

Non-FIPS-approved packages linked into `./cmd` and why each is acceptable under `fips140=on`:

| Package | Imported by | Reached at runtime? |
|---|---|---|
| `crypto/des`, `crypto/rc4`, `crypto/dsa` | standard library only (`crypto/tls`, `crypto/x509`) | No. In FIPS mode `crypto/tls` offers only approved suites and `crypto/x509` rejects DSA. |
| `crypto/md5`, `crypto/sha1` | `github.com/google/uuid` | No. Only name-based UUIDs (`NewMD5`, `NewSHA1`, `NewHash`) use them; a scan of all 894 dependency packages found no callers. |
| `crypto/sha1` | `github.com/nats-io/nats.go` (`ws.go:837`), `golang.org/x/net/websocket` | Only for WebSocket connections (`ws://`, `wss://`), to compute the RFC 6455 handshake accept key. That is a protocol requirement, not a security function, and SHA-1 remains available under `fips140=on`. |
| `golang.org/x/crypto/{curve25519,nacl/box,nacl/secretbox,salsa20,poly1305,blake2b}` | `github.com/nats-io/nkeys` (xkeys) | No. Neither nauth nor nats.go calls the xkeys API (`CreateCurveKeys`, `FromCurveSeed`, `Seal`, `Open`). |

Re-run on dependency bumps:

```sh
go list -deps ./cmd | grep -E '^(crypto/(md5|sha1|des|rc4|dsa)|golang.org/x/crypto)'
```

Direct imports in nauth are blocked by the `depguard` rule (Phase 4.1b). Use through dependencies
is only covered by this manual audit until strict mode becomes available.

## Phase 2: Dockerfile

Status: done. Implemented in `Dockerfile` (single file, both variants):

- `ARG GOFIPS140=off`. Allowed values are `off` or a frozen module version (`v<digits>...`); anything
  else, including `latest`, fails the build before compiling.
- The build passes `GOFIPS140` to `go build` and adds `-trimpath` for both variants.
- A verification step fails the FIPS build unless `go version -m manager` shows the requested
  `GOFIPS140` and `DefaultGODEBUG` containing `fips140=on`. Checked that the same test passes on a FIPS
  binary and fails on a standard one.
- The distroless runtime image is pinned to the multi-arch index digest
  `sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab` (2026-11-17).
  Renovate/Dependabot should keep it current.
- Label `io.nauth.fips140` records the module version (`off` for the standard image).

Notes:

- Any `GOFIPS140` value other than `off` makes `GODEBUG=fips140=on` the binary's default. No runtime
  env var is needed.
- `go.mod` has `godebug default=go1.25`. It does not override `fips140` (verified: the FIPS binary's
  `DefaultGODEBUG` contains `fips140=on`).

Verified locally (2026-11-17):

| Build | Result |
|---|---|
| `make docker-build` | label `off`; `--require-fips` exits 1 |
| `make docker-build-fips` | label `v1.0.0`; `--require-fips` passes, `moduleVersion: v1.0.0` |
| `--build-arg GOFIPS140=latest` | build fails with the allowed-values message |
| `GOFIPS140=v1.0.0` cross-compile for amd64, arm64, s390x, ppc64le | all build; each embeds `GOFIPS140=v1.0.0-c2097c7c` and `fips140=on` |

## Phase 3: Build, release and Helm

### 3.1 Makefile

Status: done.

- `GOFIPS140_VERSION ?= v1.0.0`.
- `docker-build-fips`: single-arch FIPS image tagged `${IMG}-fips`.
- `docker-buildx-fips`: multi-arch FIPS image tagged `${IMG}-fips`. It calls `docker-buildx` with
  `DOCKER_GOFIPS140=$(GOFIPS140_VERSION)`; `docker-buildx` now passes `--build-arg GOFIPS140`
  (default `off`) to its generated `Dockerfile.cross`.

### 3.2 GitHub workflows

Status: done. `prerelease.yml` calls `release.yaml`, so only `release.yaml` changed.

- The `docker-build` job has a matrix `variant: [standard, fips]`. The standard leg keeps its
  original name (`Build & Push Operator`); the FIPS leg is `Build & Push Operator (fips)`.
- The `fips` leg passes `GOFIPS140=v1.0.0` as a build arg and adds the `-fips` tag suffix through
  `docker/metadata-action` (`flavor: suffix=...`), e.g. `nauth-operator:0.9.0-fips`.
- New step `Verify FIPS mode` (fips leg only): runs the pushed image with `--require-fips` and fails
  unless the startup log shows `"enabled": true` with the expected module version and no
  "FIPS 140-3 check failed". Tested locally against a FIPS image (pass) and a standard image (fail).
- Trivy SARIF uploads use the category `trivy-<variant>` so the two scans do not overwrite each
  other. This also changes the category of the existing standard-image results in the Security tab.
- Not done: SBOM/provenance attestations. The workflow does not produce them for the standard image
  today either; adding them is a separate change.
- `trivy.yaml` (weekly scheduled scan) still builds only the standard image. The FIPS image shares its
  dependencies and base image, and every release scans both variants.

### 3.3 Helm chart (`charts/nauth`)

Status: done.

- New value `image.variant` (`""` or `fips`, enforced by `values.schema.json`).
- `fips` appends `-fips` to the image tag (app version or `image.tag`), unless the tag already ends
  in `-fips` (helper `nauth.imageTag` in `_helpers.tpl`), and adds `--require-fips` to the manager args.
- No `fips140=only` option for now (see "Strict mode").
- Tests: `charts/nauth/tests/deployment_fips_test.yaml` (4 cases); all 34 chart tests pass.
- `README.md` regenerated with `helm-docs`.
- `values.schema.json` was edited by hand: `mise run nauth:generate-values-schema` fails locally with
  `circular dependency detected: nats` even on the unchanged `values.yaml`. Re-run the generator
  where it works to confirm the hand edit matches its output.

## Phase 4: Verification

Status: all checks implemented. 4.2 has run locally for `basic-test` only; the full e2e suite runs in CI.

| # | Check | Where | Status |
|---|---|---|---|
| 4.1a | `make test` with `GOFIPS140=v1.0.0` | `test.yml` matrix leg `go test (fips)` | Done. Full suite passed locally with `GOFIPS140=v1.0.0`. |
| 4.1b | `depguard` denies `crypto/md5`, `crypto/sha1`, `crypto/des`, `crypto/rc4`, `golang.org/x/crypto` | `.golangci.yml` | Done. 0 issues; verified the rule fires on a probe MD5 import. |
| 4.2 | E2E against kind + NATS with the FIPS image | `test-e2e.yml` matrix leg `Run E2E Tests (fips)`; `kuttl-test.yaml` reads `NAUTH_E2E_IMAGE_VARIANT` / `NAUTH_E2E_GOFIPS140` | Done. Local run of `basic-test` with the FIPS image passed (2026-11-17). |
| 4.3 | Binary metadata | Dockerfile build step | Done. Fails the build unless `GOFIPS140` and `fips140=on` are embedded. |
| 4.4 | Startup log | E2E logs; `Verify FIPS mode` step in `release.yaml` | Done. E2E pod logged `{"enabled": true, "moduleVersion": "v1.0.0", "required": true}`. |
| 4.5 | Negative test | `cmd/fips_test.go`, manual image run | Done. Standard image with `--require-fips` exits 1; `GOFIPS140=latest` is rejected too. |
| 4.6 | Secret migration regression | `internal/core/secret_test.go` | Done. Secrets with MD5-derived names are still found by label. |

Running the FIPS e2e variant locally:

```sh
make build-e2e-runner
NAUTH_E2E_IMAGE_VARIANT=fips NAUTH_E2E_GOFIPS140=v1.0.0 PATH="$PWD/bin:$PATH" e2e-runner --test basic-test
```

## Phase 5: Documentation

Status: done. The user guide lives with the other guides on the docs site rather than in `docs/`:
`www/src/content/docs/guides/fips.mdx`, linked from the sidebar in `www/astro.config.mjs` as
"FIPS 140-3". It covers:

- What the `-fips` image is and how it is built (module `v1.0.0`, `fips140=on` by default).
- Compliance scope and the operating-environment disclaimer.
- Installing with `image.variant=fips` and verifying via the startup log and image label.
- TLS peer requirements (TLS 1.2+, approved suites, groups such as P-256/P-384; X25519-only NATS fails).
- Why `fips140=only` is not supported (nkeys).
- The Secret naming change and why existing Secrets keep working.

The CMVP certificate number is not in the guide yet (see Open items); it links to Go's FIPS 140-3 page.
The docs site builds locally (`bun run build`) and renders `/guides/fips/`.

## Order of work

1. Phase 1.1, 1.2, the regression test in 4.6, and the CI job in 4.1. These carry most of the risk.
2. Phase 1.3 and 1.4.
3. Phase 2 and 3.1, then verify locally with 4.3.
4. Phase 3.2, 3.3, then 4.2, 4.4 and 4.5.
5. Phase 5.

## Open items

- Upstream change to nats-io/nkeys: use `ed25519.NewKeyFromSeed` so nauth can run under `fips140=only`.
- Get the CMVP certificate number for Go Cryptographic Module `v1.0.0` and add it to
  `www/src/content/docs/guides/fips.mdx`.
- Re-check `$GOROOT/lib/fips140/certified.txt` on each Go toolchain bump. Move to `v1.26.0` once it is
  certified.
- Confirm the TLS settings of the NATS clusters nauth targets in production (Phase 4.2 surfaces problems).
