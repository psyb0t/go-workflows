# Workflow analyzer

The separate module `github.com/psyb0t/go-workflows/analyzer` checks workflow code for nondeterministic operations, including native goroutines, native channels, map iteration and disallowed calls. It recognizes the psyb0t workflow import path.

The analyzer uses Go's analysis framework and golangci-lint's module plugin API. Its fixture tests include valid and invalid workflows. From the repository root:

```bash
make test-analyzer
make lint
```

## Custom golangci-lint binary

The root [.custom-gcl.yml](../.custom-gcl.yml) selects this module locally. Build a custom binary with `make analyzer-plugin`. A consuming repository can instead select a published analyzer version in its custom-build configuration.

Enable the plugin in the consuming repository's golangci-lint configuration:

```yaml
version: "2"
linters:
  enable:
    - goworkflows
  settings:
    custom:
      goworkflows:
        type: module
        original-url: github.com/psyb0t/go-workflows/analyzer
```

The standard golangci-lint binary does not contain this plugin. Use the custom binary for these checks. The analyzer has its own Go module, vendor tree and `analyzer/vX.Y.Z` tags; see [releases](../docs/releases.md).
