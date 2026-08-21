# Contributing

Thanks for your interest in this fork! It's a personally maintained fork of
[Gophish](https://github.com/gophish/gophish), focused on dependency/security
updates and curated fixes from the upstream backlog. **Contributions are
very welcome** — bug reports, fixes, and small improvements especially so.

There's no formal process: open an issue or pull request. By submitting a
pull request, you confirm that you wrote the contribution yourself (or
otherwise have the right to submit it) and agree to license it under this
project's [MIT License](LICENSE). Since this is maintained by one person in
their spare time, response times may vary, and larger PRs that weren't
discussed in an issue first may take a while to review.

## Security vulnerability disclosure

Please report suspected security vulnerabilities privately — see
[SECURITY.md](SECURITY.md). Do **not** open a public issue for a
suspected vulnerability.

## Reporting bugs / requesting features

Please [file an issue](https://github.com/raginx/gophish-ng/issues/new/choose)
with as much detail as possible (steps to reproduce, expected vs. actual
behavior, Gophish version/commit). Usage questions belong in
[Discussions](https://github.com/raginx/gophish-ng/discussions), not issues.

## Development

Backend (Go 1.25+):

```sh
go build .
go test ./...
gofmt -l .        # must produce no output - CI checks this exactly
```

Frontend assets (Node 22+) are built with a single `esbuild` script:

```sh
npm ci
npm run build     # writes static/js/dist and static/css/dist
```

Linting uses `golangci-lint`; CI only fails on issues introduced by your
change (`only-new-issues`), not the pre-existing backlog, so no need to fix
unrelated files in passing.

If you're changing the `Dockerfile`, you can trigger a build-only check
without publishing anything via the "Docker Build Check" workflow under the
Actions tab (manual trigger).

## Pull requests

- Keep PRs focused - one change per PR is easier to review than a bundle of
  unrelated fixes.
- Make sure `go build .`, `go test ./...`, and `gofmt -l .` are clean before
  opening.
- CI runs the same checks (build, tests, lint, `govulncheck`) automatically.
