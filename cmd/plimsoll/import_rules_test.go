package main

import (
	"maps"
	"path"
	"slices"
	"strings"
	"testing"

	"github.com/julianzh0/plimsoll/internal/answer"
	"github.com/julianzh0/plimsoll/internal/conventions"
	"github.com/julianzh0/plimsoll/internal/links"
	"github.com/julianzh0/plimsoll/internal/snapshot"
	"github.com/julianzh0/plimsoll/internal/snapshot/snapshottest"
)

// guardAsRules writes the guard's table as import rules: each folder that may import only some packages becomes
// only rules (one naming the folder itself when it may import none), and each case the guard names as against the
// import direction between two folders that hold code becomes a never rule.
func guardAsRules(files map[string]string) string {
	holdsCode := func(folder string) bool {
		for name := range files {
			if strings.HasPrefix(name, folder+"/") && strings.HasSuffix(name, ".go") {
				return true
			}
		}
		return false
	}
	var never, only []string
	table := allowedImports()
	for _, folder := range slices.Sorted(maps.Keys(table)) {
		allowed := table[folder]
		switch {
		case allowed == nil:
		case len(allowed) == 0:
			only = append(only, `"`+conventions.ImportRule(folder, folder)+`"`)
		default:
			for _, target := range allowed {
				only = append(only, `"`+conventions.ImportRule(folder, target)+`"`)
			}
		}
	}
	for _, c := range guardCases() {
		if !strings.Contains(c.want, againstTheImportDirection) {
			continue
		}
		from, to := path.Dir(c.file.Path), strings.TrimPrefix(c.file.Imports[0], module+"/")
		if holdsCode(from) && holdsCode(to) {
			never = append(never, `"`+conventions.ImportRule(from, to)+`"`)
		}
	}
	return "[imports]\nnever = [" + strings.Join(never, ", ") + "]\nonly = [" + strings.Join(only, ", ") + "]\n"
}

func TestTheGuardTableAsImportRulesFindsWhatTheGuardFinds(t *testing.T) {
	files := moduleFiles(t, "../..")
	files[".plimsoll.toml"] = guardAsRules(files)
	ruleMarks := func(source snapshot.Source) (ids, notAnalysed []string) {
		s := snapshot.Build(source, conventions.Default(), snapshottest.Readings(t))
		f := links.Detector{}.Detect(s)
		for _, m := range f.Marks {
			if rule := answer.RuleOf(m.ID); rule == "never" || rule == "only" {
				ids = append(ids, m.ID+" "+string(m.Change)+" "+strings.Join(m.Subject.Names, "; "))
			}
		}
		for _, line := range slices.Concat(f.NotAnalysedFirst, f.NotAnalysed) {
			if strings.Contains(line, "`[imports]`") || strings.Contains(line, "import rule") {
				notAnalysed = append(notAnalysed, line)
			}
		}
		return ids, notAnalysed
	}
	if ids, lines := ruleMarks(snapshottest.Files(files, files)); len(ids) != 0 || len(lines) != 0 {
		t.Errorf("the module as it is: rule marks %q and lines %q, want none", ids, lines)
	}
	planted := maps.Clone(files)
	planted["internal/shape/planted.go"] = "package shape\n\nimport \"" + module + "/internal/conventions\"\n\nvar _ = conventions.Default\n"
	ids, _ := ruleMarks(snapshottest.Files(files, planted))
	slices.Sort(ids)
	clause := "`internal/shape/planted.go` line 3 importing the folder `internal/conventions`"
	if want := []string{"links.never:internal/shape->internal/conventions new " + clause, "links.only:internal/shape new " + clause}; !slices.Equal(ids, want) {
		t.Errorf("one planted import against the direction: rule marks %q, want %q", ids, want)
	}
}
