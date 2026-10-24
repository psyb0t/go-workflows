# Agent integrations

The usage skill under `.agents/skills/go-workflows` teaches consumers to integrate the library. It is not a hosted API or MCP server. Keep examples aligned with executable samples and public APIs.

Keep both plugin manifests named `go-workflows`. The Claude manifest omits a version; the Codex manifest follows the release tag. This passive package has no hooks, installers or MCP bridge.

The reusable ClawHub job selects only `go-workflows` and no OpenClaw plugins. Publish after the release job, which waits for default-branch checks. Store `CLAWHUB_TOKEN` only in GitHub Actions secrets.
