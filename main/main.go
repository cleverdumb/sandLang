package main

import (
	"fmt"
	"log"
	"runtime"
	"slices"
	"sync"
	"time"
	"unsafe"

	_ "image/png"

	"math/rand"

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
	gw          = 200
	gh          = 200
	scrW        = 800
	scrH        = 800
	bw          = scrW / gw
	bh          = scrH / gh
	threadCount = 7
	symX        = 1 << 0
	symY        = 1 << 1
)

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
var revIdMap = make(map[string]uint8)

var zones [gh / 10][gw / 10]sync.Mutex

type cell struct {
	x uint16
	y uint16
	t uint8

	vao uint32

	prop map[string]float32

	// mutex *sync.Mutex
}

func init() {
	// Lock OS thread to ensure OpenGL context works
	runtime.LockOSThread()
}

func main() {
	// s := time.Now()
	compile.CompileScript(false)
	atoms = compile.Atoms
	compile.LogAtoms(atoms)
	// fmt.Println(time.Since(s))
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

	for name, v := range atoms {
		texMap[v.Id] = generateColorTexture(v.Color.R, v.Color.G, v.Color.B)
		idMap[v.Id] = name
		revIdMap[name] = v.Id
	}

	for yi := uint16(0); yi < gh; yi++ {
		for xi := uint16(0); xi < gw; xi++ {
			// if yi < gh/4 {
			// 	grid[yi][xi] = *makeCell(xi, yi, 1)
			// } else if yi < gh/2 {
			// 	grid[yi][xi] = *makeCell(xi, yi, 2)
			// } else {
			// 	grid[yi][xi] = *makeCell(xi, yi, 0)
			// }
			grid[yi][xi] = *makeCell(xi, yi, 0)
		}
	}

	// changeType(4, 4, 1)
	// changeType(4, 5, 1)

	// fmt.Println(grid[4][4])

	quitCh := make(chan uint8)

	for x := 0; x < threadCount; x++ {
		go updateThread(quitCh)
	}

	window.SetMouseButtonCallback(click)

	// Render Loop
	for !window.ShouldClose() {
		// Clear screen and draw the texture
		// s := time.Now()
		gl.Clear(gl.COLOR_BUFFER_BIT)
		drawAll()
		window.SwapBuffers()
		glfw.PollEvents()
		// fmt.Println(time.Since(s))
		// time.Sleep(1000 / 60 * time.Millisecond)
	}

	quitCh <- 1
}

func click(w *glfw.Window, button glfw.MouseButton, action glfw.Action, mod glfw.ModifierKey) {
	if button == glfw.MouseButton1 && action == glfw.Press {
		newT := uint8(0)
		switch mod {
		case glfw.ModControl:
			newT = revIdMap["Water"]
		default:
			newT = revIdMap["Sand"]
		}
		posX, posY := w.GetCursorPos()
		boxX, boxY := int(posX/bw), int(posY/bh)
		for y := -10; y <= 10; y++ {
			for x := -10; x <= 10; x++ {
				if boxX+x >= 0 && boxX+x < gw && boxY+y >= 0 && boxY+y < gh {
					if grid[boxY+y][boxX+x].t == revIdMap["Empty"] {
						changeType(boxX+x, boxY+y, newT)
					}
				}
			}
		}
	}
}

func updateThread(quit chan uint8) {
outside:
	for {
		select {
		case <-quit:
			break outside
		default:
			rx, ry := rand.Intn(gw), rand.Intn(gh)
			// rx, ry := 4, 4
			zx, zy := int(rx/10), int(ry/10)

			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if zx+dx >= 0 && zx+dx < (gw/10) && zy+dy >= 0 && zy+dy < (gh/10) {
						zones[zy+dy][zx+dx].Lock()
					}
				}
			}

			if name, ok := idMap[grid[ry][rx].t]; ok {
				ref := *atoms[name]

				for ind, rule := range ref.Rules {
					ruleApply := true
					// s := 0
					// if rule.XSym && rand.Intn(2) == 0 {
					// 	s |= symX
					// }
					// if rule.YSym && rand.Intn(2) == 0 {
					// 	s |= symY
					// }

					var ox, oy int
					s := 0
					if !(rule.XSym && rand.Intn(2) == 0) {
						ox = rx - int(rule.Ox)
					} else {
						// ox = rx + int(rule.Ox) - int(rule.W)
						ox = rx - (int(rule.W) - int(rule.Ox) - 1)
						s |= symX
					}

					if !(rule.YSym && rand.Intn(2) == 0) {
						oy = ry - int(rule.Oy)
					} else {
						oy = ry - (int(rule.H) - int(rule.Oy) - 1)
						s |= symY
					}

					// fmt.Println(rule.XSym, rule.YSym, rand.Intn(2))

					// fmt.Printf("ox: %v, oy: %v, s: %v\n", ox, oy, s)

					// sx, sy := rule.XSym && rand.Intn(2) == 0, rule.YSym && rand.Intn(2) == 0
					if !matchRule(ref, ox, oy, ind, s) {
						ruleApply = false
					}

					// fmt.Println("ruleApply: ", ruleApply)

					if ruleApply {
						doSteps(rule, ox, oy, s)
					}

					// fmt.Println(ind, ruleApply)
				}
			}

			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if zx+dx >= 0 && zx+dx < (gw/10) && zy+dy >= 0 && zy+dy < (gh/10) {
						zones[zy+dy][zx+dx].Unlock()
					}
				}
			}

			time.Sleep(10 * time.Nanosecond)
			// break outside
		}
	}
}

func changeType(x, y int, newT uint8) {
	name := idMap[newT]
	grid[y][x].t = newT
	for n, val := range atoms[name].Prop {
		// if _, ok := grid[y][x].prop[n]; !ok {
		grid[y][x].prop[n] = val
		// }
	}
}

func matchRule(atom compile.AtomRef, ox, oy int, ruleIndex int, s int) bool {
	r := atom.Rules[ruleIndex]

	// fmt.Println(s&symX, s&symY)
	// ox, oy := rx-int(r.Ox), ry-int(r.Oy)
	matching := true

out:
	for dy := 0; dy < int(r.H); dy++ {
		var ruleY int
		if s&symY == 1 {
			ruleY = int(r.H) - dy - 1
		} else {
			ruleY = dy
		}
		for dx := 0; dx < int(r.W); dx++ {
			tarX, tarY := ox+dx, oy+dy
			var ruleX int
			if s&symX == 1 {
				ruleX = int(r.W) - dx - 1
			} else {
				ruleX = dx
			}
			cellRule := r.Match[ruleY*int(r.W)+ruleX]
			// fmt.Println(cellRule)
			outside := false
			if tarX < 0 || tarX >= gw || tarY < 0 || tarY >= gh {
				outside = true
			}
			if outside {
				if cellRule != "e" {
					matching = false
				}
				break out
			}
			switch cellRule {
			case "e":
				if !outside {
					matching = false
					break out
				}
			case "*":
				if outside {
					matching = false
					break out
				}
			case "x":
				continue
			case "_":
				if !outside {
					if idMap[grid[tarY][tarX].t] != "Empty" {
						matching = false
						break out
					}
				}
			case "n":
				if !outside {
					if idMap[grid[tarY][tarX].t] == "Empty" {
						matching = false
						break out
					}
				}
			default:
				if v, ok := atom.Def[cellRule]; ok {
					if !slices.Contains(v, idMap[grid[tarY][tarX].t]) {
						matching = false
						break out
					}
				} else {
					matching = false
					break out
				}
			}
		}
	}
	return matching
}

func doSteps(rule compile.Rule, ox, oy int, s int) {
	// fmt.Println("o", ox, oy)
	steps := rule.Steps
	localSymbols := make(map[string]cell)

	for _, step := range steps {
		switch step.Opcode {
		case 5:
			sym := step.Name[0]
			var cx, cy int
			if s&symX == 0 {
				cx = step.Operand[0]
			} else {
				cx = int(rule.W) - int(step.Operand[0]) - 1
			}

			if s&symY == 0 {
				cy = step.Operand[1]
			} else {
				cy = int(rule.H) - int(step.Operand[1]) - 1
			}
			localSymbols[sym] = grid[oy+cy][ox+cx]
			// fmt.Printf("c %v, %v localSymbols %+v\n", cx, cy, localSymbols)
		case 4:
			// fmt.Println("APPLY", tx, ty)
			applyPattern(rule, ox, oy, localSymbols, s)
		}
	}
}

func transfer(from cell, tx, ty int) {
	grid[ty][tx].t = from.t
	for n, v := range from.prop {
		grid[ty][tx].prop[n] = v
	}
}

func applyPattern(rule compile.Rule, ox, oy int, symbols map[string]cell, s int) {
	// fmt.Println(s)
	var cenX, cenY int
	if s&symX == 0 {
		cenX = int(rule.Ox)
	} else {
		cenX = (int(rule.W) - int(rule.Ox) - 1)
	}

	if s&symY == 0 {
		cenY = int(rule.Oy)
	} else {
		cenY = (int(rule.H) - int(rule.Oy) - 1)
	}
	// (int(rule.H)-int(rule.Oy)-1)
	tempCentre := grid[oy+cenY][ox+cenX]
	// fmt.Println(tempCentre)
	// ox, oy := tarX-int(rule.Ox), tarY-int(rule.Oy)
	// transfer(tarX, tarY, int(rule.Ox), int(rule.Oy))
	for dy := 0; dy < int(rule.H); dy++ {
		var ruleY int
		if s&symY == 1 {
			ruleY = int(rule.H) - dy - 1
		} else {
			ruleY = dy
		}

		for dx := 0; dx < int(rule.W); dx++ {
			tx, ty := ox+dx, oy+dy
			var ruleX int
			if s&symX == 1 {
				ruleX = int(rule.W) - dx - 1
			} else {
				ruleX = dx
			}

			// fmt.Println("rulePos", ruleX, ruleY)
			// fmt.Printf("tempCentre %+v\n", tempCentre)

			cellRule := rule.Pat[ruleY*int(rule.W)+ruleX]

			// fmt.Println(cellRule, tx, ty)

			switch cellRule {
			case "/":
				continue
			case "x":
				// grid[ty][tx] = tempCentre
				transfer(tempCentre, tx, ty)
			case "_":
				// grid[ty][tx].t = revIdMap["Empty"]
				changeType(tx, ty, revIdMap["Empty"])
			default:
				if v, ok := symbols[cellRule]; ok {
					transfer(v, tx, ty)
				}
			}
		}
	}
	// fmt.Printf("FF: %+v\n", grid[4][4])
}

func makeCell(x, y uint16, t uint8) *cell {
	return &cell{
		x:    x,
		y:    y,
		t:    t,
		vao:  genVao(x, y),
		prop: make(map[string]float32),
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

func (c cell) drawCell() {
	_, ok := idMap[c.t]
	if ok {
		if atoms[idMap[c.t]].ConstProp["render"] == 1 {
			t, ok := texMap[c.t]
			if ok {
				draw(t, c.vao)
			}
		}
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
