//go:build js && wasm

package glctx

import "syscall/js"

// GLTypes holds WebGL constant values.
type GLTypes struct {
	StaticDraw         js.Value
	ArrayBuffer        js.Value
	ElementArrayBuffer js.Value
	VertexShader       js.Value
	FragmentShader     js.Value
	Float              js.Value
	DepthTest          js.Value
	ColorBufferBit     js.Value
	DepthBufferBit     js.Value
	Triangles          js.Value
	UnsignedShort      js.Value
	LEqual             js.Value
	LineLoop           js.Value
	Line               js.Value
	LineStrip          js.Value
	Lines              js.Value
	Points             js.Value
	DynamicDraw        js.Value
}

// New reads the GL constants off a context.
func (types *GLTypes) New(gl js.Value) {
	types.StaticDraw = gl.Get("STATIC_DRAW")
	types.ArrayBuffer = gl.Get("ARRAY_BUFFER")
	types.ElementArrayBuffer = gl.Get("ELEMENT_ARRAY_BUFFER")
	types.VertexShader = gl.Get("VERTEX_SHADER")
	types.FragmentShader = gl.Get("FRAGMENT_SHADER")
	types.Float = gl.Get("FLOAT")
	types.DepthTest = gl.Get("DEPTH_TEST")
	types.ColorBufferBit = gl.Get("COLOR_BUFFER_BIT")
	types.Triangles = gl.Get("TRIANGLES")
	types.UnsignedShort = gl.Get("UNSIGNED_SHORT")
	types.LEqual = gl.Get("LEQUAL")
	types.DepthBufferBit = gl.Get("DEPTH_BUFFER_BIT")
	types.LineLoop = gl.Get("LINE_LOOP")
	types.Line = gl.Get("LINES")
	types.LineStrip = gl.Get("LINE_STRIP")
	types.Lines = gl.Get("LINES")
	types.Points = gl.Get("POINTS")
	types.DynamicDraw = gl.Get("DYNAMIC_DRAW")
}

// updateGradientRange scans vertices and sets min/max uniforms for x, y, and z.
// Stride is gradientStride floats per vertex: 4 for interleaved
// attractor data (x,y,z,t), 3 for packed indexed geometry (x,y,z).
// Only called on mode/param change, NOT per frame.
