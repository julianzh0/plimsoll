package snapshot_test

import (
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
