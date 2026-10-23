# Changelog

## v1.0.0

- Adopt the `github.com/psyb0t/go-workflows` module path and matching analyzer module.
- Add Docker-backed build, formatting, lint, security, dependency and generation commands with locked Go tools and vendored dependencies.
- Run complete race-enabled backend tests against isolated MySQL, PostgreSQL and Redis services with dynamic loopback ports.
- Preserve non-retryable failures, joined causes and cancellation identity through persisted workflow errors.
- Propagate `ctxscope` metadata through workflows, subworkflows and activities while suppressing replayed workflow logs.
- Fix test scheduler timer cancellation and activity-completion races, and reject invalid registration values without reflection panics.
- Report resource cleanup errors and enable `errcheck` and source-security checks.
- Replace the diagnostics UI's Create React App build with a pinned Vite and pnpm toolchain.
- Validate release tags and their Go module major versions, wait for successful default-branch checks for the same commit, and reuse their coverage artifact.
- Add a runnable SQLite quick start, current usage documentation and upstream credits.
- Mirror Git refs to GitLab and Codeberg, archive repository pages, and synchronize mirror issues through reusable workflows.

### Migration

This fork starts a separate module release line. Update imports and dependency declarations:

| Previous module | New module |
| --- | --- |
| `github.com/cschleiden/go-workflows` | `github.com/psyb0t/go-workflows` |
| `github.com/cschleiden/go-workflows/analyzer` | `github.com/psyb0t/go-workflows/analyzer` |

Use the Go version in `go.mod` and run development commands through Make. Existing workflow histories retain their supported error wire fields; arbitrary application error types are not reconstructed after persistence.
