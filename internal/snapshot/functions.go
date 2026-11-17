package snapshot

import (
	"regexp"
	"slices"
	"strings"

	"github.com/julianzh0/plimsoll/internal/conventions"
	"github.com/julianzh0/plimsoll/internal/parse"
)

// function is one function of a file, measured: a name and numbers, no text.
type function struct {
	name                 string
	startLine, endLine   int // from 1
	start, end           int // byte offsets
	complexity           int
	nesting, nestingLine int  // nestingLine is 0 when nesting is 0
	broken               bool // its subtree holds an ERROR node: not measured
	repeats              []repeat
}

// kindSet holds, by kind index, the kinds one field of a function table names.
type kindSet []bool

func kindsOf(tree parse.Tree, names []string) kindSet {
	set := make(kindSet, len(tree.Kinds))
	for i, kind := range tree.Kinds {
		set[i] = slices.Contains(names, kind)
	}
	return set
}

func (s kindSet) has(n parse.Node) bool { return int(n.Kind) < len(s) && s[n.Kind] }

// table is a function table resolved against one tree's kind names.
type table struct {
	declared, bodies, closures, namedClosures, namedBy          kindSet
	ifs, elseClauses, flatBranches, nesting, logic, parentheses kindSet
	markup, depthIfs, depth, blocks, switches                   kindSet
	declarators, names                                          kindSet
	bodyAfter                                                   bool
	bodyNames                                                   map[string]bool
	elseKind                                                    int // the index of the kind "else", or -1
}

func tableOf(k conventions.FunctionKinds, tree parse.Tree) table {
	return table{
		declared: kindsOf(tree, k.Declared), bodies: kindsOf(tree, k.Bodies), closures: kindsOf(tree, k.Closures),
		namedClosures: kindsOf(tree, k.NamedClosures), namedBy: kindsOf(tree, k.NamedBy), ifs: kindsOf(tree, k.Ifs),
		elseClauses: kindsOf(tree, k.ElseClauses), flatBranches: kindsOf(tree, k.FlatBranches), nesting: kindsOf(tree, k.Nesting),
		logic: kindsOf(tree, k.Logic), parentheses: kindsOf(tree, k.Parentheses), markup: kindsOf(tree, k.Markup),
		depthIfs: kindsOf(tree, k.DepthIfs), depth: kindsOf(tree, k.Depth), bodyAfter: k.BodyAfter,
		blocks: kindsOf(tree, k.Blocks), switches: kindsOf(tree, k.SwitchBlocks), bodyNames: namesOf(k.Bodies),
		declarators: kindsOf(tree, k.Declarators), names: kindsOf(tree, k.Names),
		elseKind: slices.Index(tree.Kinds, "else"),
	}
}

// shape is a tree's structure: each node's parent, previous sibling and the index after its subtree, -1 for none, and
// how many error nodes stand before each node.
type shape struct {
	nodes             []parse.Node
	parent, prev, end []int32
	errorsBefore      []int32 // errorsBefore[k]: the error nodes among nodes[:k]; one more entry than nodes
}

// shapeOf makes the shape in one pass with a stack: no recursion, whatever the depth.
func shapeOf(nodes []parse.Node) shape {
	s := shape{nodes: nodes, parent: make([]int32, len(nodes)), prev: make([]int32, len(nodes)), end: make([]int32, len(nodes)),
		errorsBefore: make([]int32, len(nodes)+1)}
	var stack []int32
	var after int32                        // the index past the last node seen
	lastChild := make([]int32, len(nodes)) // by parent: its last child seen, -1 for none
	for i, n := range nodes {
		after = int32(i) + 1
		s.errorsBefore[i+1] = s.errorsBefore[i]
		if n.Error {
			s.errorsBefore[i+1]++
		}
		lastChild[i] = -1
		for len(stack) > 0 && nodes[stack[len(stack)-1]].Depth >= n.Depth {
			s.end[stack[len(stack)-1]] = int32(i)
			stack = stack[:len(stack)-1]
		}
		s.parent[i], s.prev[i] = -1, -1
		if len(stack) > 0 {
			p := stack[len(stack)-1]
			s.parent[i], s.prev[i] = p, lastChild[p]
			lastChild[p] = int32(i)
		}
		stack = append(stack, int32(i))
	}
	for _, i := range stack {
		s.end[i] = after
	}
	return s
}

func (s shape) endOf(i int) int    { return int(s.end[i]) }
func (s shape) parentOf(i int) int { return int(s.parent[i]) }
func (s shape) prevOf(i int) int   { return int(s.prev[i]) }

// lineIndex turns byte offsets into lines, from 1.
type lineIndex []int // the byte offset of each newline

func linesOf(content []byte) lineIndex {
	var at lineIndex
	for i, b := range content {
		if b == '\n' {
			at = append(at, i)
		}
	}
	return at
}

func (l lineIndex) line(offset int) int {
	n, _ := slices.BinarySearch(l, offset)
	return n + 1
}

// functionsOf measures the functions of one file, in text order: every declared function with a body however deep it
// is declared, and the closures a name binds outside every function. Closures, nested functions and markup add
// nothing to the function around them.
func functionsOf(kinds conventions.FunctionKinds, tokens *regexp.Regexp, rules conventions.FunctionRules, tree parse.Tree, content []byte) []function {
	t := tableOf(kinds, tree)
	s := shapeOf(tree.Nodes)
	lines := linesOf(content)
	var found []function
	inside := 0 // the end of the outermost function or listed closure seen so far
	// the tokens of the outermost function seen so far, from its first byte: a function inside takes its part of them,
	// so the file is tokenized once
	var outer tokenSpan
	for i, n := range tree.Nodes {
		if !n.Named {
			continue
		}
		if t.declared.has(n) {
			to, ok := t.functionEnd(s, i)
			if !ok {
				continue
			}
			f := t.measure(s, i, to, functionName(tree, s, t, i, content), content, lines)
			if i >= inside {
				outer = newTokenSpan(tokensOf(tokens, content[f.start:f.end], 0), f.start)
			}
			f.repeats = repeatsOf(rules, tree, s, t, i, to, outer, lines)
			found = append(found, f)
			inside = max(inside, to)
			continue
		}
		if i >= inside && t.namedClosures.has(n) && s.parentOf(i) >= 0 && t.namedBy.has(tree.Nodes[s.parentOf(i)]) {
			if name := bindingName(tree, s, i, content); name != "" {
				f := t.measure(s, i, s.endOf(i), name, content, lines)
				outer = newTokenSpan(tokensOf(tokens, content[f.start:f.end], 0), f.start)
				f.repeats = repeatsOf(rules, tree, s, t, i, s.endOf(i), outer, lines)
				found = append(found, f)
				inside = s.endOf(i)
			}
		}
	}
	return found
}

// functionEnd is where a declared function ends: after its body, or false when it has none.
func (t table) functionEnd(s shape, i int) (int, bool) {
	if t.bodyAfter {
		next := s.endOf(i)
		if next < len(s.nodes) && s.nodes[next].Depth == s.nodes[i].Depth && s.nodes[next].Named && t.bodies.has(s.nodes[next]) {
			return s.endOf(next), true
		}
		return 0, false
	}
	for k := i + 1; k < s.endOf(i); k = s.endOf(k) {
		if s.nodes[k].Named && t.bodies.has(s.nodes[k]) {
			return s.endOf(i), true
		}
	}
	return 0, false
}

// functionName is a declared function's name: its name field when it is an identifier (else its kind); else, through
// the table's declarators, the identifier or name kind in the innermost one; else its first named child of an
// identifier kind; else the name field or the first identifier child of one of its named children; else its kind
// without "_declaration".
func functionName(tree parse.Tree, s shape, t table, i int, content []byte) string {
	bodyKinds := t.bodyNames
	text := func(k int) string {
		// a name split over lines (C++: Store::\n route) shows on one line; a space within a line stays (Kotlin)
		lines := strings.Split(string(content[s.nodes[k].Start:s.nodes[k].End]), "\n")
		for i := range lines {
			lines[i] = strings.TrimSpace(lines[i])
		}
		return strings.Trim(strings.Join(lines, ""), "`")
	}
	// a type's name (type_identifier) is not the function's: Dart writes the return type before the name
	identifier := func(k int) bool {
		kind := tree.Kinds[s.nodes[k].Kind]
		return strings.HasSuffix(kind, "identifier") && kind != "type_identifier"
	}
	children := func(k int) []int {
		var c []int
		for j := k + 1; j < s.endOf(k); j = s.endOf(j) {
			c = append(c, j)
		}
		return c
	}
	if kind := tree.Kinds[s.nodes[i].Kind]; strings.Contains(kind, "operator") {
		return strings.TrimSuffix(kind, "_declaration") // an operator's other names are its types
	}
	for _, c := range children(i) {
		if s.nodes[c].Name && (identifier(c) || !s.nodes[c].Named) { // an identifier, or a keyword such as C#'s get
			return text(c)
		}
		if s.nodes[c].Name { // a computed key or a quoted name is the file's text, not a name: the kind stands for it
			return strings.TrimSuffix(tree.Kinds[s.nodes[i].Kind], "_declaration")
		}
	}
	for k, deeper := i, true; deeper; {
		deeper = false
		for _, c := range children(k) {
			if !t.declarators.has(s.nodes[c]) {
				continue
			}
			k, deeper = c, true
			for _, g := range children(c) {
				if identifier(g) || t.names.has(s.nodes[g]) {
					return text(g)
				}
			}
			break
		}
	}
	for _, c := range children(i) {
		if s.nodes[c].Named && identifier(c) {
			return text(c)
		}
	}
	for _, c := range children(i) {
		if kind := tree.Kinds[s.nodes[c].Kind]; !s.nodes[c].Named || bodyKinds[kind] || strings.Contains(kind, "parameter") {
			continue
		}
		for _, g := range children(c) {
			if s.nodes[g].Name || s.nodes[g].Named && identifier(g) {
				return text(g)
			}
		}
	}
	return strings.TrimSuffix(tree.Kinds[s.nodes[i].Kind], "_declaration")
}

// bindingName is the name a parent gives a closure: its first named child before the closure of kind identifier or
// property_identifier.
func bindingName(tree parse.Tree, s shape, i int, content []byte) string {
	p := s.parentOf(i)
	for j := p + 1; j < i; j = s.endOf(j) {
		kind := tree.Kinds[s.nodes[j].Kind]
		if s.nodes[j].Named && (kind == "identifier" || kind == "property_identifier") {
			return string(content[s.nodes[j].Start:s.nodes[j].End])
		}
	}
	return ""
}

// measure is one function's numbers over the nodes [from, to).
func (t table) measure(s shape, from, to int, name string, content []byte, lines lineIndex) function {
	f := function{name: name, start: int(s.nodes[from].Start), end: t.extent(s, from, to),
		broken: s.errorsBefore[to] > s.errorsBefore[from]}
	f.startLine, f.endLine = lines.line(f.start), lines.line(max(f.start, f.end-1))
	f.complexity = t.complexity(s, from, to)
	f.nesting, f.nestingLine = t.depthOf(s, from, to, lines)
	return f
}

// extent is where the nodes from..to end: the end of the last of them at from's level (a Dart body follows its
// signature), without walking the nodes inside.
func (t table) extent(s shape, from, to int) int {
	end := 0
	for k := from; k < to; k = s.endOf(k) {
		end = max(end, int(s.nodes[k].End))
	}
	return end
}

// skipped says whether the node at k starts a subtree that adds nothing to the function around it: a declared function,
// a closure or markup.
func (t table) skipped(n parse.Node) bool {
	return n.Named && (t.declared.has(n) || t.closures.has(n) || t.markup.has(n))
}

// skipEnd is where a skipped node's subtree ends: for a declared function whose body follows its signature, after
// that body.
func (t table) skipEnd(s shape, k int) int {
	if t.declared.has(s.nodes[k]) {
		if to, ok := t.functionEnd(s, k); ok {
			return to
		}
	}
	return s.endOf(k)
}

func (t table) isElse(s shape, i int) bool { return i >= 0 && int(s.nodes[i].Kind) == t.elseKind }

func (t table) isIf(s shape, i int) bool { return i >= 0 && s.nodes[i].Named && t.ifs.has(s.nodes[i]) }

// elseIf says whether the if at k continues an if before it: it follows an else, it is the whole of an else clause of
// an if, or its parent follows an else of an if.
func (t table) elseIf(s shape, k int) bool {
	p := s.parentOf(k)
	return t.isElse(s, s.prevOf(k)) ||
		(p >= 0 && t.elseClauses.has(s.nodes[p]) && t.isIf(s, s.parentOf(p))) ||
		(p >= 0 && t.isElse(s, s.prevOf(p)) && t.isIf(s, s.parentOf(p)))
}

// complexity is SonarSource's cognitive complexity of the nodes [from, to): an if, loop, switch, catch or conditional
// expression counts 1 plus the levels open around it and opens one; an else, else-if or elif counts 1; each sequence of
// like boolean operators, read in text order through one expression tree, counts 1.
func (t table) complexity(s shape, from, to int) int {
	total := 0
	var open []uint32
	last := map[int]uint16{} // the root of a tree of boolean operators -> its last operator's kind
	roots := map[int]int{}   // a node -> the root of its tree of boolean operators, as found
	for k := from + 1; k < to; k++ {
		n := s.nodes[k]
		for len(open) > 0 && open[len(open)-1] >= n.Depth {
			open = open[:len(open)-1]
		}
		switch {
		case t.skipped(n):
			k = t.skipEnd(s, k) - 1
		case int(n.Kind) == t.elseKind:
			p := s.parentOf(k)
			if t.isIf(s, p) || (p >= 0 && t.elseClauses.has(s.nodes[p]) && t.isIf(s, s.parentOf(p))) {
				total++
			}
		case n.Named && t.ifs.has(n):
			if !t.elseIf(s, k) {
				total += 1 + len(open)
				open = append(open, n.Depth)
			}
		case n.Named && t.flatBranches.has(n):
			total++
		case n.Named && t.nesting.has(n):
			total += 1 + len(open)
			open = append(open, n.Depth)
		case !n.Named && t.logic.has(n):
			root := t.logicRoot(s, k, from, roots)
			if kind, seen := last[root]; !seen || kind != n.Kind {
				total++
			}
			last[root] = n.Kind
		}
	}
	return total
}

// logicRoot is the topmost node of the tree of boolean operators, through parentheses, that holds the token at k. roots
// keeps every node's root once found, so a long chain is climbed once.
func (t table) logicRoot(s shape, k, from int, roots map[int]int) int {
	holdsLogic := func(i int) bool {
		if i < 0 || !s.nodes[i].Named {
			return false
		}
		for j := i + 1; j < s.endOf(i); j = s.endOf(j) {
			if !s.nodes[j].Named && t.logic.has(s.nodes[j]) {
				return true
			}
		}
		return false
	}
	var climbed []int
	root := s.parentOf(k)
	for {
		if known, found := roots[root]; found {
			root = known
			break
		}
		climbed = append(climbed, root)
		p := s.parentOf(root)
		for p >= 0 && t.parentheses.has(s.nodes[p]) {
			p = s.parentOf(p)
		}
		if p < from || !holdsLogic(p) {
			break
		}
		root = p
	}
	for _, n := range climbed {
		roots[n] = root
	}
	return root
}

// depthOf is the deepest nesting of control structures in [from, to), and the line of the first node that reaches it.
// An else-if stays at its if's level; closures, nested functions and markup are skipped.
func (t table) depthOf(s shape, from, to int, lines lineIndex) (int, int) {
	deepest, line := 0, 0
	var open []uint32
	for k := from + 1; k < to; k++ {
		n := s.nodes[k]
		for len(open) > 0 && open[len(open)-1] >= n.Depth {
			open = open[:len(open)-1]
		}
		push := false
		switch {
		case t.skipped(n):
			k = t.skipEnd(s, k) - 1
		case n.Named && t.depthIfs.has(n):
			push = !t.elseIf(s, k)
		case n.Named && t.depth.has(n):
			push = true
		}
		if push {
			open = append(open, n.Depth)
			if len(open) > deepest {
				deepest, line = len(open), lines.line(int(n.Start))
			}
		}
	}
	return deepest, line
}

func namesOf(kinds []string) map[string]bool {
	set := map[string]bool{}
	for _, k := range kinds {
		set[k] = true
	}
	return set
}
