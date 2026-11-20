package main

import (
	"github.com/julianzh0/plimsoll/internal/answer"
	"github.com/julianzh0/plimsoll/internal/cochange"
	"github.com/julianzh0/plimsoll/internal/hotspots"
	"github.com/julianzh0/plimsoll/internal/links"
	"github.com/julianzh0/plimsoll/internal/process"
	"github.com/julianzh0/plimsoll/internal/settings"
	"github.com/julianzh0/plimsoll/internal/shape"
)

// version is plimsoll's version, changed by a release.
const version = "0.10.0"

// detectors are the detectors of the program, in the order of the report.
func detectors() []answer.Detector {
	return []answer.Detector{shape.Detector{}, process.Detector{}, settings.Detector{}, hotspots.Detector{}, cochange.Detector{}, links.Detector{}}
}
