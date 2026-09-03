# calvin
convert text to Calvin S ascii font (https://patorjk.com/software/taag/#p=display&amp;f=Calvin%20S&amp;t=)


example:

```
$ echo "Hello, World!" | go run cmd/calvin/calvin.go
╦ ╦ ┌─┐┬  ┬  ┌─┐   ╦ ╦ ┌─┐┬─┐┬  ┌┬┐┬    
╠═╣ ├┤ │  │  │ │   ║║║ │ │├┬┘│   │││    
╩ ╩ └─┘┴─┘┴─┘└─┘┘  ╚╩╝ └─┘┴└─┴─┘─┴┘o    

```

library usage example

```
package main

import (
	"github.com/0magnet/calvin"
)

func main() {
	println(calvin.AsciiFont("Hello, World!"))
}

```

## Characters

Upstream Calvin S covers `a-z`, `A-Z`, `[ ] ! @ # $ % ^ & * - _ , . ?` and
space. All of it is here, unchanged.

Everything else — the digits, and `( ) : ; ' " / \ { } | + = < > ~ `` ` `` —
is this port's **extension**. The reference font simply has no glyphs for them,
so [TAAG](https://patorjk.com/software/taag/#p=display&f=Calvin%20S) renders
`2026` as nothing at all. They are drawn in the same light box-drawing style,
three rows tall, and shaped to stay legible against the characters they most
resemble: `0` is barred so it does not read as `o`, `8` closes its lower bowl
where `a` has feet, `{` is `[` plus a notch, and `<` steps into right angles
the way `v` and `x` already do.

The result is that every printable ASCII character renders. Nothing you can
type silently disappears:

```
╔═╗ ╔╗  ╔═╗ ╔╦╗ ╔═╗ ╔═╗ ╔═╗ ╦ ╦ ╦    ╦  ╦╔═ ╦   ╔╦╗
╠═╣ ╠╩╗ ║    ║║ ║╣  ╠╣  ║ ╦ ╠═╣ ║    ║  ╠╩╗ ║   ║║║
╩ ╩ ╚═╝ ╚═╝ ═╩╝ ╚═╝ ╚   ╚═╝ ╩ ╩ ╩   ╚╝  ╩ ╩ ╩═╝ ╩ ╩
╔╗╔ ╔═╗ ╔═╗ ╔═╗ ╦═╗ ╔═╗ ╔╦╗ ╦ ╦ ╦  ╦╦ ╦ ═╗ ╦╦ ╦ ╔═╗
║║║ ║ ║ ╠═╝ ║═╬╗╠╦╝ ╚═╗  ║  ║ ║ ╚╗╔╝║║║ ╔╩╦╝╚╦╝ ╔═╝
╝╚╝ ╚═╝ ╩   ╚═╝╚╩╚═ ╚═╝  ╩  ╚═╝  ╚╝ ╚╩╝ ╩ ╚═ ╩  ╚═╝
┌─┐┌┐ ┌─┐┌┬┐┌─┐┌─┐┌─┐┬ ┬┬ ┬┬┌─┬  ┌┬┐
├─┤├┴┐│   ││├┤ ├┤ │ ┬├─┤│ │├┴┐│  │││
┴ ┴└─┘└─┘─┴┘└─┘└  └─┘┴ ┴┴└┘┴ ┴┴─┘┴ ┴
┌┐┌┌─┐┌─┐┌─┐ ┬─┐┌─┐┌┬┐┬ ┬┬  ┬┬ ┬─┐ ┬┬ ┬┌─┐
││││ │├─┘│─┼┐├┬┘└─┐ │ │ │└┐┌┘│││┌┴┬┘└┬┘┌─┘
┘└┘└─┘┴  └─┘└┴└─└─┘ ┴ └─┘ └┘ └┴┘┴ └─ ┴ └─┘
┌─┐ ┐ ┌─┐┌─┐┬ ┬┌──┌─ ──┐┌─┐┌─┐
│││ │ ┌─┘ ─┤└─┤└─┐├─┐ ┌┘├─┤└─┤
└─┘─┴─└── ─┘  ┴└─┘└─┘ ┴ └─┘ ─┘
┬    ││─┼─┼─┌┼┐  O┬    ┬   │┌┐\│/            /
│      ─┼─┼─└┼┐  ┌┘   ┌┼─   ││─ ─  ─┼─ ───  /
o           └┼┘  ┴O   └┘    └┘/│\     ┘   o/
   ┌─   ─┐ ┌─┐┌─┐  ┌─\  ─┐/\       \
oo┌┘ ═══ └┐ ┌┘│└┘  │  \  │
o┘└──   ──┘ o └──  └─  \─┘     ────
┌─│─┐
┤ │ ├┌─┐
└─│─┘  └┘
```

## Multi-line input

Each line renders as its own block:

```
$ printf 'Hello\nWorld' | calvin
╦ ╦ ┌─┐┬  ┬  ┌─┐
╠═╣ ├┤ │  │  │ │
╩ ╩ └─┘┴─┘┴─┘└─┘
╦ ╦ ┌─┐┬─┐┬  ┌┬┐
║║║ │ │├┬┘│   ││
╚╩╝ └─┘┴└─┴─┘─┴┘
```

`\r\n` and `\r` are accepted, tabs expand to spaces, and a single trailing
newline is ignored so piped input does not render an empty trailing block.

Arguments take precedence over stdin, so `calvin 'text'` works from a script
or a Makefile, where stdin is a pipe rather than a terminal.

## Sample

Ordinary prose, wrapped short because each glyph is three or four columns wide:

```
$ calvin 'Lorem ipsum dolor
sit amet, elit sed
do eiusmod tempor.'
╦   ┌─┐┬─┐┌─┐┌┬┐  ┬┌─┐┌─┐┬ ┬┌┬┐  ┌┬┐┌─┐┬  ┌─┐┬─┐
║   │ │├┬┘├┤ │││  │├─┘└─┐│ ││││   │││ ││  │ │├┬┘
╩═╝ └─┘┴└─└─┘┴ ┴  ┴┴  └─┘└─┘┴ ┴  ─┴┘└─┘┴─┘└─┘┴└─
┌─┐┬┌┬┐  ┌─┐┌┬┐┌─┐┌┬┐   ┌─┐┬  ┬┌┬┐  ┌─┐┌─┐┌┬┐
└─┐│ │   ├─┤│││├┤  │    ├┤ │  │ │   └─┐├┤  ││
└─┘┴ ┴   ┴ ┴┴ ┴└─┘ ┴ ┘  └─┘┴─┘┴ ┴   └─┘└─┘─┴┘
┌┬┐┌─┐  ┌─┐┬┬ ┬┌─┐┌┬┐┌─┐┌┬┐  ┌┬┐┌─┐┌┬┐┌─┐┌─┐┬─┐
 │││ │  ├┤ ││ │└─┐││││ │ ││   │ ├┤ │││├─┘│ │├┬┘
─┴┘└─┘  └─┘┴└─┘└─┘┴ ┴└─┘─┴┘   ┴ └─┘┴ ┴┴  └─┘┴└─o
```

A pangram, for every letter in both cases:

```
$ calvin 'Sphinx of black quartz,
judge my vow!'
╔═╗ ┌─┐┬ ┬┬┌┐┌─┐ ┬  ┌─┐┌─┐  ┌┐ ┬  ┌─┐┌─┐┬┌─  ┌─┐ ┬ ┬┌─┐┬─┐┌┬┐┌─┐
╚═╗ ├─┘├─┤││││┌┴┬┘  │ │├┤   ├┴┐│  ├─┤│  ├┴┐  │─┼┐│ │├─┤├┬┘ │ ┌─┘
╚═╝ ┴  ┴ ┴┴┘└┘┴ └─  └─┘└    └─┘┴─┘┴ ┴└─┘┴ ┴  └─┘└└─┘┴ ┴┴└─ ┴ └─┘┘
 ┬┬ ┬┌┬┐┌─┐┌─┐  ┌┬┐┬ ┬  ┬  ┬┌─┐┬ ┬┬
 ││ │ │││ ┬├┤   │││└┬┘  └┐┌┘│ │││││
└┘└─┘─┴┘└─┘└─┘  ┴ ┴ ┴    └┘ └─┘└┴┘o
```

And the parts upstream cannot render at all — digits, quotes, braces, operators:

```
$ calvin 'calvin -v 2.1 | wc -c
=> {"ok": true} ~90%'
┌─┐┌─┐┬  ┬  ┬┬┌┐┌     ┬  ┬  ┌─┐  ┐   │  ┬ ┬┌─┐     ┌─┐
│  ├─┤│  └┐┌┘││││  ───└┐┌┘  ┌─┘  │   │  ││││    ───│
└─┘┴ ┴┴─┘ └┘ ┴┘└┘      └┘   └──o─┴─  │  └┴┘└─┘     └─┘
   ─┐   ┌─││┌─┐┬┌─││   ┌┬┐┬─┐┬ ┬┌─┐─┐      ┌─┐┌─┐O┬
═══ └┐  ┤   │ │├┴┐  o   │ ├┬┘│ │├┤  ├  ┌─┐ └─┤│││┌┘
   ──┘  └─  └─┘┴ ┴  o   ┴ ┴└─└─┘└─┘─┘    └┘ ─┘└─┘┴O
```
