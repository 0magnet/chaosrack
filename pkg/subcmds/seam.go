package subcmds

import "image"

// The seam of a running loop. A trail sweeping along a chaotic trajectory
// never comes back to where it started — that is what chaos is — so a loop of
// it cannot end where it began, and its last frame would jump to its first.
// It is crossfaded instead: the run is drawn a few frames past the loop's end,
// and the loop's first frames blend from that continuation into their own
// picture, so the last frame leads into the first as any frame leads into
// the next. The camera and the colors, which do loop, are the first frames'
// throughout.

// seamFrames is how many of n frames blend across the seam: none when the
// trail does not move.
func seamFrames(n int) int {
	if !renderSweep || renderLive || n < 8 {
		return 0 // still, or every frame a fresh stretch: nothing to join
	}
	return max(n/8, 2)
}

// loopFrames draws n frames of a moving trail with draw(i, at), where i is the
// position along the run (0..n+seam) and at the frame whose camera and colors
// it takes, and crossfades the seam.
func loopFrames(n int, draw func(i, at int) *image.RGBA) []*image.RGBA {
	k := seamFrames(n)
	frames := make([]*image.RGBA, n)
	for i := range n {
		frames[i] = draw(i, i)
	}
	for t := range k {
		// t=0 is almost all the continuation past the end, which is what
		// follows the last frame; t=k-1 almost all its own, which leads into
		// frame k.
		w := float64(t+1) / float64(k+1)
		frames[t] = blendImages(draw(n+t, t), frames[t], w)
	}
	return frames
}

// blendImages is a mixed 1-w of the way and b w of it.
func blendImages(a, b *image.RGBA, w float64) *image.RGBA {
	out := image.NewRGBA(a.Bounds())
	for i := range out.Pix {
		out.Pix[i] = uint8(float64(a.Pix[i])*(1-w) + float64(b.Pix[i])*w + 0.5) //nolint:gosec // a blend of two bytes is a byte
	}
	return out
}
