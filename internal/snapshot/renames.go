package snapshot

import (
	"maps"
	"slices"
)

// namesAtHead gives, per commit, the names its renamed paths have at the head: a rename git reported in a commit
// carries the changes its file had in that commit's ancestors to the file's later name, along the commit graph,
// merges included. Names flow from each commit to its parents; a path only one branch renamed takes that name below
// the fork; where two branches renamed it differently, it keeps its own name below them, so each change is counted
// once. A path that is no key keeps its name. nil when the graph is not known.
func namesAtHead(commits []Commit, parents map[string][]string) map[string]map[string]string {
	if len(parents) == 0 {
		return nil
	}
	renames := map[string]map[string]string{} // per commit: new path -> old path
	for _, c := range commits {
		for _, f := range c.Files {
			if f.From != "" {
				if renames[c.ID] == nil {
					renames[c.ID] = map[string]string{}
				}
				renames[c.ID][f.Path] = f.From
			}
		}
	}
	children := make(map[string]int, len(parents))
	for _, ps := range parents {
		for _, p := range ps {
			children[p]++
		}
	}
	var ready []string // Kahn's order from the head down: a commit is taken once all its children are
	for id := range parents {
		if children[id] == 0 {
			ready = append(ready, id)
		}
	}
	slices.Sort(ready)
	names := make(map[string]map[string]string, len(parents)) // per commit: path after it -> name at the head
	received := map[string]bool{}                             // commits that took the names of a child
	conflicted := map[string]map[string]bool{}                // per commit: paths its children name differently
	for len(ready) > 0 {
		id := ready[len(ready)-1]
		ready = ready[:len(ready)-1]
		below := beforeRenames(names[id], renames[id])
		for _, p := range parents[id] {
			if !received[p] {
				names[p], received[p] = below, true
			} else if !maps.Equal(names[p], below) {
				if conflicted[p] == nil {
					conflicted[p] = map[string]bool{}
				}
				names[p] = joined(names[p], below, conflicted[p])
			}
			if children[p]--; children[p] == 0 {
				ready = append(ready, p)
			}
		}
	}
	return names
}

// beforeRenames are the names of the paths before a commit, from the names after it and the renames it made: an old
// path takes the name of its new path; a new path that was no old one was another file, or none, before.
func beforeRenames(after map[string]string, renames map[string]string) map[string]string {
	if len(renames) == 0 {
		return after
	}
	before := maps.Clone(after)
	if before == nil {
		before = map[string]string{}
	}
	moved := make(map[string]string, len(renames))
	for newPath, oldPath := range renames {
		moved[oldPath] = nameOf(after, newPath)
	}
	for newPath := range renames {
		if _, alsoOld := moved[newPath]; !alsoOld {
			delete(before, newPath)
		}
	}
	maps.Copy(before, moved)
	return before
}

// nameOf is a path's name at the head under a commit's names.
func nameOf(names map[string]string, p string) string {
	if name, found := names[p]; found {
		return name
	}
	return p
}

// joined are the names two children give one parent: a path only one of them renames takes that name, as the merge
// that joins the two branches carries the file under it; a path both rename differently is conflicted and keeps its
// own name from then on.
func joined(a, b map[string]string, conflicted map[string]bool) map[string]string {
	out := map[string]string{}
	for _, side := range []map[string]string{a, b} {
		for p, name := range side {
			if conflicted[p] {
				continue
			}
			if other, found := out[p]; found && other != name {
				conflicted[p] = true
				delete(out, p)
				continue
			}
			out[p] = name
		}
	}
	return out
}
