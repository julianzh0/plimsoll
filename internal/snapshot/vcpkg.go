package snapshot

import (
	"encoding/json"
	"slices"
)

// vcpkgDependency is one dependency entry: a name, or an object with its name, whether it is a tool of the build host,
// and the features it turns on.
type vcpkgDependency struct {
	Name     string
	Host     bool
	Features []string
}

// UnmarshalJSON reads a name or an object.
func (d *vcpkgDependency) UnmarshalJSON(raw []byte) error {
	if json.Unmarshal(raw, &d.Name) == nil {
		return nil
	}
	var entry struct {
		Name     string            `json:"name"`
		Host     bool              `json:"host"`
		Features []json.RawMessage `json:"features"`
	}
	if err := json.Unmarshal(raw, &entry); err != nil {
		return err
	}
	d.Name, d.Host = entry.Name, entry.Host
	d.Features = featureNames(entry.Features)
	return nil
}

// featureNames are the names of a feature list: names, or objects with a name.
func featureNames(features []json.RawMessage) []string {
	var names []string
	for _, raw := range features {
		var name string
		var entry struct {
			Name string `json:"name"`
		}
		switch {
		case json.Unmarshal(raw, &name) == nil:
		case json.Unmarshal(raw, &entry) == nil:
			name = entry.Name
		}
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

// vcpkgDependencies reads the names of the dependencies of a vcpkg.json: those at the top level and those of the
// default features (default-features), through the features of the port itself that they turn on, each feature once,
// for every platform; tools of the build host left out. readable is false when the file is no JSON object or an entry
// is neither a name nor an object with one. Features that are not default are chosen when the build is configured
// (README, "What plimsoll does not read").
func vcpkgDependencies(content []byte) (names []string, manifest, readable bool) {
	var parsed struct {
		Name            string            `json:"name"`
		Dependencies    []vcpkgDependency `json:"dependencies"`
		DefaultFeatures []json.RawMessage `json:"default-features"`
		Features        map[string]struct {
			Dependencies []vcpkgDependency `json:"dependencies"`
		} `json:"features"`
	}
	if err := json.Unmarshal(content, &parsed); err != nil {
		return nil, true, false
	}
	names = []string{}
	pending, seen := featureNames(parsed.DefaultFeatures), map[string]bool{}
	add := func(dependencies []vcpkgDependency) bool {
		for _, d := range dependencies {
			switch {
			case d.Name == "":
				return false
			case d.Name == parsed.Name && parsed.Name != "": // a feature of the port itself turns on more of its features
				pending = append(pending, d.Features...)
			case !d.Host && !slices.Contains(names, d.Name):
				names = append(names, d.Name)
			}
		}
		return true
	}
	if !add(parsed.Dependencies) {
		return nil, true, false
	}
	for len(pending) > 0 {
		feature := pending[0]
		pending = pending[1:]
		if seen[feature] {
			continue
		}
		seen[feature] = true
		if !add(parsed.Features[feature].Dependencies) {
			return nil, true, false
		}
	}
	return names, true, true
}
