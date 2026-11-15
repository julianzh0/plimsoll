package links

import (
	"path"
	"slices"
	"strings"

	"github.com/juzhxng/plimsoll/internal/snapshot"
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
		known: map[string]bool{}, byDir: map[string]bool{}, runs: map[string]bool{},
	}
	for _, file := range tree.Files {
		g.roles[file.Path] = file.Role
		rings, analysed := looksForRings[file.Language]
		if analysed {
			g.files, g.known[file.Path], g.byDir[file.Path] = append(g.files, file.Path), true, folders[file.Language]
			g.runs[file.Path] = slices.Contains(implicit[file.Language], path.Base(file.Path))
		}
		if rings {
			g.nodes = append(g.nodes, file.Path)
		}
	}
	for _, link := range tree.Links.List {
		if g.known[link.From] {
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
func (g graph) rings(withTypeOnly bool) [][]string {
	isNode := map[string]bool{}
	for _, n := range g.nodes {
		isNode[n] = true
	}
	edges := map[string][]string{}
	for _, n := range g.nodes {
		for _, link := range g.from[n] {
			if link.TypeOnly && !withTypeOnly {
				continue
			}
			for _, to := range link.Files {
				if isNode[to] {
					edges[n] = append(edges[n], to)
				}
			}
		}
	}
	return components(g.nodes, edges)
}

// components are the groups of two or more nodes that all reach each other along the edges, each sorted, the groups
// sorted by their first node. The search is iterative, so a long chain of edges cannot exhaust the stack.
func components(nodes []string, edges map[string][]string) [][]string {
	type frame struct {
		node string
		next int
	}
	index, low, onStack := map[string]int{}, map[string]int{}, map[string]bool{}
	var stack []string
	var groups [][]string
	visit := func(n string) {
		index[n], low[n] = len(index), len(index)
		stack, onStack[n] = append(stack, n), true
	}
	for _, root := range nodes {
		if _, seen := index[root]; seen {
			continue
		}
		visit(root)
		work := []frame{{node: root}}
		for len(work) > 0 {
			top := &work[len(work)-1]
			if top.next < len(edges[top.node]) {
				next := edges[top.node][top.next]
				top.next++
				if _, seen := index[next]; !seen {
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
				group = append(group, member)
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

// importers are the files with a link that reaches a code file, sorted.
func (g graph) importers(path string) []string {
	var found []string
	for _, link := range g.to[path] {
		found = append(found, link.From)
	}
	slices.Sort(found)
	return slices.Compact(found)
}

// targets counts the distinct targets of a file's links.
func (g graph) targets(path string) int {
	distinct := map[string]bool{}
	for _, link := range g.from[path] {
		distinct[link.To] = true
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
