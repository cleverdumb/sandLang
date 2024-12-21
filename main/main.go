package main

import (
	"fmt"
	"log"
	"math/rand"
	"runtime"
	"unsafe"

	_ "image/png"

	"example.com/compile"
	"github.com/go-gl/gl/v2.1/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
)

// Vertex Shader
var vertexShaderSource = `
#version 410
layout(location = 0) in vec2 position;
layout(location = 1) in vec2 texCoord;

out vec2 TexCoord;

void main() {
    gl_Position = vec4(position, 0.0, 1.0);
    TexCoord = texCoord;
}
` + "\x00"

// Fragment Shader
var fragmentShaderSource = `
#version 410
in vec2 TexCoord;
out vec4 color;

uniform sampler2D ourTexture;

void main() {
    color = texture(ourTexture, TexCoord);
}
` + "\x00"

const (
	gw   = 200
	gh   = 200
	scrW = 800
	scrH = 800
	bw   = scrW / gw
	bh   = scrH / gh
)

// Vertex Data for Full-Screen Quad
// var quadVertices = []float32{
// 	// Positions   // Texture Coords
// 	-1.0, 1.0, 0.0, 1.0, // Top-left
// 	-1.0, -1.0, 0.0, 0.0, // Bottom-left
// 	1.0, -1.0, 1.0, 0.0, // Bottom-right

// 	-1.0, 1.0, 0.0, 1.0, // Top-left
// 	1.0, -1.0, 1.0, 0.0, // Bottom-right
// 	1.0, 1.0, 1.0, 1.0, // Top-right
// }

var quadVertices = []float32{
	-1, 1, 0, 0,
	-1, 1 - float32(bh)/float32(scrH)*2, 0, 1,
	-1 + float32(bw)/float32(scrW)*2, 1 - float32(bh)/float32(scrH)*2, 1, 1,

	-1, 1, 0, 0,
	-1 + float32(bw)/float32(scrW)*2, 1, 1, 0,
	-1 + float32(bw)/float32(scrW)*2, 1 - float32(bh)/float32(scrH)*2, 1, 1,
}

var program uint32
var grid [gh][gw]cell

var texMap = make(map[uint8]uint32)

var atoms = make(map[string]*compile.AtomRef)
var idMap = make(map[uint8]string)

type cell struct {
	x uint16
	y uint16
	t uint8

	vao uint32
}

func init() {
	// Lock OS thread to ensure OpenGL context works
	runtime.LockOSThread()
}

func main() {
	compile.CompileScript(false)
	atoms = compile.Atoms
	compile.LogAtoms(atoms)
	// Initialize GLFW
	if err := glfw.Init(); err != nil {
		log.Fatalln("failed to initialize glfw:", err)
	}
	defer glfw.Terminate()

	// Configure GLFW
	glfw.WindowHint(glfw.ContextVersionMajor, 4)
	glfw.WindowHint(glfw.ContextVersionMinor, 1)
	glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)
	glfw.WindowHint(glfw.OpenGLForwardCompatible, glfw.True)

	// Create GLFW Window
	window, err := glfw.CreateWindow(800, 800, "Clear Screen with Texture", nil, nil)
	if err != nil {
		panic(err)
	}
	window.MakeContextCurrent()

	// Initialize OpenGL
	if err := gl.Init(); err != nil {
		panic(err)
	}
	// fmt.Println("OpenGL version", gl.GoStr(gl.GetString(gl.VERSION)))

	// Compile shaders and create program
	vertexShader, err := compileShader(vertexShaderSource, gl.VERTEX_SHADER)
	if err != nil {
		panic(err)
	}
	fragmentShader, err := compileShader(fragmentShaderSource, gl.FRAGMENT_SHADER)
	if err != nil {
		panic(err)
	}

	program = gl.CreateProgram()
	gl.AttachShader(program, vertexShader)
	gl.AttachShader(program, fragmentShader)
	gl.LinkProgram(program)
	gl.UseProgram(program)

	for yi := uint16(0); yi < gh; yi++ {
		for xi := uint16(0); xi < gw; xi++ {
			if rand.Intn(2) == 0 {
				grid[yi][xi] = *makeCell(xi, yi, 0)
			} else {
				grid[yi][xi] = *makeCell(xi, yi, 1)
			}
		}
	}

	for name, v := range atoms {
		texMap[v.Id] = generateColorTexture(v.Color.R, v.Color.G, v.Color.B)
		idMap[v.Id] = name
	}

	// Render Loop
	for !window.ShouldClose() {
		// Clear screen and draw the texture
		// s := time.Now()
		gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)
		drawAll()
		window.SwapBuffers()
		glfw.PollEvents()
		// fmt.Println(time.Since(s))
		// time.Sleep(1000 / 60 * time.Millisecond)
	}
}

func makeCell(x, y uint16, t uint8) *cell {
	return &cell{
		x:   x,
		y:   y,
		t:   t,
		vao: genVao(x, y),
	}
}

func drawAll() {
	for yi := range grid {
		for xi := range grid[yi] {
			grid[yi][xi].drawCell()
		}
	}
}

func genVao(x, y uint16) uint32 {
	points := make([]float32, len(quadVertices))
	copy(points, quadVertices)

	for i := range points {
		switch i % 4 {
		case 0:
			points[i] += float32(bw) / float32(scrW) * 2 * float32(x)
		case 1:
			points[i] -= float32(bw) / float32(scrW) * 2 * float32(y)
		default:
			continue
		}
	}

	// Create VAO and VBO for the full-screen quad
	var vao, vbo uint32
	gl.GenVertexArrays(1, &vao)
	gl.GenBuffers(1, &vbo)

	gl.BindVertexArray(vao)
	gl.BindBuffer(gl.ARRAY_BUFFER, vbo)
	gl.BufferData(gl.ARRAY_BUFFER, len(points)*4, gl.Ptr(points), gl.STATIC_DRAW)

	// Configure vertex attributes
	gl.VertexAttribPointer(0, 2, gl.FLOAT, false, 4*4, gl.PtrOffset(0))
	gl.EnableVertexAttribArray(0)
	gl.VertexAttribPointer(1, 2, gl.FLOAT, false, 4*4, gl.PtrOffset(2*4))
	gl.EnableVertexAttribArray(1)

	return vao
}

func draw(textureID, vao uint32) {
	// gl.UseProgram(program)
	// gl.ActiveTexture(gl.TEXTURE0)
	gl.BindTexture(gl.TEXTURE_2D, textureID)
	// gl.Uniform1i(gl.GetUniformLocation(program, gl.Str("ourTexture\x00")), 0)

	gl.BindVertexArray(vao)
	gl.DrawArrays(gl.TRIANGLES, 0, 6)
}

func (c *cell) drawCell() {
	if atoms[idMap[c.t]].Prop["render"] == 1 {
		draw(texMap[c.t], c.vao)
	}
}

// Generate a simple 1x1 color texture
func generateColorTexture(r, g, b uint8) uint32 {
	colorData := []uint8{r, g, b, 255} // RGBA color

	var texture uint32
	gl.GenTextures(1, &texture)
	gl.BindTexture(gl.TEXTURE_2D, texture)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA, 1, 1, 0, gl.RGBA, gl.UNSIGNED_BYTE, unsafe.Pointer(&colorData[0]))

	// Set texture parameters
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)

	return texture
}

// CompileShader compiles a shader from source code
func compileShader(source string, shaderType uint32) (uint32, error) {
	shader := gl.CreateShader(shaderType)

	csources, free := gl.Strs(source)
	gl.ShaderSource(shader, 1, csources, nil)
	free()
	gl.CompileShader(shader)

	var status int32
	gl.GetShaderiv(shader, gl.COMPILE_STATUS, &status)
	if status == gl.FALSE {
		var logLength int32
		gl.GetShaderiv(shader, gl.INFO_LOG_LENGTH, &logLength)

		log := make([]byte, logLength+1)
		gl.GetShaderInfoLog(shader, logLength, nil, &log[0])

		return 0, fmt.Errorf("failed to compile %v: %v", source, string(log))
	}

	return shader, nil
}
