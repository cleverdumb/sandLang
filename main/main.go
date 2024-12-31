package main

import (
	"fmt"
	"log"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"
	"unsafe"

	_ "image/png"

	"math/rand"

	"example.com/compile"
	"github.com/go-gl/gl/v2.1/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/vjeantet/govaluate"
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
	gw          = 100
	gh          = 100
	scrW        = 800
	scrH        = 800
	bw          = scrW / gw
	bh          = scrH / gh
	threadCount = 7
	symX        = 1 << 0
	symY        = 1 << 1
	updateDelay = 200 * time.Nanosecond
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
var aliasMap = make(map[string]string)

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
	window, err := glfw.CreateWindow(scrW, scrH, "Sandlang", nil, nil)
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
		if v.Alias != "" {
			aliasMap[v.Alias] = name
		}
	}

	for yi := uint16(0); yi < gh; yi++ {
		for xi := uint16(0); xi < gw; xi++ {
			if yi < gh-1 {
				grid[yi][xi] = *makeCell(xi, yi, revIdMap["Empty"])
			} else {
				grid[yi][xi] = *makeCell(xi, yi, revIdMap["Dirt"])
			}
			// 	grid[yi][xi] = *makeCell(xi, yi, 0)
			// }
			// grid[yi][xi] = *makeCell(xi, yi, 0)
		}
	}

	// changeType(int(gw/2), 1, revIdMap["Seed"])
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

// var testUpdateX, testUpdateY int

func click(w *glfw.Window, button glfw.MouseButton, action glfw.Action, mod glfw.ModifierKey) {
	if button == glfw.MouseButton1 && action == glfw.Press {
		newT := uint8(0)
		switch mod {
		case glfw.ModControl:
			newT = revIdMap["WarmGas"]
		case glfw.ModShift:
			newT = revIdMap["HotGas"]
		default:
			newT = revIdMap["Gas"]
		}
		posX, posY := w.GetCursorPos()
		boxX, boxY := int(posX/bw), int(posY/bh)

		// testUpdateX = boxX
		// testUpdateY = boxY

		// fmt.Printf("Cell %+v\n", grid[boxY][boxX])
		for y := -5; y <= 5; y++ {
			for x := -5; x <= 5; x++ {
				// for y := 0; y < 1; y++ {
				// 	for x := 0; x < 1; x++ {
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
			// s := time.Now()
			var rx, ry int
			// if testUpdateX == -1 {
			rx, ry = rand.Intn(gw), rand.Intn(gh)
			// 	continue outside
			// } else {
			// rx, ry = testUpdateX, testUpdateY
			// testUpdateX, testUpdateY = -1, -1
			// }
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

				// if len(ref.Rules) > 0 {
				for _, v := range rand.Perm(len(ref.Rules)) {
					// ind := rand.Intn(len(ref.Rules))
					ind := v
					// fmt.Println(ind)
					rule := ref.Rules[ind]
					// for ind, rule := range ref.Rules {
					if rand.Float64() > rule.Prob {
						if !rule.DontBreak {
							break
						} else {
							continue
						}
					}
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
						// fmt.Println("YSym")
						oy = ry - (int(rule.H) - int(rule.Oy) - 1)
						s |= symY
					}

					// fmt.Println(rule.XSym, rule.YSym, rand.Intn(2))

					// fmt.Printf("ox: %v, oy: %v, s: %v\n", ox, oy, s)

					// sx, sy := rule.XSym && rand.Intn(2) == 0, rule.YSym && rand.Intn(2) == 0
					if !matchRule(ref, ox, oy, ind, s) {
						ruleApply = false
						if !rule.DontBreak {
							break
						} else {
							continue
						}
					}

					// fmt.Println(ref.ConstProp)

					for _, con := range rule.MatchCon {
						res := evaluateMath(con.Expr, con.Names, s, rx, ry)

						// fmt.Println(res)

						if res == false {
							ruleApply = false
							break
						}
					}

					if !ruleApply {
						if !rule.DontBreak {
							break
						} else {
							continue
						}
					}

					// fmt.Println("ruleApply:", ruleApply)

					if ruleApply {
						// fmt.Println(ind)
						doSteps(rule, ox, oy, s, rx, ry)
						// if _, ok := grid[ry-1][rx].prop["lifetime"]; ok {
						// grid[ry-1][rx].prop["lifetime"] = grid[ry][rx].prop["lifetime"] + 1
						// }
					}

					if !rule.DontBreak {
						break
					}
				}
				// fmt.Println(ind, ruleApply)
				// }
			}

			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if zx+dx >= 0 && zx+dx < (gw/10) && zy+dy >= 0 && zy+dy < (gh/10) {
						zones[zy+dy][zx+dx].Unlock()
					}
				}
			}
			// fmt.Println(time.Since(s))

			time.Sleep(updateDelay)
			// break outside
		}
	}
}

func changeType(x, y int, newT uint8) {
	name := idMap[newT]
	grid[y][x].t = newT
	grid[y][x].prop = make(map[string]float32)
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
		if s&symY == symY {
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
					if !(slices.Contains(v, idMap[grid[tarY][tarX].t]) || slices.Contains(v, "^"+atoms[idMap[grid[tarY][tarX].t]].Alias)) {
						matching = false
						break out
					}
				} else if a, ok := aliasMap[cellRule]; ok {
					if idMap[grid[tarY][tarX].t] != a {
						matching = false
					}
				} else {
					matching = false
				}
			}
		}
	}
	return matching
}

func doSteps(rule compile.Rule, ox, oy int, s int, rx, ry int) {
	// fmt.Println("o", ox, oy)
	steps := rule.Steps
	localSymbols := make(map[string]cell)

	for _, step := range steps {
		var cx, cy int
		if step.Opcode == 5 || step.Opcode == 1 {
			if s&symX == 0 {
				cx = int(step.Operand[0])
			} else {
				cx = int(rule.W) - int(step.Operand[0]) - 1
			}

			if s&symY == 0 {
				cy = int(step.Operand[1])
			} else {
				cy = int(rule.H) - int(step.Operand[1]) - 1
			}
		}
		switch step.Opcode {
		case 5:
			sym := step.Name[0]
			localSymbols[sym] = grid[oy+cy][ox+cx]
			// fmt.Printf("c %v, %v localSymbols %+v\n", cx, cy, localSymbols)
		case 4:
			// fmt.Println("APPLY", tx, ty)
			applyPattern(rule, ox, oy, localSymbols, s)
		case 1:
			name := strings.Split(step.Name[0], "-")[0]
			// nx, ny := step.Operand[0], step.Operand[1]
			val := evaluateMath(step.Eval, step.Vars, s, rx, ry)
			// fmt.Println(val)
			// fmt.Println(ox+cx, oy+cy, "cxy", cx, cy)
			grid[ry+int(step.Operand[1])*(1-((s&symY)>>1)*2)][rx+int(step.Operand[0])*(1-(s&symX)*2)].prop[name] = float32(val.(float64))
		}
	}
}

func evaluateMath(expr *govaluate.EvaluableExpression, vars map[string][][2]int, s int, rx, ry int) interface{} {
	// ox, oy absolute position of symbol x
	param := make(map[string]interface{})
	inc := make(map[string]int)
	// fmt.Println("vars", vars)
	for n, l := range vars {
		tx, ty := rx, ry
		// if !(l[inc[n]][0] == -1 && l[inc[n]][1] == -1) {
		tx += l[inc[n]][0] * -((s&symX)*2 - 1)
		ty += l[inc[n]][1] * -(((s&symY)>>1)*2 - 1)
		// }
		if tx < 0 || ty < 0 || tx >= gw || ty >= gh {
			return false
		}
		name := strings.Split(n, "-")[0]
		// fmt.Println("name", name, "txy", tx, ty, "rxy", rx, ry, n, grid[ty][tx], "expr", expr)
		// fmt.Println("l", l)
		if v, ok := grid[ty][tx].prop[name]; ok {
			param[n] = float64(v)
		} else if v, ok := atoms[idMap[grid[ty][tx].t]].ConstProp[name]; ok {
			param[n] = float64(v)
		}
	}
	// fmt.Println("param", param)
	res, err := expr.Evaluate(param)
	if err != nil {
		panic(err)
	}
	return res
}

func transfer(from cell, tx, ty int) {
	grid[ty][tx].t = from.t
	grid[ty][tx].prop = make(map[string]float32)
	for n, v := range from.prop {
		grid[ty][tx].prop[n] = v
	}
}

func applyPattern(rule compile.Rule, ox, oy int, symbols map[string]cell, s int) {
	// fmt.Println("s", s)
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
		// fmt.Println(s & symY)
		if s&symY == symY {
			// fmt.Println("Marker")
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
				} else if a, ok := aliasMap[cellRule]; ok {
					changeType(tx, ty, revIdMap[a])
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
