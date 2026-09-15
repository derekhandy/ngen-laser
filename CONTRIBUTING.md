# Contributing to LASER

Thanks for your interest in LASER. This document explains how to set up a
development environment, run the checks, and submit a change.

---

## Getting Started

1. Fork the repository on GitHub.
2. Clone your fork:
   ```bash
   git clone git@github.com:derekhandy/ngen-laser.git
   cd ngen-laser
   ```
3. Install dependencies:
   ```bash
   go mod tidy
   ```
4. Confirm the tree builds and passes checks:
   ```bash
   gofmt -l .
   go vet ./...
   go test ./...
   go test -race ./...
   go build ./...
   ```

---

## Development Workflow

1. Create a branch off `main`:
   ```bash
   git checkout -b feat/short-description
   ```
2. Make your change. Keep commits focused and reviewable.
3. Format, vet, and test locally before pushing.
4. Push to your fork and open a pull request against `main`.

---

## Coding Standards

- **Formatting:** `gofmt` is authoritative. Run `gofmt -w .` before
  committing.
- **Vetting:** `go vet ./...` must pass with no output.
- **Linting:** `golangci-lint run` should be clean. If you add a new lint
  exclusion, explain why in the PR.
- **Tests:** every exported function should have at least a happy-path test.
  Concurrency-sensitive code must pass `go test -race ./...`.
- **Commits:** imperative mood, short subject line, body when the "why" isn't
  obvious from the diff.
- **Scope:** one logical change per PR. Refactors and behavior changes should
  not be bundled.

---

## What Belongs in a PR

- Bug fixes with a reproducing test.
- New tests for existing behavior.
- Performance improvements with a benchmark or a clear explanation.
- Documentation corrections.
- New features discussed in an issue first.

For large or architectural changes, open an issue before writing code so the
design can be reviewed.

---

## Reporting Bugs

Open a GitHub issue and include:

- The exact command you ran.
- The full output, including any stack traces.
- Your OS and `go version`.
- The relevant configuration files (redact any private paths).
- Whether the issue reproduces from a fresh clone.

If the bug involves the package format, a minimal `.lzr` file or the input
that produced it helps enormously.

---

## Reporting Security Issues

Please do not open a public issue for security problems. See
[`SECURITY.md`](SECURITY.md) for the private disclosure process.

---

## Code of Conduct

Participation in this project is governed by the
[Code of Conduct](CODE_OF_CONDUCT.md). By contributing, you agree to abide by
its terms.

---

## License

By contributing, you agree that your contributions will be licensed under the
[Apache-2.0 License](LICENSE).