package gitrepo

import (
	"slices"
	"sync"
	"testing"
	"testing/synctest"
)

func TestQuestionsAtTheSameTimeShareOneHistoryRead(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := NewReader(nil)
		release := make(chan struct{})
		var mu sync.Mutex
		reads := 0
		run := func(_ []byte, args ...string) ([]byte, error) {
			mu.Lock()
			reads++
			mu.Unlock()
			<-release
			return []byte("\x01c1\x00"), nil
		}
		results := make([][]string, 3)
		var done sync.WaitGroup
		for i := range results {
			done.Go(func() {
				commits, err := r.history(run, "/repo", "c1", nil)
				if err != nil {
					t.Errorf("history = %v", err)
				}
				for _, c := range commits {
					results[i] = append(results[i], c.ID)
				}
			})
		}
		synctest.Wait() // all three are waiting: one in git, two for its result
		mu.Lock()
		if reads != 1 {
			t.Errorf("three questions at the same time read the history %d times, want once", reads)
		}
		mu.Unlock()
		close(release)
		done.Wait()
		for i, ids := range results {
			if !slices.Equal(ids, []string{"c1"}) {
				t.Errorf("question %d: commits %q, want [c1]", i, ids)
			}
		}
	})
}
