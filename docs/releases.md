# Releases

The canonical version is the Git tag. The root module is `github.com/psyb0t/go-workflows`; the analyzer is the separate module `github.com/psyb0t/go-workflows/analyzer`.

The first release of this module path starts at `v1.0.0`, not the upstream project's version sequence. Analyzer tags use the subdirectory prefix, for example `analyzer/v1.0.0`. Future incompatible public releases need both a major tag and the matching `/vN` module path. Preserve the original MIT copyright and license.

## Pipeline

Default-branch pushes run reusable Make-backed lint, build, complete coverage, security and generation checks. Non-draft pull requests run those checks too. A weekly whole-pipeline run refreshes coverage and importer badges.

Tags first pass `make check-release-tag`, which validates SemVer and the Go module's major-version path. They then call `wait-for-default-branch.yml` before publication. It requires a successful default-branch `pipeline` run for the tag's exact commit and returns its run ID. Root-module badges reuse that coverage artifact; analyzer tags do not overwrite root-module badges. An invalid tag or a failed, missing or timed-out default-branch run prevents release.

Releases are Go source modules, not application Docker images. Dockerfiles contain development toolchains. Both module lockfiles and vendor trees belong in the release. Local databases, dependency caches, release scripts, credentials and editor state do not.

## Repository automation

The badges job publishes coverage, version, license and Go importer badges. Default-branch badges use that run's coverage; tag badges reuse the successful same-commit default-branch artifact. The weekly pipeline refreshes importer counts.

Pushes mirror Git refs to GitLab and Codeberg. Default-branch and tag pushes also archive the repository page, with a monthly archive refresh. A scheduled issue-sync workflow imports issues from both mirrors. The collaborators-only workflow closes external pull requests without checking out their code.

Set the repository Actions secrets `GITLAB_TOKEN`, `CODEBERG_TOKEN`, `ARCHIVEORG_ACCESS_KEY` and `ARCHIVEORG_SECRET_KEY` for mirror and archive access. The build, badges, release and collaborators-only jobs use GitHub's job token.

## Before tagging

Run `make format lint build`, `make test-coverage`, `make sec` and `make generate`. Check that generation leaves no unexpected changes, read the changelog and wait for default-branch checks at the intended commit. Verify both module paths before selecting tags. Never reuse a published tag.
