# Security Policy

## Supported versions

Columbus is pre-1.0 and ships from `main`. Security fixes are applied to the
latest released version. Please upgrade to the newest release before reporting.

## Reporting a vulnerability

**Please do not open a public issue for security vulnerabilities.**

Report privately via GitHub's
[private vulnerability reporting](https://github.com/rafaelfragoso/columbus/security/advisories/new)
("Report a vulnerability" on the Security tab). If that is unavailable, email
the maintainer at **rafaelfragosom@gmail.com**.

Please include:

- a description of the issue and its impact,
- steps to reproduce (a minimal repository or command sequence),
- affected version (`columbus version`).

You can expect an acknowledgement within a few days. We'll keep you informed of
progress and coordinate a disclosure timeline once a fix is ready.

## Scope notes

Columbus is **local-only** and makes **no network calls**. It reads your
working tree, writes a metadata/graph cache to your OS data directory, and
shells out to `git` and (optionally) `ripgrep`/`ast-grep`. Reports about local
attack surface — path handling, command construction, SQL, or untrusted-input
parsing — are in scope and welcome.
