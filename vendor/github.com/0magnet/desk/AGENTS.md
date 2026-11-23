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

Every name says what it is. A reader should know what a function does or what
a type holds from its name alone, without opening the definition.

Build names from the general to the specific, so related names share a prefix
and sort next to each other. The package name is part of every call site, so
do not repeat it in the names inside it.

These names do not describe anything and are not used for new code: `Data`,
`Info`, `Tmp`, `Temp`, `Item`, `Thing`, `Stuff`, `Misc`, `Do`, `Handle`,
`Handler`, `Process`, `Manager`, `Helper` and `Util`, alone or as the whole
meaning of a longer name. Name a package after its subject, never `util`,
`common`, `helpers` or `misc`. A bare `Get` is only acceptable on a type whose
name already says what it returns.

Booleans take no `is` or `has` prefix. Write `sorted`, not `isSorted`.

Abbreviations are allowed only where they are the usual name, such as `ID`,
`URL` or `JSON`.

Go keeps a few conventions that these rules do not override. Error types keep
the `Error`, `Unwrap`, `Is` and `As` methods that the standard library calls.
Receivers and loop indexes keep their short names.

Renaming an exported identifier is a change of its own, with a table of the
old and new names in the commit message. Never rename anything that leaves the
process: CLI commands, flags, file formats and JSON fields are used by other
programs.

In this repository, `Launch` and `LaunchOpts` share their prefix and sort
together, and a caller writes `desk.Register`, not `desk.RegisterApp`.

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
the tests and the guards, in both modules. `panes/` is a second module with its
own go.mod, and the Makefile recurses into it. Much of the code is js/wasm
only, so `make test-wasm` runs its tests under Node.

The browser demos in `docs/` (a TinyGo build and a standard Go build under
`docs/go/`) are rebuilt and committed by the demo workflow, so do not commit
the wasm there by hand.

`make check-names` enforces the Naming section, and `make check-inits` fails on
a `func init()` outside package main. Today's violations are listed in
`scripts/guard/`, and those allowlists only shrink.
