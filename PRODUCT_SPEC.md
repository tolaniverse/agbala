# Àgbàlá

**A sandbox-first coding agent harness. Your laptop is a terminal, not a workstation.**

_Àgbàlá_ (Yoruba) — the compound yard. A walled, shared space where the work of the household happens. Bounded by design.

---

## The problem

Every coding agent today assumes your machine is the workstation. It wants your API keys in a dotfile, your toolchain installed locally, your CPU running the loop, and your disk holding the repo. That assumption fails in three places:

1. **Credential sprawl.** Five agents, five config files, five provider keys in plaintext or a keychain you now have to manage across machines.
2. **Hardware floor.** The agent competes with your editor, your build, and your browser for the same RAM. Old or modest hardware is excluded outright.
3. **Blast radius.** An agent with shell access on your host is an agent with shell access on your host. Permission prompts are a UX patch over an architectural problem.

Àgbàlá moves the entire execution surface off your machine and puts a wall around it.

---

## What it is

Àgbàlá is three things that ship together:

- **A thin CLI** — a single static binary that authenticates once and streams a session. It holds no keys, no repo, and no model. It is closer to `mosh` than to an IDE.
- **A control plane** — a small self-hostable service that brokers credentials, tracks sessions, and serves governance rules.
- **A sandboxed agent** — the actual loop, running inside a persistent Linux VM alongside the repo and the toolchain.

The agent loop runs _in the sandbox_, not on your host. That single inversion is what makes the rest of the design fall out cleanly.

---

## Principles

**Sandbox-first, not sandbox-optional.** There is no "local mode" that runs tools on your host. The sandbox is the only execution target, so there is no privileged path to accidentally fall back to.

**One credential, ever.** You hold a single token. The control plane holds everything else and injects short-lived scoped credentials into the sandbox at boot. Provider keys never reach your machine and never land in a config file.

**Governance is enforcement, not suggestion.** Rules are checked at the tool-call boundary, not appended to a prompt and hoped for.

**Small tool surface.** Six built-in tools. Everything else arrives over MCP. A small core is auditable; a large one is a liability.

**The client is disposable.** Host and control plane speak a typed event stream. The terminal UI is one client among many — a web UI, a phone client, or an editor adapter are skins over the same protocol.

**Model and provider agnostic.** Inference is an interface. Self-hosted OSS models, managed inference, or frontier APIs all sit behind the same call.

---

## Architecture

### Host plane — `agbala`

A single static Go binary, no runtime to install. Responsibilities:

- Authenticate once, store one token
- Open a session, stream events, render the transcript
- Detach and reattach — a session survives closing your laptop
- Approve or deny gated actions

Explicitly **not** responsible for: holding provider keys, running inference, executing tools, or storing the repo.

### Control plane — `agbala-oga`

A small service you deploy once (single binary, SQLite or Postgres, runs on a free-tier VM). Self-hostable by default; there is no hosted dependency you cannot replace.

- **Credential broker** — holds provider and sandbox credentials, mints short-lived scoped tokens per session
- **Session registry** — maps session → sandbox → repo → status, so sessions are addressable and resumable
- **Òfin policy service** — resolves which rules apply to a given repo and returns verdicts on tool calls
- **Event log** — append-only transcript store; enables reattach, replay, and audit

### Execution plane — the sandbox

A persistent Linux VM per session. Inside it:

- The agent loop
- The repo, cloned and stateful across turns
- The language toolchain, installed once and kept warm
- A local cache of the applicable Òfin rules

Because the VM persists, dependency installs and build caches survive between turns. Because it hibernates when idle, you are not paying for a machine that is waiting on you to type.

---

## The agent loop

```
IDLE
  └→ PLANNING
       └→ TOOL_PROPOSED
            └→ OFIN_GATE ──allow──────→ EXECUTING → OBSERVING → PLANNING
                        ├─deny────────→ OBSERVING (carrying rule_id + rationale)
                        └─require_human→ AWAITING_APPROVAL → EXECUTING | OBSERVING
  └→ REVIEW
       └→ DONE | FAILED
```

A denial does not end the turn. It returns to the agent as an observation carrying the rule that blocked it and the reason in plain language. The agent re-plans with the constraint in hand. Governance becomes a correction inside the loop rather than a wall around it.

---

## Òfin integration

Òfin is the governance layer: RFC-style engineering rules, versioned, scoped, and served to both humans and agents.

It attaches to Àgbàlá at two points:

**At session boot — as context.** Rules matching the repo's scope (language, stack, service tier, team) are resolved and injected into the system prompt. This is the part every other harness approximates with an unversioned markdown file in the repo root.

**At every tool call — as a gate.** The proposed call is evaluated against the rule set before execution. Three verdicts:

| Verdict                    | Behaviour                                                      |
| -------------------------- | -------------------------------------------------------------- |
| `allow`                    | Executes immediately                                           |
| `deny(rule_id, rationale)` | Blocked; rationale returned to the agent as an observation     |
| `require_human`            | Suspends the turn and surfaces an approval request to the host |

Rules carry a stable ID, a scope selector, a matcher over tool name and arguments, a verdict, and human-readable rationale text. The rationale is not decoration — it is the payload the agent learns from.

Every verdict is written to the event log, so a session produces an auditable record of what was attempted, what was blocked, and by which rule.

---

## Features

### Core

- **Zero local setup** — install one binary, log in once, start working. No toolchain, no Docker, no provider accounts.
- **Runs on anything** — the host does terminal I/O and nothing else. Twenty-year-old hardware is a supported target, not a joke.
- **Detachable sessions** — close the lid, reopen on another machine, reattach where you left off.
- **Persistent workspaces** — installs and build caches survive across turns and across days.
- **Six-tool core** — `read`, `write`, `edit`, `bash`, `grep`, `glob`. Auditable, schema-defined, implemented once inside the sandbox.
- **MCP extensibility** — anything beyond the core arrives as an MCP server, including Òfin itself.
- **Provider-agnostic inference** — swap models in one place, in the control plane, without touching any client.

### Governance

- **Rule injection** — scoped standards in the system prompt, versioned rather than pasted
- **Tool-call gating** — hard enforcement at the execution boundary
- **Explained denials** — the agent is told which rule and why, and re-plans
- **Approval flow** — sensitive actions escalate to the human instead of silently proceeding
- **Audit trail** — every proposal, verdict, and result is recorded

### Parallelism

- **Snapshot and fork** — checkpoint a session, then branch it. Run three approaches to the same refactor from an identical starting point and diff the outcomes. This is a property of the persistent-VM model and is not reproducible on a local harness.
- **Concurrent sessions** — several repos or several branches of the same repo, each in its own VM, all visible from one client.
- **Background runs** — dispatch a task and disconnect. Triggered runs (a webhook, a labelled issue, a cron) use the same session machinery.

### Operations

- **Spend visibility** — token accounting per session, per repo, per model, in one place because inference goes through one place
- **Replayable transcripts** — the event log reconstructs any session deterministically
- **Runtime resize** — grow the VM for a heavy build, shrink it after
- **Self-hostable end to end** — control plane on your infrastructure, sandboxes on your infrastructure, inference on your infrastructure

---

## CLI surface

```
agbala login                      # one token, once
agbala run "<prompt>"             # one-shot task in the current repo
agbala start                      # interactive session
agbala ls                         # active sessions
agbala attach <session>           # reattach to a running session
agbala fork <session>             # branch from a snapshot
agbala kill <session>

agbala ofin rules                 # rules in scope for this repo
agbala ofin explain <rule-id>     # rationale and history
agbala ofin check                 # dry-run the gate against a proposed change

agbala logs <session>             # transcript
agbala replay <session>           # deterministic replay
agbala models                     # available models
agbala usage                      # spend
```

`--json` on any command produces structured output for scripting.

---

## Non-goals

- **A local execution mode.** The sandbox boundary is the product. An escape hatch would undermine it.
- **An IDE in the terminal.** The client renders a transcript and takes input. Rich editing belongs in your editor.
- **A model.** Àgbàlá is a harness. It ships no weights and takes no position on which model you should use.
- **Managed-only.** Every component is self-hostable. A hosted convenience tier may exist; it will never be the only way to run this.

---

## Roadmap

**v0 — prove the ergonomics**
Single binary. Creates a sandbox, clones a repo, runs a three-tool loop against one model. Òfin rules read from a flat file in the repo; the gate is a straightforward matcher. No control plane. The goal is one real task completed end-to-end on modest hardware.

**v0.2 — the control plane**
Extract credential brokering and the session registry. One token replaces per-provider configuration. Multiple concurrent sessions become addressable.

**v0.3 — Òfin as a service**
Versioned rules, scope resolution, verdicts over MCP, audit log.

**v0.4 — fork and parallel runs**
Snapshot, branch, diff.

**v0.5 — clients**
Web client and editor adapter over the same session protocol, proving the client is genuinely disposable.

**Later**
Background and triggered runs. Team-shared rule sets. Self-hosted inference recipes.

---

## Contributing

The two interfaces that matter most are the **tool ABI** and the **session protocol**. Everything else — the sandbox provider, the inference provider, the client, the storage backend — is intended to be swappable, and a pull request that adds an implementation behind an existing interface is the easiest kind to merge.

Changes to the tool ABI or the Òfin verdict schema go through a proposal first, because both are load-bearing for anything built on top.

## License

Apache-2.0.
