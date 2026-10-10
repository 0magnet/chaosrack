# seat

The layer under a browser page that switches its whole screen between
screens — consoles, a desktop, the page itself — as a machine's virtual
terminals switch between its text consoles and its graphical session.
Ctrl+Alt+1…9 picks one.

```go
s := seat.New(seat.Options{})
s.AddPage("instrument", nil)   // the page itself, under the seat
s.Add("console", consolePane)  // anything with Mount(el) error and Close()
s.Add("desktop", deskPane)
s.Show("instrument")
```

It imports neither a terminal nor a desktop. A desktop
([desk](https://github.com/0magnet/desk)) is a graphical session and a
terminal ([websh](https://github.com/0magnet/websh)) is a console; a page
that wants both brings them and names them here. A `Screen` has the same two
methods as a desk pane, so a pane is a screen as it is.

- **Screens are mounted the first time they are shown and kept running.**
  Switching away hides a screen (hidden and inert, still laid out); it does
  not stop it. A screen that implements `Shown(bool)` is told when it comes
  and goes, to pause what nobody can see.
- **The page is a screen too** (`AddPage`): in front, every screen of the
  seat's own is hidden and the page shows through. One page can be several —
  an instrument and a desktop over the same scene — each told when it comes
  and goes.
- **The keyboard comes back where it was.** Each screen keeps what had focus
  when it was left.
- **Ctrl+Alt and a digit, read by position and by what it typed.** On an
  AltGr layout Ctrl+Alt+7 types `{`, and that stays a character. The F keys
  are not used: on Linux the machine takes Ctrl+Alt+F1… for its own consoles
  before a page sees them.
