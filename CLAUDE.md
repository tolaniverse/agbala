# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository status

Pre-implementation. The repository contains `PRODUCT_SPEC.md` and nothing else — no source, no build system, no tests, not yet a git repository. There are no build/lint/test commands to run yet; when the first Go module lands, replace this section with the real ones.

`PRODUCT_SPEC.md` is the authoritative design document. Read it before writing code — the constraints below are load-bearing and derive from it.

## What Àgbàlá is

A sandbox-first coding agent harness. The defining inversion: **the agent loop runs inside a remote sandbox VM, not on the user's machine.** The local binary is a terminal — it streams events and renders a transcript. Everything else in the design follows from that.

Three planes, three separate deployables:

| Plane | Name | Runs where | Holds |
| --- | --- | --- | --- |
| Host | `agbala` | user's machine | one auth token, terminal I/O |
| Control | `agbala-oga` | a small self-hosted VM | provider credentials, session registry, Òfin policy, event log |
| Execution | the sandbox | persistent Linux VM per session | agent loop, repo, toolchain, cached rules |

Implementation language for the host binary is Go (single static binary, no runtime to install). Control plane: single binary backed by SQLite or Postgres.

## Invariants

These are non-negotiable in the spec; treat a change to any of them as a design change requiring the user's sign-off, not an implementation detail.

- **No local execution path.** Tools execute in the sandbox only. Do not add a "local mode", a Docker fallback, or a dev shortcut that runs a tool on the host — the absence of a privileged path is the product.
- **The host holds exactly one credential.** Provider keys never reach the host and never land in a config file. The control plane mints short-lived scoped tokens per session and injects them into the sandbox at boot.
- **Six built-in tools, no more:** `read`, `write`, `edit`, `bash`, `grep`, `glob`. Anything else arrives over MCP — including Òfin itself. Adding a seventh built-in is a spec change.
- **Governance is enforcement at the tool-call boundary**, not prompt text. Rules are checked before execution, never merely appended to the system prompt and hoped for.
- **The client is disposable.** Host and control plane speak a typed event stream; the terminal UI is one client among many. No protocol feature may assume a TTY.
- **Inference is an interface.** No provider-specific logic outside the one place in the control plane that dispatches a model call.

## Agent loop and the Òfin gate

```
IDLE → PLANNING → TOOL_PROPOSED → OFIN_GATE ──allow──────→ EXECUTING → OBSERVING → PLANNING
                                            ├─deny────────→ OBSERVING (carries rule_id + rationale)
                                            └─require_human→ AWAITING_APPROVAL → EXECUTING | OBSERVING
     → REVIEW → DONE | FAILED
```

The critical behaviour: **a denial does not end the turn.** It returns to the agent as an observation carrying the blocking rule's ID and its plain-language rationale, and the agent re-plans with that constraint. The rationale is the payload the agent learns from, not decoration. Implementations that abort the turn on `deny` are wrong.

Three verdicts only: `allow`, `deny(rule_id, rationale)`, `require_human`. Every verdict is appended to the event log, so a session yields an auditable record of what was attempted and what blocked it.

Òfin attaches at two points: at session boot rules matching the repo's scope are resolved into the system prompt; at every tool call the proposed call is matched against the rule set.

## Interfaces that matter

The **tool ABI** and the **session protocol** are the two contracts everything else is built on. Changes to the tool ABI or the Òfin verdict schema go through a proposal first. By contrast, the sandbox provider, inference provider, client, and storage backend are all meant to be swappable behind existing interfaces — a new implementation behind one of those is routine.

The event log is append-only and must support deterministic replay (`agbala replay <session>`) and reattach after disconnect. Treat "can this session be reconstructed from its events alone?" as the test for whether state belongs in the log.

## Roadmap position

v0 is deliberately smaller than the spec: single binary, creates a sandbox, clones a repo, runs a three-tool loop against one model, Òfin rules read from a flat file in the repo with a straightforward matcher, **no control plane**. Credential brokering and the session registry are extracted in v0.2; Òfin becomes a service in v0.3; fork/parallel runs in v0.4; alternate clients in v0.5. Don't build v0.3 machinery while v0 is unproven — but don't design v0 in a way that forecloses it either.

## CLI surface

Target shape (see the spec for the full list): `login`, `run`, `start`, `ls`, `attach`, `fork`, `kill`, `ofin rules|explain|check`, `logs`, `replay`, `models`, `usage`. Every command takes `--json` for structured output.

## Non-goals

Not a local execution mode, not an IDE in the terminal, not a model, not managed-only. Every component must remain self-hostable end to end.

## License

Apache-2.0.
