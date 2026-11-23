# AGENTS.md

Guidance for AI coding agents working in this repository.

## Communication style

Use simple direct language everywhere outside the code. That includes commit
messages, issues and review comments.

Lead with the change and the reason for it. Include a measurement, a table or
a log excerpt only where it is the evidence for a claim.

Avoid extra colons, semicolons and dashes. Write plain sentences instead.

## Comments

Omit redundant comments. Where a comment is needed, keep it short. Where a
comment records a specific requirement, cite the source.

## Naming

Names follow the original, WinBox.js, so the two read side by side. Code with no
counterpart in WinBox.js follows plain Go naming.

Renaming an exported identifier is a change of its own. Never rename anything
that leaves the process, such as options or file formats.

## No coding tool attributions

Never add `Co-Authored-By`, a "Generated with" footer, or any other coding tool
attribution to a commit, issue or comment.

## Verify before you submit

Run the change. Do not submit code that is only plausibly correct. Where
something was not verified, say so plainly rather than implying it was.

## Commits

The subject line is `package: lowercase summary` with no trailing period. Use
`all:` for a repository wide change. Any behavior change gets a body giving the
symptom, the cause and the fix.

Commits go to `main` directly.

## Working in this repository

`make check` runs the linters (a host pass, then again under GOOS=js), `go vet`,
the tests and the init guard. `make test-wasm` runs the js/wasm tests under
Node.

The browser demo lives in `docs/` and is rebuilt and committed by the demo
workflow, so do not commit `docs/demo.wasm` by hand.

`make check-inits` fails on a `func init()` outside package main. Today's
violations are listed in `scripts/guard/`, and that allowlist only shrinks.
