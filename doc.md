*In this guide's code blocks, `keyword`, `[replace inside]`, `(Optional)?`*
**All blocks are opened by `{` and closed with `}`**

Comments start with `//` they are single lined and skipped entirely
**There must always be a `Empty` element**

`atom` starts an atom declaration with this syntax
`atom [name] (alias [Symbol])? [Block]`

eg 
`atom Sand alias S {`

`}`

Alias is a usually single rune used to address easier in sets and rules. Later (Lower) atoms' alias overwrite higher atoms' alias with the same alias symbol

### Sections
There are three optional sections in an atom declaration
1) **Property - Made with `section property [Block]`**
   - Where you can define properties of the atom
   - All properties have type of a float64 (or color if name is `color`)
   - Define **static** properties with `cdef [name] value`
    These is shared by all atom of the element
   - Define **non-static** property with `def [name] value`
    Each atom gets an individual copy of the property
    Keep these to a minimum to reduce memory usage
   - There are a couple special `cdef`
     - `color` - defines color in `#RRGGBB` form (can be ignored if invisible)
     - `render` - not optional 0 = invisible, 1 = visible
2) **Definition - Made with `section definition [block]`**
   - Where you define sets for use in rules
   - Syntax: `def symbol <Name1, Name2, ...>`
   - eg `def F <Empty, Water, Gas>` makes a set of those 3 atoms and referenced with `F`
   - Names in sets can be replaced by an alias. To signify that it is an alias, you must precede it with `^` eg `^S` instead of `Sand`
   - Definitions in this section are **local** - therefore can only be used in this atom
3) **Update - Made with `section update [block]`**
   - This is where rules are put
   - Rules are described below


### Rules
Rules in sandlang works in the following way:
1) In parallel the simulator chooses multiple squares at random
2) For each chosen square, all the rules of the atom type is checked in order, centred on the atom based on position of origin property
3) Each square of the *match rule* is then checked. If all of them matches the corresponding square, then the *result rule* is executed

A rule have a couple properties:
- Width and height: How big the rule is - **Do not make it over 10 blocks in each direction**
- Posiiton of origin: The coordinate of the atom that the rule centres on (Described more below)
- Symmetries on either direction (Optional)
- Probability of execution (Optional) (from 0-1 inclusive)

#### Match
A match block is defined in this way:
`match ([Origin X], [Origin Y], [Width], [Height]) (Symmetries)? [Block]`
Eg: `match (0, 0, 2, 2) {}` defines a match block with `width` and `height` both at 2, and centred on `(0, 0)`, with no symmetry

Symmetry is defined with `sym([x or y or xy])`

Each line in the block defines a condition that must be satisfied, except `pattern`, which matches the *pattern* that comes in the next few lines

#### Update
All match must have an update
An update block is defined in this way:
`-> (P-[Probability])? [Block]`
Each line in the update block corresponds to a step of one of these:
1) Defining a symbol to be a cell at a certain position, at the time of execution of this command - `def [symbol] = pick([x], [y])` eg `def L = pick(1, 1)` defines `L` to be the cell at `(1, 1)`
2) Mapping onto pattern - `pattern` followed by the *pattern*



### Patterns
Each line of the pattern correspond to a row of cells, and each cell in the row must be seperated by any amount of spaces

In a pattern used to match:
`*` matches anything not out of bounds (OOB)
`_` matches *Empty* not OOB
`e` matches OOB
`[alias]` matches only that block type
`[set symbol]` matches anything that is in the set (sets have priority over alias if they are the same symbol)
`n` matches non-*Empty*

In a pattern used to map:
`x` map to the cell at the origin
`/` map to no change
`_` map to *Empty*
`[alias]` map to the type of the element

Non-static properties are copied with default values to the new cell

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