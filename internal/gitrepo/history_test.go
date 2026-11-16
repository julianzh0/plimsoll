package gitrepo

import (
	"context"
	"errors"
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
				commits, err := r.history(context.Background(), run, "/repo", "c1", nil)
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

func TestAQuestionThatReadsTheHistoryForOthersMayBeCancelled(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := NewReader(nil)
		release := make(chan struct{})
		killed := func(_ []byte, _ ...string) ([]byte, error) { // git, stopped with the question that started it
			<-release
			return nil, errors.New("signal: killed")
		}
		var mu sync.Mutex
		reads := 0
		works := func(_ []byte, _ ...string) ([]byte, error) {
			mu.Lock()
			reads++
			mu.Unlock()
			return []byte("\x01c1\x00"), nil
		}
		first, cancel := context.WithCancel(context.Background())
		var done sync.WaitGroup
		done.Go(func() {
			if _, err := r.history(first, killed, "/repo", "c1", nil); err == nil {
				t.Errorf("the cancelled question's history read succeeded, want its error")
			}
		})
		synctest.Wait() // the first question is inside git
		results := make([][]string, 2)
		for i := range results {
			done.Go(func() {
				commits, err := r.history(context.Background(), works, "/repo", "c1", nil)
				if err != nil {
					t.Errorf("waiting question %d: %v, want the history read again", i, err)
				}
				for _, c := range commits {
					results[i] = append(results[i], c.ID)
				}
			})
		}
		synctest.Wait() // both wait for the first question's read
		cancel()
		close(release)
		done.Wait()
		if reads != 1 || !slices.Equal(results[0], []string{"c1"}) || !slices.Equal(results[1], []string{"c1"}) {
			t.Errorf("after the reading question was cancelled: %d new reads, results %q; want one read shared by both", reads, results)
		}
	})
}

func TestAWaitingQuestionMayBeCancelled(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := NewReader(nil)
		release := make(chan struct{})
		slow := func(_ []byte, _ ...string) ([]byte, error) {
			<-release
			return []byte("\x01c1\x00"), nil
		}
		var done sync.WaitGroup
		done.Go(func() { _, _ = r.history(context.Background(), slow, "/repo", "c1", nil) })
		synctest.Wait()
		waiting, cancel := context.WithCancel(context.Background())
		returned := make(chan error, 1)
		go func() {
			_, err := r.history(waiting, slow, "/repo", "c1", nil)
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
		close(release)
		done.Wait()
	})
}
