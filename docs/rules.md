# What plimsoll checks

Six detectors look at a change from different sides:

- **Size** (`shape`): how much product and test code there is, the largest files, new dependencies and new settings,
  and the functions a change made complex or nested deeper.
- **Working habits** (`process`): planning or review notes committed into the repository, and more churn in
  documentation than in code.
- **Settings** (`settings`): names defined twice, and variables only the container setup knows.
- **Hotspots** (`hotspots`): the files changed most often, weighed by their size.
- **Hidden coupling** (`co-change`): files that keep changing together in the same commits.
- **Wiring** (`links`): who imports whom, import rings, folders that depend on each other in a circle, product code
  that imports test code, imports that break a rule the repository declares, how far a change reaches, and the code a
  change added that nothing uses.

Each detector compares now with a base you choose, such as `main`, so old debt stays quiet and new growth stands out.

## What it measures in detail

Every number is measured at the base and now, from the tracked files only:

- product, test and other code lines, and the test ratio (test lines per product line). Test code inside a product file counts as test lines where a language marks it
- when the ratio is over its limit, test code split into files that define tests and test infrastructure that
  defines none, with the largest infrastructure files
- the three largest code files
- for the product files a change touched, each function's cognitive complexity and deepest nesting, at the base and now; code the change moved, split out or copied, files from another project, benchmarks and generated or minified files are left out, and nothing is said about functions the change did not touch
- the settings in `.env.example` and the runtime dependencies of `package.json`, `go.mod`, `pyproject.toml` (`[project] dependencies`), `pubspec.yaml`, `pom.xml`, `build.gradle` or `build.gradle.kts`, `Cargo.toml`, `Package.swift`, `vcpkg.json`, and every `*.csproj` outside test code
- the footprint of the change since the base
- churn by area (product, tests, docs, other) over the last 150 non-merge commits
- tracked planning and review files, and documentation churn against product and test churn since the base
- the settings audit: names defined twice in `.env.example`, and compose variables that are not settings
- hotspots: the three code files (or as many as `hotspot_files` sets) with the most changes in usable commits times their lines now, as report rows, with no rule of its own
- changed together: pairs of code files that share usable commits, at least one product code, neither a file that defines tests, and not two files that the language treats as one unit
- test folders and package roots that a project declares in its own configuration or in `.plimsoll.toml`, added to the defaults and named with their source
- import links, read from the code: the files that import the changed files and that the change did not touch, the three files that import the most others, import rings, cycles between sibling folders, and each new product file that no file imports. With `files`, also the files that import each named file.
- added code without a use: the functions, types and values a change added at the top level of a product file whose name no code or configuration file holds outside its own declaration. A name that only test code holds, or only documentation names, says so. A list in `report` and one count in `status`, never a mark.

What it could not see is always named under "not analysed".

## Rules

| Identifier | Fires when | Limit and source |
|---|---|---|
| `shape.test-ratio:repository` | the test ratio is over its limit now and was not at the base (new), or rose by 0.1 or more while over it (grown) | 2.0, plimsoll's rule: test code at most twice the product code |
| `shape.file-size:<path>` | a code file in a language plimsoll reads is over its line limit now and was not at the base (new), or grew while over it (grown); a file in another one (listed as not verified) is not measured | 1,000 lines, plimsoll's default, taken from Pylint and SonarQube |
| `shape.dependencies:<manifest>` | a runtime dependency was added since the base. Its verify line names the new ones that no tracked lockfile holds (never installed, or not a real package) | any addition |
| `shape.settings:.env.example` | a setting was added since the base | any addition |
| `shape.function-complexity:<path>:<function>` | a function of a product file the change touched has a cognitive complexity over its limit now and is new, or was at or under it at the base and rose by 8 or more; and it nests control flow 4 levels deep, or holds a block of at least 4 statements and 40 tokens that the change wrote twice inside it. Left out: code the change moved or split out of another function, code found in a file at the base, files from another project (by their header or a vendor folder with a licence), benchmarks, generated and minified files. At most three per change, the largest rise first (candidate) | 25, plimsoll's rule, read blind on agent pull requests; counted as SonarQube's cognitive complexity |
| `shape.function-nesting:<path>:<function>` | a function of a product file the change touched nests control flow 4 or more levels deep now and deeper than at the base (new), or was that deep and got deeper (grown). The same code is left out; at most three per change (candidate). In `report` only | 4 levels, CodeScene's threshold for deep nesting |
| `process.planning-file:<path>` | a file under a planning or review path is tracked now and was not at the base | repositories hold no planning records |
| `process.doc-churn:repository` | over the commits since the base, documentation churn is above product plus test churn | documentation above code |
| `process.limit-change:<key>` | `.plimsoll.toml` sets a limit to another value now than at the base, so a change that raised a limit to clear a mark says so | any change |
| `process.declaration-change:<key>` | `.plimsoll.toml` declares other values under a key of `[layout]`, `[ignore]`, `[imports]` or `[defaults]`, or other entries in `[acknowledged]` (the key is then `acknowledged`), now than at the base; editing only a reason is no change | any change |
| `settings.defined-twice:<NAME>` | a name on two or more lines of `.env.example` that are not commented out (candidate) | plimsoll's rule: one name, one setting |
| `settings.compose-only:<NAME>` | a compose `environment` key that takes its value from the host (no value, or `$`) and that `.env.example` does not name (candidate) | the three lists agree |
| `co-change.pair:<a>+<b>` | two code files changed together in at least 5 usable commits and did not at the base (new), or their share of the commits that touched either rose by at least 0.1 (grown). The mark shows that share and comes after every other mark | 5 shared commits, plimsoll's rule |
| `links.test-import:<path>` | a product file has an import of test code now and had none at the base (new), or has more (grown), and the change added one of these imports (candidate) | product code imports no test code |
| `links.cycle:<path>` | two or more files import each other in a ring, by imports that are not type-only, and did not at the base (new), or the ring has more files (grown), and the change added one of its imports. The path is the ring's smallest (candidate) | files do not import each other in a ring |
| `links.never:<folder>-><folder>` | product code under the first folder imports code under the second, against a `never` rule of `.plimsoll.toml`, and the change wrote one of these imports: new when there were none at the base, grown otherwise, also when the count did not rise. More than at the base without one the change wrote is touched (candidate) | the repository's own rule in `.plimsoll.toml` |
| `links.only:<folder>` | product code under the folder imports code outside it and outside the folders its `only` rule names, and the change wrote one of these imports; new, grown and touched as for `never` (candidate) | the repository's own rule in `.plimsoll.toml` |
| `links.folder-cycle:<folder>` | two or more folders directly under one folder depend on each other in a cycle, through imports between product files below them, and none of them did at the base (new), or the cycle has more folders (grown), and the change added one of the imports between them. The folder is the cycle's smallest (candidate) | the Acyclic Dependencies Principle (Robert C. Martin) |

Something over its limit at the base and no longer over it is reported as resolved. Something over it on both
sides that did not grow is standing. The function rules look only at the files a change touched, so they have no
standing and no resolved items. A function is named by its path and name, and a second function of the same name in
one file by `#2`.

Some kinds are shown in `report` only: a file over its line limit (new or grown), settings added,
documentation churn, compose-only settings, pairs of files that changed together, imports of test
code, folder cycles and functions nested deeper. Seven outside models judged real answers of each kind and found them useful as background
but too noisy to show on every change. A file crossing its line limit was read blind in 150 agent pull requests: worth
acting on in 45, most others a file long by nature, a small addition that tipped it over, or moved, vendored or
generated code. `status` counts them in one line; `report` lists them; `explain` gives their
evidence.

A link item that is new or larger since the base although the change added none of its imports (a file moved, or a
new file made existing imports readable) is touched: `status` counts it in one line, `report` lists it, and `explain`
gives its imports. An import is added when the change wrote its statement: the file is new, or the file at the base
neither wrote the same import nor reached the same target.

### Your own limits and layout

A repository may set its own limits and declare its layout in `.plimsoll.toml` at its root, committed and reviewed
like any other file. Every key is optional:

```toml
[limits]
file_lines = 1500                           # a code file's line limit (default 1,000)
test_ratio = 3.0                            # test lines per product line (default 2.0)
shared_commits = 8                          # commits two files share before they are a pair (default 5)
hotspot_files = 5                           # files the report lists as hotspots (default 3)

[layout]
test_folders = ["checks", "tools/harness"]  # folders whose code is test code
test_files = ["*_spec.rb", "*IT.java"]      # names of test files

[ignore]
folders = ["src/test/resources/fixtures"]   # folders whose files plimsoll leaves out
files = ["SPEC.md", "*.generated.go"]       # names of files plimsoll leaves out

[defaults]
base = "main"                               # the base of a question that names none

[imports]
never = ["src/domain -> src/adapters"]      # code under the first folder imports none under the second
only = ["src/domain -> src/shared"]         # code under the first folder imports only there and in itself

[acknowledged]                              # standing items reviewed and kept on purpose, each with a reason
"process.planning-file:SPEC.md" = "a lasting specification, not a plan"
```

| Key | What it declares |
|---|---|
| `file_lines`, `test_ratio`, `shared_commits` | The limits of the file-size, test-ratio and co-change rules |
| `hotspot_files` | How many files the report lists as hotspots |
| `test_folders` | Folders, as paths from the root, whose code files count as test code, beside the defaults |
| `test_files` | File names of code files that count as test code wherever they lie: `*` is any run of characters, `?` one |
| `folders` under `[ignore]` | Folders, as paths from the root, whose files plimsoll leaves out: no code, no planning record, no documentation it reads |
| `files` under `[ignore]` | File names, with the same patterns as `test_files`, of files plimsoll leaves out wherever they lie |
| an entry of `[acknowledged]` | A mark identifier exactly as an answer printed it, and a reason: a standing item with that identifier is listed under "Acknowledged" in `report` instead of "Standing", and `status` counts such items in one line. The item is still measured and counted; when it grows, or a new item gets its identifier, it shows as usual. Function marks never stand, so they are not acknowledged. The reason is required and never printed |
| `never`, `only` under `[imports]` | Import rules, each `"<folder> -> <folder>"` (below) |
| `base` | The branch, remote branch or tag a question without a base compares with. A remote branch is named with its remote (`origin/main`): `main` alone finds no remote branch. A branch of the name comes before a remote branch, and a remote branch before a tag |

The function limits cannot be set in the file: they are the ones the measurement was read against.

A declaration adds to the defaults and takes none away, and now's file applies to both sides of a question. Every
mark names its limit's source, for example "1,500 lines, set by `.plimsoll.toml`", every report names what the file
declares, and a limit or a declaration that changed since the base is itself a mark. The file switches no rule off:
an acknowledged item is still measured and counted, only listed apart, and `[ignore]` is the one way to leave files out of the count: test code declared here counts as test code. A folder is a plain path, and a
bare name is a folder at the root. A test folder that is the root or a package root, and a file name that is only
`*`, are not applied.

A key plimsoll does not know, a table it does not read, a value that is no number above 0 (a whole number up to 2^53 for a limit
that counts), a list that is no list of text, a line outside a table or a key set twice is named under "not
analysed", and what it names keeps its default (a key set twice its first value). A key or table name that is no
plain name (letters, digits, `_`, `-`, parts joined by `.`) is counted there, not printed. A declared value that matches no
tracked file, or that is no plain path or file-name pattern, is named there too and not used. A question without a
base whose repository declares no usable default base is refused with one sentence that says why.

#### Import rules

A rule is one text `"<folder> -> <folder>"`: two folders from the root, in the form of `test_folders`, joined by one
`->`. Product code under the first folder is the importing side; test code and other code never are.

- `never = ["a -> b"]`: code under `a` imports no code under `b`. An import breaks the rule when every code file it
  reaches lies under `b`. The two folders differ and neither lies inside the other.
- `only = ["a -> b", "a -> c"]`: code under `a` imports code only under `a` itself and under the folders its entries
  name; every entry with the same first folder belongs to one rule. An import breaks the rule when none of the code
  files it reaches lies there, so a folder created later is outside until an entry names it.
- Rules are checked each on its own, and each has one mark. `only = ["a -> s"]` with `never = ["a -> s/internal"]`
  allows `s` except `s/internal`. A `never` whose target the same folder's `only` rule already leaves out repeats it,
  and one import then gives two marks.
- An import counts once per importing file and target, type-only imports included. The mark names up to two imports
  the change added, by file and line.
- A break already at the base is standing, also when the rule was declared in this change. A break made only by
  moving a file into the folder, or by an import that only became readable, is touched. A file renamed inside the
  folder keeps its imports and stays standing.
- A rule whose folder holds no product code, whose `never` target holds no code, or whose folder holds only files
  whose imports are not read, checks nothing and says so first under "not analysed". A value that is not in this
  form is counted there, never printed.

A rule sees the links plimsoll reads, nothing else: an import of an outside package, a use through a third file
(one that only passes imports on), and an import that is ambiguous, not found or built at run time are not seen. Per
language, besides:

| Language | A rule does not see |
|---|---|
| Go | modules behind `replace` or `go.work` (they count as outside) |
| Java, Kotlin | a class written with its full name and no import |
| C# | use of the same or a parent namespace, which needs no `using`; what a `global using` serves |
| Swift | use between two folders of one target; a rule whose folder lies inside a target that also holds code outside it |
| TypeScript, JavaScript | aliases inherited from an extended file; any tsconfig other than the root one; `import()` and `require()` with a computed name |
| Python | `import_module()` with a computed name |
| Dart | nothing beyond the common list |
| Rust | a path written in code without `use`; a module named by a `path` attribute |
| C, C++ | an include folder only the build knows; an include a macro names; C++ modules |
