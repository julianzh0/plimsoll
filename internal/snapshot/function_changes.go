package snapshot

import (
	"cmp"
	"slices"

	"github.com/julianzh0/plimsoll/internal/conventions"
)

// measuredFiles are the paths whose functions are measured on each side: now, the changed product files whose entry
// has function tables; at the base, their old paths and the changed product files gone since.
func measuredFiles(rules conventions.Rules, changes []Change, now, before Tree) (nowPaths, basePaths map[string]bool) {
	nowPaths, basePaths = map[string]bool{}, map[string]bool{}
	role := func(t Tree) map[string]File {
		files := map[string]File{}
		for _, f := range t.Files {
			files[f.Path] = f
		}
		return files
	}
	nowFiles, baseFiles := role(now), role(before)
	measures := func(f File, found bool) bool {
		if !found || f.Role != Product {
			return false
		}
		language, known := rules.LanguageOf(f.Path)
		return known && language.Functions != nil
	}
	for _, change := range changes {
		f, found := nowFiles[change.Path]
		if measures(f, found) {
			nowPaths[change.Path] = true
			basePaths[cmp.Or(change.From, change.Path)] = true
			continue
		}
		if b, atBase := baseFiles[cmp.Or(change.From, change.Path)]; !found && measures(b, atBase) {
			basePaths[b.Path] = true
		}
	}
	return nowPaths, basePaths
}

// candidate is a function now that crossed a rule, with its partner at the base when it has one.
type candidate struct {
	change  FunctionChange
	complex bool // crossed the complexity rule with a reason
	deeper  bool // crossed the nesting rule
}

// functionChanges finds the functions of the changed product files that the change made complex or nested deeper.
func functionChanges(rules conventions.Rules, changes []Change, nowPaths map[string]bool, nowReadings, beforeReadings map[string]reading) Functions {
	fr := rules.FunctionRules
	limit, nest := rules.Limits.FunctionComplexity, rules.Limits.FunctionNesting
	result := Functions{PerMeasure: fr.PerMeasure, MovedShare: fr.MovedShare, CopyShare: fr.CopyShare, MinifiedLineBytes: fr.MinifiedLineBytes}
	var candidates []candidate
	for _, change := range changes {
		if !nowPaths[change.Path] {
			continue
		}
		r := nowReadings[change.Path]
		result.Files++
		basePath := cmp.Or(change.From, change.Path)
		base := byName(beforeReadings[basePath].functions)
		seen := map[string]int{}
		for _, f := range r.functions {
			seen[f.name]++
			if f.broken {
				result.Broken++
				continue
			}
			result.Measured++
			c := FunctionChange{Path: change.Path, Name: f.name, Occurrence: seen[f.name], Start: f.startLine, End: f.endLine,
				Complexity: f.complexity, Nesting: f.nesting, NestingLine: f.nestingLine, New: true}
			partners := base[f.name]
			if len(partners) >= seen[f.name] {
				b := partners[seen[f.name]-1]
				if b.broken {
					result.Broken++
					continue
				}
				c.New, c.BasePath, c.BaseStart, c.BaseEnd = false, basePath, b.startLine, b.endLine
				c.ComplexityBefore, c.NestingBefore = b.complexity, b.nesting
			}
			complexRise := c.Complexity - c.ComplexityBefore
			crossed := float64(c.Complexity) > limit.Value &&
				(c.New || float64(c.ComplexityBefore) <= limit.Value && float64(complexRise) >= limit.Step)
			deeper := float64(c.Nesting) >= nest.Value && (c.New || c.Nesting > c.NestingBefore)
			_, tracked := beforeReadings[basePath]
			if c.New && !tracked && len(f.repeats) > 0 {
				first := f.repeats[0].copies
				c.Repeated = []LineRange{{first[0].startLine, first[0].endLine}, {first[1].startLine, first[1].endLine}}
			}
			reason := c.Nesting >= fr.ReasonNesting || c.Repeated != nil
			if crossed && reason || deeper {
				candidates = append(candidates, candidate{change: c, complex: crossed && reason, deeper: deeper})
			}
		}
	}
	var complexList, deeperList []FunctionChange
	for _, c := range candidates {
		if c.complex {
			complexList = append(complexList, c.change)
		}
	}
	slices.SortFunc(complexList, func(a, b FunctionChange) int {
		return cmp.Or(cmp.Compare(b.Complexity-b.ComplexityBefore, a.Complexity-a.ComplexityBefore), cmp.Compare(b.Complexity, a.Complexity), byPlace(a, b))
	})
	result.ComplexCrossed = len(complexList)
	result.Complex = complexList[:min(len(complexList), fr.PerMeasure)]
	kept := map[[2]string]bool{}
	for _, c := range result.Complex {
		kept[[2]string{c.Path, c.Name}] = true
	}
	for _, c := range candidates {
		if c.deeper && !kept[[2]string{c.change.Path, c.change.Name}] {
			deeperList = append(deeperList, c.change)
		}
	}
	slices.SortFunc(deeperList, func(a, b FunctionChange) int {
		return cmp.Or(cmp.Compare(b.Nesting-b.NestingBefore, a.Nesting-a.NestingBefore), cmp.Compare(b.Complexity, a.Complexity), byPlace(a, b))
	})
	result.DeeperCrossed = len(deeperList)
	result.Deeper = deeperList[:min(len(deeperList), fr.PerMeasure)]
	return result
}

// byPlace orders two functions by path, name and line.
func byPlace(a, b FunctionChange) int {
	return cmp.Or(cmp.Compare(a.Path, b.Path), cmp.Compare(a.Name, b.Name), cmp.Compare(a.Start, b.Start))
}

// byName groups a file's functions by name, in text order.
func byName(functions []function) map[string][]function {
	found := map[string][]function{}
	for _, f := range functions {
		found[f.name] = append(found[f.name], f)
	}
	return found
}
