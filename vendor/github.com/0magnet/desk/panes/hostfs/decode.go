// This file has no build tag, like mount.go and for the same reason: decoding
// a response is pure, and the part of it that matters — that a caller can still
// use os.IsNotExist on what comes back — is exactly the sort of contract that
// is easy to break and invisible until a file manager offers to overwrite a
// file that is not there.

package hostfs

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"

	"github.com/0magnet/desk/panes/hostproto"
)

// decodeUserDefined undoes the x-user-defined encoding: bytes below 0x80 are
// themselves, and everything above is at U+F700 plus the byte.
func decodeUserDefined(s string) []byte {
	out := make([]byte, 0, len(s))
	for _, r := range s {
		out = append(out, byte(r&0xff))
	}
	return out
}

// decodeError rebuilds an error the CALLER CAN TEST.
//
// afero's callers do not read error strings, they call os.IsNotExist — and so
// does afero ITSELF: memmap.go takes the create-if-missing path on it, and
// util.Exists is built on it. So this has to satisfy the OLD helper, not just
// errors.Is.
//
// THAT IS WHY THE SENTINEL IS RETURNED BARE, and the agent's message dropped
// for the kinds that have one. os.IsNotExist predates errors.Is and does not
// unwrap: it looks one level inside a PathError and compares the result to the
// sentinel by identity. Wrapping with %w satisfies errors.Is and FAILS
// os.IsNotExist, which was the first version of this and would have had afero
// refuse to create a file because it could not tell the file was missing.
//
// Little is lost. The caller puts this in a PathError with the operation and
// the path, so "open /etc/nope: file does not exist" still says everything the
// agent's own message would have; only a generic failure, which has no
// sentinel to be confused with, keeps its text.
func decodeError(body []byte, status int) error {
	var rep hostproto.ErrorReply
	if err := json.Unmarshal(body, &rep); err != nil {
		return fmt.Errorf("hostfs: the agent answered %d", status)
	}
	switch rep.Kind {
	case hostproto.ErrNotExist:
		return fs.ErrNotExist
	case hostproto.ErrExist:
		return fs.ErrExist
	case hostproto.ErrPermission:
		return fs.ErrPermission
	}
	if rep.Msg == "" {
		return fmt.Errorf("hostfs: the agent answered %d", status)
	}
	return errors.New(rep.Msg)
}
