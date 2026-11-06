package attractor

// zoomedDist is the camera's distance from the model for a zoom setting.
//
// The zoom knob is in world units: a permalink's z is how many units nearer
// the camera stands than the fitted distance. That reads well for a model
// fitted from a hundred units or more away, and the knob's ±95 was sized for
// one. But fits run from 5 to 300, and for anything fitted nearer than the
// zoom reaches — a torus, a polyhedron, the maps, the flat planes — the same
// units walked the camera through the model and out the other side: the
// distance went negative, the model was drawn from behind the camera, and
// turning the zoom up made it a dot.
//
// So a near fit takes the zoom as a share of its own distance instead: at 95
// the camera is 95% of the way in, never past. At a fit of 100 and over the
// units are exactly what they were, so the links saved for those mean what
// they always did.
func zoomedDist(fit, zoom float32) float32 {
	if fit < 100 {
		zoom *= fit / 100
	}
	return fit - zoom
}
