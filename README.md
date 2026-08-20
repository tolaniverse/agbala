# Àgbàlá

**A sandbox-first coding agent harness. Your laptop is a terminal, not a workstation.**

_Àgbàlá_ (Yoruba) — the compound yard. A walled, shared space where the work of the household happens. Bounded by design.

---

Every coding agent today assumes your machine is the workstation: your API keys in a dotfile, your toolchain installed locally, your CPU running the loop, your disk holding the repo. Àgbàlá moves the entire execution surface off your machine and puts a wall around it.

- **The agent loop runs in the sandbox, not on your host.** That single inversion is what makes the rest of the design fall out cleanly.
- **One credential, ever.** The control plane holds everything else and injects short-lived scoped credentials into the sandbox at boot.
- **Governance is enforcement, not suggestion.** Rules are checked at the tool-call boundary, not appended to a prompt and hoped for.
- **Six built-in tools.** `read`, `write`, `edit`, `bash`, `grep`, `glob`. Everything else arrives over MCP.

Read [PRODUCT_SPEC.md](PRODUCT_SPEC.md) for the full design.

## Status

Early. The host client's terminal UI is being built first — see [AGENTS.md](AGENTS.md) for how the work is organised. Nothing here is usable yet.

## Building

```sh
make build      # build ./agbala
make test       # test suite, with the race detector
make lint       # golangci-lint
make help       # everything else
```

Requires Go 1.26 or later. No cgo, no other toolchain.

## Contributing

The two interfaces that matter most are the **tool ABI** and the **session protocol**. Everything else — the sandbox provider, the inference provider, the client, the storage backend — is intended to be swappable, and a pull request that adds an implementation behind an existing interface is the easiest kind to merge.

Changes to the tool ABI or the Òfin verdict schema go through a proposal first, because both are load-bearing for anything built on top.

## License

[Apache-2.0](LICENSE).
