# 0magnet/glamour

A fork of [charmbracelet/glamour](https://github.com/charmbracelet/glamour),
whose upstream module path is now `charm.land/glamour/v2`.

## Branches

| branch | module path | what it is |
| --- | --- | --- |
| `upstream` | `charm.land/glamour/v2` | an exact mirror of upstream's default branch, never modified |
| `main` | `github.com/0magnet/glamour` | what other repos import: every fix below, plus the module rename |
| `no-bluemonday` | `charm.land/glamour/v2` | one fix, based on `upstream`, shaped for a pull request |

A fix lands on its own branch off `upstream` first, so it can be offered
upstream unchanged, and is then merged into `main`. `main` differs from the sum
of those branches only by the module path.

## Fixes carried here

- **`no-bluemonday`** — `ansi.RenderContext` used a bluemonday `StrictPolicy`
  for a single call, which is a tag stripper wearing a sanitiser's clothes. It
  was the only use of bluemonday, and brought `golang.org/x/net/html` and five
  `golang.org/x/text` packages with it. `ansi/striptags.go` replaces it, checked
  against the policy it replaces over 400,000 generated inputs.
