package snapshot_test

import (
	"fmt"
	"os"
	"reflect"
	"sync"
	"testing"

	"github.com/julianzh0/plimsoll/internal/conventions"
	"github.com/julianzh0/plimsoll/internal/snapshot"
	"github.com/julianzh0/plimsoll/internal/snapshot/snapshottest"
)

func TestKeptReadingsEqualFreshOnes(t *testing.T) {
	rules := conventions.Default()
	samples, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatal(err)
	}
	for _, sample := range samples {
		source := snapshottest.Load(t, "testdata/"+sample.Name())
		fresh := snapshot.Build(source, rules, snapshottest.Readings(t))
		shared := snapshottest.Readings(t)
		first, second := snapshot.Build(source, rules, shared), snapshot.Build(source, rules, shared)
		together := snapshottest.Readings(t)
		var left, right snapshot.Snapshot
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); left = snapshot.Build(source, rules, together) }()
		go func() { defer wg.Done(); right = snapshot.Build(source, rules, together) }()
		wg.Wait()
		for name, got := range map[string]snapshot.Snapshot{"first": first, "second": second, "left": left, "right": right} {
			if !reflect.DeepEqual(got, fresh) {
				t.Errorf("Build(%s) on kept readings, %s = %+v, want the build on fresh ones %+v", sample.Name(), name, got.Now.Links, fresh.Now.Links)
			}
		}
	}
	files := map[string]string{}
	for i := range snapshot.KeptReadings + 1 {
		files[fmt.Sprintf("f%d.go", i)] = fmt.Sprintf("package a\n// %d\n", i)
	}
	source := snapshottest.Files(nil, files)
	fresh := snapshot.Build(source, rules, snapshottest.Readings(t))
	shared := snapshottest.Readings(t)
	snapshot.Build(source, rules, shared)
	if got := snapshot.Build(source, rules, shared); !reflect.DeepEqual(got, fresh) {
		t.Errorf("Build(%d files) twice on one readings differs from the build on fresh ones", snapshot.KeptReadings+1)
	}
}
