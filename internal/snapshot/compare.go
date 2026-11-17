package snapshot

import "bytes"

// compareCells is the most cells one comparison may keep for reading its path back: past it the lines between the two
// contents' common first and last lines count as replaced.
// NOTE: provisional; it bounds memory at 16 MiB (4-byte cells) per comparison. Task 15 derives it from the largest
// changed file pairs of the field repositories; a pair over it is compared only by its common ends.
const compareCells = 4_000_000

// lineChange is what differs between two versions of a file, as line ranges from 1.
type lineChange struct {
	added   []LineRange // lines of the content now
	removed []LineRange // lines of the base content
}

// compareLines aligns the lines of two contents by their bytes (a line keeps its "\r").
func compareLines(base, now []byte) lineChange {
	a, b := interned(splitLines(base), splitLines(now))
	keptA, keptB := common(a, b)
	return lineChange{added: dropped(keptB), removed: dropped(keptA)}
}

// alike is how alike two lists of lines are: twice the lines they share in order, over the lines of both; 0 for two
// empty lists.
func alike(a, b [][]byte) float64 {
	if len(a)+len(b) == 0 {
		return 0
	}
	ia, ib := interned(a, b)
	keptA, _ := common(ia, ib)
	shared := 0
	for _, k := range keptA {
		if k {
			shared++
		}
	}
	return 2 * float64(shared) / float64(len(a)+len(b))
}

// splitLines splits content at "\n"; a last newline ends a line, it does not start one.
func splitLines(content []byte) [][]byte {
	if len(content) == 0 {
		return nil
	}
	lines := bytes.SplitAfter(content, []byte("\n"))
	if len(lines[len(lines)-1]) == 0 {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// interned gives each distinct line of both lists one number.
func interned(a, b [][]byte) ([]int32, []int32) {
	ids := map[string]int32{}
	number := func(lines [][]byte) []int32 {
		out := make([]int32, len(lines))
		for i, l := range lines {
			id, found := ids[string(l)]
			if !found {
				id = int32(len(ids))
				ids[string(l)] = id
			}
			out[i] = id
		}
		return out
	}
	return number(a), number(b)
}

// common marks the lines of a and b that a shortest edit script keeps: the common first and last lines, and between
// them Myers' algorithm while it stays within compareCells; past that the lines between are not kept.
func common(a, b []int32) (keptA, keptB []bool) {
	keptA, keptB = make([]bool, len(a)), make([]bool, len(b))
	lo := 0
	for lo < len(a) && lo < len(b) && a[lo] == b[lo] {
		keptA[lo], keptB[lo] = true, true
		lo++
	}
	hiA, hiB := len(a), len(b)
	for hiA > lo && hiB > lo && a[hiA-1] == b[hiB-1] {
		hiA--
		hiB--
		keptA[hiA], keptB[hiB] = true, true
	}
	myers(a[lo:hiA], b[lo:hiB], keptA[lo:hiA], keptB[lo:hiB])
	return keptA, keptB
}

// myers marks the lines of a and b on a shortest edit script's diagonals. It keeps, for each step d, the furthest
// reach of each diagonal before the step, and reads the path back from them; it gives up, marking nothing, when those
// would pass compareCells.
func myers(a, b []int32, keptA, keptB []bool) {
	n, m := len(a), len(b)
	if n == 0 || m == 0 {
		return
	}
	limit := n + m
	v := make([]int32, 2*limit+3) // v[k+limit+1]: the furthest x on diagonal k
	at := func(k int) *int32 { return &v[k+limit+1] }
	var trace [][]int32 // trace[d]: the reach of diagonals -(d-1)..d-1 before step d
	cells := 0
	for d := 0; d <= limit; d++ {
		cells += 2*d + 1
		if cells > compareCells {
			return
		}
		trace = append(trace, append([]int32(nil), v[limit+1-max(d-1, 0):limit+1+max(d-1, 0)+1]...))
		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || k != d && *at(k - 1) < *at(k + 1) {
				x = int(*at(k + 1))
			} else {
				x = int(*at(k - 1)) + 1
			}
			y := x - k
			for x < n && y < m && a[x] == b[y] {
				x++
				y++
			}
			*at(k) = int32(x)
			if x >= n && y >= m {
				walkBack(trace, n, m, keptA, keptB)
				return
			}
		}
	}
}

// walkBack follows the path from (n, m) to (0, 0) through the kept reaches, marking the lines of its diagonal moves.
func walkBack(trace [][]int32, n, m int, keptA, keptB []bool) {
	x, y := n, m
	for d := len(trace) - 1; d >= 0; d-- {
		prevX, prevY := 0, 0
		if d > 0 {
			before := trace[d]
			reach := func(k int) int { return int(before[k+d-1]) }
			k := x - y
			prevK := k - 1
			if k == -d || k != d && reach(k-1) < reach(k+1) {
				prevK = k + 1
			}
			prevX = reach(prevK)
			prevY = prevX - prevK
		}
		for x > prevX && y > prevY {
			x--
			y--
			keptA[x], keptB[y] = true, true
		}
		x, y = prevX, prevY
	}
}

// dropped turns the lines not kept into ranges, from 1.
func dropped(kept []bool) []LineRange {
	var ranges []LineRange
	for i, k := range kept {
		switch {
		case k:
		case len(ranges) > 0 && ranges[len(ranges)-1].End == i:
			ranges[len(ranges)-1].End = i + 1
		default:
			ranges = append(ranges, LineRange{Start: i + 1, End: i + 1})
		}
	}
	return ranges
}
