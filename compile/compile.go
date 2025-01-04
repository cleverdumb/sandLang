package compile

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/vjeantet/govaluate"
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
	Key       rune
	Prop      map[string]float32
	ConstProp map[string]float32
	Def       map[string][]string
	Rules     []Rule
	Alias     string
	Init      []Step
}

type Color struct {
	R uint8
	G uint8
	B uint8
}

type Rule struct {
	W              uint8
	H              uint8
	Ox             int8
	Oy             int8
	Match          []string
	MatchCon       []Condition
	Pat            []string
	Steps          []Step
	Id             uint16
	XSym           bool
	YSym           bool
	Prob           float64
	DontBreak      bool
	NoMatchPattern bool
}

// opcode:
// 0 - set
// 1 - change [name[0]] by [0]
// 2 - clamp [name[0]] into [0] and [1] ([0] < [1])
// 3 - randomise [name[0]] between [0] and [1] ([0] < [1]), steps [2]
// 4 - map to pattern
// 5 - set symbol [name[0]] to cell at coord [1][0]
// 6 - remember names [name[0], name[1], ...] at coord [1][0]
type Step struct {
	Opcode   uint8
	Name     []string
	Operand  []float64
	Eval     *govaluate.EvaluableExpression
	Vars     map[string][][2]int
	RandVars map[string][3]float64
}

type Condition struct {
	Names    map[string][][2]int
	Expr     *govaluate.EvaluableExpression
	RandVars map[string][3]float64
}

var reg = make(map[string]*regexp.Regexp)

func CompileScript(log bool) map[string]*AtomRef {
	currAtomId := 0
	// f, err := os.ReadFile("../periodicTable/Density.txt")
	f, err := os.ReadFile("../script.txt")
	if err != nil {
		panic(err)
	}
	reg["atom"] = regexp.MustCompile(`\s*atom\s+([A-Za-z0-9]+)\s*(alias\s([A-Za-z0-9]+))?\s*{`)
	reg["sectionName"] = regexp.MustCompile(`\s*section\s*([a-z]+)\s+{`)
	reg["anySpace"] = regexp.MustCompile(`\s+`)
	reg["colorRGB"] = regexp.MustCompile(`#([A-F0-9]{2})([A-F0-9]{2})([A-F0-9]{2})`)
	reg["splitSet"] = regexp.MustCompile(`\s*,\s*`)
	reg["matchStatement"] = regexp.MustCompile(`\s*match\s+\((\d+)\s*,\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)\)\s*(sym\s*\(\s*[xy]+\s*\))?\s*{`)
	reg["spacedEqual"] = regexp.MustCompile(`\s*=\s*`)
	reg["pickCoord"] = regexp.MustCompile(`\((\d*),\s+(\d*)\)`)
	reg["fromSym"] = regexp.MustCompile(`sym\(([xy]*)\)`)
	reg["fromArrow"] = regexp.MustCompile(`->\s*(P\s*-\s*([\d\.]*))?\s*{`)
	reg["fromInherit"] = regexp.MustCompile(`inherit\s+([a-zA-Z0-9]*)\s*(.*)?`)
	reg["getEvalBracket"] = regexp.MustCompile(`\[([a-zA-Z0-9]*\s*(-\s*([0-9]+)\s*,\s*([0-9]+)\s*)?)\]`)
	reg["getRandomBracket"] = regexp.MustCompile(`\[\$([a-zA-Z0-9]+)'([\d\.]+)'([\d\.]+)'([\d\.]+)\]`)
	reg["modifyFlag"] = regexp.MustCompile(`-([a-zA-Z]+)=(.*)`)
	reg["fromRuleset"] = regexp.MustCompile(`ruleset\s+([a-zA-Z0-9]+)\s+{`)
	inAtomDeclaration := false
	currentAtom := ""
	inComment := false
	sections := map[string]bool{
		"property":   false,
		"definition": false,
		"update":     false,
		"init":       false,
	}
	// 0 - not in rule, 1 - in match phase, 2 - in effect phase
	inRule := 0
	newRuleId := uint16(0)
	inPattern := false
	patternLineCount := 0
	newRule := Rule{}
	globalSets := make(map[string][]string)
	globalRules := make(map[string][]Rule)
	currentGlobalRule := ""
outsideLoop:
	for lineNum, l := range strings.Split(string(f), "\n") {
		l = strings.TrimSpace(l)
		switch {
		case strings.HasPrefix(l, "*/"):
			inComment = false
		case strings.HasPrefix(l, "/*") || inComment:
			inComment = true
			continue outsideLoop
		case l == "":
			if log {
				fmt.Println(lineNum, "Empty line")
			}
			continue outsideLoop

		case strings.HasPrefix(l, "//"):
			continue outsideLoop

		case strings.HasPrefix(l, "global"):
			split := reg["anySpace"].Split(l, -1)
			sym, set := split[1], strings.Join(split[2:], " ")
			comps := reg["splitSet"].Split(set[1:len(set)-1], -1)
			globalSets[sym] = comps
			if log {
				fmt.Printf("%v Set global set %v to %v\n", lineNum, sym, comps)
			}

		case strings.HasPrefix(l, "atom"):
			matched := reg["atom"].FindStringSubmatch(l)
			name := matched[1]
			Atoms[name] = &AtomRef{Id: uint8(currAtomId), Prop: make(map[string]float32), ConstProp: make(map[string]float32), Def: make(map[string][]string), Key: ' '}
			if len(matched) >= 4 && matched[3] != "" {
				Atoms[name].Alias = matched[3]
			} else {
				Atoms[name].Alias = ""
			}
			currentAtom = name
			inAtomDeclaration = true
			for sym, a := range globalSets {
				Atoms[currentAtom].Def[sym] = a
			}
			if log {
				fmt.Println(lineNum, "Start of atom dec:", name, currAtomId)
			}
			currAtomId++

		case strings.HasPrefix(l, "ruleset"):
			match := reg["fromRuleset"].FindStringSubmatch(l)
			name := match[1]
			currentGlobalRule = name

		case l == "}":
			if inRule == 1 {
				inRule = 0
				inPattern = false
				if len(newRule.Match) <= 0 || newRule.Match[0] == "" {
					newRule.NoMatchPattern = true
				}
				if log {
					fmt.Printf("%v End of match phase\n", lineNum)
				}
				continue outsideLoop
			}
			if inRule == 2 {
				inRule = 0
				if currentGlobalRule == "" {
					Atoms[currentAtom].Rules = append(Atoms[currentAtom].Rules, newRule)
				} else {
					globalRules[currentGlobalRule] = append(globalRules[currentGlobalRule], newRule)
				}
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
			if currentGlobalRule != "" {
				currentGlobalRule = ""
				continue outsideLoop
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
			} else if n == "key" {
				r := reg["anySpace"].Split(l, -1)[2]
				Atoms[currentAtom].Key = rune(r[0])
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
				} else if strings.HasPrefix(l, "inherit") {
					split := reg["fromInherit"].FindStringSubmatch(l)
					name := split[1]
					probMod := float64(0)
					if split[2] != "" {
						flags := reg["anySpace"].Split(l, -1)[2:]
						for _, f := range flags {
							fsplit := reg["modifyFlag"].FindStringSubmatch(f)
							n := fsplit[1]
							v := fsplit[2]
							if n == "P" {
								p, err := strconv.ParseFloat(v, 64)
								checkErr(err)

								probMod = p
							}
						}
					}
					target := make([]Rule, 1)
					if v, ok := Atoms[name]; ok {
						target = v.Rules
					} else if v, ok := globalRules[name]; ok {
						target = v
					}

					for _, r := range target {
						Atoms[currentAtom].Rules = append(Atoms[currentAtom].Rules, r)
						Atoms[currentAtom].Rules[len(Atoms[currentAtom].Rules)-1].Id = newRuleId
						if probMod != 0 {
							Atoms[currentAtom].Rules[len(Atoms[currentAtom].Rules)-1].Prob = probMod
						}
						newRuleId++
					}
				} else if strings.HasPrefix(l, "repeat") {
					p2 := reg["anySpace"].Split(l, -1)[1]
					prev := Atoms[currentAtom].Rules[len(Atoms[currentAtom].Rules)-1]
					if p2 == "match" {
						newRule.Id = newRuleId
						newRule.MatchCon = prev.MatchCon
						newRule.Match = prev.Match
						newRule.W = prev.W
						newRule.H = prev.H
						newRule.Ox = prev.Ox
						newRule.Oy = prev.Oy
						newRule.XSym = prev.XSym
						newRule.YSym = prev.YSym
					} else if p2 == "effect" {
						newRule.DontBreak = prev.DontBreak
						newRule.Pat = prev.Pat
						newRule.Steps = prev.Steps
						newRule.Prob = prev.Prob

						// inRule = 0
						if currentGlobalRule == "" {
							Atoms[currentAtom].Rules = append(Atoms[currentAtom].Rules, newRule)
						} else {
							globalRules[currentGlobalRule] = append(globalRules[currentGlobalRule], newRule)
						}
						inPattern = false
						newRule = Rule{}
						newRuleId++
						continue outsideLoop
					}
				}
			}

			if inRule == 1 {
				if strings.HasPrefix(l, "eval ") {
					expr := l[5:]
					// expr := "x + y"
					vars, randVars, eval := compileMath(expr, int(newRule.Ox), int(newRule.Oy), false)
					// // fmt.Println(vars)
					// eval, err := govaluate.NewEvaluableExpression(expr)
					if err != nil {
						panic(err)
					}

					newRule.MatchCon = append(newRule.MatchCon, Condition{Names: vars, Expr: eval, RandVars: randVars})
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
					newRule.Steps = append(newRule.Steps, Step{Opcode: 5, Name: []string{sym}, Operand: []float64{float64(x), float64(y)}})
					if log {
						fmt.Printf("%v Added step to define %v at coord (%v, %v)\n", lineNum, sym, x, y)
					}
				} else if strings.HasPrefix(l, "set") {
					split := reg["spacedEqual"].Split(l, -1)
					n := strings.TrimSpace(split[0][4:])
					splitn := reg["getEvalBracket"].FindStringSubmatch(n)[1:]
					var operand []float64
					if len(splitn) > 3 && splitn[2] != "" {
						ox, err := strconv.Atoi(splitn[2])
						checkErr(err)

						oy, err := strconv.Atoi(splitn[3])
						checkErr(err)

						operand = []float64{float64(ox) - float64(newRule.Ox), float64(oy) - float64(newRule.Oy)}
						// fmt.Println("operand", operand, "n", n)
					} else {
						operand = []float64{0, 0}
					}
					expr := split[1]
					vars, randVars, eval := compileMath(expr, int(newRule.Ox), int(newRule.Oy), false)

					newRule.Steps = append(newRule.Steps, Step{Opcode: 1, Name: []string{n[1 : len(n)-1]}, Eval: eval, Vars: vars, Operand: operand, RandVars: randVars})
				} else if strings.HasPrefix(l, "non-break") {
					newRule.DontBreak = true
				}
			}

		case sections["init"]:
			if strings.HasPrefix(l, "set") {
				split := reg["spacedEqual"].Split(l, -1)
				n := strings.TrimSpace(split[0][4:])
				splitn := reg["getEvalBracket"].FindStringSubmatch(n)[1:]
				name := splitn[0]

				expr := split[1]
				operand := []float64{0, 0}

				vars, randVars, eval := compileMath(expr, 0, 0, true)

				// fmt.Println(n, splitn)

				Atoms[currentAtom].Init = append(Atoms[currentAtom].Init, Step{Opcode: 5, Name: []string{name}, Operand: operand, Eval: eval, Vars: vars, RandVars: randVars})
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
		fmt.Printf("%+v\n\n", *v)
	}
}

func compileMath(expr string, ox, oy int, initMode bool) (map[string][][2]int, map[string][3]float64, *govaluate.EvaluableExpression) {
	possibleVars := reg["getEvalBracket"].FindAllStringSubmatch(expr, -1)
	vars := make(map[string][][2]int)
	for _, match := range possibleVars {
		if len(match) > 3 && match[3] != "" && !initMode {
			i3, err := strconv.Atoi(match[3])
			checkErr(err)
			i4, err := strconv.Atoi(match[4])
			checkErr(err)

			vars[match[1]] = append(vars[match[1]], [2]int{i3 - ox, i4 - oy})
		} else {
			vars[match[1]] = append(vars[match[1]], [2]int{0, 0})
		}
	}

	possibleRands := reg["getRandomBracket"].FindAllStringSubmatch(expr, -1)
	randVars := make(map[string][3]float64)

	for _, match := range possibleRands {
		fmt.Println(match)
		min, err := strconv.ParseFloat(match[2], 64)
		checkErr(err)

		max, err := strconv.ParseFloat(match[3], 64)
		checkErr(err)

		step, err := strconv.ParseFloat(match[4], 64)
		checkErr(err)

		randVars[match[0]] = [3]float64{min, max, step}
	}

	eval, err := govaluate.NewEvaluableExpression(expr)
	if err != nil {
		panic(err)
	}

	return vars, randVars, eval
}
