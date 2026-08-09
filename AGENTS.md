# AGENTS.md

Operating notes for coding agents working in this repository. Each rule exists
because getting it wrong has already cost a lost banner or a broken release.

## What the tool is, in one line

`gh pr-banner` splices a delimited, HTML-comment-fenced region into a pull
request body. It **never regenerates a body** — it fetches, replaces the fenced
region, and patches. This is the whole point of the tool. Do not "improve" it
into a body-rewriting tool.

The single most important property: **a body whose markers are malformed is an
error, not an opportunity.** The tool must refuse to guess at a damaged region
and must change nothing. If a change makes it silently tolerant of a damaged
body, it is a regression, no matter how well-tested the happy path is.

## Layout

- `internal/banner/` — pure, network-free marker logic. This is where the
  correctness lives: `splice.go` (parse/validate/apply) and `splice_test.go`.
  Any change here needs a test; the malformed-marker and phantom-marker cases
  are load-bearing.
- `internal/ghapi/` — thin client over the GitHub REST API via `gh api`
  (so the operator's existing `gh` auth applies). Only reads the body, patches
  the body, and resolves repo + PR number.
- `cmd/` — cobra commands. Thin wrappers: resolve target, fetch, apply, write
  - verify, emit.

## Concurrency model

GitHub has no conditional PATCH on issue bodies. The lost-update race is handled
by (a) idempotency — a `set` whose result equals the current body makes **no**
write at all — and (b) re-fetch-and-verify after every write: the region must be
exactly as written, else the command fails loudly. Keep both. Do not advertise
a stronger guarantee than this.

## Running inside a worktree

`go build` / `go test` fail in a linked worktree with:

```
error obtaining VCS status: exit status 128
```

Pass `-buildvcs=false`. This affects local runs only; CI checks out normally.

```sh
go test -buildvcs=false ./...
```

## Releasing

Read [`.github/workflows/release.yml`](.github/workflows/release.yml) before
cutting a release. Rules that matter:

1. **A tag-push run uses the workflow file at the tagged commit**, not the one
   on `main`. Merge a workflow fix first, then tag.
2. **Tags are lightweight and match `v*`.** Pushing the tag triggers the release:
   ```sh
   git tag v1.0.0 <sha> && git push origin v1.0.0
   ```
3. **`workflow_dispatch` does not work for releases** (the tag is derived from
   `github.ref_name`, which is the branch name on a manual dispatch).
4. Verify before announcing:
   ```sh
   gh release view v1.0.0 --json tagName,isDraft,assets
   ```
   Expect five assets built by `script/build.sh` via
   `cli/gh-extension-precompile`.

CI and releases run on **`ubuntu-latest`**. This is a deliberate deviation from the
sibling extension repos, which run on the self-hosted **`arc-runners`** label: the
ARC GitHub App is installed per-repo and does not cover a freshly created repo, so
jobs would hang queued forever. If a runner is ever registered for this repo, remate
the workflows to `arc-runners` to match. For release-time tooling, `ubuntu-latest`
ships `gh` preinstalled, so the install step in `release.yml` short-circuits.

## Pull requests

- PRs are **squash-merged**, so a branch's individual commits never appear on
  `main`.
- Titles follow [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/):
  `feat:`, `fix:`, `docs:`, `ci:`, `test:`.
- This repository is **public and on a personal account**. Keep examples,
  fixtures and prose generic — no private product names, trackers, hosts, or
  people anywhere in the tree, including git history.
