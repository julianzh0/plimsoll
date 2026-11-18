package links

import (
	"maps"
	"path"
	"slices"
	"strings"

	"github.com/julianzh0/plimsoll/internal/snapshot"
)

// siblingImport is one import that counts between two folders directly under one parent.
type siblingImport struct {
	parent, from, to string // the parent ("." is the repository's top folder) and two of its child folders, as paths
	link             snapshot.Link
}

// siblingImports are the imports between product files that count for folder cycles, by importing file and line:
// the lowest folder that holds both ends is the parent, and an import whose file or target lies directly in the
// parent, or whose two files are in one of the rings, gives none.
func (g graph) siblingImports(rings [][]string) []siblingImport {
	ringOf := map[string]int{}
	for i, ring := range rings {
		for _, member := range ring {
			ringOf[member] = i + 1
		}
	}
	var found []siblingImport
	for _, from := range g.files {
		if g.roles[from] != snapshot.Product {
			continue
		}
		for _, link := range g.from[from] {
			target, counts := g.targetFolder(link, ringOf)
			if !counts {
				continue
			}
			parent, fromChild, toChild, siblings := siblingsOf(path.Dir(from), target)
			if siblings {
				found = append(found, siblingImport{parent: parent, from: fromChild, to: toChild, link: link})
			}
		}
	}
	return found
}

// targetFolder is the folder an import reaches: the folder it names where a language imports folders, else the folder
// of its target file. It counts when the import reaches a product file and does not join two files of one ring.
func (g graph) targetFolder(link snapshot.Link, ringOf map[string]int) (string, bool) {
	product := slices.ContainsFunc(link.Files, func(f string) bool { return g.roles[f] == snapshot.Product })
	if !product {
		return "", false
	}
	if link.Folder {
		return link.To, true
	}
	if ring := ringOf[link.From]; ring > 0 && ringOf[link.To] == ring {
		return "", false
	}
	return path.Dir(link.To), true
}

// siblingsOf finds the lowest folder that holds two folders and the two children of it that hold them; siblings is
// false when one folder holds the other.
func siblingsOf(a, b string) (parent, aChild, bChild string, siblings bool) {
	as, bs := segments(a), segments(b)
	shared := 0
	for shared < len(as) && shared < len(bs) && as[shared] == bs[shared] {
		shared++
	}
	if shared == len(as) || shared == len(bs) {
		return "", "", "", false
	}
	parent = "."
	if shared > 0 {
		parent = strings.Join(as[:shared], "/")
	}
	return parent, strings.Join(as[:shared+1], "/"), strings.Join(bs[:shared+1], "/"), true
}

// segments are a folder path's folder names; the top folder "." has none.
func segments(folder string) []string {
	if folder == "." {
		return nil
	}
	return strings.Split(folder, "/")
}

// folderCycles are, per parent, the groups of child folders that reach each other, sorted by their first folder.
func folderCycles(imports []siblingImport) [][]string {
	nodes, edges := map[string]map[string]bool{}, map[string]map[string][]string{}
	for _, imp := range imports {
		if edges[imp.parent] == nil {
			nodes[imp.parent], edges[imp.parent] = map[string]bool{}, map[string][]string{}
		}
		nodes[imp.parent][imp.from], nodes[imp.parent][imp.to] = true, true
		edges[imp.parent][imp.from] = append(edges[imp.parent][imp.from], imp.to)
	}
	var groups [][]string
	for parent, children := range nodes {
		groups = append(groups, components(slices.Sorted(maps.Keys(children)), edges[parent])...)
	}
	slices.SortFunc(groups, func(a, b []string) int { return strings.Compare(a[0], b[0]) })
	return groups
}

// folderAtBase gives the names a folder had at the base: its own path and, when the folder did not exist at the base,
// the folders its renamed files came from, cut to its depth. A folder that existed keeps its own name alone, so one
// file moved into it does not make a new cycle an old one.
func folderAtBase(s snapshot.Snapshot) func(folder string) []string {
	return func(folder string) []string {
		names := []string{folder}
		if slices.ContainsFunc(s.Before.Paths, func(p string) bool { return lies(p, folder) }) {
			return names
		}
		depth := len(segments(folder))
		for _, change := range s.Changes {
			if change.From == "" || !lies(change.Path, folder) {
				continue
			}
			from := segments(path.Dir(change.From))
			if len(from) >= depth {
				if name := strings.Join(from[:depth], "/"); !slices.Contains(names, name) {
					names = append(names, name)
				}
			}
		}
		return names
	}
}
