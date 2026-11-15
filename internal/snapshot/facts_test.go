package snapshot_test

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/juzhxng/plimsoll/internal/conventions"
	"github.com/juzhxng/plimsoll/internal/snapshot"
	"github.com/juzhxng/plimsoll/internal/snapshot/snapshottest"
)

func TestBuildReadsSettingNamesNeverValues(t *testing.T) {
	source := snapshottest.Files(nil, map[string]string{".env.example": "PORT=8080\n# DEBUG=true\nlowercase=x\n\nPORT=9090\n"})
	s := snapshot.Build(source, conventions.Default())
	if s.Now.Settings == nil || !reflect.DeepEqual(s.Now.Settings.Names, []string{"PORT", "DEBUG", "PORT"}) {
		t.Fatalf("Build(.env.example).Now.Settings = %+v, want PORT, DEBUG, PORT", s.Now.Settings)
	}
	if printed := fmt.Sprintf("%+v", s); strings.Contains(printed, "8080") || strings.Contains(printed, "9090") {
		t.Errorf("Build(.env.example) holds a setting's value: %s", printed)
	}
	if got := snapshot.Build(snapshottest.Files(nil, map[string]string{"src/a.ts": "x\n"}), conventions.Default()).Now.Settings; got != nil {
		t.Errorf("Build(no .env.example).Now.Settings = %+v, want nil", got)
	}
	unreadable := snapshottest.Files(nil, map[string]string{".env.example": "PORT=1\n"})
	unreadable.Now[0].Content, unreadable.Now[0].Binary = nil, true
	broken := snapshot.Build(unreadable, conventions.Default())
	if broken.Now.Settings != nil || !slices.Contains(broken.Now.Broken, ".env.example") || !slices.Contains(broken.Unreadable, ".env.example") {
		t.Errorf("Build(unreadable .env.example) = settings %+v, broken %v, unreadable %v; want none, and it named twice", broken.Now.Settings, broken.Now.Broken, broken.Unreadable)
	}
}

func TestBuildReadsRuntimeDependencies(t *testing.T) {
	cases := []struct {
		name   string
		source snapshot.Source
		want   []snapshot.Manifest
	}{
		{"typescript", snapshottest.Load(t, "testdata/typescript"), []snapshot.Manifest{{Path: "package.json", Dependencies: []string{"express", "pg"},
			Lockfiles: []string{"package-lock.json"}, Unlocked: []string{"express", "pg"}}}},
		{"go", snapshottest.Load(t, "testdata/go"), []snapshot.Manifest{{Path: "go.mod", Dependencies: []string{"github.com/a/b"}, Lockfiles: []string{"go.sum"}, Unlocked: []string{}}}},
		{"mixed", snapshottest.Load(t, "testdata/mixed"), []snapshot.Manifest{{Path: "go.mod", Dependencies: []string{}}, {Path: "package.json", Dependencies: []string{}}}},
		{"unproven", snapshottest.Load(t, "testdata/unproven"), nil},
		{"python", snapshottest.Load(t, "testdata/python"), pyprojectNames("httpx", "pydantic-settings")},
		{"a nested manifest", snapshottest.Files(nil, map[string]string{"web/package.json": `{"dependencies": {"x": "1"}}`}), nil},
		{"a comment in a require block", snapshottest.Files(nil, map[string]string{"go.mod": "module m\n\nrequire (\n\t// pinned\n\tgithub.com/a/b v1.0.0\n)\n"}), []snapshot.Manifest{{Path: "go.mod", Dependencies: []string{"github.com/a/b"}}}},
		{"a one-line require", snapshottest.Files(nil, map[string]string{"go.mod": "module m\n\nrequire github.com/a/b v1.0.0\n"}), []snapshot.Manifest{{Path: "go.mod", Dependencies: []string{"github.com/a/b"}}}},
		{"pyproject", pyproject("[project]\ndependencies = [\"a\", 'b']\n"), pyprojectNames("a", "b")},
		{"pyproject with comments over lines", pyproject("[project] # the package\ndependencies = [\n  \"a\",  # why\n  \"b @ https://x.example/b#egg=b\",\n]\n"), pyprojectNames("a", "b")},
		{"pyproject extras, markers, dots", pyproject("[project]\ndependencies = [\"uvicorn[standard]>=0.34\", \"torch>=2 ; sys_platform == 'linux'\", \"zope.interface\"]\n"),
			pyprojectNames("torch", "uvicorn", "zope-interface")},
		{"pyproject optional dependencies left out", pyproject("[project]\ndependencies = [\"a\"]\n\n[project.optional-dependencies]\ndev = [\"b\"]\n"), pyprojectNames("a")},
		{"pyproject without dependencies", pyproject("[tool.x]\ndependencies = [\"z\"]\n\n[project]\nname = \"x\"\n"), pyprojectNames()},
		{"pyproject normalises names", pyproject("[project]\ndependencies = [\"Pydantic_Settings>=2\", \"httpx>=0.28\"]\n"), pyprojectNames("httpx", "pydantic-settings")},
		{"pyproject for tools only", pyproject("[tool.ruff]\nline-length = 120\n"), nil},
		{"pyproject with inline tables and a long string first", pyproject("[project]\nauthors = [{name = \"a\", email = \"b\"}]\ndescription = \"\"\"\nline\n\"\"\"\ndependencies = [\"httpx\"]\n"), pyprojectNames("httpx")},
		{"pyproject with a byte order mark", pyproject("\uFEFF[project]\ndependencies = [\"a\"]\n"), pyprojectNames("a")},
		{"a nested pyproject", snapshottest.Files(nil, map[string]string{"pkg/pyproject.toml": "[project]\ndependencies = [\"a\"]\n"}), nil},
		{"dart", snapshottest.Load(t, "testdata/dart"), pubspecNames("flutter", "http", "provider")},
		{"pubspec with quoted names and a nested git entry", pubspec("name: 'x'\ndependencies:\n    'a': ^1.0.0\n    b:\n      git:\n        url: https://x.example/b\n    c: any # why\nflutter:\n  assets: []\n"),
			pubspecNames("a", "b", "c")},
		{"pubspec without dependencies", pubspec("name: x\ndev_dependencies:\n  test: ^1.0.0\n"), pubspecNames()},
		{"pubspec without a name", pubspec("dependencies:\n  a: ^1.0.0\n"), nil},
		{"pubspec of a workspace without dependencies", pubspec("name: _ws\nworkspace:\n  - app\n"), nil},
		{"a pom's own dependencies in compile and runtime scope", oneFile("pom.xml", "<project>"+pomOwn+"</project>"), dependenciesOf("pom.xml", "a:b", "c:d")},
		{"a pom's managed and plugin dependencies left out", oneFile("pom.xml", "<project>"+pomManaged+pomOwn+
			"<build><plugins><plugin><dependencies><dependency><groupId>p</groupId><artifactId>q</artifactId></dependency></dependencies></plugin></plugins></build></project>"),
			dependenciesOf("pom.xml", "a:b", "c:d")},
		{"a pom that lists modules and declares no dependency", oneFile("pom.xml", "<project><modules><module>core</module></modules>"+pomManaged+"</project>"), nil},
		{"a pom without dependencies and without modules", oneFile("pom.xml", "<project><artifactId>x</artifactId></project>"), dependenciesOf("pom.xml")},
		{"a pom with a name space, a byte order mark and CRLF", oneFile("pom.xml", "\uFEFF<?xml version=\"1.0\" encoding=\"UTF-8\"?>\r\n<project xmlns=\"http://maven.apache.org/POM/4.0.0\">\r\n"+pomOwn+"\r\n</project>\r\n"), dependenciesOf("pom.xml", "a:b", "c:d")},
		{"a file named pom.xml with another root element", oneFile("pom.xml", "<settings/>"), nil},
		{"a nested pom", oneFile("core/pom.xml", "<project>"+pomOwn+"</project>"), nil},
		{"csharp", snapshottest.Load(t, "testdata/csharp"), dependenciesOf("Acme.Notes/Acme.Notes.csproj", "Microsoft.Extensions.Configuration", "Npgsql", "System.Text.Json")},
		{"swift", snapshottest.Load(t, "testdata/swift"), []snapshot.Manifest{{Path: "Package.swift", Dependencies: []string{"swift-argument-parser", "swift-log"},
			Lockfiles: []string{"Package.resolved"}, Unlocked: []string{"swift-log"}}}},
		{"rust", snapshottest.Load(t, "testdata/rust"), dependenciesOf("Cargo.toml", "anyhow", "clap", "libc", "log", "notes-store", "serde")},
		{"java", snapshottest.Load(t, "testdata/java"), dependenciesOf("pom.xml", "com.fasterxml.jackson.core:jackson-databind", "org.postgresql:postgresql")},
		{"kotlin", snapshottest.Load(t, "testdata/kotlin"), dependenciesOf("build.gradle.kts", ":shared", "io.ktor:ktor-bom", "io.ktor:ktor-client-core", "libs.kotlinx.coroutines.core", "org.slf4j:slf4j-simple")},
		{"a Groovy build file", oneFile("build.gradle", "dependencies {\n  implementation 'org.springframework.boot:spring-boot-starter-web'\n"+
			"  runtimeOnly \"org.postgresql:postgresql:$pg\"\n  implementation platform('g:bom:1')\n  implementation project(':core')\n"+
			"  implementation libs.guava\n  implementation libs.guava\n  testImplementation 'junit:junit:4.13'\n}\n"),
			dependenciesOf("build.gradle", ":core", "g:bom", "libs.guava", "org.postgresql:postgresql", "org.springframework.boot:spring-boot-starter-web")},
		{"a Gradle root that declares plugins only", oneFile("build.gradle.kts", "plugins {\n    alias(libs.plugins.android.application) apply false\n}\n"), nil},
		{"Gradle dependencies of the build script and of subprojects left out", oneFile("build.gradle.kts",
			"buildscript {\n    dependencies {\n        classpath(\"g:a:1\")\n    }\n}\nsubprojects {\n    dependencies {\n        implementation(\"g:b:1\")\n    }\n}\n"), nil},
		{"a Gradle block with tabs and CRLF that ends with the file", oneFile("build.gradle.kts", "dependencies {\r\n\timplementation(\"g:a:1\") // why\r\n\tapi(project(path = \":x\"))\r\n"),
			dependenciesOf("build.gradle.kts", ":x", "g:a")},
		{"a Gradle block with test and compile-only lines alone", oneFile("build.gradle.kts", "dependencies {\n    testImplementation(kotlin(\"test\"))\n    compileOnly(\"g:a:1\")\n}\n"),
			dependenciesOf("build.gradle.kts")},
		{"a nested build file", oneFile("app/build.gradle.kts", "dependencies {\n    implementation(\"g:a:1\")\n}\n"), nil},
		{"a project file's references, with and without a version", oneFile("src/App/App.csproj", sdkProject(csprojOwn)), dependenciesOf("src/App/App.csproj", "A.B", "C")},
		{"a project file at the root", oneFile("App.csproj", sdkProject(csprojOwn)), dependenciesOf("App.csproj", "A.B", "C")},
		{"private assets, as an attribute and as an element, left out", oneFile("src/App/App.csproj", sdkProject(`<ItemGroup><PackageReference Include="A" PrivateAssets="All" />`+
			`<PackageReference Include="B"><PrivateAssets>all</PrivateAssets><IncludeAssets>runtime; build</IncludeAssets></PackageReference>`+
			`<PackageReference Include="C" PrivateAssets="analyzers" /></ItemGroup>`)), dependenciesOf("src/App/App.csproj", "C")},
		{"conditions, a repeated name, update and remove", oneFile("src/App/App.csproj", sdkProject(`<ItemGroup Condition="'$(TargetFramework)' == 'net462'">`+
			`<PackageReference Include="A" Version="1" Condition="'$(OS)' == 'Windows_NT'" /></ItemGroup><ItemGroup><PackageReference Include="A" Version="1" />`+
			`<PackageReference Update="B" Version="2" /><PackageReference Remove="C" /></ItemGroup>`)), dependenciesOf("src/App/App.csproj", "A")},
		{"a project file without references", oneFile("src/App/App.csproj", sdkProject("")), dependenciesOf("src/App/App.csproj")},
		{"a project file with a byte order mark, a declaration, the old name space and CRLF", oneFile("src/App/App.csproj",
			"\uFEFF<?xml version=\"1.0\" encoding=\"utf-8\"?>\r\n<Project ToolsVersion=\"15.0\" xmlns=\"http://schemas.microsoft.com/developer/msbuild/2003\">\r\n"+csprojOwn+"\r\n</Project>\r\n"),
			dependenciesOf("src/App/App.csproj", "A.B", "C")},
		{"an old-style project without package references", oneFile("src/App/App.csproj",
			`<Project ToolsVersion="4.0" xmlns="http://schemas.microsoft.com/developer/msbuild/2003"><ItemGroup><Reference Include="System" /></ItemGroup></Project>`), nil},
		{"references of a target and under choose left out", oneFile("src/App/App.csproj", sdkProject(`<Target Name="x"><ItemGroup><PackageReference Include="T" /></ItemGroup></Target>`+
			`<Choose><When Condition="true"><ItemGroup><PackageReference Include="W" /></ItemGroup></When></Choose>`)), dependenciesOf("src/App/App.csproj")},
		{"project files of test code, other code and a skipped folder", snapshottest.Files(nil, map[string]string{
			"tests/App.Tests/App.Tests.csproj": sdkProject(csprojOwn), "App.Tests/App.Tests.csproj": sdkProject(csprojOwn),
			"examples/Demo/Demo.csproj": sdkProject(csprojOwn), "node_modules/x/X.csproj": sdkProject(csprojOwn)}), nil},
		{"two projects", snapshottest.Files(nil, map[string]string{"src/A/A.csproj": sdkProject(csprojOwn), "src/B/B.csproj": sdkProject("")}),
			[]snapshot.Manifest{{Path: "src/A/A.csproj", Dependencies: []string{"A.B", "C"}}, {Path: "src/B/B.csproj", Dependencies: []string{}}}},
		{"central versions and a build props file are no manifest", snapshottest.Files(nil, map[string]string{
			"Directory.Packages.props": `<Project><ItemGroup><PackageVersion Include="A" Version="1" /></ItemGroup></Project>`,
			"Directory.Build.props":    `<Project>` + csprojOwn + `</Project>`}), nil},
		{"a Swift package: urls with and without .git, a path, a registry id, an old name argument", oneFile("Package.swift",
			"// swift-tools-version:5.9\nimport PackageDescription\n\nlet package = Package(\n    name: \"x\",\n    dependencies: [\n"+
				"        .package(url: \"https://github.com/apple/swift-argument-parser.git\", from: \"1.3.0\"),\n"+
				"        .package(url: \"https://github.com/Apple/Swift-Log\", from: \"1.5.0\"),\n"+
				"        .package(path: \"../LocalKit\"),\n        .package(id: \"mona.LinkedList\", from: \"1.0.0\"),\n"+
				"        .package(\n            name: \"Old\",\n            url: \"git@github.com:a/old-kit.git\", .upToNextMajor(from: \"2.0.0\")\n        ),\n    ]\n)\n"),
			dependenciesOf("Package.swift", "localkit", "mona.linkedlist", "old-kit", "swift-argument-parser", "swift-log")},
		{"a Swift package without dependencies", oneFile("Package.swift", "import PackageDescription\n\nlet package = Package(name: \"x\", targets: [.target(name: \"X\")])\n"), dependenciesOf("Package.swift")},
		{"a file named Package.swift that declares no package", oneFile("Package.swift", "print(1)\n"), nil},
		{"a nested Package.swift", oneFile("Tools/Package.swift", "let package = Package(name: \"x\", dependencies: [.package(path: \"..\")])\n"), nil},
		{"a Cargo package: keys, a value over lines, a dotted key, a sub-table, a target table", oneFile("Cargo.toml",
			"[package]\nname = \"x\"\n\n[dependencies]\nanyhow = \"1\"\nserde = { version = \"1\", features = [\n    \"derive\",\n] }\nlog.workspace = true\n\n"+
				"[dependencies.clap]\nversion = \"4\"\nfeatures = [\"derive\"]\n\n[target.'cfg(unix)'.dependencies]\nlibc = \"0.2\"\n\n"+
				"[dev-dependencies]\ntempfile = \"3\"\n\n[build-dependencies]\ncc = \"1\"\n\n[workspace.dependencies]\nlog = \"0.4\"\n"),
			dependenciesOf("Cargo.toml", "anyhow", "clap", "libc", "log", "serde")},
		{"a Cargo workspace root without a package", oneFile("Cargo.toml", "[workspace]\nmembers = [\"a\"]\n\n[workspace.dependencies]\nlog = \"0.4\"\n"), nil},
		{"a Cargo package without dependencies", oneFile("Cargo.toml", "[package]\nname = \"x\"\n\n[dev-dependencies]\ntempfile = \"3\"\n"), dependenciesOf("Cargo.toml")},
		{"Cargo keys in quotes, a comment, CRLF and a byte order mark", oneFile("Cargo.toml",
			"\uFEFF[package]\r\nname = \"x\"\r\n\r\n[dependencies] # runtime\r\n\"a\" = \"1\"\r\n'b' = { version = \"1\" } # why\r\nc.workspace = true\r\n"),
			dependenciesOf("Cargo.toml", "a", "b", "c")},
		{"a Cargo target sub-table, and a name in two tables", oneFile("Cargo.toml",
			"[package]\nname = \"x\"\n\n[dependencies]\nlibc = \"0.2\"\n\n[target.'cfg(all(unix, target_env = \"musl\"))'.dependencies.jemalloc]\nversion = \"0.5\"\n\n"+
				"[target.x86_64-pc-windows-msvc.dependencies]\nlibc = \"0.2\"\nwinapi = \"0.3\"\n"),
			dependenciesOf("Cargo.toml", "jemalloc", "libc", "winapi")},
		{"a Cargo array over lines outside the dependencies, with a bracket in a string and a comment", oneFile("Cargo.toml",
			"[package]\nname = \"x\"\n\n[features]\ndefault = [ # all\n  \"a[\",\n]\n[dependencies]\nx = \"1\"\n"),
			dependenciesOf("Cargo.toml", "x")},
		{"a nested Cargo.toml", oneFile("crates/a/Cargo.toml", "[package]\nname = \"a\"\n\n[dependencies]\nlog = \"0.4\"\n"), nil},
	}
	for _, tc := range cases {
		if got := snapshot.Build(tc.source, conventions.Default()).Now.Manifests; !reflect.DeepEqual(got, tc.want) {
			t.Errorf("Build(%s).Now.Manifests = %#v, want %#v", tc.name, got, tc.want)
		}
	}
}

func TestBuildNamesDependenciesNoLockfileHolds(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  []snapshot.Manifest
	}{
		{"uv.lock by normalised name", map[string]string{"pyproject.toml": "[project]\ndependencies = [\"Pydantic_Settings>=2\", \"httpx\"]\n",
			"uv.lock": "[[package]]\nname = \"pydantic-settings\"\n"},
			[]snapshot.Manifest{{Path: "pyproject.toml", Dependencies: []string{"httpx", "pydantic-settings"}, Lockfiles: []string{"uv.lock"}, Unlocked: []string{"httpx"}}}},
		{"go.sum by module path", map[string]string{"go.mod": "module m\n\nrequire (\n\tgithub.com/a/b v1.0.0\n\tgithub.com/a/bc v1.0.0\n)\n",
			"go.sum": "github.com/a/bc v1.0.0 h1:x=\n"},
			[]snapshot.Manifest{{Path: "go.mod", Dependencies: []string{"github.com/a/b", "github.com/a/bc"}, Lockfiles: []string{"go.sum"}, Unlocked: []string{"github.com/a/b"}}}},
		{"pubspec.lock", map[string]string{"pubspec.yaml": "name: x\ndependencies:\n  http: ^1.0.0\n  provider: ^6.0.0\n", "pubspec.lock": "packages:\n  http:\n    version: 1.0.0\n"},
			[]snapshot.Manifest{{Path: "pubspec.yaml", Dependencies: []string{"http", "provider"}, Lockfiles: []string{"pubspec.lock"}, Unlocked: []string{"provider"}}}},
		{"Package.resolved by identity", map[string]string{"Package.swift": "let package = Package(name: \"x\", dependencies: [\n" +
			".package(url: \"https://github.com/apple/swift-log.git\", from: \"1.0.0\"),\n.package(url: \"https://github.com/apple/swift-nio\", from: \"2.0.0\"),\n])\n",
			"Package.resolved": "{\n  \"pins\" : [\n    {\n      \"identity\" : \"swift-log\",\n      \"kind\" : \"remoteSourceControl\"\n    }\n  ],\n  \"version\" : 2\n}\n"},
			[]snapshot.Manifest{{Path: "Package.swift", Dependencies: []string{"swift-log", "swift-nio"}, Lockfiles: []string{"Package.resolved"}, Unlocked: []string{"swift-nio"}}}},
		{"Cargo.lock by name", map[string]string{"Cargo.toml": "[package]\nname = \"x\"\n\n[dependencies]\nanyhow = \"1\"\nserde = \"1\"\n",
			"Cargo.lock": "[[package]]\nname = \"anyhow\"\nversion = \"1.0.0\"\n"},
			[]snapshot.Manifest{{Path: "Cargo.toml", Dependencies: []string{"anyhow", "serde"}, Lockfiles: []string{"Cargo.lock"}, Unlocked: []string{"serde"}}}},
		{"no lockfile tracked", map[string]string{"package.json": `{"dependencies": {"a": "1"}}`},
			[]snapshot.Manifest{{Path: "package.json", Dependencies: []string{"a"}}}},
	}
	for _, tc := range cases {
		if got := snapshot.Build(snapshottest.Files(nil, tc.files), conventions.Default()).Now.Manifests; !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: Manifests = %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func TestBuildLeavesAnUnreadableManifestOut(t *testing.T) {
	for _, tc := range []struct{ file, content string }{
		{"package.json", "{not json"},
		{"pyproject.toml", "[project]\nname = \"x\"\ndynamic = [\"dependencies\"]\n"},
		{"pyproject.toml", "[project]\ndependencies = [\n  \"a\",\n"},
		{"pyproject.toml", "[project]\ndependencies = \"a\"\n"},
		{"pubspec.yaml", "name: x\ndependencies: {a: ^1.0.0}\n"},
		{"pom.xml", "<project><dependencies>"},
		{"build.gradle.kts", "dependencies {\n    implementation(kotlin(\"stdlib\"))\n}\n"},
		{"build.gradle", "dependencies {\n    implementation group: 'g', name: 'a', version: '1'\n}\n"},
		{"build.gradle.kts", "dependencies {\n    implementation(files(\"libs/a.jar\"))\n}\n"},
		{"build.gradle.kts", "dependencies {\n    implementation(\n        libs.a.b\n    )\n}\n"},
		{"build.gradle", "dependencies {\n    implementation \"nocolon\"\n}\n"},
		{"pom.xml", "<?xml version=\"1.0\" encoding=\"ISO-8859-1\"?><project><dependencies/></project>"},
		{"Cargo.toml", "[package]\nname = \"x\"\n\n[dependencies]\nnot a key line\n"},
		{"Package.swift", "let package = Package(name: \"x\", dependencies: [.package(url: kitURL, from: \"1.0.0\")])\n"},
		{"src/App/App.csproj", `<Project Sdk="Microsoft.NET.Sdk"><ItemGroup>`},
		{"src/App/App.csproj", sdkProject(`<ItemGroup><PackageReference Include="$(Driver)" /></ItemGroup>`)},
		{"src/App/App.csproj", sdkProject(`<ItemGroup><PackageReference Include="A;B" /></ItemGroup>`)},
		{"src/App/App.csproj", `<?xml version="1.0" encoding="utf-16"?><Project Sdk="Microsoft.NET.Sdk"></Project>`},
	} {
		s := snapshot.Build(snapshottest.Files(nil, map[string]string{tc.file: tc.content}), conventions.Default())
		if len(s.Now.Manifests) != 0 || !reflect.DeepEqual(s.Unreadable, []string{tc.file}) {
			t.Errorf("Build(%s %q) = manifests %v, unreadable %v; want none and [%s]", tc.file, tc.content, s.Now.Manifests, s.Unreadable, tc.file)
		}
	}
}

func pyproject(content string) snapshot.Source {
	return snapshottest.Files(nil, map[string]string{"pyproject.toml": content})
}

const csprojOwn = `<ItemGroup><PackageReference Include="A.B" Version="1.0.0" /><PackageReference Include=" C " /></ItemGroup>`

// sdkProject is a project file of the .NET SDK with the given items.
func sdkProject(items string) string {
	return `<Project Sdk="Microsoft.NET.Sdk">` + items + `</Project>`
}

const pomOwn = "<dependencies>" +
	"<dependency><groupId>a</groupId><artifactId>b</artifactId></dependency>" +
	"<dependency><groupId> c </groupId><artifactId>d</artifactId><scope>runtime</scope></dependency>" +
	"<dependency><groupId>e</groupId><artifactId>f</artifactId><scope>test</scope></dependency>" +
	"<dependency><groupId>g</groupId><artifactId>h</artifactId><scope>provided</scope></dependency></dependencies>"

const pomManaged = "<dependencyManagement><dependencies><dependency><groupId>x</groupId><artifactId>y</artifactId></dependency></dependencies></dependencyManagement>"

// oneFile is a repository of one file.
func oneFile(path, content string) snapshot.Source {
	return snapshottest.Files(nil, map[string]string{path: content})
}

// dependenciesOf is one manifest; no name gives an empty list.
func dependenciesOf(file string, names ...string) []snapshot.Manifest {
	return []snapshot.Manifest{{Path: file, Dependencies: append([]string{}, names...)}}
}

func pubspec(content string) snapshot.Source {
	return snapshottest.Files(nil, map[string]string{"pubspec.yaml": content})
}

func pubspecNames(names ...string) []snapshot.Manifest {
	return []snapshot.Manifest{{Path: "pubspec.yaml", Dependencies: append([]string{}, names...)}}
}

func pyprojectNames(names ...string) []snapshot.Manifest {
	return []snapshot.Manifest{{Path: "pyproject.toml", Dependencies: append([]string{}, names...)}}
}

func TestBuildSumsTheFootprintByArea(t *testing.T) {
	now := map[string]string{
		"src/a.ts": "a\n", "tests/a.test.ts": "test(\"a\", () => {});\n", "docs/x.md": "x\n", "config.json": "{}\n",
	}
	base := map[string]string{"src/a.ts": "b\n", "src/old.ts": "old\n", "tests/a.test.ts": "x\n", "config.json": "[]\n"}
	changes := []snapshot.Change{
		{Path: "src/a.ts", Added: 120, Removed: 30}, {Path: "src/old.ts", Removed: 50},
		{Path: "tests/a.test.ts", Added: 300, Removed: 10}, {Path: "docs/x.md", Added: 40}, {Path: "config.json", Added: 5},
	}
	source := snapshottest.Files(base, now)
	source.Changes = changes
	source.Since = []string{"e", "d", "c", "b", "a"}
	ratio := 2.5
	want := snapshot.Footprint{
		Commits: 5, Files: 5, Product: snapshot.Lines{Added: 120, Removed: 80}, Test: snapshot.Lines{Added: 300, Removed: 10},
		Docs: snapshot.Lines{Added: 40}, TestPerProduct: &ratio,
	}
	if got := snapshot.Build(source, conventions.Default()).Footprint; !reflect.DeepEqual(got, want) {
		t.Fatalf("Build(changes).Footprint = %+v, want %+v", got, want)
	}
	source.Changes = changes[2:]
	if got := snapshot.Build(source, conventions.Default()).Footprint.TestPerProduct; got != nil {
		t.Errorf("Build(no product change).Footprint.TestPerProduct = %v, want nil", *got)
	}
	product := func(n int) string { return strings.Repeat("fn p() {}\n", n) }
	tests := func(n int) string { return "#[cfg(test)]\nmod tests {\n" + strings.Repeat("    // t\n", n-3) + "}\n" }
	for _, tc := range []struct {
		name          string
		base, now     string // src/a.rs on each side; "" is no file
		change        snapshot.Change
		product, test snapshot.Lines
	}{
		{"test code that grows", product(10) + tests(5), product(12) + tests(9), snapshot.Change{Added: 6}, snapshot.Lines{Added: 2}, snapshot.Lines{Added: 4}},
		{"one added line makes a block test code", product(4) + "mod helpers {\n    fn h() {}\n}\n", product(4) + "#[cfg(test)]\nmod helpers {\n    fn h() {}\n}\n",
			snapshot.Change{Added: 1}, snapshot.Lines{}, snapshot.Lines{Added: 1}},
		{"test code that shrinks", product(10) + tests(9), product(7) + tests(5), snapshot.Change{Removed: 7}, snapshot.Lines{Removed: 3}, snapshot.Lines{Removed: 4}},
		{"test code rewritten at its size", product(10) + tests(5), product(10) + strings.ReplaceAll(tests(5), "// t", "// u"),
			snapshot.Change{Added: 2, Removed: 2}, snapshot.Lines{Added: 2, Removed: 2}, snapshot.Lines{}},
		{"a new file", "", product(3) + tests(4), snapshot.Change{Added: 7}, snapshot.Lines{Added: 3}, snapshot.Lines{Added: 4}},
		{"a deleted file", product(3) + tests(4), "", snapshot.Change{Removed: 7}, snapshot.Lines{Removed: 3}, snapshot.Lines{Removed: 4}},
	} {
		side := func(content string) map[string]string {
			if content == "" {
				return map[string]string{}
			}
			return map[string]string{"src/a.rs": content}
		}
		inside := snapshottest.Files(side(tc.base), side(tc.now))
		tc.change.Path = "src/a.rs"
		inside.Changes = []snapshot.Change{tc.change}
		if got := snapshot.Build(inside, conventions.Default()).Footprint; got.Product != tc.product || got.Test != tc.test {
			t.Errorf("%s: footprint product %+v, test %+v; want %+v, %+v", tc.name, got.Product, got.Test, tc.product, tc.test)
		}
	}
}

func TestBuildFlagsUsableCommits(t *testing.T) {
	touching := func(n int) []snapshot.FileChange {
		files := make([]snapshot.FileChange, n)
		for i := range files {
			files[i].Path = fmt.Sprintf("src/f%d.ts", i)
		}
		return files
	}
	source := snapshot.Source{
		Since:   []string{"c1"},
		Commits: []snapshot.Commit{{ID: "c1", Files: touching(21)}, {ID: "c2"}, {ID: "c3", Files: touching(20)}},
	}
	history := snapshot.Build(source, conventions.Default()).History
	for i, want := range []struct{ since, usable bool }{{true, false}, {false, false}, {false, true}} {
		if got := history.Commits[i]; got.Since != want.since || got.Usable != want.usable {
			t.Errorf("Build(history).Commits[%d] = since %v, usable %v; want %v, %v", i, got.Since, got.Usable, want.since, want.usable)
		}
	}
	if history.Read != 3 || history.Usable != 1 {
		t.Errorf("Build(history) = read %d, usable %d; want 3, 1", history.Read, history.Usable)
	}
}

func TestBuildGivesEachHistoryPathItsArea(t *testing.T) {
	areas := map[string]snapshot.Area{
		"src/a.ts": snapshot.AreaProduct, "tests/a.test.ts": snapshot.AreaTests, "docs/a.md": snapshot.AreaDocs,
		"README.md": snapshot.AreaDocs, "package.json": snapshot.AreaOther, "scripts/x.ts": snapshot.AreaOther,
		"gone/old.ts":                     snapshot.AreaProduct,
		"app/src/androidTest/kotlin/A.kt": snapshot.AreaTests, "lib/src/testFixtures/java/A.java": snapshot.AreaTests,
		"src/testDemo/a.ts": snapshot.AreaTests, "src/latest/a.ts": snapshot.AreaProduct, "src/testing/a.ts": snapshot.AreaProduct,
		"lib/androidTest/a.ts": snapshot.AreaProduct, "gradle.properties": snapshot.AreaOther,
		"src/main/resources/META-INF/services/a.B": snapshot.AreaOther,
		"docs/superpowers/plans/a.md":              snapshot.AreaRecords, "projects/notes.md": snapshot.AreaRecords,
		"docs/qualification/run.json": snapshot.AreaRecords, "evidence/run.txt": snapshot.AreaRecords,
		"src/projects/list.ts": snapshot.AreaProduct,
		"Acme.Tests/A.cs":      snapshot.AreaTests, "src/Acme.Core.Test/Util/A.cs": snapshot.AreaTests,
		"Acme.Testing/A.cs": snapshot.AreaProduct, "Tests/NotesTests/StoreTests.swift": snapshot.AreaTests,
		"Sources/Notes/Tests.swift": snapshot.AreaProduct, "src/Contests/A.cs": snapshot.AreaProduct, "UnitTests/A.cs": snapshot.AreaProduct,
		"Acme.tests/A.cs": snapshot.AreaProduct, "src/Acme/Acme.csproj": snapshot.AreaOther, "Acme.sln": snapshot.AreaOther,
		"Directory.Build.props": snapshot.AreaOther, "src/Acme/Form1.Designer.cs": snapshot.AreaProduct,
	}
	for p, want := range areas {
		source := snapshot.Source{Commits: []snapshot.Commit{{ID: "x", Files: []snapshot.FileChange{{Path: p}}}}}
		if got := snapshot.Build(source, conventions.Default()).History.Commits[0].Files[0].Area; got != want {
			t.Errorf("area of %s = %v, want %v", p, got, want)
		}
	}
}

func TestWantsNamesWhatTheReaderReads(t *testing.T) {
	w := snapshot.WantsOf(conventions.Default())
	paths := map[string]bool{
		"src/a.ts": true, "README.md": false, "package.json": true, "web/package.json": false, ".env.example": true,
		"tsconfig.json": true, "web/tsconfig.json": false, "go.mod": true, "web/go.mod": false, "pyproject.toml": true, "web/pyproject.toml": false, "pytest.ini": false, "pytest.toml": false,
		"src/main/resources/application.properties": false, "docs/design.pdf": false, "src/main/resources/META-INF/services/a.B": false,
		"pom.xml": true, "core/pom.xml": false, "build.gradle": true, "build.gradle.kts": true,
		"Acme.sln": false, "Acme.slnx": false, "Directory.Build.props": false, "eng/Common.targets": false, "NuGet.Config": false,
		"nuget.config": false, "Acme.sln.DotSettings": false, "src/Acme/Strings.resx": false,
		"src/Acme/Form1.Designer.cs": true, "src/Acme/Views.g.cs": true,
		"src/Acme/Acme.csproj": true, "Acme.csproj": true, "tests/Acme.Tests/Acme.Tests.csproj": false, "Acme.Tests/Acme.Tests.csproj": false,
		"examples/Demo/Demo.csproj": false, "node_modules/x/X.csproj": false, "Directory.Packages.props": false,
		"Package.swift": true, "Package.resolved": true,
		"Cargo.toml": true, "Cargo.lock": true, "crates/cli/Cargo.toml": true, "vendor/x/Cargo.toml": false, "src/main.rs": true,
		"pubspec.yaml": true, "app/pubspec.yaml": true, "packages/core/pubspec.yaml": true, "node_modules/x/pubspec.yaml": false,
		"docs/.env.example": false, "node_modules/x/a.js": false, "assets/logo.bin": true,
		"docker-compose.yml": true, "deploy/compose.prod.yaml": true, "node_modules/x/compose.yml": false, "compose.txt": false,
	}
	for p, want := range paths {
		if got := w.Path(p); got != want {
			t.Errorf("Wants.Path(%q) = %v, want %v", p, got, want)
		}
	}
	if !w.Binary([]byte{'a', 0}) || w.Binary([]byte("ab")) || w.SniffBytes != 8192 {
		t.Errorf("Wants.Binary or SniffBytes wrong: %v, %v, %d", w.Binary([]byte{'a', 0}), w.Binary([]byte("ab")), w.SniffBytes)
	}
}

func TestTouchesCountUsableCommitsPerPath(t *testing.T) {
	var wide []string
	for i := range 21 {
		wide = append(wide, fmt.Sprintf("src/f%d.ts", i))
	}
	source := snapshot.Source{Since: []string{"c3"}, Commits: []snapshot.Commit{
		snapshottest.Commit("c3", "src/a.ts"), snapshottest.Commit("c2", "src/a.ts", "src/b.ts"),
		snapshottest.Commit("c1", append(wide, "src/a.ts")...), snapshottest.Commit("c0", "src/b.ts"),
	}}
	got := snapshot.Build(source, conventions.Default()).History.Touches()
	want := map[string]snapshot.Touches{"src/a.ts": {All: 2, AtBase: 1}, "src/b.ts": {All: 2, AtBase: 2}}
	if !maps.Equal(got, want) {
		t.Errorf("Touches() = %v, want %v", got, want)
	}
}
