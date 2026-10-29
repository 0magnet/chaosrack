package rackhost

import (
	"github.com/0magnet/chaosrack/pkg/attractor"
	"github.com/0magnet/chaosrack/pkg/meters"
	"github.com/0magnet/chaosrack/pkg/rasterview"
)

// Sound as a gradient source, natively: the sources past X, Y, Z and the
// trail color the figure from the rack's signal, through the page's table
// (attractor/audiocolorlut.go) — each stretch of an embedding's trail by the
// audio it was drawn from, and any other model as one, by the sound now.

// colorFeed is what the audio sources color from: a table a grid cell, the
// trail's own audio when the model on show draws a window of it, and the
// sound now, followed as the page follows its features.
type colorFeed struct {
	tables [attractor.GridMax]attractor.AudioColor
	// w is the trail's audio, one channel in trail order, and l and r the
	// pair; nil while the model's trail is not a time axis.
	w, l, r []float32
	// amp and centroid are the sound now, each 0..1, and ampPeak what the
	// level is measured against, and clock when they last followed it.
	amp, centroid, ampPeak float32
	clock                  followClock
	// k is how many 60 Hz frames the last follow stood for, which the
	// tables' auto-range relaxes by.
	k float32
}

// colorFFT is how much of the signal the sound-now features are taken
// over: the page's spectrogram transform.
const colorFFT = 1024

// trailAudio records the audio an embedding's trail was drawn from, of l
// and rt: the channel its CHAN knob picks, or the Stereo Embedding's. Called
// with r.mu held, from audioFigure.
func (r *Rack) trailAudio(key string, l, rt []float32) {
	c := &r.color
	switch key {
	case "takens", "polar":
		c.w = attractor.FoldChan(float32(r.number(key+"-chan")), l, rt)
	case "stereo":
		// By its left channel, and the pair for the sources that read both:
		// the one model the page walks both of.
		c.w, c.l, c.r = l, l, rt
	}
}

// soundNow follows the sound's level and brightness, once a frame. Called
// with r.mu held.
func (r *Rack) soundNow() {
	c := &r.color
	l, rt := r.sound.window(colorFFT)
	if len(l) < colorFFT {
		return
	}
	ml, mr := meters.ComputeFFTMags(l), meters.ComputeFFTMags(rt)
	_, _, _, cl := attractor.BandEnergies(ml, sampleRate)
	_, _, _, cr := attractor.BandEnergies(mr, sampleRate)
	k := c.clock.frames(r.sound.now())
	c.k = k
	c.amp = attractor.FeatureSmooth(c.amp, attractor.FeatureNorm(&c.ampPeak, (rms(l)+rms(rt))/2, k), k)
	c.centroid = attractor.FeatureSmooth(c.centroid, min(max(float32((cl+cr)/2), 0), 1), k)
}

// audioColored is fig colored by audio source src through table i: each
// point's place along the trail looked up in the table, as the page's vertex
// shader looks it up, and the gradient following that. A figure that brings
// its own colors keeps them. Called with r.mu held.
func (r *Rack) audioColored(fig attractor.Figure, g *rasterview.Gradient, src, i int) attractor.Figure {
	if !attractor.IsAudioSource(src) || len(fig.Colors) == len(fig.Points) && len(fig.Points) > 0 {
		return fig
	}
	c := &r.color
	t := &c.tables[i]
	t.Fill(src, c.w, c.l, c.r, sampleRate, c.centroid, c.amp, r.value("color-lock") == "1", c.k)
	lut := t.LUT()
	n := len(fig.Points)
	ages := make([]float32, n)
	for k := range ages {
		u := float32(k) / float32(max(n-1, 1))
		if len(fig.Ages) == n {
			u = fig.Ages[k]
		}
		ages[k] = lut[int(min(max(u, 0), 1)*float32(len(lut)-1)+0.5)]
	}
	fig.Ages = ages
	g.Source = rasterview.SourceTrail
	return fig
}

// gradientSource is the panel's gradient source, with the sound now followed
// when it, or a cell of g, is colored by it. Called with r.mu held.
func (r *Rack) gradientSource(g gridSet) int {
	src := int(r.number("gradient-source"))
	if attractor.IsAudioSource(src) || g.soundColored() {
		r.soundNow()
	}
	return src
}
