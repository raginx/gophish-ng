<p align="center">
  <img src="https://raw.github.com/raginx/gophish-ng/master/static/images/gophish_purple.png" alt="gophish logo" width="500">
</p>

<h1 align="center">Gophish-NG</h1>

<p align="center">
  <a href="https://github.com/raginx/gophish-ng/actions/workflows/ci.yml"><img src="https://github.com/raginx/gophish-ng/workflows/CI/badge.svg" alt="Build Status"></a>
  <a href="https://github.com/raginx/gophish-ng/blob/master/LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue" alt="License"></a>
  <img src="https://img.shields.io/github/go-mod/go-version/raginx/gophish-ng" alt="Go Version">
  <a href="https://github.com/raginx/gophish-ng/commits/master"><img src="https://img.shields.io/github/last-commit/raginx/gophish-ng" alt="Last Commit"></a>
</p>

<p align="center"><strong>An actively maintained fork of Gophish, the open-source phishing toolkit.</strong></p>

---

## Why a fork?

[Gophish](https://github.com/gophish/gophish) is a widely used, battle-tested
toolkit for running phishing simulations and security awareness training.
Unfortunately, **upstream `gophish/gophish` has had no commits since
September 2024**, `go.mod` still requires Go 1.13, and several of its core
dependencies carry known CVEs — not great for a tool that security teams
run inside their own networks.

This fork exists to keep that foundation solid, and to fill the gaps that
show up when a team runs it. So far that's meant:

- **Dependency & toolchain modernization**: Go 1.13 → 1.25+, a full
  migration off the long-abandoned `jinzhu/gorm` (v1) to `gorm.io/gorm`
  (v2), the unmaintained `bitbucket.org/liamstask/goose` replaced with the
  actively maintained `pressly/goose`, and every dependency brought current.
  `govulncheck` findings went from 47 reachable vulnerabilities down to a
  single one with no upstream fix yet available.
- **Real, verified security fixes** found along the way: a stored XSS in
  the campaign delete dialog, a crash that could take down the whole
  process (not just one request), an SSRF-adjacent bug where configuring
  `allowed_internal_hosts` silently blocked *all* external traffic instead
  of just internal ranges, and an authorization bypass letting a locked-out
  user unlock their own account via their still-valid API key.
- **A rebuilt, minimal frontend toolchain**: Gulp + Webpack (18 npm
  packages, two build systems quietly stepping on each other) replaced
  with a single `esbuild` script.
- **CI that actually catches things**: `govulncheck`, linting, and
  automated dependency updates, none of which upstream had.
- **Features upstream lacks**: teams, so several operators share the same
  campaigns and assets instead of each working in their own silo; a
  read-only auditor role for reviewers and clients; OAuth 2.0 for IMAP
  reporting; SMTP CC and per-profile send rates.

Every change here is tested and verified against a running instance, not
just "looks right." See [CONTRIBUTING.md](CONTRIBUTING.md) for how this
fork is maintained and [SECURITY.md](SECURITY.md) for reporting
vulnerabilities.

## Table of Contents

- [What is Gophish?](#what-is-gophish)
- [Install](#install)
- [Building From Source](#building-from-source)
- [Docker](#docker)
- [Setup](#setup)
- [Documentation](#documentation)
- [Issues](#issues)
- [Contact](#contact)
- [License](#license)

## What is Gophish?

Gophish is an open-source phishing toolkit designed for businesses and
penetration testers. It provides the ability to quickly and easily set up
and execute phishing engagements and security awareness training —
landing pages, email templates, target groups, sending profiles, and
per-recipient tracking, all through a single web UI.

## Install

Pre-built binaries for this fork are available on the
[releases page](https://github.com/raginx/gophish-ng/releases). Alternatively,
build from source (see below). If you're looking for official upstream
binaries instead, see the
[upstream releases page](https://github.com/gophish/gophish/releases/).

## Building From Source

**Requires Go 1.25 or above.**

```sh
git clone https://github.com/raginx/gophish-ng.git
cd gophish-ng
go build
```

After this, you should have a binary called `gophish` in the current
directory.

The built frontend assets (`static/js/dist/`, `static/css/dist/`) are
checked into git, so this is all you need for a normal build. If you
change anything under `static/js/src/` or `static/css/`, rebuild them
with:

```sh
npm install
npm run build
```

## Docker

You can also use upstream Gophish via the official Docker container
[here](https://hub.docker.com/r/gophish/gophish/). This fork does not
currently publish its own Docker images.

## Setup

After running the Gophish binary, open a browser to `https://localhost:3333`
and log in with the default username and password printed in the log
output, e.g.:

```
time="2020-07-29T01:24:08Z" level=info msg="Please login with the username admin and the password 4304d5255378177d"
```

## Documentation

The full user guide is available at
**[raginx.github.io/gophish-ng](https://raginx.github.io/gophish-ng/)**.
Since this fork tracks upstream Gophish closely, the
[upstream documentation](http://getgophish.com/documentation) also mostly
applies.

## Issues

Found a bug specific to this fork? Please
[file an issue](https://github.com/raginx/gophish-ng/issues/new) here. For
issues that also affect upstream Gophish, consider checking the
[upstream issue tracker](https://github.com/gophish/gophish/issues) as
well.

## Contact

reinhard [at] westerholt [dot] me

## License

MIT, same as upstream Gophish — see [LICENSE](LICENSE) for the full text.
