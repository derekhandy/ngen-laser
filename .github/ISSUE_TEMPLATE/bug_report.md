---
name: Bug report
about: Something isn't working as expected
title: "[bug] "
labels: bug
assignees: ''
---

## Description

A clear, short description of what went wrong.

## Reproduction

Exact command(s) run:

```bash
./laser ...
```

## Expected behavior

What you expected to happen.

## Actual behavior

What actually happened. Include the full output, error messages, and any
stack traces.

## Environment

- LASER version (`./laser --help` output or git tag):
- Go version (`go version`):
- OS and architecture:
- CPU count:
- Data directory layout (redact private paths):

## Configuration

Relevant fields from `training-config.json`, `training-flags.json`, or
`environment-rubric.json`. Redact anything private.

## Additional context

- Does it reproduce from a fresh clone?
- Does it reproduce with `-race`?
- Any relevant logs under `data/analytics/`?