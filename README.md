# gh pr-banner

[![Ask DeepWiki](https://deepwiki.com/badge.svg)](https://deepwiki.com/elecnix/gh-pr-banner)

A deterministic `gh` extension that sets, updates, clears, and inspects
**banner regions** in a pull request body — without ever regenerating the body.

A banner is a delimited region fenced by a pair of invisible HTML-comment
markers the extension owns:

```md
<!-- gh-pr-banner:do-not-merge -->

**⚠️ Do not merge — the mainline gate is red**
<!-- /gh-pr-banner:do-not-merge -->
```

When rendered, the HTML comments are invisible; only the banner text shows.
`set`, `clear`, and `get` operate _only_ on the region between the two markers
for a given name. **Everything outside the markers is byte-preserved** — the
extension fetches the current body, splices one region, and patches it back. It
is never asked to reproduce the prose it did not change.

## Why this exists

Regenerating a whole PR body to add or remove a one-line banner is expensive
(an entire multi-thousand-word body re-emitted) and unsafe (a regeneration is
a chance for a model to silently drop or mutate content it no longer remembers).
`gh pr-banner` makes a banner toggle a targeted splice: fetch, replace the
fenced region, patch. Nothing else moves, and no content is re-derived.

## Install

```sh
gh extension install elecnix/gh-pr-banner
```

Requires [the GitHub CLI](https://cli.github.com/) (used for authentication) and
Go when building from source. Precompiled binaries are published per release, so
install is a no-op for most users.

## Usage

Every command targets a PR by `--pr NUMBER` (or derives it from the current
branch when omitted) and `--repo OWNER/REPO` (or detects the current repo).

Alternatively, pass `--file PATH` to target a **local file containing a PR
body** instead of a PR. The same splice runs against the file's contents, and
the file is rewritten in place. This lets a caller stamp the body _before_
creating the PR — no body-edit event ever fires:

```sh
gh pr-banner tldr set --sha 1a2b3c4 --body "fixes the flaky retry loop" --file body.md
gh pr create --body-file body.md   # banner already inside; no edit needed
```

`--file` is mutually exclusive with `--pr` and `--repo` (passing both is an
error, and so is passing neither target). With `--file`, no repository or PR
is resolved and no network call is made; `set`, `clear`, `get`, `present`,
`list` and `tldr set`/`tldr get` all work, with the same idempotency and
malformed-body guarantees as the PR target.

```sh
# Add a banner (or update it in place if already present).
gh pr-banner set do-not-merge --body "staging is red — do not merge" --pr 42

# Read its current content (exit 0 if present, 2 if absent).
gh pr-banner get do-not-merge --pr 42

# Boolean probe for scripts.
gh pr-banner present do-not-merge --pr 42

# Remove it. Clearing an absent banner succeeds quietly.
gh pr-banner clear do-not-merge --pr 42

# List every banner currently in the body.
gh pr-banner list --pr 42
```

Multiple separately-named banners can coexist in one body — `do-not-merge`,
`needs-code-owner`, `build-report` — and each is set/cleared independently.

### The `tldr` banner kind

The `tldr` subcommand manages a reserved banner kind that pairs an
**author-written summary** — one or two sentences saying what a pull request
ships and why it matters — with the **head SHA it describes**:

```sh
# Stamp the summary and the SHA it describes.
gh pr-banner tldr set --sha 1a2b3c4 --body "fixes the flaky retry loop" --pr 42

# Read it back (exit 0 if present, 2 if absent).
gh pr-banner tldr get --pr 42
gh pr-banner tldr get --pr 42 --json   # {"banner": "...", "sha": "...", ...}
```

The banner region looks like this in the body:

```md
<!-- gh-pr-banner:tldr -->

<!-- gh-pr-banner:tldr-head-sha: 1a2b3c4 -->

fixes the flaky retry loop

<!-- /gh-pr-banner:tldr -->
```

The SHA line is itself an HTML comment, so it is invisible when the body
renders — only the summary shows. It stays machine-readable: `tldr get --json`
and the parser both return it, and bodies written by earlier versions (where
the SHA sat on a plain `tldr-head-sha:` first line) are still read back
correctly; only new writes use the comment form.

Design rules, all deliberate:

- **The SHA is stored, never enforced.** The banner carries it; freshness is
  computed by whoever reads the banner (for example, comparing changed-file
  sets of the stamped SHA against the current head, and rendering the summary
  marked stale when they differ). A head that moved with an identical file set
  (rebase, fixup) is still fresh. Nothing here blocks a merge — a tldr is
  never a gate or a check.
- **Never generated from the diff.** The summary is always written by a human;
  the command has no fallback that synthesizes one.
- **Clearing is shared with the generic commands.** `gh pr-banner clear tldr`
  and `gh pr-banner present tldr` work on the same region.

### Flags

| Flag                    | Meaning                                                                                                   |
| ----------------------- | --------------------------------------------------------------------------------------------------------- |
| `-R, --repo OWNER/REPO` | target repo (default: current directory's repo)                                                           |
| `--pr NUMBER`           | PR number (default: PR for the current branch)                                                            |
| `--file PATH`           | operate on a local file instead of a PR (rewritten in place; mutually exclusive with `--pr` and `--repo`) |
| `--at top\|bottom`      | where a _new_ region goes (`set` only; default top)                                                       |
| `--body TEXT`           | banner content (`set`; mutually exclusive with `--body-file`)                                             |
| `-F, --body-file PATH`  | read banner content from a file                                                                           |
| `--dry-run`             | resolve and print what would change; **never** write to GitHub                                            |
| `--json`                | machine-readable output                                                                                   |

## Semantics that matter

- **Deterministic splice, never regeneration.** The region between the markers
  is the only thing that changes.
- **Idempotent.** `set` with identical content is a no-op that makes **no**
  network write. `clear` of an absent banner succeeds quietly. Both are safe to
  call from a script that cannot check first.
- **Never destroys unmanaged content.** If the body's markers are malformed —
  unbalanced, overlapping, mis-nested, or duplicated — the command **fails
  loudly and changes nothing**. A body it cannot parse is an error, never a
  guessing opportunity.
- **Marker recognition is whole-line.** A line that merely contains
  marker-looking text inline (in a sentence or a code fence) is ordinary text
  and is never touched.

### Exit codes

| Code | Meaning                                                                        |
| ---- | ------------------------------------------------------------------------------ |
| `0`  | success (for `set`/`clear`/`list`; for `get`/`present`, the banner is present) |
| `2`  | a `get`/`present` found the banner absent — a state, not an error              |
| `1`  | a real error (network, malformed body, usage)                                  |

So a caller can branch without parsing output:

```sh
gh pr-banner present do-not-merge --pr 42
case $? in
  0) echo "frozen";;
  2) echo "open";;
esac
```

### Concurrency

Two agents banner-ing the same PR simultaneously is a classic lost-update race:
read → modify → write on a body, and the second writer's read is stale. GitHub
offers no server-side compare-and-swap on issue body content, so this is
handled **and** documented:

- Idempotency removes most writes — calling `set` when the banner already matches
  is a pure read (no PATCH, no race window).
- After any write the extension re-fetches and confirms the region landed as
  intended. If a concurrent writer replaced or corrupted it, the command **fails
  loudly with the unexpected state** instead of silently accepting a lost or
  clobbered banner.

A vanishingly small window remains (two writes to the _same_ banner name in the
same instant), but a lost region now surfaces as an explicit error rather than
silence.

## Marker format

```
<!-- gh-pr-banner:NAME -->

<banner content>

<!-- /gh-pr-banner:NAME -->
```

- `NAME` is `1–64` chars of `[a-z0-9]` and hyphens, starting alphanumeric.
- The opening marker is exactly `<!-- gh-pr-banner:NAME -->`.

## Development

```sh
go test ./...            # unit tests, incl. malformed-marker and phantom cases
script/build.sh          # build the published binaries
```

CI (`.github/workflows/ci.yml`) runs tests, `golangci-lint`, a prettier check on
markdown, and a cross-compile check of every published target. Releases are
built by [gh-extension-precompile](https://github.com/cli/gh-extension-precompile)
when a release is published.

## License

MIT. See [LICENSE](LICENSE).
