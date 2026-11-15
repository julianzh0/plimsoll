package answer

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
)

// perDetectorFirst is how many marks of one detector are shown before the marks of the others.
const perDetectorFirst = 2

// epsilon absorbs floating-point error in a rise: 3.8 - 3.7 is a little less than 0.1.
const epsilon = 1e-9

// ID builds a mark's identifier. It is the only place that builds one, and it is never parsed back.
func ID(detector, rule, subject string) string {
	return detector + "." + rule + ":" + subject
}

// Reading is one side of a rule: whether its condition holds there, and its value.
type Reading struct {
	Holds bool
	Value float64
}

// Compare says what changed between the base and now. step is the smallest rise that counts as grown. found is
// false when the condition holds on neither side.
func Compare(base, now Reading, step float64) (change Change, found bool) {
	switch {
	case !base.Holds && now.Holds:
		return New, true
	case base.Holds && now.Holds && now.Value-base.Value >= step-epsilon:
		return Grown, true
	case base.Holds && now.Holds:
		return Standing, true
	case base.Holds:
		return Resolved, true
	default:
		return "", false
	}
}

// Order sorts new and grown marks: a mark that another detector backs with a remark first, then new before grown,
// exact before candidate, the farthest past its limit first, then by identifier; then no more than two of one
// detector come before the others. It returns a new slice.
func Order(marks []Mark, backed map[string]bool) []Mark {
	sorted := slices.Clone(marks)
	slices.SortStableFunc(sorted, func(a, b Mark) int {
		return cmp.Or(
			cmp.Compare(rank(!backed[a.ID]), rank(!backed[b.ID])),
			cmp.Compare(rank(a.Change == Grown), rank(b.Change == Grown)),
			cmp.Compare(rank(a.Confidence == Candidate), rank(b.Confidence == Candidate)),
			cmp.Compare(distance(b), distance(a)),
			strings.Compare(a.ID, b.ID),
		)
	})
	ordered := make([]Mark, 0, len(sorted))
	var setAside []Mark
	shown := map[string]int{}
	for _, m := range sorted {
		detector := DetectorOf(m.ID)
		if shown[detector] == perDetectorFirst {
			setAside = append(setAside, m)
			continue
		}
		shown[detector]++
		ordered = append(ordered, m)
	}
	return append(ordered, setAside...)
}

// DetectorOf is the detector's name in an identifier: the text before its first dot.
func DetectorOf(id string) string {
	name, _, _ := strings.Cut(id, ".")
	return name
}

// RuleOf is the rule's name in an identifier: the text between the first dot and the first colon after it.
func RuleOf(id string) string {
	_, rest, _ := strings.Cut(id, ".")
	rule, _, _ := strings.Cut(rest, ":")
	return rule
}

func rank(later bool) int {
	if later {
		return 1
	}
	return 0
}

// distance is how far a mark is past its limit, relative to the limit; 0 without a limit.
func distance(m Mark) float64 {
	if m.Measured.Limit == nil || *m.Measured.Limit == 0 {
		return 0
	}
	return (m.Measured.Value - *m.Measured.Limit) / *m.Measured.Limit
}

// Count prints a whole number with thousands separators: 3515 is "3,515".
func Count(value float64) string {
	digits := strconv.FormatInt(int64(value), 10)
	sign := ""
	if strings.HasPrefix(digits, "-") {
		sign, digits = "-", digits[1:]
	}
	var grouped strings.Builder
	for i, digit := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			grouped.WriteByte(',')
		}
		grouped.WriteRune(digit)
	}
	return sign + grouped.String()
}

// Decimal prints a number with one decimal: 2 is "2.0".
func Decimal(value float64) string {
	return strconv.FormatFloat(value, 'f', 1, 64)
}
