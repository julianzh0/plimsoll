package snapshot

import (
	"encoding/xml"
	"slices"
	"strings"
)

// msbuildProject is what the reader needs of a project file: its root element, the SDK it names, and the package
// references directly in its item groups. Items of a target or under Choose lie on other paths.
type msbuildProject struct {
	XMLName    xml.Name
	Sdk        string `xml:"Sdk,attr"`
	ItemGroups []struct {
		PackageReferences []struct {
			Include          string `xml:"Include,attr"`
			PrivateAttribute string `xml:"PrivateAssets,attr"`
			PrivateElement   string `xml:"PrivateAssets"`
		} `xml:"PackageReference"`
	} `xml:"ItemGroup"`
}

// csprojDependencies reads the packages a .NET project references for itself: the Include of each PackageReference
// directly in an ItemGroup of the project, under any condition, listed once. A reference whose PrivateAssets is
// "all" is a build tool that does not flow to consumers and is left out. manifest is false when the root element
// is not Project, or when the project names no SDK and holds no package reference (an old-style project, whose
// packages are listed in packages.config); readable is false when the XML cannot be decoded, an encoding other
// than UTF-8 included, or when a name comes from a property, an item list or a pattern.
// NOTE: references of Directory.Build.props, of an imported file and under Choose are not counted, and a project
// reference is no dependency. Upgrade: read the imports.
func csprojDependencies(content []byte) (names []string, manifest, readable bool) {
	var project msbuildProject
	if err := xml.Unmarshal(content, &project); err != nil {
		return nil, true, false
	}
	if project.XMLName.Local != "Project" {
		return nil, false, true
	}
	names = []string{}
	for _, group := range project.ItemGroups {
		for _, reference := range group.PackageReferences {
			name := strings.TrimSpace(reference.Include)
			switch {
			case name == "":
				continue
			case strings.ContainsAny(name, "$@;*"):
				return nil, true, false
			case strings.EqualFold(strings.TrimSpace(reference.PrivateAttribute+reference.PrivateElement), "all"):
				continue
			}
			if !slices.Contains(names, name) {
				names = append(names, name)
			}
		}
	}
	if project.Sdk == "" && len(names) == 0 {
		return nil, false, true
	}
	return names, true, true
}
