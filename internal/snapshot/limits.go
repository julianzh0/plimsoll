package snapshot

import "github.com/julianzh0/plimsoll/internal/conventions"

// DeclaredLimit is one limit a repository's limits file sets on either side: its key, plimsoll's default, and the
// value on each side, nil where the file does not set it.
type DeclaredLimit struct {
	conventions.LimitKey
	Default     float64
	Now, Before *float64
	Compared    bool // the own file was read on both sides, so a change of the limit is known
}

// declaredLimits are the limits of one question, from the own file of both sides: those it sets now, each with the
// file as its source; a key it does not set keeps plimsoll's default.
func declaredLimits(rules conventions.Rules, now, before ownFile) (limits conventions.Limits, declared []DeclaredLimit) {
	limits = rules.Limits
	for _, key := range rules.LimitKeys {
		nowValue, setNow := now.limits[key.Key]
		beforeValue, setBefore := before.limits[key.Key]
		if !setNow && !setBefore {
			continue
		}
		limit := limits.ByKey(key.Key)
		d := DeclaredLimit{LimitKey: key, Default: limit.Value, Compared: !now.unreadable && !before.unreadable}
		if setNow {
			d.Now = &nowValue
			*limit = conventions.Limit{Value: nowValue, Step: limit.Step, Source: "set by `" + rules.OwnFile + "`"}
		}
		if setBefore {
			d.Before = &beforeValue
		}
		declared = append(declared, d)
	}
	return limits, declared
}
