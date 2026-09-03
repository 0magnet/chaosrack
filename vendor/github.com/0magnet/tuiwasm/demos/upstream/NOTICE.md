# tcell's own demos

Copyright The TCell Authors. Licensed under the Apache License, Version 2.0 —
see `LICENSE`, which is the copy that ships with tcell.

Original: <https://github.com/gdamore/tcell/tree/main/_demos>

These are Garrett D'Amore's demos, not imitations of them. That distinction is
the point: an approximation written to show off an adapter will only exercise
what its author thought to exercise, and these were written to exercise tcell.

`unicode` is the one that earns its place. Double-wide CJK, combining marks,
zero-width joiners and regional indicators are exactly where a cell grid and an
escape-sequence translation disagree if anything is wrong, and nothing I would
have written by hand would have covered them as thoroughly.

## What was changed

Each demo's `main` is replaced by a `run` in `run.go`; everything else in the
package is upstream's file with its `//go:build ignore` lines removed and its
`package main` renamed, so each demo can be its own package. `boxes` and
`colors` both define `makebox`, so they cannot share one.

`run` differs from `main` in three ways, and no others:

- It takes the screen rather than creating one. Here the screen may be a window
  among several, already initialised and bound to a terminal by whatever opened
  it, so this cannot be the thing that makes it — or finalises it.
- It returns instead of blocking forever, and treats a nil event as the end.
  `PollEvent` returns nil once the screen is finalised, which is what happens
  when a window closes; without that the goroutine spins on a dead screen for
  as long as the page is open.
- It drops the timing summary `main` printed to stdout after `Fini`. There is
  no stdout to print it to in a browser window.

Unused imports left behind by removing `main` were deleted. Nothing else in the
upstream files was touched.
