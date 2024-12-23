package compile

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

func checkErr(e error) {
	if e != nil {
		panic(e)
	}
}

var Atoms = make(map[string]*AtomRef)

type AtomRef struct {
	Id        uint8
	Color     Color
	Prop      map[string]float32
	ConstProp map[string]float32
	Def       map[string][]string
	Rules     []Rule
}

type Color struct {
	R uint8
	G uint8
	B uint8
}

type Rule struct {
	W     uint8
	H     uint8
	Ox    int8
	Oy    int8
	Match []string
	// matchCon []Condition
	Pat   []string
	Steps []Step
	Id    uint16
	XSym  bool
	YSym  bool
	Prob  float64
}

// opcode:
// 0 - set [name[0]] to [0]
// 1 - change [name[0]] by [0]
// 2 - clamp [name[0]] into [0] and [1] ([0] < [1])
// 3 - randomise [name[0]] between [0] and [1] ([0] < [1]), steps [2]
// 4 - map to pattern
// 5 - set symbol [name[0]] to cell at coord [1][0]
// 6 - remember names [name[0], name[1], ...] at coord [1][0]
type Step struct {
	Opcode  uint8
	Name    []string
	Operand []int
}

// type Condition struct {
// 	opcode  uint8
// 	name    string
// 	operand []int
// }

func CompileScript(log bool) map[string]*AtomRef {
	currAtomId := 0
	f, err := os.ReadFile("../script.txt")
	if err != nil {
		panic(err)
	}
	reg := make(map[string]*regexp.Regexp)
	reg["atom"] = regexp.MustCompile(`\s*atom\s+([A-Za-z0-9]+)\s*{`)
	reg["sectionName"] = regexp.MustCompile(`\s*section\s*([a-z]+)\s+{`)
	reg["anySpace"] = regexp.MustCompile(`\s+`)
	reg["colorRGB"] = regexp.MustCompile(`#([A-F0-9]{2})([A-F0-9]{2})([A-F0-9]{2})`)
	reg["splitSet"] = regexp.MustCompile(`,\s*`)
	reg["matchStatement"] = regexp.MustCompile(`\s*match\s+\((\d+)\s*,\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)\)\s*(sym\s*\(\s*[xy]+\s*\))?\s*{`)
	reg["spacedEqual"] = regexp.MustCompile(`\s*=\s*`)
	reg["pickCoord"] = regexp.MustCompile(`\((\d*),\s+(\d*)\)`)
	reg["fromSym"] = regexp.MustCompile(`sym\(([xy]*)\)`)
	reg["fromArrow"] = regexp.MustCompile(`->\s*(P\s*-\s*([\d\.]*))?\s*{`)
	inAtomDeclaration := false
	currentAtom := ""
	sections := map[string]bool{
		"property":   false,
		"definition": false,
		"update":     false,
	}
	// 0 - not in rule, 1 - in match phase, 2 - in effect phase
	inRule := 0
	var newRuleId uint16 = 0
	inPattern := false
	patternLineCount := 0
	newRule := Rule{}
outsideLoop:
	for lineNum, l := range strings.Split(string(f), "\n") {
		l = strings.TrimSpace(l)
		switch {
		case l == "":
			if log {
				fmt.Println(lineNum, "Empty line")
			}
			continue outsideLoop

		case strings.HasPrefix(l, "//"):
			continue outsideLoop

		case strings.HasPrefix(l, "atom"):
			name := reg["atom"].FindStringSubmatch(l)[1]
			Atoms[name] = &AtomRef{Id: uint8(currAtomId), Prop: make(map[string]float32), ConstProp: make(map[string]float32), Def: make(map[string][]string)}
			currentAtom = name
			inAtomDeclaration = true
			if log {
				fmt.Println(lineNum, "Start of atom dec:", name, currAtomId)
			}
			currAtomId++

		case l == "}":
			if inRule == 1 {
				inRule = 0
				inPattern = false
				if log {
					fmt.Printf("%v End of match phase\n", lineNum)
				}
				continue outsideLoop
			}
			if inRule == 2 {
				inRule = 0
				Atoms[currentAtom].Rules = append(Atoms[currentAtom].Rules, newRule)
				inPattern = false
				newRule = Rule{}
				if log {
					fmt.Printf("%v End of effect phase\n", lineNum)
				}
				newRuleId++
				continue outsideLoop
			}
			for k, v := range sections {
				if v {
					sections[k] = false
					if log {
						fmt.Println(lineNum, "End of section:", k)
					}
					continue outsideLoop
				}
			}
			if inAtomDeclaration {
				inAtomDeclaration = false
				if log {
					fmt.Println(lineNum, "End of atom dec:", currentAtom)
				}
				continue outsideLoop
			}

		case strings.HasPrefix(l, "section"):
			name := reg["sectionName"].FindStringSubmatch(l)[1]
			for k := range sections {
				sections[k] = false
			}
			sections[name] = true
			if log {
				fmt.Println(lineNum, "Start of section:", name)
			}

		case sections["property"] && (strings.HasPrefix(l, "cdef") || strings.HasPrefix(l, "def")):
			split := reg["anySpace"].Split(l, -1)
			n, v := split[1], split[2]
			if n == "color" {
				temp := reg["colorRGB"].FindStringSubmatch(v)
				r, err := strconv.ParseUint(temp[1], 16, 8)
				checkErr(err)

				g, err := strconv.ParseUint(temp[2], 16, 8)
				checkErr(err)

				b, err := strconv.ParseUint(temp[3], 16, 8)
				checkErr(err)

				Atoms[currentAtom].Color = Color{uint8(r), uint8(g), uint8(b)}
				if log {
					fmt.Println(lineNum, "Set property color of", currentAtom, "to (", r, g, b, ")")
				}
			} else {
				num, err := strconv.ParseFloat(v, 32)
				checkErr(err)
				if l[0] == 'c' {
					Atoms[currentAtom].ConstProp[n] = float32(num)
				} else {
					Atoms[currentAtom].Prop[n] = float32(num)
				}
				if log {
					fmt.Println(lineNum, "Set property", n, "of", currentAtom, "to", num)
				}
			}

		case sections["definition"] && strings.HasPrefix(l, "def"):
			split := reg["anySpace"].Split(l, -1)
			sym, set := split[1], strings.Join(split[2:], " ")
			comps := reg["splitSet"].Split(set[1:len(set)-1], -1)
			Atoms[currentAtom].Def[sym] = comps
			if log {
				fmt.Printf("%v Set definition %v of %v to %v\n", lineNum, sym, currentAtom, comps)
			}

		case strings.HasPrefix(l, "pattern"):
			inPattern = true
			if inRule != 0 {
				patternLineCount = int(newRule.H)
				if log {
					fmt.Printf("%v Start of pattern with height %v\n", lineNum, patternLineCount)
				}
				if inRule == 2 {
					newRule.Steps = append(newRule.Steps, Step{Opcode: 4})
				}
			}

		case sections["update"]:
			if inPattern && patternLineCount > 0 {
				split := reg["anySpace"].Split(l, int(newRule.W))
				if inRule == 1 {
					newRule.Match = append(newRule.Match, split...)
					patternLineCount--
					// fmt.Println(newRule.match)
				} else if inRule == 2 {
					newRule.Pat = append(newRule.Pat, split...)
					patternLineCount--
					// fmt.Println(newRule.pat)
				}
				continue outsideLoop
			}
			if inPattern && patternLineCount == 0 {
				inPattern = false
			}

			if inRule == 0 {
				if strings.HasPrefix(l, "match") {
					nums := reg["matchStatement"].FindStringSubmatch(l)[1:]
					ox, err := strconv.ParseInt(nums[0], 10, 8)
					checkErr(err)

					oy, err := strconv.ParseInt(nums[1], 10, 8)
					checkErr(err)

					w, err := strconv.ParseInt(nums[2], 10, 8)
					checkErr(err)

					h, err := strconv.ParseInt(nums[3], 10, 8)
					checkErr(err)

					newRule.W = uint8(w)
					newRule.H = uint8(h)
					newRule.Ox = int8(ox)
					newRule.Oy = int8(oy)
					newRule.Id = newRuleId

					// fmt.Println(nums[4])
					// fmt.Println(reg["fromSym"].FindStringSubmatch(nums[4]))
					if nums[4] != "" {
						switch reg["fromSym"].FindStringSubmatch(nums[4])[1] {
						case "xy":
							newRule.XSym = true
							newRule.YSym = true
						case "x":
							newRule.XSym = true
							newRule.YSym = false
						case "y":
							newRule.YSym = true
							newRule.XSym = false
						case "":
							newRule.XSym = false
							newRule.YSym = false
						}
					}

					inRule = 1
					if log {
						fmt.Printf("%v Start of match phase\n", lineNum)
					}
					continue outsideLoop
				} else if strings.HasPrefix(l, "->") {
					inRule = 2
					p := reg["fromArrow"].FindStringSubmatch(l)
					var prob float64
					if len(p) >= 3 && p[2] != "" {
						prob, err = strconv.ParseFloat(p[2], 64)
						if err != nil {
							panic(err)
						}
					} else {
						prob = 1
					}

					newRule.Prob = prob

					if log {
						fmt.Printf("%v Start of effect phase\n", lineNum)
					}
					continue outsideLoop
				}
			}

			if inRule == 2 {
				if strings.HasPrefix(l, "def") {
					split := reg["spacedEqual"].Split(l, 2)
					sym, val := split[0][4:], reg["pickCoord"].FindStringSubmatch(split[1])[1:]
					x, err := strconv.ParseInt(val[0], 10, 8)
					checkErr(err)

					y, err := strconv.ParseInt(val[1], 10, 8)
					checkErr(err)
					newRule.Steps = append(newRule.Steps, Step{Opcode: 5, Name: []string{sym}, Operand: []int{int(x), int(y)}})
					if log {
						fmt.Printf("%v Added step to define %v at coord (%v, %v)\n", lineNum, sym, x, y)
					}
				}
			}
		}
	}

	// for k, v := range Atoms {
	// 	fmt.Print(k)
	// 	fmt.Printf("%+v\n", *v)
	// }

	// LogAtoms(Atoms)

	return Atoms
}

func LogAtoms(atoms map[string]*AtomRef) {
	for k, v := range atoms {
		fmt.Print(k)
		fmt.Printf("%+v\n", *v)
	}
}
