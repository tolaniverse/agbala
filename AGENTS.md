# AGENTS.md

How work is done in this repository. This file is the source of truth for
workflow; `CLAUDE.md` covers architecture and links here rather than restating
any of it.

## Commands

```sh
make build       # build ./agbala (CGO_ENABLED=0, static)
make test        # go test -race -count=1 ./...
make lint        # golangci-lint (pinned to v2.13.1, see .github/workflows/ci.yml)
make bench       # render benchmarks
make bench-gate  # fail if allocation metrics regressed against the baseline
make golden      # regenerate goldens — always review the diff before committing
make check-fmt   # fail if anything is unformatted
make help        # everything else
```

Single test: `go test ./internal/theme/ -run TestRailWidth -v`.

Requires Go 1.26+. `CGO_ENABLED=0` is a product requirement for the shipped
binary (single static executable, no runtime to install) — the `build` target
and CI/release cross-builds set it. Test, bench, and golden targets do not.

CI runs, on every PR: `make check-fmt`, `go mod tidy` checked with
`git diff --exit-code`, `make test`, golangci-lint, `make bench-gate`, and four
static cross-builds (darwin/linux × amd64/arm64).

The perf gate blocks on **allocation counts and byte counts only** — those are a
function of the code and its input, so a change in them is a real change.
Wall-clock on a shared runner is not, so it is written to the job summary and
never fails a build. Re-record with `make bench-baseline` and review the diff:
a moved baseline is a claim that the new number is correct. Run the same before pushing; `make build` locally
is a good final check.

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

## Where the code is

The only executable is `cmd/agbala`, currently a visual-demo shell. It renders
one of five hand-authored scenes (`boot`, `loop`, `deny`, `approval`, `fork`)
through Bubble Tea v2. There is no backend, sandbox, control plane, network
transport, or event schema yet; scenes are projections of the intended event
stream, shaped like a fold would produce. When that stream lands they become the
expected output of folding a fixture log. Dependency direction is
`cmd → tui → scene|ui|theme`, with `internal/bench` instrumenting frame output.

| Path | What lives there |
| --- | --- |
| `cmd/agbala` | binary entrypoint; flags `-version`, `-state`, `-frametap`, `-ascii` |
| `internal/tui` | Bubble Tea model: input, animation ticks, approval keys, alt-screen view |
| `internal/scene` | the five designed session states, as data |
| `internal/ui` | rendering, layout, viewport, wrapping, transcript cache, rail, chrome |
| `internal/theme` | design tokens — colours, glyphs, dimensions. No hex literals anywhere else. |
| `internal/bench` | synchronized-output frame tap used by tests and `-frametap` |

## Go rules that are not stylistic

**Every goroutine takes a `context.Context`, and every spawn site documents how
it exits.** Goroutine leaks are the failure mode this project is most exposed to:
long-lived streaming sessions with detach and reattach mean any goroutine
blocked on a channel read is a candidate. Garbage collection does not save you
here — a blocked goroutine is permanently reachable and retains its entire stack.
When a test spawns a goroutine, end it with `defer goleak.VerifyNone(t)` and add
`goleak` to `go.mod` — it is not currently a dependency.

Maps that only grow are a leak in practice: Go maps never shrink. A registry
that adds and removes holds its peak footprint for the life of the process.

## The alt-screen invariant

The client renders in the **alternate screen buffer**, and the transcript is
drawn by the app rather than printed into the terminal's scrollback.

This is forced by the design: the rail is a full-height right-hand column that
never scrolls with the transcript, and terminal scrollback is line-oriented and
full-width — you cannot reserve a right column in it. Do not reintroduce
`tea.Println` as a performance fix.

The transcript lives in our heap, so it must be **virtualized**. The cache in
`internal/ui` memoises rendered blocks by `(Block.ID, Block.Rev)` and holds one
width/density at a time. Identity cannot be positional: an event stream mutates
blocks that are no longer last — the design's fork state ticks one fork over
while newer output sits beneath it — and position alone cannot tell a changed
block from its neighbour shifting. A block with a zero `ID` is anonymous and
re-rendered every frame, which is the honest fallback for hand-built blocks.

Per-frame layout must stay proportional to what changed, never to session
length. A frame still walks every block to assemble output and mark cache
entries live, so an idle redraw is O(blocks) in map lookups (0.20ms at 100
blocks, 0.28ms at 2000); the fix for a very long session is to cap the retained
transcript, not to shave the walk.

Go maps never shrink, so the cache evicts entries for blocks that are gone and
rebuilds the map once the dead outnumber the live.

## Design fidelity

The terminal UI implements a fixed visual design. Colours, glyphs, and dimensions
live in `internal/theme` and nowhere else — no hex literals outside that package.

Colour is the only channel carrying block type in the transcript: no boxes, no
avatars, no per-class icons. If two event classes ever resolve to the same hue,
the tests fail on purpose.

A denial renders **inline as an observation and the loop continues** — never a
modal, never a turn-ender. Only `require_human` takes the input line.

Golden files hold raw true-colour ANSI, not plain text — `cat`ting one shows the
primitive exactly as a terminal draws it. A change that repaints cells is a
design change, so review the golden diff before committing. In a failure, escape
sequences are shown as `\e`.
