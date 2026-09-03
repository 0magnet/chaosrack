// Renderer type + WebGL pipeline (shader compile/link, buffer
// uploads, per-frame draw). Shader source strings live here too since
// they are tightly coupled to the renderer's attribute/uniform
// expectations.
package stlview

import (
	"syscall/js"
	u "unsafe"

	"github.com/go-gl/mathgl/mgl32"
)

const gul = "getUniformLocation"

const vertShaderCode = `
attribute vec3 position;
uniform mat4 Pmatrix;
uniform mat4 Vmatrix;
uniform mat4 Mmatrix;
attribute vec3 color;
varying vec3 vColor;

void main(void) {
	gl_Position = Pmatrix*Vmatrix*Mmatrix*vec4(position, 1.);
	vColor = color;
}
`

const fragShaderCode = `
precision mediump float;
varying vec3 vColor;
void main(void) {
	gl_FragColor = vec4(vColor, 1.);
}
`

const fragShaderCode1 = `
precision mediump float;
uniform vec3 uBaseColor; // Color value at the base
uniform vec3 uTopColor;  // Color value at the top
varying vec3 vPosition;  // Interpolated vertex position
void main(void) {
	float t = (vPosition.y + 1.0) * 0.5; // Normalize the y-coordinate to [0, 1]
	vec3 rainbowColor = mix(uBaseColor, uTopColor, t);
	gl_FragColor = vec4(rainbowColor, 1.0);
}
`

const vertShaderCode1 = `
	attribute vec3 position;
	uniform mat4 Pmatrix;
	uniform mat4 Vmatrix;
	uniform mat4 Mmatrix;
	varying vec3 vPosition;  // Pass vertex position to fragment shader
	void main(void) {
		gl_Position = Pmatrix * Vmatrix * Mmatrix * vec4(position, 1.0);
		vPosition = position;  // Pass vertex position to fragment shader
	}
	`

// InitialConfig is the initial config
type InitialConfig struct {
	W        int
	H        int
	X        float32
	Y        float32
	Z        float32
	Colors   []float32
	Vertices []float32
	Indices  []uint32
	FSC      string
	VSC      string
}

// Renderer is the renderer
type Renderer struct {
	glContext      js.Value
	glTypes        GLTypes
	colors         js.Value
	v              js.Value
	i              js.Value
	colorBuffer    js.Value
	vertexBuffer   js.Value
	indexBuffer    js.Value
	numIndices     int
	numVertices    int
	fragShader     js.Value
	vertShader     js.Value
	shaderProgram  js.Value
	tmark          float32
	rX             float32 //rotation X
	rY             float32
	rZ             float32
	movMatrix      mgl32.Mat4
	PositionMatrix js.Value
	ViewMatrix     js.Value
	ModelMatrix    js.Value
	height         int
	width          int
	sX             float32
	sY             float32
	sZ             float32
}

// NewRenderer returns a new renderer & error
func NewRenderer(gl js.Value, config InitialConfig) (r Renderer, err js.Value) {
	// Get some WebGL bindings
	r.glContext = gl
	err = r.glTypes.New(r.glContext)
	r.numIndices = len(config.Indices)
	r.numVertices = len(config.Vertices)
	r.movMatrix = mgl32.Ident4()
	r.width = config.W
	r.height = config.H

	r.sX = config.X
	r.sY = config.Y
	r.sZ = config.Z

	// Convert buffers to JS TypedArrays
	r.UpdateColorBuffer(config.Colors)
	r.UpdateVerticesBuffer(config.Vertices)
	r.UpdateIndicesBuffer(config.Indices)

	r.UpdateFragmentShader(config.FSC)
	r.UpdateVertexShader(config.VSC)
	r.updateShaderProgram()
	r.attachShaderProgram()

	r.setContextFlags()

	r.createMatrixes()
	r.EnableObject()
	return
}

// SetModel sets a new model
func (r *Renderer) SetModel(Colors []float32, Vertices []float32, Indices []uint32) {
	r.numIndices = len(Indices)
	r.UpdateColorBuffer(Colors)
	r.UpdateVerticesBuffer(Vertices)
	r.UpdateIndicesBuffer(Indices)
	r.EnableObject()
}

// Release releases the renderer
func (r *Renderer) Release() {
}

// EnableObject enables the object
func (r *Renderer) EnableObject() {
	r.glContext.Call("bindBuffer", r.glTypes.ElementArrayBuffer, r.indexBuffer)
}

// SetX set rotation x axis speed
func (r *Renderer) SetX(x float32) {
	r.sX = x
}

// SetY set rotation y axis speed
func (r *Renderer) SetY(y float32) {
	r.sY = y
}

// SetZ set rotation z axis speed
func (r *Renderer) SetZ(z float32) {
	r.sZ = z
}

// GetSpeed returns the rotation speeds
func (r *Renderer) GetSpeed() (x, y, z float32) {
	return r.sX, r.sY, r.sZ
}

// SetSize sets the size of the rendering
func (r *Renderer) SetSize(height, width int) {
	r.height = height
	r.width = width
}

func (r *Renderer) createMatrixes() {
	ratio := float32(r.width) / float32(r.height)
	//	fmt.Println("Renderer.createMatrixes")
	projMatrix := mgl32.Perspective(mgl32.DegToRad(45.0), ratio, 1, 100000.0)
	projMatrixBuffer := (*[16]float32)(u.Pointer(&projMatrix)) // nolint
	typedProjMatrixBuffer := S2TA((*projMatrixBuffer)[:])
	r.glContext.Call("uniformMatrix4fv", r.PositionMatrix, false, typedProjMatrixBuffer)

	viewMatrix := mgl32.LookAtV(mgl32.Vec3{3.0, 3.0, 3.0}, mgl32.Vec3{0.0, 0.0, 0.0}, mgl32.Vec3{0.0, 1.0, 0.0})
	viewMatrixBuffer := (*[16]float32)(u.Pointer(&viewMatrix)) // nolint
	typedViewMatrixBuffer := S2TA((*viewMatrixBuffer)[:])
	r.glContext.Call("uniformMatrix4fv", r.ViewMatrix, false, typedViewMatrixBuffer)
}

func (r *Renderer) setContextFlags() {
	r.glContext.Call("clearColor", 0.0, 0.0, 0.0, 0.0)    // Color the screen is cleared to
	r.glContext.Call("viewport", 0, 0, r.width, r.height) // Viewport size
	r.glContext.Call("depthFunc", r.glTypes.LEqual)
}

// UpdateFragmentShader Updates the Fragment Shader
func (r *Renderer) UpdateFragmentShader(shaderCode string) {
	r.fragShader = r.glContext.Call("createShader", r.glTypes.FragmentShader)
	r.glContext.Call("shaderSource", r.fragShader, shaderCode)
	r.glContext.Call("compileShader", r.fragShader)
}

// UpdateVertexShader updates the vertex shader
func (r *Renderer) UpdateVertexShader(shaderCode string) {
	r.vertShader = r.glContext.Call("createShader", r.glTypes.VertexShader)
	r.glContext.Call("shaderSource", r.vertShader, shaderCode)
	r.glContext.Call("compileShader", r.vertShader)
}

func (r *Renderer) updateShaderProgram() {
	if r.fragShader.IsUndefined() || r.vertShader.IsUndefined() {
		return
	}
	r.shaderProgram = r.glContext.Call("createProgram")
	r.glContext.Call("attachShader", r.shaderProgram, r.vertShader)
	r.glContext.Call("attachShader", r.shaderProgram, r.fragShader)
	r.glContext.Call("linkProgram", r.shaderProgram)
}

func (r *Renderer) attachShaderProgram() {
	r.PositionMatrix = r.glContext.Call(gul, r.shaderProgram, "Pmatrix")
	r.ViewMatrix = r.glContext.Call(gul, r.shaderProgram, "Vmatrix")
	r.ModelMatrix = r.glContext.Call(gul, r.shaderProgram, "Mmatrix")

	r.glContext.Call("bindBuffer", r.glTypes.ArrayBuffer, r.vertexBuffer)
	position := r.glContext.Call("getAttribLocation", r.shaderProgram, "position")
	r.glContext.Call("vertexAttribPointer", position, 3, r.glTypes.Float, false, 0, 0)
	r.glContext.Call("enableVertexAttribArray", position)

	r.glContext.Call("bindBuffer", r.glTypes.ArrayBuffer, r.colorBuffer)
	color := r.glContext.Call("getAttribLocation", r.shaderProgram, "color")
	r.glContext.Call("vertexAttribPointer", color, 3, r.glTypes.Float, false, 0, 0)
	r.glContext.Call("enableVertexAttribArray", color)

	r.glContext.Call("useProgram", r.shaderProgram)
	if stlFileName == ".stl" || stlFileName == "" {

		uBaseColor := r.glContext.Call(gul, r.shaderProgram, "uBaseColor")
		uTopColor := r.glContext.Call(gul, r.shaderProgram, "uTopColor")
		uColor := r.glContext.Call(gul, r.shaderProgram, "uColor")
		r.glContext.Call("uniform3f", uBaseColor, 1.0, 0.0, 0.0)
		r.glContext.Call("uniform3f", uTopColor, 0.0, 0.0, 1.0)
		r.glContext.Call("uniform3f", uColor, 1.0, 1.0, 1.0)
	}
}

// UpdateColorBuffer Updates the ColorBuffer
func (r *Renderer) UpdateColorBuffer(buffer []float32) {
	r.colors = S2TA(buffer)
	if r.colorBuffer.IsUndefined() {
		r.colorBuffer = r.glContext.Call("createBuffer")
	}
	r.glContext.Call("bindBuffer", r.glTypes.ArrayBuffer, r.colorBuffer)
	r.glContext.Call("bufferData", r.glTypes.ArrayBuffer, r.colors, r.glTypes.StaticDraw)
}

// UpdateVerticesBuffer Updates the VerticesBuffer
func (r *Renderer) UpdateVerticesBuffer(buffer []float32) {
	r.v = S2TA(buffer)
	if r.vertexBuffer.IsUndefined() {
		r.vertexBuffer = r.glContext.Call("createBuffer")
	}
	r.glContext.Call("bindBuffer", r.glTypes.ArrayBuffer, r.vertexBuffer)
	r.glContext.Call("bufferData", r.glTypes.ArrayBuffer, r.v, r.glTypes.StaticDraw)
}

// UpdateIndicesBuffer Updates the IndicesBuffer
func (r *Renderer) UpdateIndicesBuffer(buffer []uint32) {
	r.i = S2TA(buffer)
	if r.indexBuffer.IsUndefined() {
		r.indexBuffer = r.glContext.Call("createBuffer")
	}
	r.glContext.Call("bindBuffer", r.glTypes.ElementArrayBuffer, r.indexBuffer)
	r.glContext.Call("bufferData", r.glTypes.ElementArrayBuffer, r.i, r.glTypes.StaticDraw)
}

// Render renders
func (r *Renderer) Render(_ js.Value, args []js.Value) interface{} { // nolint
	now := float32(args[0].Float())
	tdiff := now - r.tmark
	r.tmark = now
	r.rX = r.rX + r.sX*float32(tdiff)/500
	r.rY = r.rY + r.sY*float32(tdiff)/500
	r.rZ = r.rZ + r.sZ*float32(tdiff)/500

	r.movMatrix = mgl32.HomogRotate3DX(r.rX)
	r.movMatrix = r.movMatrix.Mul4(mgl32.HomogRotate3DY(r.rY))
	r.movMatrix = r.movMatrix.Mul4(mgl32.HomogRotate3DZ(r.rZ))

	modelMatrixBuffer := (*[16]float32)(u.Pointer(&r.movMatrix)) // nolint
	typedModelMatrixBuffer := S2TA((*modelMatrixBuffer)[:])

	r.glContext.Call("uniformMatrix4fv", r.ModelMatrix, false, typedModelMatrixBuffer)

	r.glContext.Call("enable", r.glTypes.DepthTest)
	r.glContext.Call("clear", r.glTypes.ColorBufferBit)
	r.glContext.Call("clear", r.glTypes.DepthBufferBit)
	usegltype := r.glTypes.Triangles
	if stlFileName == ".stl" || stlFileName == "" {
		usegltype = r.glTypes.Line
		r.glContext.Call("drawArrays", r.glTypes.LineLoop, 0, r.numVertices/3)
	}
	r.glContext.Call("drawElements", usegltype, r.numIndices, r.glTypes.UnsignedInt, 0)

	return nil
}

// SetZoom Sets the Zoom
func (r *Renderer) SetZoom(currentZoom float32) {
	viewMatrix := mgl32.LookAtV(mgl32.Vec3{currentZoom, currentZoom, currentZoom}, mgl32.Vec3{0.0, 0.0, 0.0}, mgl32.Vec3{0.0, 1.0, 0.0})
	viewMatrixBuffer := (*[16]float32)(u.Pointer(&viewMatrix)) // nolint
	typedViewMatrixBuffer := S2TA((*viewMatrixBuffer)[:])
	r.glContext.Call("uniformMatrix4fv", r.ViewMatrix, false, typedViewMatrixBuffer)
}
