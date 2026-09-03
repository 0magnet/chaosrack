//go:build js && wasm

package attractor

import (
	"strconv"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/audiosrc"
)

// Shared plumbing for audio-driven modes (spectrogram, xy scope, and
// eventually audio-modulated attractors). The audiosrc.Source is created
// lazily on first entry into any audio mode and kept alive across mode
// switches so the mic permission prompt only appears once. It's also
// safe to have live when the user is in an attractor mode — no cost, no
// CPU work, just an idle AudioContext.

var (
	audioSource      audiosrc.Source
	audioSourceTried bool
	audioModeActive  bool // last frame we rendered an audio mode
	audioOverlay     js.Value
	audioMsgLast     string  // last status text shown (so we don't re-show every frame)
	audioHideFn      js.Func // cached auto-hide callback (never re-created)

	// Function generator: a client-side signal source. When useFuncGen is on it
	// overrides ws/mic, so audio-reactive features (attractor modulation), the
	// spectrogram and the xy scope all work with no server and no mic.
	funcGen    *audiosrc.FuncGen
	useFuncGen bool
)

// ensureAudioSource returns the shared audio source, creating it on first
// call. Safe to call every frame — the second and subsequent calls just
// return the cached value. Returns nil only if the source can't be
// constructed at all (unlikely; a failed source still returns non-nil
// with Ready()==false and Err()!=nil).
//
// The backend is chosen from the ?audio= query param:
//   - audio=ws (or websocket): stream from a WebSocket server (the
//     audioprism-go-style /ws feed served by cmd/audiows). An optional
//     ?wsurl= overrides the endpoint; default is same-origin /ws.
//   - audio=wt (or webtransport): the same audio over WebTransport
//     datagrams, which is an OPTION and not a replacement — it only pays
//     off on a remote or lossy link (see pkg/audiosrc/datagram.go), and
//     it falls back to the WebSocket by itself when the browser has no
//     WebTransport or the handshake fails. The WebSocket remains what a
//     page with no query parameter gets.
//   - anything else / absent: the browser microphone via getUserMedia.
func ensureAudioSource() audiosrc.Source {
	// The function generator, when on, is the source — regardless of ws/mic.
	if useFuncGen {
		if funcGen == nil {
			funcGen = audiosrc.NewFuncGen()
		}
		return funcGen
	}
	if audioSource != nil {
		return audioSource
	}
	if audioSourceTried {
		return nil
	}
	audioSourceTried = true
	ws := audiosrc.WSOptions{
		URL:        queryParam("wsurl"),
		SampleRate: wsSampleRate(),
	}
	switch audioBackendKind() {
	case "ws", "websocket":
		audioSource = audiosrc.NewWebSocket(ws)
	case "wt", "webtransport":
		// The same WSOptions go along as the fallback, so ?wsurl= and
		// ?wsrate= keep working when WebTransport is declined.
		audioSource = audiosrc.NewWebTransport(audiosrc.WTOptions{
			URL:        queryParam("wturl"),
			CertHash:   queryParam("wthash"),
			SampleRate: wsSampleRate(),
			WS:         ws,
		})
	default:
		// The capture graph rides the shared context. The mic lease is held
		// for the session — the source is kept alive across mode switches, so
		// suspending its context out from under it would stall the rings.
		audioSource = audiosrc.NewMic(audiosrc.MicOptions{
			Stereo:  true,
			Context: acquireAudioCtx("mic"),
		})
	}
	return audioSource
}

// fg returns the function generator, creating it on first use.
func fg() *audiosrc.FuncGen {
	if funcGen == nil {
		funcGen = audiosrc.NewFuncGen()
	}
	return funcGen
}

// setFuncGen switches the audio source to (or away from) the function
// generator. Ensures features/spectrogram/xy read the FG immediately.
func setFuncGen(on bool) {
	useFuncGen = on
	if on {
		ensureAudioSource()
	}
}

// activeAudioSource is the source actually feeding the analysis paths: the
// function generator while it is on, otherwise whatever backend was created.
//
// It exists because ensureAudioSource returns the generator without ever
// storing it in audioSource, so code that read that variable directly saw
// nothing when the generator was the source — the xy scope, which asks for
// the source properly, drew a Lissajous while the spectrogram and the FVF
// display next to it stayed black. Unlike ensureAudioSource this creates
// nothing: the render loop must not be what pops a microphone prompt.
func activeAudioSource() audiosrc.Source {
	if useFuncGen {
		if funcGen == nil {
			return nil
		}
		return funcGen
	}
	return audioSource
}

// audioBackendKind reads the ?audio= query param (empty when absent).
// audioBackendKind picks where audio comes from: the WebSocket feed, the
// WebTransport one, or this machine's microphone.
//
// THE SERVER ANSWERS THIS unless the URL overrules it. A server started with
// --audio (and cmd/audiows, which is nothing but that) writes window.__crAudio
// into the page it serves, so a page that HAS a feed uses it without being
// asked to. ?audio= is still honored first, because an explicit request is a
// request — including "?audio=mic" to refuse a feed that is on offer.
//
// It used to read the query alone, which meant the one arrangement that cannot
// fail — a server capturing audio, serving the page, on the same port — still
// showed silence until you knew to add ?audio=ws. And the reverse mistake was
// worse: ?audio=ws against a server without --audio is a socket that cannot
// connect and an error overlay, which is the app reporting a fault when nothing
// is faulty. Neither case can arise now: the page asks for a feed exactly when
// one exists.
func audioBackendKind() string {
	if q := queryParam("audio"); q != "" {
		return q
	}
	if a := js.Global().Get("__crAudio"); a.Truthy() {
		return a.String()
	}
	return "" // no feed offered: the microphone, as before
}

// wsSampleRate is the rate the WS server records at. The wire format carries
// no rate, so the consumer must be told; cmd/audiows defaults to 24000 — the
// same rate audioprism-go records at (pulse.RecordSampleRate(24000)) and the
// rate its spectrogram package (sg.SampleRate) is designed around, so the
// scroll speed and frequency bins match the reference exactly. An optional
// ?wsrate= overrides it. This rate drives the scroll pacing and the FVF
// pitch/AudioContext, so a mismatch shifts pitch (and scroll speed) by the ratio.
func wsSampleRate() int {
	if v := queryParam("wsrate"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 24000
}

// queryParam returns the named URL query parameter from window.location,
// or "" if not present.
func queryParam(name string) string {
	search := js.Global().Get("location").Get("search")
	params := js.Global().Get("URLSearchParams").New(search)
	if v := params.Call("get", name); v.Truthy() {
		return v.String()
	}
	return ""
}

// isAudioMode reports whether the mode bypasses the 3D pipeline and draws
// on its own 2D program. Only the xy scope does now — the spectrogram is a
// textured plane model that goes through the normal 3D render path (so it
// rotates/zooms like other models); it's handled in generateForMode.
func isAudioMode(mode string) bool {
	return mode == "xy"
}

// renderAudioFrame dispatches to a bypass audio mode's renderer (xy).
// Called from renderLoop when isAudioMode(selectedMode) is true.
func renderAudioFrame(mode string) {
	switch mode {
	case "xy":
		renderXYFrame()
	}
	maybeShowAudioStatus()
}

// activateAudioMode runs once when the mode dispatch transitions from an
// attractor mode into an audio mode. It kicks off the audio source (mic
// permission prompt, or WebSocket connect) but otherwise does nothing
// heavy — the individual renderers do their own lazy shader/texture setup.
func activateAudioMode() {
	audioModeActive = true
	ensureAudioSource()
}

// deactivateAudioMode runs once when we transition back out of an audio
// mode. Restores the attractor pipeline: rebinds the attractor shader
// program and marks static geometry dirty so the next indexed upload
// re-sets attribute pointers (audio modes leave their own attribute
// pointers active on aPos).
func deactivateAudioMode() {
	audioModeActive = false
	if !shaderProgram.IsUndefined() {
		gl.Call("useProgram", shaderProgram)
	}
	staticGeomDirty = true
	if audioOverlay.Truthy() {
		audioOverlay.Get("style").Set("display", "none")
	}
}

// maybeShowAudioStatus creates a small overlay message when the mic
// isn't Ready yet (permission still pending, or denied). The overlay
// lives at the top-center of the canvas. Hidden once the source starts
// producing samples, or when we leave audio mode.
func maybeShowAudioStatus() {
	src := activeAudioSource()
	if src == nil {
		return
	}
	if src.Ready() && src.Err() == nil {
		// A working source may still have something to say. The
		// WebTransport source falls back to a WebSocket by itself when
		// WebTransport is unavailable, and that fallback is Ready with
		// no Err — so without this it would be reported by nothing at
		// all, and a page silently on the wrong transport looks exactly
		// like a page on the right one.
		if n, ok := src.(audiosrc.Notifier); ok {
			if notice := n.Notice(); notice != "" {
				showAudioStatus(notice)
				return
			}
		}
		if audioOverlay.Truthy() {
			audioOverlay.Get("style").Set("display", "none")
		}
		audioMsgLast = "" // source recovered; a future error will re-show
		return
	}
	msg := "Requesting microphone access…"
	if err := src.Err(); err != nil {
		msg = "Audio unavailable: " + err.Error()
	}
	showAudioStatus(msg)
}

// showAudioStatus puts one line in the overlay, creating it on first use.
func showAudioStatus(msg string) {
	// Only (re)show when the status text CHANGES — otherwise a persistent error
	// (mic denied, ws down) would re-appear every frame and could never be
	// dismissed. Once shown it auto-hides after a few seconds and can be tapped
	// away; a genuinely new status shows again.
	if msg == audioMsgLast {
		return
	}
	audioMsgLast = msg
	if !audioOverlay.Truthy() {
		audioOverlay = doc.Call("createElement", "div")
		audioOverlay.Set("id", "audio-status-overlay")
		style := audioOverlay.Get("style")
		style.Set("position", "fixed")
		style.Set("top", "20px")
		style.Set("left", "50%")
		style.Set("transform", "translateX(-50%)")
		style.Set("padding", "8px 14px")
		style.Set("background", "rgba(0,0,0,0.7)")
		style.Set("color", "#fff")
		style.Set("font-family", "monospace")
		style.Set("font-size", "13px")
		style.Set("border", "1px solid #555")
		style.Set("border-radius", "4px")
		style.Set("z-index", "var(--z-status)")
		style.Set("cursor", "pointer") // tap to dismiss
		style.Set("pointer-events", "auto")
		audioOverlay.Call("addEventListener", "click", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
			audioOverlay.Get("style").Set("display", "none")
			return nil
		}))
		body.Call("appendChild", audioOverlay)
	}
	audioOverlay.Set("textContent", msg+"   ✕")
	audioOverlay.Get("style").Set("display", "block")
	// Auto-hide after 5s so it never sticks; the source keeps trying
	// regardless. One cached js.Func — a fresh FuncOf per status change never
	// got released, which leaked a closure every message on a flaky source.
	if audioHideFn.IsUndefined() {
		audioHideFn = trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
			if audioOverlay.Truthy() {
				audioOverlay.Get("style").Set("display", "none")
			}
			return nil
		})
	}
	js.Global().Call("setTimeout", audioHideFn, 5000)
}
