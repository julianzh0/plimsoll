package snapshot_test

import (
	"path"
	"slices"
	"strings"
	"testing"

	"github.com/julianzh0/plimsoll/internal/conventions"
	"github.com/julianzh0/plimsoll/internal/snapshot"
	"github.com/julianzh0/plimsoll/internal/snapshot/snapshottest"
)

// Any content under every lockfile name ends, and no manifest has more unlocked names than dependencies.
func FuzzBuildReadsAnyLockfile(f *testing.F) {
	for _, seed := range []string{
		`{"lockfileVersion": 3, "packages": {"node_modules/a": {}}}`, "lockfileVersion: '9.0'\npackages:\n  a@1.0.0:\n",
		"# yarn lockfile v1\na@^1:\n  version \"1\"\n", "__metadata:\n  version: 8\n\"a@npm:^1\":\n", "packages:\n  a:\n",
		"a v1 h1:x\n", "version = 4\n[[package]]\nname = \"a\"\n", `{"pins": [{"identity": "a"}], "version": 2}`,
	} {
		f.Add(seed)
	}
	readings := snapshottest.Readings(f)
	f.Fuzz(func(t *testing.T, lock string) {
		files := map[string]string{
			"package.json": `{"dependencies": {"a": "1"}}`, "package-lock.json": lock, "web/package.json": `{"dependencies": {"a": "1"}}`,
			"web/pnpm-lock.yaml": lock, "app/package.json": `{"dependencies": {"a": "1"}}`, "app/yarn.lock": lock,
			"pubspec.yaml": "name: x\ndependencies:\n  a: ^1\n", "pubspec.lock": lock, "go.mod": "module m\n\nrequire a v1\n", "go.sum": lock,
			"Cargo.toml": "[package]\nname = \"x\"\n\n[dependencies]\na = \"1\"\n", "Cargo.lock": lock,
		}
		s := snapshot.Build(snapshottest.Files(nil, files), conventions.Default(), readings)
		for _, m := range s.Now.Manifests {
			if len(m.Unlocked) > len(m.Dependencies) {
				t.Fatalf("Build(lock %q): %s has more unlocked names than dependencies: %+v", lock, m.Path, m)
			}
		}
	})
}

// Any content under every manifest name ends, and every dependency name a manifest gives has its ecosystem's form: no
// text of a file reaches an answer as a name.
func FuzzBuildReadsAnyManifest(f *testing.F) {
	for _, seed := range []string{
		`{"dependencies": {"a": "1"}}`, "module m\n\nrequire a/b v1\n", "[project]\ndependencies = [\"a>=1\"]\n",
		"[tool.poetry.dependencies]\npython = \"^3\"\na = \"1\"\n", "name: x\ndependencies: {a: ^1, b: {path: ../b}}\n",
		"<project><dependencies><dependency><groupId>a</groupId><artifactId>b</artifactId></dependency></dependencies></project>",
		"dependencies {\n  implementation(\"a:b:1\")\n}\n", "[package]\nname = \"x\"\n\n[dependencies]\na = { package = \"b\" }\n",
		`<Project Sdk="Microsoft.NET.Sdk"><ItemGroup><PackageReference Include="A" /></ItemGroup></Project>`,
		"let package = Package(name: \"x\", dependencies: [.package(url: \"https://x.example/a\", from: \"1.0.0\")])\n",
		`{"dependencies": ["a", {"name": "b"}]}`,
	} {
		f.Add(seed)
	}
	rules := conventions.Default()
	readings := snapshottest.Readings(f)
	f.Fuzz(func(t *testing.T, content string) {
		files := map[string]string{}
		for _, m := range rules.Manifests {
			name := strings.ReplaceAll(strings.ReplaceAll(m.File, "*", "x"), "Package@swift-x", "Package@swift-5.9")
			files["m/"+name] = content
		}
		s := snapshot.Build(snapshottest.Files(nil, files), rules, readings)
		for _, m := range s.Now.Manifests {
			i := slices.IndexFunc(rules.Manifests, func(r conventions.Manifest) bool { matched, _ := path.Match(r.File, path.Base(m.Path)); return matched })
			for _, name := range m.Dependencies {
				if i < 0 || !rules.Manifests[i].Name.MatchString(name) {
					t.Fatalf("Build(%s %q): dependency %q outside its ecosystem's form", m.Path, content, name)
				}
			}
		}
	})
}
