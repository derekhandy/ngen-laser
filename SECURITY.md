# Security Policy

## Supported Versions

| Version | Supported |
| ------- | --------- |
| 1.0.x   | ✅        |
| < 1.0   | ❌        |

## Reporting a Vulnerability

Please do **not** open a public GitHub issue for security vulnerabilities.

Instead, report them privately via one of:

1. GitHub's [private security advisory](https://docs.github.com/en/code-security/security-advisories/guidance-on-reporting-and-writing-information-about-vulnerabilities/privately-reporting-a-security-vulnerability)
   feature on this repository.
2. Email the maintainer at `derekhandy@users.noreply.github.com`.

Include:

- A description of the issue and its impact.
- Steps to reproduce, ideally with a minimal input.
- The affected version(s).
- Any suggested mitigation, if you have one.

You can expect an acknowledgment within a few days and a status update within
a week. If the report is accepted, a fix will be prepared privately and
released as a patch version, with credit to the reporter unless you prefer to
remain anonymous.

## Scope

LASER is experimental compression software. In scope:

- Path traversal or arbitrary write during `unpack`.
- Memory corruption or panics reachable from untrusted input.
- Data races detectable with `go test -race` in code paths triggered by user
  input.
- Denial of service via malformed `.lzr` packages (unbounded allocation,
  infinite loops, etc.).

Out of scope:

- Compression ratio claims or fitness-score disagreements.
- Issues that require a hostile local user with write access to the data
  directory.
- Bugs in dependencies that are not exploitable through LASER.

## Safe Usage

- Always keep independent backups. LASER is not a substitute for a backup
  system.
- Treat `.lzr` files from untrusted sources with the same caution as any other
  archive.
- Unpack into a directory you control; the restore path is confined to the
  target directory but still writes files.