package snapshot

import (
	"cmp"
	"encoding/xml"
	"strings"
)

// pomProject is what the reader needs of a pom.xml: its root element, its own and its parent's group, its properties,
// whether it lists modules, and the dependencies it declares for itself. Managed, plugin and profile dependencies lie
// on other paths.
type pomProject struct {
	XMLName    xml.Name
	GroupID    string `xml:"groupId"`
	ArtifactID string `xml:"artifactId"`
	Parent     struct {
		GroupID string `xml:"groupId"`
	} `xml:"parent"`
	Properties struct {
		Entries []struct {
			XMLName xml.Name
			Value   string `xml:",chardata"`
		} `xml:",any"`
	} `xml:"properties"`
	Modules      *struct{} `xml:"modules"`
	Dependencies *struct {
		Dependency []struct {
			GroupID    string `xml:"groupId"`
			ArtifactID string `xml:"artifactId"`
			Scope      string `xml:"scope"`
			Type       string `xml:"type"`
			Classifier string `xml:"classifier"`
		} `xml:"dependency"`
	} `xml:"dependencies"`
}

// pomDependencies reads the dependencies a Maven project declares for itself: the <dependency> entries directly
// under <project><dependencies> whose scope is compile (the default) or runtime, named "groupId:artifactId", with
// ${project.groupId} and the properties the same pom sets filled in; a pom without a groupId takes its parent's. A
// placeholder that stays is kept as written. manifest is false when the root element is not project, or when the
// file lists modules and declares no dependencies of its own; readable is false when the XML cannot be decoded, an
// encoding other than UTF-8 included. Profiles, a parent outside the repository and properties set outside the pom
// are known only to Maven itself (README, "What plimsoll does not read").
func pomDependencies(content []byte) (names []string, manifest, readable bool) {
	var project pomProject
	if err := xml.Unmarshal(content, &project); err != nil {
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
	group := strings.TrimSpace(project.GroupID)
	if group == "" {
		group = strings.TrimSpace(project.Parent.GroupID)
	}
	pairs := []string{}
	if group != "" {
		pairs = append(pairs, "${project.groupId}", group, "${pom.groupId}", group)
	}
	for _, p := range project.Properties.Entries {
		pairs = append(pairs, "${"+p.XMLName.Local+"}", strings.TrimSpace(p.Value))
	}
	fill := strings.NewReplacer(pairs...)
	for _, d := range project.Dependencies.Dependency {
		switch strings.TrimSpace(d.Scope) {
		case "", "compile", "runtime":
			names = appendNew(names, fill.Replace(strings.TrimSpace(d.GroupID))+":"+fill.Replace(strings.TrimSpace(d.ArtifactID)))
		}
	}
	return names, true, true
}

// pomTestJars reads a pom's own coordinate ("groupId:artifactId", the parent's group when it has none) and the
// coordinates of the modules whose tests it depends on: a dependency of type test-jar or classifier tests. Nothing
// when the XML cannot be read.
func pomTestJars(content []byte) (coordinate string, testJars []string) {
	var project pomProject
	if err := xml.Unmarshal(content, &project); err != nil || project.XMLName.Local != "project" {
		return "", nil
	}
	group := cmp.Or(strings.TrimSpace(project.GroupID), strings.TrimSpace(project.Parent.GroupID))
	if project.Dependencies != nil {
		for _, d := range project.Dependencies.Dependency {
			if strings.TrimSpace(d.Type) == "test-jar" || strings.TrimSpace(d.Classifier) == "tests" {
				testJars = append(testJars, strings.ReplaceAll(strings.TrimSpace(d.GroupID), "${project.groupId}", group)+":"+strings.TrimSpace(d.ArtifactID))
			}
		}
	}
	return group + ":" + strings.TrimSpace(project.ArtifactID), testJars
}
