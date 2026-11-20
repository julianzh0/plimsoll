package links

import (
	"path"
	"slices"
	"strings"

	"github.com/julianzh0/plimsoll/internal/snapshot"
)

// graph is one side's links from code files of the analysed languages, with every code file's role.
type graph struct {
	roles map[string]snapshot.Role
	from  map[string][]snapshot.Link // the links of each importing file, by line
	to    map[string][]snapshot.Link // the links that reach each code file
	files []string                   // the code files of the analysed languages, sorted
	nodes []string                   // the code files rings are looked for among, sorted
	known map[string]bool            // the code files of the analysed languages
	byDir map[string]bool            // the code files of languages whose imports name folders
	runs  map[string]bool            // the code files their language runs without an import of them
	// reached are the folders an import names: a file of such a package may be used by its peers without one.
	reached map[string]bool
}

// graphsOf are the graphs of both sides.
func graphsOf(s snapshot.Snapshot, languages []snapshot.LanguageImports) (now, before graph) {
	return graphOf(s.Now, languages), graphOf(s.Before, languages)
}

func graphOf(tree snapshot.Tree, languages []snapshot.LanguageImports) graph {
	looksForRings, folders, implicit := map[string]bool{}, map[string]bool{}, map[string][]string{} // per analysed language
	for _, l := range languages {
		looksForRings[l.Language], folders[l.Language], implicit[l.Language] = l.NoRings == "", l.Folders, l.Implicit
	}
	g := graph{
		roles: map[string]snapshot.Role{}, from: map[string][]snapshot.Link{}, to: map[string][]snapshot.Link{},
		known: map[string]bool{}, byDir: map[string]bool{}, runs: map[string]bool{}, reached: map[string]bool{},
	}
	for _, file := range tree.Files {
		g.roles[file.Path] = file.Role
		rings, analysed := looksForRings[file.Language]
		if analysed {
			g.files, g.known[file.Path], g.byDir[file.Path] = append(g.files, file.Path), true, folders[file.Language]
			g.runs[file.Path] = slices.ContainsFunc(implicit[file.Language], func(pattern string) bool {
				matched, _ := path.Match(pattern, path.Base(file.Path))
				return matched
			})
		}
		if rings {
			g.nodes = append(g.nodes, file.Path)
		}
	}
	for _, link := range tree.Links.List {
		if g.known[link.From] {
			g.reached[link.To] = g.reached[link.To] || link.Folder
			g.from[link.From] = append(g.from[link.From], link)
			for _, file := range link.Files {
				g.to[file] = append(g.to[file], link)
			}
		}
	}
	return g
}

// testImports are the links of a file that reach test code only, by line.
func (g graph) testImports(path string) []snapshot.Link {
	var found []snapshot.Link
	for _, link := range g.from[path] {
		onlyTest := true
		for _, file := range link.Files {
			onlyTest = onlyTest && g.roles[file].IsTest()
		}
		if onlyTest {
			found = append(found, link)
		}
	}
	return found
}

// testTargets counts the distinct targets of a file's links that reach test code only.
func (g graph) testTargets(path string) int {
	targets := map[string]bool{}
	for _, link := range g.testImports(path) {
		targets[link.To] = true
	}
	return len(targets)
}

// rings are the groups of two or more files that all reach each other, each sorted, the groups sorted by their first
// path. withTypeOnly counts type-only links as edges too.
func (g graph) rings() (rings, withTypeOnly [][]string) {
	index := make(map[string]int, len(g.nodes))
	for i, n := range g.nodes {
		index[n] = i
	}
	edges, all := make([][]int, len(g.nodes)), make([][]int, len(g.nodes))
	for i, n := range g.nodes {
		for _, link := range g.from[n] {
			for _, to := range link.Files {
				if j, node := index[to]; node {
					all[i] = append(all[i], j)
					if !link.TypeOnly {
						edges[i] = append(edges[i], j)
					}
				}
			}
		}
	}
	return groupsOf(g.nodes, edges), groupsOf(g.nodes, all)
}

// components are the groups of two or more nodes that all reach each other along the edges, each sorted, the groups
// sorted by their first node.
func components(nodes []string, edges map[string][]string) [][]string {
	names := slices.Clone(nodes)
	index := make(map[string]int, len(names))
	for i, n := range names {
		index[n] = i
	}
	numbered := make([][]int, len(names))
	for i := 0; i < len(names); i++ { // names grows by the targets that are no node, which the search visits too
		for _, to := range edges[names[i]] {
			j, known := index[to]
			if !known {
				j = len(names)
				index[to], names = j, append(names, to)
				numbered = append(numbered, nil)
			}
			numbered[i] = append(numbered[i], j)
		}
	}
	return groupsOf(names, numbered)
}

// groupsOf is the search behind components over numbered nodes: Tarjan's, iterative, so a long chain of edges cannot
// exhaust the stack; numbers instead of names keep a graph of many thousand files fast.
func groupsOf(names []string, edges [][]int) [][]string {
	type frame struct{ node, next int }
	index, low := make([]int, len(names)), make([]int, len(names))
	for i := range index {
		index[i] = -1
	}
	onStack := make([]bool, len(names))
	var stack []int
	var groups [][]string
	count := int(0)
	visit := func(n int) {
		index[n], low[n], count = count, count, count+1
		stack, onStack[n] = append(stack, n), true
	}
	for root := range len(names) {
		if index[root] >= 0 {
			continue
		}
		visit(root)
		work := []frame{{node: root}}
		for len(work) > 0 {
			top := &work[len(work)-1]
			if int(top.next) < len(edges[top.node]) {
				next := edges[top.node][top.next]
				top.next++
				if index[next] < 0 {
					visit(next)
					work = append(work, frame{node: next})
				} else if onStack[next] {
					low[top.node] = min(low[top.node], index[next])
				}
				continue
			}
			done := top.node
			work = work[:len(work)-1]
			if len(work) > 0 {
				parent := work[len(work)-1].node
				low[parent] = min(low[parent], low[done])
			}
			if low[done] != index[done] {
				continue
			}
			var group []string
			for {
				member := stack[len(stack)-1]
				stack, onStack[member] = stack[:len(stack)-1], false
				group = append(group, names[member])
				if member == done {
					break
				}
			}
			if len(group) > 1 {
				slices.Sort(group)
				groups = append(groups, group)
			}
		}
	}
	slices.SortFunc(groups, func(a, b []string) int { return strings.Compare(a[0], b[0]) })
	return groups
}

// ringLinks are the links between the members of a ring that are not type-only, by importing file and line.
func (g graph) ringLinks(members []string) []snapshot.Link {
	member := map[string]bool{}
	for _, m := range members {
		member[m] = true
	}
	var found []snapshot.Link
	for _, from := range members {
		for _, link := range g.from[from] {
			if !link.TypeOnly && slices.ContainsFunc(link.Files, func(to string) bool { return member[to] }) {
				found = append(found, link)
			}
		}
	}
	return found
}

// importers are the files with a link that reaches a code file, sorted; a module's declaration in its parent (Rust
// `mod x;`) is containment, not use, and makes no importer.
func (g graph) importers(path string) []string {
	var found []string
	for _, link := range g.to[path] {
		if link.Uses() {
			found = append(found, link.From)
		}
	}
	slices.Sort(found)
	return slices.Compact(found)
}

// targets counts the distinct targets of a file's links.
func (g graph) targets(path string) int {
	distinct := map[string]bool{}
	for _, link := range g.from[path] {
		if link.Uses() { // a module's declaration is containment, not an import
			distinct[link.To] = true
		}
	}
	return len(distinct)
}

// has says whether a path is a code file on this side.
func (g graph) has(path string) bool {
	_, isCode := g.roles[path]
	return isCode
}

// analysed says whether a path is a code file of an analysed language on this side.
func (g graph) analysed(path string) bool { return g.known[path] }
