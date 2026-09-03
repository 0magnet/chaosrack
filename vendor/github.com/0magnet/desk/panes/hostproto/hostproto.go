// Package hostproto is the wire between a desk pane and the host agent.
//
// It has no build tag and no dependency outside encoding/json, because both
// ends of the wire have to agree on it and the two ends compile for different
// platforms — the agent is native, the pane is js/wasm. A protocol defined
// twice is a protocol that can differ, and a round-trip test in each half is no
// guard at all: it passes perfectly against a framing the other half rejects.
// So it is defined once, here, and both halves import it.
//
// # The shape
//
// Server to client is RAW BINARY: whatever the pty produced, unframed. A
// terminal's output is already a self-describing byte stream — that is what
// escape sequences are — so wrapping it would mean parsing a wrapper to hand
// the contents to a parser. Chunk boundaries carry no meaning and need not be
// preserved.
//
// Client to server is JSON TEXT, one message per frame. It has to be framed
// because there are two kinds of thing to say and they are not distinguishable
// as bytes: keystrokes, and "the window changed size". Sending resize down the
// same channel as input is why this does not reuse xterm-go's Attach, which
// sends keystrokes as bare text frames and has nowhere to put anything else.
//
// The asymmetry is deliberate rather than an oversight: only one direction has
// more than one kind of message in it.
package hostproto

// The client-to-server message types.
const (
	// TypeInput carries keystrokes in D.
	TypeInput = "i"

	// TypeResize carries a new grid size in C and R. The agent passes it to
	// the pty as a TIOCSWINSZ, which is what makes a full-screen program
	// redraw and what makes $COLUMNS right.
	TypeResize = "r"

	// TypeBinary carries input that is not text, base64-encoded in D.
	//
	// It exists for one real case: a terminal in mouse-reporting mode
	// answers with byte values above 127, and the X10 encoding in
	// particular is bytes and not characters. Those cannot ride in D as a
	// string — JSON demands valid UTF-8 and encoding/json substitutes
	// U+FFFD for what is not, so a click near the right edge of a wide
	// window would arrive as a different column, silently. Base64 costs a
	// third more bytes on a path that carries a few bytes per click.
	TypeBinary = "b"
)

// Msg is one client-to-server message.
//
// One struct with a type tag rather than a union: the whole thing is four
// fields, and a tagged struct decodes in one pass without a second unmarshal to
// find out what it was.
type Msg struct {
	// T is one of the Type constants above. A message with any other value
	// is ignored rather than fatal — an older agent should survive a newer
	// pane inventing a message, since the alternative is a dropped shell.
	T string `json:"t"`

	// D is the input bytes for TypeInput, as a string.
	//
	// A string and not []byte (which JSON would base64) because terminal
	// input IS text: it is what the keyboard produced, plus escape sequences
	// for the keys that are not characters. Go strings do not require valid
	// UTF-8, but encoding/json escapes what it must, so a paste of arbitrary
	// bytes survives the round trip as the replacement character rather than
	// corrupting the frame.
	D string `json:"d,omitempty"`

	// C and R are columns and rows for TypeResize.
	C int `json:"c,omitempty"`
	R int `json:"r,omitempty"`
}

// TokenParam is the query parameter the token is presented in.
//
// A query parameter and not a header because the browser's WebSocket
// constructor cannot set headers — it takes a URL and a subprotocol list and
// nothing else. The token therefore appears in the agent's logs if it logs
// URLs, which is survivable for a value that lives as long as one process and
// is never written to disk.
const TokenParam = "token"

// ColsParam and RowsParam carry the initial grid on the handshake.
//
// On the URL rather than as a first message because the size is needed before
// the shell exists, not after: it is an argument to starting one.
const (
	ColsParam = "cols"
	RowsParam = "rows"
)

// Path is where the agent serves the pty endpoint.
const Path = "/host/pty"

// The filesystem endpoints.
//
// Separate from the pty and separately enabled, because they are separately
// useful: a file manager over the real filesystem is worth having without
// handing out a shell, and it is a smaller thing to hand out.
//
// Plain HTTP rather than another socket. Every operation is a request and a
// response with no session between them, which is exactly what an
// afero.Fs call is — and it means the browser's cache, range requests and
// content types work on file reads without anything being reimplemented.
const (
	FSPath     = "/host/fs/"
	FSStat     = FSPath + "stat"
	FSList     = FSPath + "list"
	FSRead     = FSPath + "read"
	FSWrite    = FSPath + "write"
	FSMkdir    = FSPath + "mkdir"
	FSRemove   = FSPath + "remove"
	FSRename   = FSPath + "rename"
	FSChmod    = FSPath + "chmod"
	FSChtimes  = FSPath + "chtimes"
	FSTruncate = FSPath + "truncate"
)

// Query parameters for the filesystem endpoints.
const (
	PathParam  = "p"
	ToParam    = "to"
	PermParam  = "perm"
	AllParam   = "all"
	SizeParam  = "n"
	ATimeParam = "at"
	MTimeParam = "mt"
)

// FileInfo is one entry, as os.FileInfo reduced to what crosses a wire.
//
// Short field names because a directory listing is thousands of these and the
// whole point of the listing is that it appears instantly.
type FileInfo struct {
	Name string `json:"n"`
	Size int64  `json:"s"`
	Mode uint32 `json:"m"` // as os.FileMode bits
	Mod  int64  `json:"t"` // unix nanoseconds
	Dir  bool   `json:"d"`
}

// Error kinds, so the client can rebuild an error the caller can test.
//
// This exists because afero's callers do not read error strings, they call
// os.IsNotExist — and a "no such file" that fails that test sends a file
// manager down the "something went wrong" path when it should be creating the
// file. The kind is what survives the wire; the message is only for a human.
const (
	ErrNotExist   = "notexist"
	ErrExist      = "exist"
	ErrPermission = "permission"
	ErrOther      = "other"
)

// ErrorReply is the body of any failed filesystem request.
type ErrorReply struct {
	Kind string `json:"k"`
	Msg  string `json:"m"`
}
