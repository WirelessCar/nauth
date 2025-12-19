# Repository Guidelines

## Project Structure & Module Organization
Go controller code lives under `cmd/main.go` and `internal/` (controller logic, services, helpers). Custom resource types and generated clients sit in `api/`, while installable manifests originate from `charts/nauth` and `config/`. Tooling and boilerplate live in `hack/`, reusable scenarios in `examples/`, UI assets in `www/`, and Kind-based suites in `test/e2e`. The `operator-bootstrap/` directory hosts CLI aids for local operators; extend modules in place to keep this map predictable.

## Build, Test, and Development Commands
`make help` shows the curated task list. `make build` compiles the controller into `bin/manager`, and `make run` executes it locally against your current kubeconfig. Run `make test` for unit and integration coverage (envtest binaries are bootstrapped automatically). `make test-e2e` runs the KUTTL suite under `test/e2e` and will create/delete the Kind cluster as part of the run. `make lint` (or `make lint-fix`) invokes `golangci-lint`, and `make build-installer` emits `dist/install.yaml` for packaging.

## Coding Style & Naming Conventions
The project targets Go 1.22+ with modules tracked in `go.mod`. Run `go fmt`/`make fmt` to enforce standard formatting; imports stay grouped (std, third-party, internal). Keep Go files tab-indented, use explicit struct field names, and name controllers `<resource>Controller`. Tests live in `_test.go` files, table cases use `t.Run` with descriptive snake_case labels, and generated code marked `// Code generated` must remain untouched.

## Testing Guidelines
Write unit tests beside the code inside `internal/...`; prefer fake clients over live clusters. For CRD changes, add regression coverage in `test/e2e` and document KUTTL setup requirements. `make test` emits `cover.out`; inspect with `go tool cover -html=cover.out`. After touching APIs or manifests, run `make generate` and `make manifests`, committing both sources and derived YAML.

## Commit & Pull Request Guidelines
Commit messages follow a Conventional Commits style (`feat:`, `fix:`, `chore:`) with present-tense summaries; optional scopes (`refactor:`, `docs:`) improve triage. Squash fixups before opening a PR. Pull requests should describe motivation, link Jira or GitHub issues, and include screenshots when UI assets change. Always run `make fmt lint test` and confirm generated files stay synchronized.

## Security & Configuration Tips
Avoid committing real NATS credentials; rely on fixtures in `examples/` or create throw-away operators through `operator-bootstrap`. When testing cluster installs, use the default Docker tagging scheme (`IMG=ghcr.io/wirelesscar/nauth:<tag>`) and clean up CRDs with `make uninstall` to prevent drift across namespaces.
