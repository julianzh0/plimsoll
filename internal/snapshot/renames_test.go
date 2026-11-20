package snapshot_test

import (
	"maps"
	"testing"

	"github.com/julianzh0/plimsoll/internal/conventions"
	"github.com/julianzh0/plimsoll/internal/snapshot"
	"github.com/julianzh0/plimsoll/internal/snapshot/snapshottest"
)

// change is one changed path of a commit; from is the old path of a rename.
func change(p, from string) snapshot.FileChange {
	return snapshot.FileChange{Path: p, From: from, Added: 1}
}

// A rename carries a file's earlier changes to its name at the head, along the commit graph only.
func TestTouchesFollowRenamesAlongTheGraph(t *testing.T) {
	commit := func(id string, files ...snapshot.FileChange) snapshot.Commit {
		return snapshot.Commit{ID: id, Files: files}
	}
	cases := []struct {
		name    string
		commits []snapshot.Commit // newest first, non-merge
		parents map[string][]string
		want    map[string]snapshot.Touches
	}{
		{"an edit before a rename counts for the new name; a path reused after it keeps its own",
			[]snapshot.Commit{commit("c3", change("a.go", "")), commit("c2", change("b.go", "a.go")), commit("c1", change("a.go", ""))},
			map[string][]string{"c3": {"c2"}, "c2": {"c1"}, "c1": nil},
			map[string]snapshot.Touches{"a.go": {All: 1, AtBase: 1}, "b.go": {All: 2, AtBase: 2}}},
		{"a commit that changed two paths of one name at the head counts once",
			[]snapshot.Commit{commit("c3", change("b.go", "a.go")), commit("c2", change("b.go", "")), commit("c1", change("a.go", ""), change("b.go", ""))},
			map[string][]string{"c3": {"c2"}, "c2": {"c1"}, "c1": nil},
			map[string]snapshot.Touches{"b.go": {All: 3, AtBase: 3}}},
		{"two files that swap names keep their histories",
			[]snapshot.Commit{commit("c2", change("b.go", "a.go"), change("a.go", "b.go")), commit("c1", change("a.go", ""), change("x.go", ""))},
			map[string][]string{"c2": {"c1"}, "c1": nil},
			map[string]snapshot.Touches{"a.go": {All: 1, AtBase: 1}, "b.go": {All: 2, AtBase: 2}, "x.go": {All: 1, AtBase: 1}}},
		{"an edit on a sibling branch keeps its path",
			[]snapshot.Commit{commit("p", change("b.go", "a.go")), commit("q", change("a.go", "")), commit("o", change("a.go", ""))},
			map[string][]string{"m": {"p", "q"}, "p": {"o"}, "q": {"o"}, "o": nil},
			map[string]snapshot.Touches{"a.go": {All: 1, AtBase: 1}, "b.go": {All: 2, AtBase: 2}}},
		{"a path two branches rename differently keeps its own name below them",
			[]snapshot.Commit{commit("p", change("b.go", "a.go")), commit("q", change("c.go", "a.go")), commit("o", change("a.go", ""))},
			map[string][]string{"m": {"p", "q"}, "p": {"o"}, "q": {"o"}, "o": nil},
			map[string]snapshot.Touches{"a.go": {All: 1, AtBase: 1}, "b.go": {All: 1, AtBase: 1}, "c.go": {All: 1, AtBase: 1}}},
		{"a rename after a merge carries the merged branch's edit",
			[]snapshot.Commit{commit("r", change("new.go", "old.go")), commit("q", change("old.go", "")), commit("p", change("x.go", ""))},
			map[string][]string{"r": {"m"}, "m": {"p", "q"}, "p": {"o"}, "q": {"o"}, "o": nil},
			map[string]snapshot.Touches{"new.go": {All: 2, AtBase: 2}, "x.go": {All: 1, AtBase: 1}}},
	}
	for _, tc := range cases {
		source := snapshot.Source{Commits: tc.commits, Names: snapshot.NamesAtHead(tc.commits, tc.parents)}
		if got := snapshot.Build(source, conventions.Default(), snapshottest.Readings(t)).History.Touches(); !maps.Equal(got, tc.want) {
			t.Errorf("%s: Touches() = %v, want %v", tc.name, got, tc.want)
		}
	}
}
