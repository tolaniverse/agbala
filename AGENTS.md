# AGENTS.md

How work is done in this repository. This file is the source of truth for
workflow; `CLAUDE.md` covers the architecture and links here rather than
restating any of it.

## Commands

```sh
make build       # build ./agbala
make test        # go test -race -count=1 ./...
make lint        # golangci-lint (pinned to v2.13.1, see .github/workflows/ci.yml)
make check-fmt   # fail if anything is unformatted
make bench       # render benchmarks
make golden      # regenerate golden files — always review the diff before committing
make help        # everything else
```

Run a single test: `go test ./internal/theme/ -run TestRailWidth -v`.

Requires Go 1.26+. `CGO_ENABLED=0` everywhere — the host binary ships as a single
static executable with no runtime to install, and that is a product requirement
rather than a preference.

## Branches and pull requests

Branch names are prefixed `feat/` or `fix/`. Nothing else.

Work is delivered as **stacked pull requests**, capped at **four PRs per stack**.
Four is a review budget, not a target: a stack that would run longer should be
split into a second stack rather than stretched. Each branch is one discrete,
independently reviewable concern, and layers are ordered by dependency —
foundations at the bottom, consumers on top.

### gh-stack must be driven non-interactively

Every one of these commands opens a prompt or a TUI without the flag shown, and
will hang forever in an agent session:

| Do | Never |
| --- | --- |
| `gh stack view --json` | `gh stack view` |
| `gh stack submit --auto` | `gh stack submit` |
| `gh stack init feat/thing` | `gh stack init` |
| `gh stack add feat/other` | `gh stack add` |
| `gh stack checkout feat/thing` | `gh stack checkout` |

Configure once per clone, so `init` and `push` never stop to ask:

```sh
git config rerere.enabled true
git config remote.pushDefault origin
```

To change a lower layer while working on a higher one, do not patch around it at
the top. Navigate down (`gh stack down`), commit the change where it belongs,
`gh stack rebase --upstack`, then `gh stack top` and carry on. A fix committed to
the wrong branch lands in the wrong PR and pollutes both diffs.

On a rebase conflict `gh stack rebase` exits 3: resolve the files, `git add`
them, then `gh stack rebase --continue`. `gh stack rebase --abort` restores
everything.

## Checkpoint every turn

Write a Kontinuo checkpoint at the end of every working turn:

```
handoff_write(cwd, checkpoint: { version, capture_mode: "agent_write", goal,
                                 exact_stopping_point, next_action,
                                 commands_run, verification })
```

`goal`, `exact_stopping_point`, and `next_action` are required and are linted for
specificity — "continue the work" fails, "wire the rail into layout.go and
regenerate goldens" passes. `agent_write` mode also requires `commands_run` and
`verification`. Entries under `completed` / `partially_done` / `not_done` each
need `text` and `evidence`; `deferred` entries additionally need a
`reopen_condition`. Evidence must be a real recorded command — `go test`, a
commit, `ci` — not prose.

## Go rules that are not stylistic

**Every goroutine takes a `context.Context`, and every spawn site documents how
it exits.** Goroutine leaks are the failure mode this project is most exposed to:
long-lived streaming sessions with detach and reattach mean any goroutine
blocked on a channel read is a candidate. Garbage collection does not save you
here — a blocked goroutine is permanently reachable and retains its entire stack.

Any test that spawns a goroutine ends with `defer goleak.VerifyNone(t)`.

Maps that only grow are a leak in practice: Go maps never shrink. A registry
that adds and removes holds its peak footprint for the life of the process.

## The alt-screen invariant

The client renders in the **alternate screen buffer**, and the transcript is
drawn by the app rather than printed into the terminal's scrollback.

This is forced by the design: the rail is a full-height right-hand column that
never scrolls with the transcript, and terminal scrollback is line-oriented and
full-width — you cannot reserve a right column in it. Printing transcript blocks
to scrollback with `tea.Println` would break the rail, so do not reintroduce it
as a performance fix.

What that costs us, and how it is paid for:

- Native selection, copy, and terminal search are gone. They are replaced in-app.
- The transcript lives in our heap, so it must be **virtualized**: render only
  the visible window, cache wrapped lines by `(blockID, width)`, invalidate the
  cache on resize, and recompute only the one streaming block.

Per-frame work must stay proportional to the visible window, never to session
length. If a change makes rendering cost grow with transcript size, it is wrong
regardless of how fast it currently benchmarks.

## Design fidelity

The terminal UI implements a fixed visual design. Colours, glyphs, and dimensions
live in `internal/theme` and nowhere else — no hex literals outside that package.

Colour is the only channel carrying block type in the transcript: no boxes, no
avatars, no per-class icons. If two event classes ever resolve to the same hue,
the tests fail on purpose.

A denial renders **inline as an observation and the loop continues** — never a
modal, never a turn-ender. Only `require_human` takes the input line.
