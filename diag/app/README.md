# Diagnostics web UI

React 18 and TypeScript, built with Vite and pnpm. The Go `diag` package embeds `build`; application users do not run Node.

From the repository root, run `make build-ui` to install locked dependencies, typecheck and rebuild the embedded assets. Run `make ui-lock` after changing dependency pins. Both use the pinned Node toolchain in `Dockerfile.ui` and preserve host ownership. Commit the lockfile and rebuilt assets with their source changes.

For backend integration and access restrictions, see [diagnostics](../../docs/backends.md#diagnostics).
