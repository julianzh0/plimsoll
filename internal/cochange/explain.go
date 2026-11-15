package cochange

import (
	"fmt"

	"github.com/juzhxng/plimsoll/internal/answer"
	"github.com/juzhxng/plimsoll/internal/snapshot"
)

// Explain counts the pair's commits again and lists its shared commits, newest first.
func (Detector) Explain(s snapshot.Snapshot, m answer.Mark) []string {
	x, y := m.Subject.Paths[0], m.Subject.Paths[1]
	var shared []string
	sharedNow, sharedBase, either := 0, 0, 0
	for _, commit := range s.History.Commits {
		if !commit.Usable {
			continue
		}
		hasX, hasY := lists(commit, x), lists(commit, y)
		if hasX || hasY {
			either++
		}
		if !hasX || !hasY {
			continue
		}
		sharedNow++
		line := "commit " + answer.Short(commit.ID)
		if commit.Since {
			line += ", since the base"
		} else {
			sharedBase++
		}
		shared = append(shared, line)
	}
	roles := eligible(s)
	return append([]string{
		fmt.Sprintf("shared usable commits: %d; at the base: %d", sharedNow, sharedBase),
		fmt.Sprintf("usable commits that touched either file: %d", either),
		fmt.Sprintf("`%s`: %s; `%s`: %s", x, roles[x].Role, y, roles[y].Role),
	}, shared...)
}

func lists(commit snapshot.Commit, path string) bool {
	for _, change := range commit.Files {
		if change.Path == path {
			return true
		}
	}
	return false
}
