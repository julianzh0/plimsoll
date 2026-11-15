package main

import (
	"github.com/juzhxng/plimsoll/internal/answer"
	"github.com/juzhxng/plimsoll/internal/cochange"
	"github.com/juzhxng/plimsoll/internal/hotspots"
	"github.com/juzhxng/plimsoll/internal/links"
	"github.com/juzhxng/plimsoll/internal/process"
	"github.com/juzhxng/plimsoll/internal/settings"
	"github.com/juzhxng/plimsoll/internal/shape"
)

// version is plimsoll's version, changed by a release.
const version = "0.1.0"

// detectors are the detectors of the program, in the order of the report.
func detectors() []answer.Detector {
	return []answer.Detector{shape.Detector{}, process.Detector{}, settings.Detector{}, hotspots.Detector{}, cochange.Detector{}, links.Detector{}}
}
