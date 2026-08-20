# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

**Workflow, commands, branch and PR conventions, and the Go rules that are not
stylistic all live in [AGENTS.md](AGENTS.md). Read it first.** This file covers
architecture only, so the two cannot drift apart.

`PRODUCT_SPEC.md` is the authoritative design document. The constraints below
derive from it and are load-bearing.

## Where the code is

Early. The host client's terminal UI is being built first, ahead of any protocol
or backend work.

| Path | What lives there |
| --- | --- |
| `cmd/agbala` | the binary entrypoint |
| `internal/theme` | design tokens — colours, glyphs, dimensions. No hex literals anywhere else. |
| `internal/ui` | rendering primitives and screen layout |
| `internal/scene` | the designed session states, as data |

There is no sandbox, control plane, network transport, or event schema yet. The
UI is built first deliberately: the session protocol is load-bearing and needs a
proposal to change, so it should be designed from what the UI turns out to need
rather than guessed at up front.

## What Àgbàlá is

A sandbox-first coding agent harness. The defining inversion: **the agent loop
runs inside a remote sandbox VM, not on the user's machine.** The local binary is
a terminal — it streams events and renders a transcript. Everything else in the
design follows from that.

Three planes, three separate deployables:

| Plane | Name | Runs where | Holds |
| --- | --- | --- | --- |
| Host | `agbala` | user's machine | one auth token, terminal I/O |
| Control | `agbala-oga` | a small self-hosted VM | provider credentials, session registry, Òfin policy, event log |
| Execution | the sandbox | persistent Linux VM per session | agent loop, repo, toolchain, cached rules |

## Invariants

These are non-negotiable in the spec; treat a change to any of them as a design
change requiring the user's sign-off, not an implementation detail.

- **No local execution path.** Tools execute in the sandbox only. Do not add a
  "local mode", a Docker fallback, or a dev shortcut that runs a tool on the
  host — the absence of a privileged path is the product.
- **The host holds exactly one credential.** Provider keys never reach the host
  and never land in a config file. The control plane mints short-lived scoped
  tokens per session and injects them into the sandbox at boot.
- **Six built-in tools, no more:** `read`, `write`, `edit`, `bash`, `grep`,
  `glob`. Anything else arrives over MCP — including Òfin itself. Adding a
  seventh built-in is a spec change.
- **Governance is enforcement at the tool-call boundary**, not prompt text.
- **The client is disposable.** Host and control plane speak a typed event
  stream; the terminal UI is one client among many. No protocol feature may
  assume a TTY.
- **Inference is an interface.** No provider-specific logic outside the one
  place in the control plane that dispatches a model call.

## Agent loop and the Òfin gate

```
IDLE → PLANNING → TOOL_PROPOSED → OFIN_GATE ──allow──────→ EXECUTING → OBSERVING → PLANNING
                                            ├─deny────────→ OBSERVING (carries rule_id + rationale)
                                            └─require_human→ AWAITING_APPROVAL → EXECUTING | OBSERVING
     → REVIEW → DONE | FAILED
```

The critical behaviour: **a denial does not end the turn.** It returns to the
agent as an observation carrying the blocking rule's ID and its plain-language
rationale, and the agent re-plans with that constraint. The rationale is the
payload the agent learns from, not decoration. Implementations that abort the
turn on `deny` are wrong, and the UI renders a denial inline rather than as a
modal for exactly this reason.

Three verdicts only: `allow`, `deny(rule_id, rationale)`, `require_human`. Every
verdict is appended to the event log.

## The host client's rendering model

The client runs in the **alternate screen buffer** and owns every cell. This is
forced by the design's full-height rail, which never scrolls with the transcript;
scrollback is line-oriented and full-width, so a right-hand column cannot live
in it.

The consequence that matters when writing code here: the transcript is in our
heap and must be **virtualized**. Render only the visible window, cache wrapped
lines by `(blockID, width)`, invalidate on resize, recompute only the streaming
block. Per-frame work must stay proportional to the visible window, never to
session length.

`AGENTS.md` has the full invariant and what it costs.

## Interfaces that matter

The **tool ABI** and the **session protocol** are the two contracts everything
else is built on. Changes to either, or to the Òfin verdict schema, go through a
proposal first. By contrast, the sandbox provider, inference provider, client,
and storage backend are all meant to be swappable behind existing interfaces.

The event log is append-only and must support deterministic replay
(`agbala replay <session>`) and reattach after disconnect. Treat "can this
session be reconstructed from its events alone?" as the test for whether state
belongs in the log — the rail, for instance, is a projection over the log and
never independent state.

## Roadmap position

v0 is deliberately smaller than the spec: single binary, creates a sandbox,
clones a repo, runs a three-tool loop against one model, Òfin rules read from a
flat file with a straightforward matcher, **no control plane**. Credential
brokering and the session registry are extracted in v0.2; Òfin becomes a service
in v0.3; fork and parallel runs in v0.4; alternate clients in v0.5. Don't build
v0.3 machinery while v0 is unproven — but don't foreclose it either.

## CLI surface

Target shape (see the spec for the full list): `login`, `run`, `start`, `ls`,
`attach`, `fork`, `kill`, `ofin rules|explain|check`, `logs`, `replay`, `models`,
`usage`. Every command takes `--json` for structured output.

## Non-goals

Not a local execution mode, not an IDE in the terminal, not a model, not
managed-only. Every component must remain self-hostable end to end.

## License

Apache-2.0.
