package snapshot

import (
	"encoding/json"
	"slices"
)

// vcpkgDependencies reads the names of the top-level dependencies of a vcpkg.json: a name, or an object with its
// name, without those marked as tools of the build host. readable is false when the file is no JSON object or an
// entry is neither.
// NOTE: the dependencies of features, the default ones included, are not counted. Upgrade: add those of the
// default features.
func vcpkgDependencies(content []byte) (names []string, manifest, readable bool) {
	var parsed struct {
		Dependencies []json.RawMessage `json:"dependencies"`
	}
	if err := json.Unmarshal(content, &parsed); err != nil {
		return nil, true, false
	}
	names = []string{}
	for _, raw := range parsed.Dependencies {
		var name string
		var entry struct {
			Name string `json:"name"`
			Host bool   `json:"host"`
		}
		switch {
		case json.Unmarshal(raw, &name) == nil:
		case json.Unmarshal(raw, &entry) == nil && entry.Name != "":
			if entry.Host {
				continue
			}
			name = entry.Name
		default:
			return nil, true, false
		}
		if !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	return names, true, true
}
