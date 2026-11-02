//go:build !js

package subcmds

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/0magnet/chaosrack/pkg/cmdfs"
	"github.com/0magnet/chaosrack/pkg/rackhost"
)

// Video: render writes .mp4, .webm, .mkv and .mov through ffmpeg, which it
// hands raw frames on a pipe; and a video is drawn by the rack itself
// (rackhost.Offline) rather than the figure renderer, so every control the
// panel has can be set from here by the id the panel gives it — Persist, the
// phosphor, the palette, the zoom, the Mixer — and what runs in time runs at
// the video's pace however long a frame takes to draw.

var (
	renderAudioFile  string
	renderAudioStart float64
	renderEngine     string
	renderCRF        int
	renderDuration   float64
)

// videoFlags adds what writing a video and reading a recording take.
func videoFlags(renderCmd *cobra.Command) {
	renderCmd.Flags().StringVar(&renderAudioFile, "audio-file", "", "for an audio model, draw this recording (any format ffmpeg reads) instead of recording; --audio limits it to that many seconds")
	renderCmd.Flags().Float64Var(&renderAudioStart, "audio-start", 0, "with --audio-file, start this many seconds into it")
	renderCmd.Flags().StringVar(&renderEngine, "engine", "auto", "what draws: rack (the panel's own scene, every control settable with --set), figure (the gallery's renderer, with --turn, --colors and the rest), or auto (rack for a video, figure otherwise)")
	renderCmd.Flags().IntVar(&renderCRF, "crf", 0, "a video's quality: lower is better and larger (0 = 18 for H.264, 30 for VP9)")
	renderCmd.Flags().Float64Var(&renderDuration, "duration", 0, "with the rack, how many seconds of video to draw (instead of --frames; a signal sets it otherwise)")
}

// videoExts are the containers render writes through ffmpeg.
var videoExts = map[string]bool{".mp4": true, ".webm": true, ".mkv": true, ".mov": true}

// isVideo reports whether name is a video by its extension.
func isVideo(name string) bool { return videoExts[strings.ToLower(filepath.Ext(name))] }

// useRack reports whether the rack draws this render.
func useRack() (bool, error) {
	switch renderEngine {
	case "rack":
		return true, nil
	case "figure":
		return false, nil
	case "auto", "":
		return isVideo(renderOut), nil
	}
	return false, fmt.Errorf("--engine %q: use rack, figure or auto", renderEngine)
}

// decodeAudio reads a recording through ffmpeg as stereo at the rate render
// draws at: from --audio-start, for --audio seconds or to its end.
func decodeAudio(name string) (l, r []float32, err error) {
	if _, err := os.Stat(name); err != nil {
		return nil, nil, err
	}
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin"}
	if renderAudioStart > 0 {
		args = append(args, "-ss", strconv.FormatFloat(renderAudioStart, 'f', -1, 64))
	}
	args = append(args, "-i", name)
	if renderAudioSecs > 0 {
		args = append(args, "-t", strconv.FormatFloat(renderAudioSecs, 'f', -1, 64))
	}
	args = append(args, "-vn", "-f", "f32le", "-ac", "2", "-ar", strconv.Itoa(renderSampleRate), "-")
	cmd := exec.CommandContext(runCtx, ffmpegPath(), args...) //nolint:gosec // the user's own file, to the user's own ffmpeg
	var out, msg bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &msg
	if err := cmd.Run(); err != nil {
		return nil, nil, fmt.Errorf("ffmpeg could not read %s: %w %s", name, err, strings.TrimSpace(msg.String()))
	}
	n := out.Len() / 8
	l, r = make([]float32, n), make([]float32, n)
	b := out.Bytes()
	for i := range n {
		l[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[8*i:]))
		r[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[8*i+4:]))
	}
	if n == 0 {
		return nil, nil, fmt.Errorf("%s: no audio", name)
	}
	errf("chaosrack: read %.1fs of %s\n", float64(n)/renderSampleRate, filepath.Base(name))
	return l, r, nil
}

// ffmpegPath is ffmpeg on PATH, or its bare name for the error to name.
func ffmpegPath() string {
	if p, err := exec.LookPath("ffmpeg"); err == nil {
		return p
	}
	return "ffmpeg"
}

// ── the encoder ─────────────────────────────────────────────────────────

// videoOut is a video being written: frames go to ffmpeg as they are drawn,
// into a file beside the output, and the sound to a file of its own, which
// a second pass puts beside the picture, since the rack's speakers are heard
// only as its frames are drawn.
type videoOut struct {
	name, part string
	w, h, bpp  int
	cmd        *exec.Cmd
	in         io.WriteCloser
	msg        bytes.Buffer
	frames     int
	snd        *os.File
	sndw       *bufio.Writer
	loud       bool
}

// videoCodec is the picture's and the sound's encoder for a container.
func videoCodec(ext string) (v, a []string) {
	crf := renderCRF
	if ext == ".webm" {
		if crf <= 0 {
			crf = 30
		}
		return []string{"-c:v", "libvpx-vp9", "-crf", strconv.Itoa(crf), "-b:v", "0", "-row-mt", "1", "-pix_fmt", "yuv420p"},
			[]string{"-c:a", "libopus", "-b:a", "160k"}
	}
	if crf <= 0 {
		crf = 18
	}
	return []string{"-c:v", "libx264", "-preset", "medium", "-crf", strconv.Itoa(crf), "-pix_fmt", "yuv420p"},
		[]string{"-c:a", "aac", "-b:a", "192k"}
}

// startVideo starts ffmpeg on a w by h video at fps, taking frames of pixel
// format pixfmt (rgb24 or rgba).
func startVideo(name string, w, h, fps int, pixfmt string) (*videoOut, error) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return nil, errors.New("writing a video needs ffmpeg on PATH")
	}
	ext := strings.ToLower(filepath.Ext(name))
	v := &videoOut{name: name, part: strings.TrimSuffix(name, ext) + ".part" + ext, w: w, h: h, bpp: 3}
	if pixfmt == "rgba" {
		v.bpp = 4
	}
	vc, _ := videoCodec(ext)
	args := []string{
		"-hide_banner", "-loglevel", "error", "-y",
		"-f", "rawvideo", "-pix_fmt", pixfmt, "-s", fmt.Sprintf("%dx%d", w, h), "-framerate", strconv.Itoa(fps), "-i", "-",
		// 4:2:0 needs an even size.
		"-vf", "pad=ceil(iw/2)*2:ceil(ih/2)*2",
	}
	args = append(append(args, vc...), v.part)
	v.cmd = exec.CommandContext(runCtx, ffmpegPath(), args...) //nolint:gosec // fixed arguments and the user's own output name
	v.cmd.Stderr = &v.msg
	in, err := v.cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	v.in = in
	if err := v.cmd.Start(); err != nil {
		return nil, err
	}
	snd, err := os.CreateTemp("", "chaosrack-*.f32")
	if err != nil {
		v.abort()
		return nil, err
	}
	v.snd, v.sndw = snd, bufio.NewWriterSize(snd, 1<<16)
	return v, nil
}

// frame writes one frame of w*h*bpp bytes.
func (v *videoOut) frame(px []byte) error {
	if len(px) != v.w*v.h*v.bpp {
		return fmt.Errorf("a frame of %d bytes, not %d", len(px), v.w*v.h*v.bpp)
	}
	if _, err := v.in.Write(px); err != nil {
		return v.failed(err)
	}
	v.frames++
	return nil
}

// sound adds a stretch of the soundtrack.
func (v *videoOut) sound(l, r []float32) error {
	var b [8]byte
	for i := range l {
		if l[i] != 0 || r[i] != 0 {
			v.loud = true
		}
		binary.LittleEndian.PutUint32(b[:4], math.Float32bits(l[i]))
		binary.LittleEndian.PutUint32(b[4:], math.Float32bits(r[i]))
		if _, err := v.sndw.Write(b[:]); err != nil {
			return err
		}
	}
	return nil
}

// failed is err with what ffmpeg said about it.
func (v *videoOut) failed(err error) error {
	if s := strings.TrimSpace(v.msg.String()); s != "" {
		return fmt.Errorf("ffmpeg: %s", s)
	}
	return fmt.Errorf("ffmpeg: %w", err)
}

// abort stops ffmpeg and removes what it had written.
func (v *videoOut) abort() {
	if v.in != nil {
		v.in.Close() //nolint:errcheck,gosec // abandoning it
	}
	if v.cmd != nil && v.cmd.Process != nil {
		v.cmd.Wait() //nolint:errcheck,gosec // abandoning it
	}
	os.Remove(v.part) //nolint:errcheck,gosec // may never have been made
	if v.snd != nil {
		v.snd.Close()           //nolint:errcheck,gosec // abandoning it
		os.Remove(v.snd.Name()) //nolint:errcheck,gosec // abandoning it
	}
}

// finish ends the picture and puts the sound beside it, if there was any.
func (v *videoOut) finish(fps int) error {
	defer func() {
		v.snd.Close()           //nolint:errcheck,gosec // read by ffmpeg already
		os.Remove(v.snd.Name()) //nolint:errcheck,gosec // a temporary file
	}()
	if err := v.in.Close(); err != nil {
		v.abort()
		return v.failed(err)
	}
	if err := v.cmd.Wait(); err != nil {
		os.Remove(v.part) //nolint:errcheck,gosec // a broken part
		return v.failed(err)
	}
	if err := v.sndw.Flush(); err != nil {
		return err
	}
	ext := strings.ToLower(filepath.Ext(v.name))
	if !v.loud {
		if err := os.Rename(v.part, v.name); err != nil {
			return err
		}
		errf("chaosrack: wrote %s: %d frames, %.1fs at %d fps, no sound\n", v.name, v.frames, float64(v.frames)/float64(fps), fps)
		return nil
	}
	_, ac := videoCodec(ext)
	args := []string{
		"-hide_banner", "-loglevel", "error", "-y",
		"-i", v.part,
		"-f", "f32le", "-ar", strconv.Itoa(renderSampleRate), "-ac", "2", "-i", v.snd.Name(),
		"-map", "0:v", "-map", "1:a", "-c:v", "copy",
	}
	args = append(args, ac...)
	if ext == ".mp4" || ext == ".mov" {
		args = append(args, "-movflags", "+faststart")
	}
	args = append(args, v.name)
	mux := exec.CommandContext(runCtx, ffmpegPath(), args...) //nolint:gosec // fixed arguments and the user's own output name
	var msg bytes.Buffer
	mux.Stderr = &msg
	err := mux.Run()
	os.Remove(v.part) //nolint:errcheck,gosec // muxed, or failed either way
	if err != nil {
		return fmt.Errorf("ffmpeg, adding the sound: %w %s", err, strings.TrimSpace(msg.String()))
	}
	errf("chaosrack: wrote %s: %d frames, %.1fs at %d fps, with sound\n", v.name, v.frames, float64(v.frames)/float64(fps), fps)
	return nil
}

// rgbaVideo writes n frames, each drawn by draw(i) — a pure function of i, so
// they are drawn on every core at once and written in order — with l and r
// as the soundtrack, if there is one.
func rgbaVideo(name string, n int, draw func(i int) *image.RGBA, l, r []float32) error {
	if n <= 0 {
		return errors.New("a video of no frames")
	}
	first := draw(0)
	b := first.Bounds()
	v, err := startVideo(name, b.Dx(), b.Dy(), renderFPS, "rgba")
	if err != nil {
		return err
	}
	if err := v.sound(l, r); err != nil {
		v.abort()
		return err
	}
	batch := max(runtime.GOMAXPROCS(0), 1) * 2
	imgs := make([]*image.RGBA, batch)
	for at := 0; at < n; at += batch {
		k := min(batch, n-at)
		var wg sync.WaitGroup
		for j := range k {
			if at+j == 0 {
				imgs[0] = first
				continue
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				imgs[j] = draw(at + j)
			}()
		}
		wg.Wait()
		for j := range k {
			if err := v.frame(imgs[j].Pix); err != nil {
				v.abort()
				return err
			}
		}
		progress(at+k, n)
	}
	return v.finish(renderFPS)
}

// progress says how far a long render has got, on one line of the terminal.
func progress(done, of int) {
	if of < 2*renderFPS || done%renderFPS != 0 && done < of {
		return
	}
	errf("\rchaosrack: frame %d of %d", done, of)
	if done >= of {
		errf("\n")
	}
}

// ── the rack ────────────────────────────────────────────────────────────

// figureOnly are render's flags that the figure renderer reads and the rack
// does not, with the panel's control that does the same, where there is one.
var figureOnly = map[string]string{
	"angle":      "rotation-controls-x/y/z turn it; a drag is the page's",
	"turn":       "rotation-controls-x/y/z",
	"rock":       "",
	"tumble":     "",
	"walk":       "",
	"colors":     "gradient-colors",
	"gradient":   "gradient-source",
	"background": "ck-color-bg-hue and ck-color-bg-lvl",
	"crop":       "camera-zoom, pan-x, pan-y",
	"fit-trim":   "camera-zoom",
	"sweep":      "",
	"live":       "",
	"points":     "trail-slider",
	"trail":      "trail-slider",
	"seconds":    "",
	"transient":  "",
	"tau":        "takens-tau",
	"window":     "takens-win (and the other models' -win)",
	"impulse":    "",
	"check":      "",
}

// renderRack draws the render with the rack: a video, a GIF, or the last
// frame of a run as a PNG.
func renderRack(cmd *cobra.Command) error {
	ext := strings.ToLower(filepath.Ext(renderOut))
	if renderOut == "" || !(isVideo(renderOut) || ext == ".gif" || ext == ".png") {
		return errors.New("the rack draws to -o a .mp4, .webm, .mkv, .mov, .gif or .png")
	}
	for name, hint := range figureOnly {
		if cmd.Flags().Changed(name) {
			msg := fmt.Sprintf("--%s is the figure renderer's (--engine figure); the rack draws a video", name)
			if hint != "" {
				msg += "; on the rack, --set " + hint
			}
			return errors.New(msg)
		}
	}
	if renderModel == "" {
		return errors.New("--model is required (chaosrack models lists them)")
	}
	o, err := rackhost.NewOffline(renderFPS)
	if err != nil {
		return err
	}
	if err := o.SelectModel(renderModel); err != nil {
		return err
	}
	if renderIntg != "" && renderIntg != "rk4" {
		// The rack writes its INTG positions into the integrator as it
		// builds a figure, so the switch is turned on the rack.
		if err := setRackIntegrator(o); err != nil {
			return err
		}
	}
	for _, s := range renderSet {
		id, val, ok := strings.Cut(s, "=")
		if !ok {
			return fmt.Errorf("--set %q is not id=value", s)
		}
		if err := o.Set(strings.TrimSpace(id), val); err != nil {
			return fmt.Errorf("--set: %w", err)
		}
	}
	// The capture, if one is given: an audio model on the rack draws the rack's
	// signal, which its own generators and Model Out can make with no capture
	// at all, as on the page.
	var l, r []float32
	if renderAudioFile != "" || renderSignal != "" || renderAudioSecs > 0 {
		if l, r, err = audioSignal(); err != nil {
			return err
		}
	}
	frames := renderFrames
	switch {
	case cmd.Flags().Changed("frames"):
	case renderDuration > 0:
		frames = int(math.Ceil(renderDuration * float64(renderFPS)))
	case len(l) > 0:
		frames = int(math.Ceil(float64(len(l)) * float64(renderFPS) / renderSampleRate))
	}
	if frames <= 0 {
		if isAudioInput(renderModel) {
			return fmt.Errorf("%s is drawn from the rack's signal: %s, or --duration SECONDS draws what the rack's generators make", renderModel, audioHint)
		}
		return errors.New("how long? --duration SECONDS or --frames N (or a signal to run the length of)")
	}

	w, h := renderW, renderH
	var v *videoOut
	var gifFrames []*image.RGBA
	if isVideo(renderOut) {
		if v, err = startVideo(renderOut, w, h, renderFPS, "rgb24"); err != nil {
			return err
		}
	}
	at := 0
	var last []byte
	for i := range frames {
		n := o.Samples(i)
		fl, fr := clip(l, at, n), clip(r, at, n)
		at += n
		img, sl, sr, err := o.Frame(w, h, fl, fr)
		if err != nil && img == nil {
			if v != nil {
				v.abort()
			}
			return err
		}
		switch {
		case v != nil:
			if err := v.frame(img.Px); err != nil {
				v.abort()
				return err
			}
			if err := v.sound(mixed(fl, sl), mixed(fr, sr)); err != nil {
				v.abort()
				return err
			}
		case ext == ".gif":
			gifFrames = append(gifFrames, rgbaOf(img.Px, w, h))
		}
		last = img.Px
		progress(i+1, frames)
	}
	switch {
	case v != nil:
		return v.finish(renderFPS)
	case ext == ".gif":
		return writeGIFFrames(renderOut, gifFrames)
	}
	return writePNG(renderOut, rgbaOf(last, w, h))
}

// setRackIntegrator turns the rack's INTG switch to --integrator: any one of
// them, since the rack keeps them all in step.
func setRackIntegrator(o *rackhost.Offline) error {
	for _, c := range o.Controls() {
		if strings.HasSuffix(c.ID, "-intg") {
			return o.Set(c.ID, "1")
		}
	}
	return errors.New("the rack has no INTG switch")
}

// clip is n samples of s from at, or what there is of them.
func clip(s []float32, at, n int) []float32 {
	if at >= len(s) {
		return nil
	}
	return s[at:min(at+n, len(s))]
}

// mixed is what is heard: the signal, as it played, and the rack's speakers
// over it, held inside full scale.
func mixed(sig, spk []float32) []float32 {
	out := make([]float32, len(spk))
	for i := range out {
		x := spk[i]
		if i < len(sig) {
			x += sig[i]
		}
		out[i] = max(-1, min(1, x))
	}
	return out
}

// writePNG writes one picture as a PNG.
func writePNG(name string, m image.Image) error {
	f, err := cmdfs.Create(runCtx, name)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck // the encode error below is the one that matters
	return png.Encode(f, m)
}

// rgbaOf is rgb24 pixels as an image.
func rgbaOf(px []byte, w, h int) *image.RGBA {
	m := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := range w * h {
		copy(m.Pix[4*i:4*i+3], px[3*i:3*i+3])
		m.Pix[4*i+3] = 255
	}
	return m
}

// listRackParams prints the rack's controls, which a render with the rack
// can turn with --set: the panel's own ids, as `chaosrack ctl` names them.
func listRackParams() error {
	o, err := rackhost.NewOffline(renderFPS)
	if err != nil {
		return err
	}
	var b strings.Builder
	w := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "\nPANEL CONTROL (--engine rack, and any video)\tMODULE\tVALUE\tRANGE OR CHOICES") //nolint:errcheck // a strings.Builder
	for _, c := range o.Controls() {
		var rng string
		switch {
		case c.IsSwitch:
			rng = "on off"
		case c.IsSelect:
			opts := c.Options
			if len(c.Labels) == len(c.Options) && len(c.Options) <= 16 {
				opts = make([]string, len(c.Options))
				for i, op := range c.Options {
					opts[i] = op
					if lb := strings.TrimSpace(c.Labels[i]); lb != "" && lb != op {
						opts[i] = op + "=" + strings.Join(strings.Fields(lb), " ")
					}
				}
			}
			if len(opts) > 16 {
				opts = append(opts[:16:16], fmt.Sprintf("… %d in all", len(c.Options)))
			}
			rng = strings.Join(opts, " ")
		case c.IsText:
			rng = "text"
		case c.Max > c.Min:
			rng = fmt.Sprintf("%g .. %g", c.Min, c.Max)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", c.ID, c.Module, c.Value, rng) //nolint:errcheck // a strings.Builder
	}
	if err := w.Flush(); err != nil {
		return err
	}
	_, err = io.WriteString(stdout, b.String())
	return err
}
