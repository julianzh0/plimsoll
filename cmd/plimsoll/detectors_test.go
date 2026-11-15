package main

import (
	"slices"
	"testing"
)

func TestDetectorsAreListedInReportOrder(t *testing.T) {
	var names []string
	for _, detector := range detectors() {
		names = append(names, detector.Name())
	}
	if want := []string{"shape", "process", "settings", "hotspots", "co-change", "links"}; !slices.Equal(names, want) {
		t.Errorf("detectors() = %v, want %v", names, want)
	}
}
