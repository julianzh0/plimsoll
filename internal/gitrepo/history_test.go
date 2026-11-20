package gitrepo

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
)

// fakeGit answers the history commands for a repository: rev-list lists the commits a head reaches ("id parents"
// lines, newest first, from heads), and log prints every commit it is asked for. A log call blocks on gate when it is
// set, fails with fail when that is set, and is counted with the commits it was asked for.
type fakeGit struct {
	mu    sync.Mutex
	heads map[string]string
	gate  chan struct{}
	fail  error
	logs  [][]string // per log call: the commits asked for
}

func (g *fakeGit) run(input []byte, args ...string) ([]byte, error) {
	switch args[0] {
	case "rev-parse":
		return nil, nil
	case "rev-list":
		g.mu.Lock()
		defer g.mu.Unlock()
		return []byte(g.heads[args[len(args)-1]]), nil
	}
	g.mu.Lock()
	var asked []string
	if slices.Contains(args, "--stdin") {
		asked = strings.Fields(string(input))
	} else {
		for line := range strings.Lines(g.heads[args[len(args)-1]]) {
			asked = append(asked, strings.Fields(line)[0])
		}
	}
	g.logs = append(g.logs, asked)
	gate, fail := g.gate, g.fail
	g.mu.Unlock()
	if gate != nil {
		<-gate
	}
	if fail != nil {
		return nil, fail
	}
	var out strings.Builder
	for _, id := range asked {
		out.WriteString("\x01" + id + "\x00")
	}
	return []byte(out.String()), nil
}

// asked are the commits each log call was asked for.
func (g *fakeGit) asked() [][]string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return slices.Clone(g.logs)
}

func ids(k keptHistory) []string {
	var out []string
	for _, c := range k.commits {
		out = append(out, c.ID)
	}
	return out
}

func TestQuestionsAtTheSameTimeShareOneHistoryRead(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := NewReader(nil)
		git := &fakeGit{heads: map[string]string{"c1": "c1\n"}, gate: make(chan struct{})}
		results := make([][]string, 3)
		var done sync.WaitGroup
		for i := range results {
			done.Go(func() {
				kept, err := r.history(context.Background(), git.run, "/repo", "c1", nil)
				if err != nil {
					t.Errorf("history = %v", err)
				}
				results[i] = ids(kept)
			})
		}
		synctest.Wait() // all three are waiting: one in git, two for its result
		if n := len(git.asked()); n != 1 {
			t.Errorf("three questions at the same time read the history %d times, want once", n)
		}
		close(git.gate)
		done.Wait()
		for i, got := range results {
			if !slices.Equal(got, []string{"c1"}) {
				t.Errorf("question %d: commits %q, want [c1]", i, got)
			}
		}
	})
}

func TestAQuestionThatReadsTheHistoryForOthersMayBeCancelled(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := NewReader(nil)
		killed := &fakeGit{heads: map[string]string{"c1": "c1\n"}, gate: make(chan struct{}), fail: errors.New("signal: killed")}
		works := &fakeGit{heads: map[string]string{"c1": "c1\n"}}
		first, cancel := context.WithCancel(context.Background())
		var done sync.WaitGroup
		done.Go(func() {
			if _, err := r.history(first, killed.run, "/repo", "c1", nil); err == nil {
				t.Errorf("the cancelled question's history read succeeded, want its error")
			}
		})
		synctest.Wait() // the first question is inside git
		results := make([][]string, 2)
		for i := range results {
			done.Go(func() {
				kept, err := r.history(context.Background(), works.run, "/repo", "c1", nil)
				if err != nil {
					t.Errorf("waiting question %d: %v, want the history read again", i, err)
				}
				results[i] = ids(kept)
			})
		}
		synctest.Wait() // both wait for the first question's read
		cancel()
		close(killed.gate)
		done.Wait()
		if n := len(works.asked()); n != 1 || !slices.Equal(results[0], []string{"c1"}) || !slices.Equal(results[1], []string{"c1"}) {
			t.Errorf("after the reading question was cancelled: %d new reads, results %q; want one read shared by both", n, results)
		}
	})
}

func TestAWaitingQuestionReadsAgainAfterAFailedRead(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := NewReader(nil)
		failing := &fakeGit{heads: map[string]string{"c1": "c1\n"}, gate: make(chan struct{}), fail: errors.New("exit status 128")}
		works := &fakeGit{heads: map[string]string{"c1": "c1\n"}}
		var done sync.WaitGroup
		done.Go(func() {
			if _, err := r.history(context.Background(), failing.run, "/repo", "c1", nil); err == nil {
				t.Errorf("the failing read succeeded, want its error")
			}
		})
		synctest.Wait() // the first question is inside git
		var waited []string
		done.Go(func() {
			kept, err := r.history(context.Background(), works.run, "/repo", "c1", nil)
			if err != nil {
				t.Errorf("waiting question: %v, want the history read again", err)
			}
			waited = ids(kept)
		})
		synctest.Wait() // the second waits for the first's read
		close(failing.gate)
		done.Wait()
		if !slices.Equal(waited, []string{"c1"}) {
			t.Errorf("after another question's read failed: commits %q, want [c1] from a read of its own", waited)
		}
	})
}

func TestAWaitingQuestionMayBeCancelled(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := NewReader(nil)
		slow := &fakeGit{heads: map[string]string{"c1": "c1\n"}, gate: make(chan struct{})}
		var done sync.WaitGroup
		done.Go(func() { _, _ = r.history(context.Background(), slow.run, "/repo", "c1", nil) })
		synctest.Wait()
		waiting, cancel := context.WithCancel(context.Background())
		returned := make(chan error, 1)
		go func() {
			_, err := r.history(waiting, slow.run, "/repo", "c1", nil)
			returned <- err
		}()
		synctest.Wait()
		cancel()
		synctest.Wait() // the waiting question returns while the read still runs
		select {
		case err := <-returned:
			if err == nil {
				t.Errorf("a cancelled waiting question returned no error")
			}
		default:
			t.Errorf("a cancelled waiting question still waits for another question's read")
		}
		close(slow.gate)
		done.Wait()
	})
}

// A new head reads only the commits it adds; a commit whose parents changed (a shallow clone deepened) is read again;
// two heads alternate without a read; a failed read keeps nothing.
func TestANewHeadReadsOnlyTheCommitsItAdds(t *testing.T) {
	r := NewReader(nil)
	git := &fakeGit{heads: map[string]string{
		"c2": "c2 c1\nc1\n", "c3": "c3 c2\nc2 c1\nc1\n", "b1": "b1 c1\nc1\n",
	}}
	ask := func(head string) []string {
		t.Helper()
		kept, err := r.history(context.Background(), git.run, "/repo", head, nil)
		if err != nil {
			t.Fatalf("history(%s) = %v", head, err)
		}
		return ids(kept)
	}
	if got := ask("c2"); !slices.Equal(got, []string{"c2", "c1"}) {
		t.Fatalf("history(c2) = %q", got)
	}
	ask("c3")
	ask("b1")
	ask("c3") // c3 and b1 are the two most recent heads: no read
	want := [][]string{{"c2", "c1"}, {"c3"}, {"b1"}}
	if got := git.asked(); !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("log calls asked for %q, want %q", got, want)
	}

	git.heads["c4"] = "c4 c3\nc3 c2\nc2 c1\nc1 c0\n" // deepened: c1 now has a parent
	ask("c4")
	if got := git.asked(); !slices.Equal(got[len(got)-1], []string{"c4", "c1"}) {
		t.Errorf("after deepening, the last log asked for %q, want [c4 c1]", got[len(got)-1])
	}

	git.fail = errors.New("git failed")
	git.heads["d1"] = "d1 c4\nc4 c3\nc3 c2\nc2 c1\nc1 c0\n"
	if _, err := r.history(context.Background(), git.run, "/repo", "d1", nil); err == nil {
		t.Fatalf("a failing read succeeded")
	}
	git.fail = nil
	ask("d1")
	if got := git.asked(); !slices.Equal(got[len(got)-1], []string{"d1"}) {
		t.Errorf("after a failed read, the next asked for %q, want only [d1]: the failed read kept nothing and the rest was kept", got[len(got)-1])
	}
}

// Two reads of different heads that end close together both publish their records.
func TestTwoHeadsReadAtTheSameTimeBothStay(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := NewReader(nil)
		git := &fakeGit{heads: map[string]string{"a": "a c1\nc1\n", "b": "b c1\nc1\n"}, gate: make(chan struct{})}
		var done sync.WaitGroup
		for _, head := range []string{"a", "b"} {
			done.Go(func() { _, _ = r.history(context.Background(), git.run, "/repo", head, nil) })
		}
		synctest.Wait()
		close(git.gate)
		done.Wait()
		before := len(git.asked())
		for _, head := range []string{"a", "b"} {
			if _, err := r.history(context.Background(), git.run, "/repo", head, nil); err != nil {
				t.Fatal(err)
			}
		}
		if got := git.asked(); len(got) != before {
			t.Errorf("asking again for both heads read %q, want no read", got[before:])
		}
	})
}

// Any log output parses or fails with an error: the reader never crashes on what git prints.
func FuzzParseLog(f *testing.F) {
	for _, seed := range []string{"\x01c1\x00\n3\t1\ta.ts\x00", "\x01c1\x00\n1\t0\t\x00old.ts\x00new.ts\x00", "\x01c1\x00\n-\t-\tbin\x00", "x\x00"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, output string) {
		commits, err := parseLog([]byte(output))
		if err != nil && commits != nil {
			t.Fatalf("parseLog(%q) = %d commits and %v, want no commits with an error", output, len(commits), err)
		}
	})
}
