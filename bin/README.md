# bin -- repo tooling

Scripts that run from a checkout: the commit and CI gates, the release helpers,
the upstream sync, and the two curl-able entry points a machine fetches straight
from `main`. None of them are packaged; the desktop's own commands ship as RPMs
from `ryoku/`. Each script's header comment is its full usage.

## Gates

Run by the git hooks in `.githooks/` and by CI. A failure blocks the commit,
push, or merge.

| Script | Checks | Runs in |
|---|---|---|
| `ryoku-dev-scan-slop` | added comment lines carry no chat filler. `--staged` or `--range <base> <head>`. | `pre-commit`, `ai-slop.yml` |
| `ryoku-dev-verify-delivery` | every shipped config reaches an installed box through a package, the installer, or `deploy.sh` (`docs/updates.md`). | `pre-commit`, `post-commit`, `delivery-check.yml` |
| `ryoku-dev-verify-wm-isolation` | no compositor name, socket, or import leaks outside `ryoku/wm` (`docs/compositors.md`). | `pre-commit` |
| `ryoku-dev-verify-wm-vendor` | every vendored copy of `ryoku/wm` matches the original. | `pre-push` |
| `ryoku-dev-audit-shell-binds` | every `ryoku-shell <cmd>` a keybind fires is handled by the daemon. `--check` fails, `--refresh` rewrites `ryoku/shell/ipc/ipc-surface.txt`. | `shell-ipc-parity.yml` |
| `ryoku-dev-lint-qml <root>...` | QML still loads (the qmllint findings that stop a file instantiating). Needs the Qt modules installed, so run it after `ryoku deploy`. | manual, see `CONTRIBUTING.md` |

## Release

| Script | Does |
|---|---|
| `ryoku-release <X.Y.Z[-rc.N]>` | tags the tip of `main`; pushing the tag runs `publish-copr.yml` (`docs/updates.md`, "Cutting a release"). |
| `ryoku-release-notes <from> <to>` | builds the GitHub release body from the `Note:` commit trailers. Used by `ryoku-release` and `build-fedora-iso.yml`. |

## Upstream

| Script | Does |
|---|---|
| `ryoku-dev-sync-upstream` | merges upstream `ryoku-arch` into the current branch and resolves the conflicts that have a fixed answer. `--report` only classifies. Policy in `MergeToFedora.md`; run weekly by `upstream-sync.yml`. |

## Fetched by machines

These are served from `raw.githubusercontent.com/.../main/bin/`, so their path
is a public contract: do not move or rename them.

| Script | Does |
|---|---|
| `ryoku-track <main\|unstable-dev>` | builds a box from a git checkout and makes `ryoku update` follow the branch. Fetched by `ryoku track --source` (`ryoku/cli/track.go`). |
| `ryoku-recovery` | the panic button: pulls `main`, reinstalls the base packages, and redeploys the desktop over the user's configs. Linked from `README.md`; tested by `ryoku-recovery.yml`. |

## Art

`art/` bakes the decor set and the Hub's tiling previews. See `art/README.md`.

