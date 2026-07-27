# What is Gophish?

Gophish is a phishing framework that makes the simulation of real-world
phishing attacks dead-simple. The idea behind Gophish is simple – make
industry-grade phishing training available to everyone. "Available" in
this case means two things:

* **Affordable** – Gophish is open-source software that is completely free for anyone to use.
* **Accessible** – Gophish is written in the Go programming language. This has the benefit that Gophish is distributed as a single compiled binary with no runtime dependencies, so running it is as simple as "build and run."

## Why Gophish-NG?

Gophish-NG is an actively maintained fork of upstream
[gophish/gophish](https://github.com/gophish/gophish). Upstream has had no
commits since September 2024, its `go.mod` still required Go 1.13, and
several of its core dependencies carried known CVEs — not ideal for a tool
security teams run inside their own networks.

This fork exists to keep that foundation solid without changing what
Gophish actually does:

* **Dependency & toolchain modernization** – Go 1.13 → 1.25+, migrated off the abandoned `jinzhu/gorm` (v1) to `gorm.io/gorm` (v2), replaced the unmaintained `bitbucket.org/liamstask/goose` with `pressly/goose`, and brought every dependency current.
* **Real, verified security fixes** – including a stored XSS in the campaign delete dialog, a process-wide crash bug, an SSRF-adjacent `allowed_internal_hosts` misconfiguration, and an authorization bypass in account unlocking.
* **A rebuilt, minimal frontend toolchain** – Gulp + Webpack replaced with a single `esbuild` script.
* **CI that actually catches things** – `govulncheck`, linting, and automated dependency updates.

The features, workflows, and screenshots described in the rest of this
guide apply equally to Gophish-NG and upstream Gophish, since this fork
doesn't change user-facing behavior. See the
[project README](https://github.com/raginx/gophish#readme) for the full
rationale.
