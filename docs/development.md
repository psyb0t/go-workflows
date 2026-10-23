# Development

The host needs Docker, Make and Git. `Dockerfile.dev` pins Go and scanner tooling; `Dockerfile.ui` pins Node and pnpm. Make runs as the host UID/GID. Go dependencies and tools share one `go.mod`, `go.sum` and `vendor` tree.

## Commands

| Command | Result |
| --- | --- |
| `make run` | Execute the SQLite quick start |
| `make run-greeting` | Run a client and worker with in-memory SQLite |
| `make format` | Format first-party Go and shell source |
| `make lint` | Lint Go, shell, Dockerfiles and workflows |
| `make build` | Typecheck/build the UI and compile the library and samples |
| `make generate` | Run package-owned generators |
| `make test` | Complete race-enabled unit and real-backend tests |
| `make test-unit` | Unit tests without external database containers |
| `make test-coverage` | Complete suite and coverage gate |
| `make test-recovery` | Independent worker processes, forced crashes and persisted recovery |
| `make test-mysql` | One real backend; equivalents exist for postgres, redis, sqlite and monoprocess |
| `make sec` | Secret, filesystem and reachable Go vulnerability checks |
| `make lint-fix` | Apply first-party formatter/linter fixes |
| `make help` | Complete command index |

Integration targets mount the Docker socket and use host networking to reach dynamic loopback ports. Run them on a trusted development or CI host with a Unix Docker socket. Lint, build, generation and unit targets have no socket access. Missing Docker fails integration setup.

See [testing](testing.md) for suite selection, race instrumentation, real infrastructure, coverage gates and retained reports.

## Local resource limits

Go and UI containers have no CPU quota by default. To limit local runs, copy `.env.example` to `.env` and set `DEV_CPUS` to a positive Docker CPU quota such as `2.5`. An exported `DEV_CPUS` or a Make command-line assignment also works; command-line assignments override `.env`. With `CI=true`, Make ignores `.env` and CPU quotas, so GitHub Actions can use all CPUs available on its runner. Memory and PID limits remain unchanged.

## Dependencies

```bash
make pkg-lock
make pkg-add PKG=github.com/psyb0t/ctxerrors@v0.9.0
make pkg-add-tool PKG=github.com/vektra/mockery/v3@v3.7.0
make pkg-update PKG=example.org/module
make pkg-upgrade
make pkg-remove PKG=example.org/module
```

Replace `example.org/module` with an actual module. Dependency commands operate on the root module only. External additions and upgrades must be at least seven days old. First-party `psyb0t` modules follow their upstream release line. Upgrade candidates are checked before changes are applied; direct dependencies and tool providers are both included. Lock synchronization tidies and vendors, then runs security checks.

See [generated mocks](generation.md) for the Mockery configurations, imports, regeneration workflow and CI drift guard.

## Diagnostics UI

The React UI uses Vite and pnpm. `make ui-lock` updates its lockfile with a seven-day release-age floor. `make build-ui` installs the frozen lockfile, typechecks and updates embedded assets. Commit source, lockfile and rebuilt assets together. The Go diagnostics mux serves the UI; there is no production Node service.

## Security reports

`make sec` writes `sec.sarif` and fails on secrets, high/critical filesystem findings and reachable Go vulnerabilities. Reviewed exceptions in `.trivyignore.yaml` are scoped to exact lockfiles and advisories. They do not exempt the host Docker daemon from updates. Vulnerability checks include tooling dependencies. Secret scanning includes vendored source, with a narrow exception for a generated SQLite Windows type alias; minified browser bundles are excluded while their source is scanned separately.
