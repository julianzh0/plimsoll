package snapshot_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/julianzh0/plimsoll/internal/snapshot"
)

// TypeScript: a file's own config, extends chains, solution configs with references, and workspace packages with and
// without exports.
func TestBuildFollowsTypeScriptConfigsAndWorkspacePackages(t *testing.T) {
	imp := func(specs ...string) string {
		var lines []string
		for _, s := range specs {
			lines = append(lines, "import { x } from \""+s+"\";")
		}
		return strings.Join(lines, "\n") + "\n"
	}
	for _, tc := range []struct {
		name                          string
		files                         map[string]string
		want                          []snapshot.Link
		notFound, unreadable, outside int
	}{
		{"a nested config's paths", map[string]string{
			"web/tsconfig.json": `{"compilerOptions": {"paths": {"@/*": ["./src/*"]}}}`, "web/src/a.ts": imp("@/b"), "web/src/b.ts": "",
		}, []snapshot.Link{link("web/src/a.ts", 1, "web/src/b.ts")}, 0, 0, 0},
		{"paths inherited along extends, from the base's folder", map[string]string{
			"config/base.json": `{"compilerOptions": {"paths": {"~/*": ["../shared/*"]}}}`, "shared/s.ts": "",
			"app/tsconfig.json": `{"extends": "../config/base.json"}`, "app/a.ts": imp("~/s"),
		}, []snapshot.Link{link("app/a.ts", 1, "shared/s.ts")}, 0, 0, 0},
		{"an extends loop ends, the file's own options kept", map[string]string{
			"tsconfig.json": `{"extends": "./base.json", "compilerOptions": {"paths": {"@/*": ["./src/*"]}}}`, "base.json": `{"extends": "./tsconfig.json"}`,
			"src/a.ts": imp("@/b"), "src/b.ts": "",
		}, []snapshot.Link{link("src/a.ts", 1, "src/b.ts")}, 0, 0, 0},
		{"a later base replaces an earlier one", map[string]string{
			"one.json": `{"compilerOptions": {"paths": {"x/*": ["./one/*"]}}}`, "two.json": `{"compilerOptions": {"paths": {"x/*": ["./two/*"]}}}`,
			"tsconfig.json": `{"extends": ["./one.json", "./two"]}`, "one/m.ts": "", "two/m.ts": "", "a.ts": imp("x/m"),
		}, []snapshot.Link{link("a.ts", 1, "two/m.ts")}, 0, 0, 0},
		{"a solution's referenced config that takes the file, its path written with or without ./", map[string]string{
			"tsconfig.json":      `{"files": [], "references": [{"path": "tsconfig.app.json"}, {"path": "./tsconfig.test.json"}]}`,
			"tsconfig.app.json":  `{"include": ["src"], "compilerOptions": {"paths": {"#/*": ["./src/*"]}}}`,
			"tsconfig.test.json": `{"include": ["test"], "compilerOptions": {"paths": {"#/*": ["./test/*"]}}}`,
			"src/a.ts":           imp("#/b"), "src/b.ts": "", "test/b.ts": "",
		}, []snapshot.Link{link("src/a.ts", 1, "src/b.ts")}, 0, 0, 0},
		{"a workspace package with exports", map[string]string{
			"packages/ui/package.json": `{"name": "@acme/ui", "exports": {".": {"types": "./src/index.ts", "default": "./dist/index.js"},
				"./button": "./src/button.ts", "./icons/*": "./src/icons/*.ts", "./internal/*": null}}`,
			"packages/ui/src/index.ts": "", "packages/ui/src/button.ts": "", "packages/ui/src/icons/star.ts": "",
			"app/a.ts": imp("@acme/ui", "@acme/ui/button", "@acme/ui/icons/star", "@acme/ui/internal/x", "@acme/ui/missing"),
		}, []snapshot.Link{link("app/a.ts", 1, "packages/ui/src/index.ts"), link("app/a.ts", 2, "packages/ui/src/button.ts"),
			link("app/a.ts", 3, "packages/ui/src/icons/star.ts")}, 2, 0, 0},
		{"a workspace package without exports, its build output mapped to its sources", map[string]string{
			"packages/core/package.json":  `{"name": "core", "main": "./dist/index.js", "types": "./dist/index.d.ts"}`,
			"packages/core/tsconfig.json": `{"compilerOptions": {"outDir": "dist", "rootDir": "lib"}}`,
			"packages/core/lib/index.ts":  "", "packages/core/util.ts": "",
			"app/a.ts": imp("core", "core/util", "left-pad"),
		}, []snapshot.Link{link("app/a.ts", 1, "packages/core/lib/index.ts"), link("app/a.ts", 2, "packages/core/util.ts")}, 0, 0, 1},
		{"exclude, and include with a suffix", map[string]string{
			"tsconfig.json": `{"include": ["src/**/*.ts"], "exclude": ["src/gen"], "compilerOptions": {"paths": {"@/*": ["./src/*"]}}}`,
			"src/a.ts":      imp("@/b"), "src/b.ts": "", "src/gen/g.ts": imp("@/b"),
		}, []snapshot.Link{link("src/a.ts", 1, "src/b.ts")}, 0, 0, 1}, // the excluded file has no config: its alias is outside
		{"a package's own config with include maps its build output", map[string]string{
			"packages/core/package.json":  `{"name": "core", "main": "./dist/index.js"}`,
			"packages/core/tsconfig.json": `{"include": ["src"], "compilerOptions": {"outDir": "dist", "rootDir": "src"}}`,
			"packages/core/src/index.ts":  "", "app/a.ts": imp("core"),
		}, []snapshot.Link{link("app/a.ts", 1, "packages/core/src/index.ts")}, 0, 0, 0},
		{"of two export patterns, the longer prefix", map[string]string{
			"packages/ui/package.json":         `{"name": "ui", "exports": {"./icons/*": "./src/icons/*.ts", "./icons/brand/*": "./src/brand/*.ts"}}`,
			"packages/ui/src/icons/brand/x.ts": "", "packages/ui/src/brand/x.ts": "", "app/a.ts": imp("ui/icons/brand/x"),
		}, []snapshot.Link{link("app/a.ts", 1, "packages/ui/src/brand/x.ts")}, 0, 0, 0},
		{"build output in the root package's own folder", map[string]string{
			"package.json": `{"name": "core", "main": "./index.js"}`, "tsconfig.json": `{"compilerOptions": {"outDir": ".", "rootDir": "src"}}`,
			"src/index.ts": "", "app/a.ts": imp("core"),
		}, []snapshot.Link{link("app/a.ts", 1, "src/index.ts")}, 0, 0, 0},
		{"without exports, the first field with a source", map[string]string{
			"packages/ui/package.json": `{"name": "ui", "types": "./index.d.ts", "main": "./index.js"}`,
			"packages/ui/index.d.ts":   "", "packages/ui/index.js": "", "app/a.ts": imp("ui"),
		}, []snapshot.Link{link("app/a.ts", 1, "packages/ui/index.d.ts")}, 0, 0, 0},
		{"${configDir} in a shared base", map[string]string{
			"config/base.json":  `{"compilerOptions": {"paths": {"@/*": ["${configDir}/src/*"]}}}`,
			"app/tsconfig.json": `{"extends": "../config/base.json"}`, "app/src/a.ts": imp("@/b"), "app/src/b.ts": "",
		}, []snapshot.Link{link("app/src/a.ts", 1, "app/src/b.ts")}, 0, 0, 0},
		{"a base config from a workspace package", map[string]string{
			"packages/tsconfig/package.json": `{"name": "@repo/tsconfig"}`, "packages/tsconfig/base.json": `{"compilerOptions": {"baseUrl": ".", "paths": {"~/*": ["${configDir}/*"]}}}`,
			"apps/web/tsconfig.json": `{"extends": "@repo/tsconfig/base.json"}`, "apps/web/a.ts": imp("~/b"), "apps/web/b.ts": "",
		}, []snapshot.Link{link("apps/web/a.ts", 1, "apps/web/b.ts")}, 0, 0, 0},
		{"a package imports itself, not its copy under examples", map[string]string{
			"examples/ui/package.json": `{"name": "@acme/ui"}`, "examples/ui/index.ts": "",
			"packages/ui/package.json": `{"name": "@acme/ui"}`, "packages/ui/index.ts": "", "packages/ui/b.ts": imp("@acme/ui"),
		}, []snapshot.Link{link("packages/ui/b.ts", 1, "packages/ui/index.ts")}, 0, 0, 0},
		{"a test package named like a dependency is no workspace package", map[string]string{
			"test/fixtures/react/package.json": `{"name": "react"}`, "test/fixtures/react/index.js": "", "src/a.ts": imp("react"),
		}, nil, 0, 0, 1},
		{"a target with no tracked source is not readable", map[string]string{
			"packages/gen/package.json": `{"name": "gen", "main": "./dist/index.js"}`, "app/a.ts": imp("gen"),
		}, nil, 0, 1, 0},
	} {
		got := linksOf(t, tc.files)
		l := got.Languages[0]
		if !reflect.DeepEqual(got.List, tc.want) || l.NotFound != tc.notFound || l.Unreadable != tc.unreadable || l.Outside != tc.outside {
			t.Errorf("%s: List = %+v, %+v; want %+v, not found %d, not readable %d, outside %d", tc.name, got.List, l, tc.want,
				tc.notFound, tc.unreadable, tc.outside)
		}
	}
}
