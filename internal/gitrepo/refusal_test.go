package gitrepo

import (
	"errors"
	"testing"

	"github.com/julianzh0/plimsoll/internal/snapshot"
)

func TestRefusalKeepsItsKindOnlyForGitsOwnAnswer(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want snapshot.RefusalKind
	}{
		{"git answered no", &gitError{code: 1, line: ""}, snapshot.NoCommit},
		{"git refused an option it does not know", &gitError{code: 129, line: "error: unknown option `attr-source'"}, snapshot.GitFailed},
		{"git could not start", errors.New("exec: git not found"), snapshot.GitFailed},
	}
	for _, tc := range cases {
		got, ok := errors.AsType[*snapshot.Refusal](refusal(tc.err, snapshot.NoCommit, ""))
		if !ok || got.Kind != tc.want {
			t.Errorf("refusal(%s) = %+v, want kind %d", tc.name, got, tc.want)
		}
	}
}
