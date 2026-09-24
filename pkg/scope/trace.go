package scope

// What the tube can actually show.
//
// The sweep is one point per SAMPLE, and the number of samples across the
// screen is the timebase's business: ten divisions of secPerDiv at the
// source rate. At the default 2 ms/div and 48 kHz that is 960 points onto a
// 480-pixel face — two per pixel column. At the slowest detent, half a
// second a division, it is 240,000 points onto the same 480 pixels, which is
// five hundred per column.
//
// Canvas does not care that they are invisible. Measured in Brave on the
// rack's own 480x384 tube, stroking one polyline with the beam's shadow on:
//
//	    480 segments   3.9 ms
//	   2400 segments   6.7 ms
//	   9600 segments  38.6 ms
//	  24000 segments  78.5 ms
//
// — linear in the segment count, against a 16.7 ms frame. So the TIME/DIV
// knob was a frame-rate knob: turning it counter-clockwise walked the whole
// rack down to single figures, and the detail bought with it landed inside
// a pixel where nothing could see it.
//
// A real digital scope does not do this either. It has more samples in
// acquisition memory than it has columns on the display, and what it puts in
// a column is the MIN AND MAX of the samples that fall there — the vertical
// extent of the signal over that slice of time. That is not an
// approximation of the dense trace, it is what the dense trace already looks
// like once the rasterizer has finished with it, and it is why a scope shows
// a filled band for a waveform faster than its own timebase instead of
// aliasing it into a slow phantom the way a point-sampled decimation would.
//
// So: one column per pixel, two points in it, and the cost of a sweep stops
// depending on the timebase at all.

// TraceCols is how many columns a tube this wide has. One per pixel:
// the min/max pair in a column already covers everything between them, so a
// second column inside the same pixel adds nothing that can be seen.
func TraceCols(w float64) int {
	c := int(w)
	if c < 2 {
		return 2
	}
	return c
}

// TraceEnvelope reduces a sweep to one min/max pair per column.
//
// Writes 2 float32s per column into dst — the column's lowest sample then
// its highest — and returns how many columns it filled. Works on the raw
// samples rather than on screen coordinates because the deflection is
// affine in the sample value, so the lowest sample is the lowest point on
// the screen; transforming two values per column instead of every sample is
// the rest of the saving.
func TraceEnvelope(dst, src []float32, cols int) int {
	if cols < 1 || len(src) == 0 || len(dst) < 2 {
		return 0
	}
	if cols > len(dst)/2 {
		cols = len(dst) / 2
	}
	n := len(src)
	if cols > n {
		cols = n
	}
	for c := range cols {
		lo := c * n / cols
		hi := min((c+1)*n/cols, n)
		if hi <= lo {
			// A column narrower than one sample still has to show that
			// sample; an empty column would be a gap in the trace.
			hi = lo + 1
		}
		mn, mx := src[lo], src[lo]
		for _, v := range src[lo+1 : hi] {
			if v < mn {
				mn = v
			}
			if v > mx {
				mx = v
			}
		}
		dst[c*2], dst[c*2+1] = mn, mx
	}
	return cols
}
