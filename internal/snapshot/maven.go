package snapshot

import (
	"bytes"
	"encoding/xml"
	"strings"
)

// pomProject is what the reader needs of a pom.xml: its root element, whether it lists modules, and the
// dependencies it declares for itself. Managed, plugin and profile dependencies lie on other paths.
type pomProject struct {
	XMLName      xml.Name
	Modules      *struct{} `xml:"modules"`
	Dependencies *struct {
		Dependency []struct {
			GroupID    string `xml:"groupId"`
			ArtifactID string `xml:"artifactId"`
			Scope      string `xml:"scope"`
		} `xml:"dependency"`
	} `xml:"dependencies"`
}

// pomDependencies reads the dependencies a Maven project declares for itself: the <dependency> entries directly
// under <project><dependencies> whose scope is compile (the default) or runtime, named "groupId:artifactId".
// manifest is false when the root element is not project, or when the file lists modules and declares no
// dependencies of its own; readable is false when the XML cannot be decoded, an encoding other than UTF-8 included.
// NOTE: dependencies inherited from a parent, declared in a profile or in a module's own pom.xml are not counted.
// Upgrade: read every tracked pom.xml.
func pomDependencies(content []byte) (names []string, manifest, readable bool) {
	var project pomProject
	if err := xml.Unmarshal(bytes.TrimPrefix(content, []byte(byteOrderMark)), &project); err != nil {
		return nil, true, false
	}
	switch {
	case project.XMLName.Local != "project":
		return nil, false, true
	case project.Dependencies == nil && project.Modules != nil:
		return nil, false, true
	}
	names = []string{}
	if project.Dependencies == nil {
		return names, true, true
	}
	for _, d := range project.Dependencies.Dependency {
		switch strings.TrimSpace(d.Scope) {
		case "", "compile", "runtime":
			names = append(names, strings.TrimSpace(d.GroupID)+":"+strings.TrimSpace(d.ArtifactID))
		}
	}
	return names, true, true
}
