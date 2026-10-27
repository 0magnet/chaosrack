# Remote access and the host shell

[← README](../README.md)

Off by default, and worth being. A browser tab that can run commands as you is
the most valuable target on the machine, and any page you visit may try to
reach localhost.

```
chaosrack serve --shell                    # a real pty, in a desk window
chaosrack serve --fs                       # real files, for the shell and the file manager
chaosrack serve --fs --fs-root ~/projects  # ...confined to a subtree
chaosrack serve -b 0.0.0.0 -p 8080         # serve the LAN too, by pairing (not with --shell or --fs)
```

- Nothing is served unless `--shell` or `--fs` is passed.
- The server binds **127.0.0.1 by default**, whatever is on: the audio
  capture, the rack link and the routing switch answer whoever can reach them.
  `--bind 0.0.0.0` serves the LAN; with `--shell` or `--fs` a bind that is
  not loopback is **refused**, with the reason.
- The `Origin` must match a page this listener served, and ordinary requests
  are checked with `Sec-Fetch-Site` as well — a browser does **not** send
  `Origin` on a same-origin GET, so requiring one would refuse exactly the
  traffic the check exists to permit.
- A token from `crypto/rand`, per run, never written to disk — put in the
  served page by default, or printed to your terminal instead with `--auth`.
- `--fs-root` confines paths for real: they are resolved through
  `EvalSymlinks` on the longest *existing* prefix, so a symlink planted inside
  the root cannot be followed out of it.
- The audio capture, the rack link and the routing switch take a page by its
  **name**: its `Origin` must be its `Host`, and that host `localhost`,
  `127.0.0.0/8`, `::1`, or a name given with `--host` (a reverse proxy's).
  Matching `Origin` to `Host` alone is not enough — a site whose DNS answer is
  switched to 127.0.0.1 after its page has loaded (DNS rebinding) sends a
  matching pair — but a browser cannot send `Host: localhost` for a page that
  is not at localhost.

## From another machine: pairing

With `--bind` on a LAN address, a browser on another machine pairs once:

```
chaosrack serve --bind 0.0.0.0 --audio
# prints: https://chaosrack.magnetosphere.net/#pair=…&wt=192.168.1.5:8080&cert=…
```

Open the printed link there. The page comes from the Pages build over https —
a page fetched from this server over plain http could have been changed on the
way, so it is for looking at only and is offered no channel. The link's
fragment, which a browser never sends anywhere, carries the server's
WebTransport address, its certificate hashes and this run's pair secret. The
page makes an ed25519 key in its own origin's storage and opens each session
with a signature in the CONNECT request, inside QUIC's TLS 1.3, pinned to the
certificate by hash: encrypted end to end, to a key only the server holds.

- The server keeps the paired **public** keys, in full, in
  `~/.config/chaosrack/paired-tabs`; delete a line to unpair. It keeps its
  certificate there too, so a paired browser comes back after a restart, and
  hands out the next one a week before the current one ends.
- The audio and the rack link reconnect by themselves when the server
  restarts.
- `--pair-page` names another https page of this build to send links to.
- A browser behind a proxy (`--proxy-server`, SOCKS) cannot reach the server:
  QUIC does not go through one. Add the server's address to the bypass list.

## On this machine: the public page, this server's audio

The same pairing, with the server on loopback: `--pair` makes it print a link
to the Pages build that reaches it at 127.0.0.1. The page at
chaosrack.magnetosphere.net then gets this machine's audio and the rack link
without being loaded from here.

```
chaosrack serve --audio --pair
# prints: https://chaosrack.magnetosphere.net/#pair=…&wt=127.0.0.1:8080&cert=…
```

The first time, the browser asks to let the site reach "other apps and
services on this device" (Chrome's Local Network Access): allow it. A page
denied it, or never asked, waits; its console says so after a while. Another
site's page gets nothing: it holds no key the server knows and no secret to
pair one with. A loopback address is not proxied, so this works behind
`--proxy-server` too.

## On a machine with other people on it

Add `--auth`. The default injects the token into the served page, which is
frictionless and right when the only person who can read that page is you — and
wrong on a shared box, for a precise reason: **the Origin check does not stop a
local process.** It stops a hostile web *page*, because a browser sets `Origin`
itself and script cannot change it, but anything that is not a browser sends
whatever header it likes. So another user can fetch the page, read the token,
forge an `Origin`, and have a shell as you.

```
chaosrack serve --shell --auth
```

The token is printed to the terminal that started the server and left out of
the page. Opening the **host shell** window then asks for it, in the terminal
itself, and remembers it for the rest of the tab; a wrong one is refused and it
asks again. There is deliberately no `--password` flag: a password on a command
line is not a secret, since `/proc/<pid>/cmdline` is readable by other users on
exactly the machines where this threat exists.

**Nothing needs a reload.** The `--fs` half used to be composed at startup —
when, under `--auth`, there was nothing to compose it from — so the file manager
and websh stayed on the in-memory filesystem until the page was loaded again,
having just been told the host shell was connected. That reads as the token not
working. The desk's shared filesystem is now a *switch*: every consumer takes an
`afero.Fs` once and keeps it, so the backing is replaced under them the moment
the token is accepted, and the file manager re-renders on the event that
announces it. Verified by typing a token into the host shell and watching a
websh session that started *before* the token existed write a file to the real
`/tmp`. A refused token is withdrawn again and the desk goes back to memory,
because a filesystem answering 403 to everything looks broken rather than
unauthorized.

The Origin check is the load-bearing one and the token is honestly the weaker
guard: a browser sets `Origin` itself and script cannot forge it, whereas a
local process running as you can read the token out of the served page — and
such a process already has your shell without asking this one. Defense in
depth, not a boundary. Stopping the server revokes both.

None of this is a sandbox. With `--shell` the pty is your account, and `--fs`
without `--fs-root` is your whole filesystem — which is the right default only
because `--shell` already implies it, and a fence beside an open gate is not a
fence. The agent itself lives in
[desk](https://github.com/0magnet/desk); this serves it.


## The host shell

With `--shell`, typing `host` in the desk's terminal attaches a shell on this machine to
THAT terminal, the way ssh does on a desktop: it takes over the terminal you
typed it in, and `exit` puts you back at your prompt with the
scrollback intact. `open host` still gives it a window of its own if
that is what you want.

Add `--reconnect` and a NAMED shell outlives the thing displaying it.
`host build` twice is the same shell the second time, with whatever
it printed in between replayed into it — so a long compile survives closing
the window, and survives closing the browser. The name is yours to choose
rather than invented, because only you know whether a new window is meant to
be the old one; a plain `host` is an ordinary shell that ends with
its socket.

What ends a detached shell is the idle timeout (`--reconnect-idle`,
an hour by default, negative for never) or stopping the server. That second
one is not a figure of speech: stopping the process kills them, deliberately,
because otherwise a shell that ignored the resulting SIGHUP would be
reparented to init and keep running as you with nothing left that knows it is
there.
