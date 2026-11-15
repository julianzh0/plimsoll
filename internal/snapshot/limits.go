package snapshot

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/juzhxng/plimsoll/internal/conventions"
)

// DeclaredLimit is one limit a repository's limits file sets on either side: its key, plimsoll's default, and the
// value on each side, nil where the file does not set it.
type DeclaredLimit struct {
	conventions.LimitKey
	Default     float64
	Now, Before *float64
}

// declaredLimits reads the limits file at the root of both sides. The limits of one question are those it sets now,
// each with the file as its source; a key it does not set keeps plimsoll's default. problems are what the file says
// now that is not read.
func declaredLimits(rules conventions.Rules, nowContent, beforeContent map[string][]byte) (limits conventions.Limits, declared []DeclaredLimit, problems []string) {
	now, problems := readLimits(rules, nowContent)
	before, _ := readLimits(rules, beforeContent)
	limits = rules.Limits
	for _, key := range rules.LimitKeys {
		nowValue, setNow := now[key.Key]
		beforeValue, setBefore := before[key.Key]
		if !setNow && !setBefore {
			continue
		}
		limit := limits.ByKey(key.Key)
		d := DeclaredLimit{LimitKey: key, Default: limit.Value}
		if setNow {
			d.Now = &nowValue
			*limit = conventions.Limit{Value: nowValue, Step: limit.Step, Source: "set by `" + rules.LimitsFile + "`"}
		}
		if setBefore {
			d.Before = &beforeValue
		}
		declared = append(declared, d)
	}
	return limits, declared, problems
}

// readLimits reads the [limits] table of one side's limits file: a key and a number above 0 per line, a whole number
// for a limit that counts, a comment allowed. Anything else is a problem, and the key it names keeps its default.
func readLimits(rules conventions.Rules, content map[string][]byte) (map[string]float64, []string) {
	file, tracked := content[rules.LimitsFile]
	switch {
	case !tracked:
		return nil, nil
	case file == nil:
		return nil, []string{"it could not be read"}
	}
	values := map[string]float64{}
	var problems []string
	problem := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }
	table, found := "", false
	for line := range strings.Lines(strings.TrimPrefix(string(file), byteOrderMark)) {
		if header, isHeader := tomlHeader(line); isHeader {
			table = header
			found = found || header == "limits"
			if header != "limits" && header != "" {
				problem("`[%s]` is no table plimsoll reads", header)
			}
			continue
		}
		text, _, _ := strings.Cut(line, "#")
		if strings.TrimSpace(text) == "" {
			continue
		}
		key, value, assigns := strings.Cut(text, "=")
		key = dottedName(key)
		switch {
		case table == "" && assigns:
			problem("`%s` lies outside `[limits]`", key)
			continue
		case table != "limits":
			continue
		case !assigns:
			problem("`[limits]` holds a line that is no `key = value`")
			continue
		}
		if !slices.ContainsFunc(rules.LimitKeys, func(k conventions.LimitKey) bool { return k.Key == key }) {
			problem("`%s` is no limit plimsoll knows", key)
			continue
		}
		if _, twice := values[key]; twice {
			problem("`%s` is set twice", key)
			continue
		}
		number, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		whole := rules.Limits.ByKey(key).Step >= 1
		switch {
		case err != nil || math.IsNaN(number) || math.IsInf(number, 0) || number <= 0:
			problem("`%s` is not a number above 0", key)
		case whole && number != math.Trunc(number):
			problem("`%s` is not a whole number above 0", key)
		default:
			values[key] = number
		}
	}
	if !found {
		problem("it holds no `[limits]` table")
	}
	slices.Sort(problems)
	return values, slices.Compact(problems)
}
