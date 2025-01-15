*In this guide's code blocks, `keyword`, `[replace inside]`, `(Optional)?`*

**All blocks are opened by `{` and closed with `}`**

Comments start with `//` they are single lined and skipped entirely
Anything between `/*` and `*/` will also be ignored as multiline comment

**There must always be a `Empty` element as it is used in internal mechanics**

`atom` starts an atom declaration with this syntax\
`atom [name] (alias [Symbol])? [Block]`

Example:
```
atom Sand alias S {

}
```

Alias is a usually single rune used to address easier in sets and rules.\
Later (Lower) atoms' alias overwrite higher atoms' alias with the same alias symbol

### Sections
There are three optional sections in an atom declaration
1) **Property - Made with `section property [Block]`**
   - Where you can define properties of the atom
   - All properties have type of a float64 (or color if name is `color`)
   - Define **static** properties with `cdef [name] value`\
    These are shared by all atom of the element\
    These stay constant due to risk of race conditions
   - Define **non-static** property with `def [name] value`\
    Each atom gets an individual copy of the property\
    Keep these to a minimum to reduce memory usage
   - There are a couple special `cdef`
     - `color` - defines color in `#RRGGBB` form (can be ignored if invisible) or `dynamic` if dynamic coloring
     - `render` - not optional 0 = invisible, 1 = visible
     - `key` - the key needed to press before placing the block. Use lowercase letters and numbers only
     - `size` - the placed block size when you click (size = width = height)
     - `dragCD` - the cooldown time of placing when dragging - always integer. Each increment is equal roughly to 16ms (therefore 60 is roughly a second)
2) **Definition - Made with `section definition [block]`**
   - Where you define sets for use in rules
   - Syntax: `def [symbol] <Name1, Name2, ...>`
   - eg `def F <Empty, Water, Gas>` makes a set of those 3 atoms and referenced with `F`
   - Names in sets can be replaced by an alias. To signify that it is an alias, you must precede it with `^` eg `^S` instead of `Sand`
   - Definitions in this section are **local** - therefore can only be used in this atom
3) **Update - Made with `section update [block]`**
   - This is where rules are put
   - Rules are described below
4) **Init - Made with `section init [block]`**
   - This happens whenever a *change of type* happen
   - Each line would be a step, similar to in an update block. All properties can only target the atom itself
   - No pattern mapping can happen
5) **Color - Made with `section color [block]`**
   - Used for dynamic color rules which are described in more detail below
   - the cdef of `color` in the *property* block must be set to `dynamic`


### Rules
Rules in sandlang works in the following way:
1) In parallel the simulator chooses multiple squares at random
2) For each chosen square, one of the rule is picked, centred on the atom based on position of origin property
3) Each square of the *match rule* is then checked. Each condition of the match block is also checked. If all of them matches the corresponding square, then the *result rule* is executed

Rules can be copied from other atoms with `inherit [name]`\
All rules of the atom will be copied, including ones that they inherited from others

Inherited rules can be modified with `-P=[Probability]` where all rules will have the same specified probability

A rule have a couple properties:
- Width and height: How big the rule is - **Do not make it over 10 blocks in width OR height**
- Posiiton of origin: The coordinate of the atom that the rule centres on (Described more below)
- Symmetries on either direction (Optional)
- Probability of execution (Optional) (from 0-1 inclusive)

#### Match
A match block is defined in this way:\
`match ([Origin X], [Origin Y], [Width], [Height]) (Symmetries)? [Block]`\
Eg: `match (0, 0, 2, 2) {}` defines a match block with `width` and `height` both at 2, and centred on `(0, 0)`, with no symmetry

Symmetry is defined with `sym([x or y or xy])`

Each line in the block defines a condition that must be satisfied, except `pattern`, which matches the *pattern* that comes in the next few lines. Patterns are optional

A condition can also be an `eval` which precedes a *Maths statement*. If all conditions evaluates to true, only then the update block is executed

The entire block can be replaced with `repeat match` to repeat from the previous rule, along with symmetry and other properties

#### Update
All match must have an update\
An update block is defined in this way:\
`-> (P-[Probability])? [Block]`\
Each line in the update block corresponds to a step of one of these:
1) Defining a symbol to be a cell at a certain position, at the time of execution of this command - `def [symbol] = pick([x], [y])` eg `def L = pick(1, 1)` defines `L` to be the cell at `(1, 1)`
2) Mapping onto pattern - `pattern` followed by the *pattern*
3) Setting a non-static property - `set [property] = [Maths statement]`
4) `non-break` picks another rule to execute after this one, instead of choosing another random block
5) Incrementing a property `inc [property] by [Increment (Maths statement)]` (Also decrements - ie negative increment)
6) Clamping a variable between two values (inclusive) - `clamp [property] in [min], [max]` (min and max are maths statement)
7) `always-run` makes the rule ran everytime the cell is picked. They always run before other rules, and all rules with this tag is ran, then one of the normal rules is picked. Do not move the `x` in these rules - Other rules will still be centered on the original centre

The entire block can be replaced with `repeat effect` to repeat *update block* along with probability from the previous rule

### Patterns
Each line of the pattern correspond to a row of cells, and each cell in the row must be seperated by any amount of spaces

In a pattern used to match:\
`*` matches anything not out of bounds (OOB)\
`_` matches *Empty* not OOB\
`e` matches OOB\
`[alias]` matches only that block type\
`[set symbol]` matches anything that is in the set (sets have priority over alias if they are the same symbol)\
`~[set symbol]` matches anything not in the set
`n` matches non-*Empty*

In a pattern used to map:\
`x` map to the cell at the origin\ *(Transfer)*
`/` map to no change\ 
`_` map to *Empty*\ *(Change of type)*
`[alias]` map to the type of the element *(Change of type)*
`[defined symbol]` defined to be a cell in a previous step *(Transfer)*

Transfer vs Change of type - *Transfer* keeps the properties of the source, while *change of type* resets them to default values\
Also only *change of type* executes *init* block

Non-static properties are copied with default values to the new cell

### Maths statements and Properties
Properties are referenced in a rule in square brackets `[]` in which `[name](-[x], [y])?`\
The `x` and `y` coords corresponds to canvas coordinate of the block referenced in the rule\
The coordinate can be ignored. That way by default it would be the coordinate of the centre block `x` 

Static properties are referenced in the same way, taking the property of the type of atom at the coordinate

In a *maths statement* you can use the normal `+ - * / %` and `== <= >= < >` and `()`

A random element can be added with `[]` that includes `$[symbol]'[min]'[max]'[step]`\
The random number *x* will be created `min ≤ x < max`, with resolution of `step`\
The symbol is used such that identical random piece with same min, max, step and symbol will have the same value each time\
A different symbol is needed to make the results different\
For example all instances of `[$a'0'2'1]` will make the same 0 or 1, and `[$b'0'2'1]` will make a potentially different 0 or 1 to the ones with `a`

### Dynamic coloring
Each line in the *color* block has syntax of `[condition] => [red], [green], [blue]`\
Each bit can be replaced with a maths statement, with *condition* must evaluate to true to be used\
Red, green and blue must evaluate to an integer between 0 - 255 inclusive

Tested from top to bottom, with the first line with true *condition* used

Always put a line with statement `true` for a default color

At the top of the file you can preload color for more efficient simulation by `preload [color] [color]?`\
All colors with components all between the two colors will be preloaded. Only preload colors that are needed to save memory\
The second color can be ignored. In that case only the first color will be loaded. Colors are in format #RRGGBB

### External functions
There are a couple default functions implemented. Each are included with `ext [name] <[paramName]=[value], ...>`\
They should be put where a rule would normally go, therefore in the *update* block\
One rule will be picked, either from other rules or these external rules\
They are *conditionless*, therefore they will always execute (although not necessarily make change)

Parameters should be simple strings, or numbers, but **not** maths statements

All external function has a `prob` parameter, which determines the probability of its execution when it is picked. It should be a number between 0 and 1, where 0 is never execute and 1 is always execute

Functions (and their parameters indented):
- `randomMove` - Randomly move the particle to an adjacent (including diagonal) square by swapping position with the target
  - `repl` - required - must be either symbol of a set or global set (inverted by prepending `~`), or an alias of an element. It describes which elements the particle can swap with (similar to a rule of a cell)
- `sandLike` - Similar to randomMove, but only the bottom three squares
  - `repl` - required - same as randomMove

### Other features
- Global sets are automatically added to all atoms defined after the definition of the global set. They are defined with `global [symbol] <Name1, Name2, Name3, ...>`. Similar to definition section in an atom, the names can be replaced by `^[alias]`
- Global rule sets can contain a set of rules to be inherited by other atoms for less redundancy. They are defined with `ruleset [name] [Block]` and the `[Block]` contains rules. Atoms have a higher priority over rule sets if they have the same name
- Default value of property can be defined with `default [symbol] [value]` at the start of file. These can be used to ensure that all cells have a certain property (or it will error if it tries to access non-existent properties)
- Press `/` to clear the world

### Examples
#### Sand
```
atom Empty {
    section property {
        // no render
        cdef render 0
    }
}

atom Sand {
    section property {
        cdef render 1
        // red color
        cdef color #FF0000
    }
    section update {
        // fall straight down
        match (0, 0, 1, 2) {
            pattern
            x
            _
        }
        -> {
            pattern
            _
            x
        }

        // fall down to the sides with x-axis symmetry
        match (0, 0, 2, 2) sym(x) {
            pattern
            x _
            n _
        }
        -> {
            pattern
            _ /
            / x
        }
    }
}
```
