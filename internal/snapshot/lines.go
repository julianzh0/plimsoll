package snapshot

import "cmp"

// lineChanges are the line changes of the changed files, each made once, on first need.
type lineChanges struct {
	changes     []Change
	now, before map[string][]byte
	made        map[string]lineChange // by path now, or by base path for a file gone since
}

func newLineChanges(changes []Change, now, before map[string][]byte) *lineChanges {
	return &lineChanges{changes: changes, now: now, before: before, made: map[string]lineChange{}}
}

// of is the line change of the file at the path now, or of a file gone since at its base path.
func (l *lineChanges) of(change Change) lineChange {
	key := cmp.Or(change.Path, change.From)
	if made, found := l.made[key]; found {
		return made
	}
	made := compareLines(l.before[cmp.Or(change.From, change.Path)], l.now[change.Path])
	l.made[key] = made
	return made
}

// addedMask says, for each line of a function from start to end, whether the change added it.
func addedMask(added []LineRange, start, end int) []bool {
	mask := make([]bool, end-start+1)
	for _, r := range added {
		for line := max(r.Start, start); line <= min(r.End, end); line++ {
			mask[line-start] = true
		}
	}
	return mask
}

// lineText is the text of a range of lines, from 1, a last line without a newline included.
func lineText(content []byte, newlines lineIndex, r LineRange) []byte {
	return content[lineOffset(newlines, len(content), r.Start):lineOffset(newlines, len(content), r.End+1)]
}

// lineOffset is the byte offset where the line starts, from 1; past the last line, the end of the content.
func lineOffset(newlines lineIndex, size, line int) int {
	switch {
	case line <= 1:
		return 0
	case line-2 < len(newlines):
		return newlines[line-2] + 1
	}
	return size
}
