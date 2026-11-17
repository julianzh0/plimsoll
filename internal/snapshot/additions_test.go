package snapshot_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/julianzh0/plimsoll/internal/conventions"
	"github.com/julianzh0/plimsoll/internal/snapshot"
	"github.com/julianzh0/plimsoll/internal/snapshot/snapshottest"
)

// additionsOf builds a snapshot and returns its additions as "<path> <name> <kind> used=<bool> tests=<bool>", sorted.
// changes, when given, replace the changes the two sides differ by.
func additionsOf(t *testing.T, base, now map[string]string, changes ...snapshot.Change) []string {
	t.Helper()
	source := snapshottest.Files(base, now)
	if len(changes) > 0 {
		source.Changes = changes
	}
	var got []string
	for _, a := range snapshot.Build(source, conventions.Default(), snapshottest.Readings(t)).Additions {
		got = append(got, fmt.Sprintf("%s %s %s used=%v tests=%v", a.Path, a.Name, a.Kind, a.Used, a.Tests))
	}
	slices.Sort(got)
	return got
}

func TestBuildListsWhatAChangeDeclaredInEachLanguage(t *testing.T) {
	cases := []struct {
		entry, path, content string
		want                 map[string]string // name -> kind
	}{
		{"typescript", "src/a.ts", strings.Join([]string{
			"export function exportedFn() {}", "function localFn() {}", "export async function* gen() {}",
			"export default function defaultFn() {}", "export class Shape {}", "abstract class Base {}", "export interface Props {}",
			"type Alias = string;", "export enum Color { Red }", "export const LIMIT = 3;", "export const arrow = () => 1;",
			"const localArrow = async (x: number) => x;", "const localValue = 3;", "@Component({})", "export class Widget {}",
			"class Outer { method() {} }",
		}, "\n") + "\n", map[string]string{
			"arrow": "function", "exportedFn": "function", "gen": "function", "localArrow": "function", "localFn": "function",
			"Alias": "type", "Base": "type", "Color": "type", "Outer": "type", "Props": "type", "Shape": "type", "LIMIT": "value",
		}},
		{"python", "src/a.py", strings.Join([]string{
			"def helper(): pass", "async def fetch(): pass", "class Model: pass", "@app.route(\"/\")", "def handler(): pass",
			"def __repr__(): pass", "def main(): pass", "if True:", "    def nested(): pass",
		}, "\n") + "\n", map[string]string{"fetch": "function", "helper": "function", "Model": "type"}},
		{"go", "internal/a/a.go", strings.Join([]string{
			"package a", "func Run() {}", "func (t T) Method() {}", "type T struct{}", "type (", "\tGrouped int", ")",
			"var Exported = 1", "var unexported = 1", "const Limit = 2", "func init() {}",
		}, "\n") + "\n", map[string]string{"Run": "function", "Grouped": "type", "Exported": "value", "Limit": "value"}},
		{"rust", "src/a.rs", strings.Join([]string{
			"pub fn run() {}", "fn helper() {}", "pub struct Config;", "enum Mode { A }", "trait Store {}", "type Alias = u8;",
			"pub const LIMIT: u8 = 1;", "const PRIVATE: u8 = 1;", "pub static NAME: &str = \"x\";", "#[derive(Debug)]", "pub struct Derived;",
			"#[frb(mirror(Other))]", "pub struct _Mirror;", "#[cfg(test)]", "fn only_in_tests() {}", "mod inner { pub fn nested() {} }",
		}, "\n") + "\n", map[string]string{"helper": "function", "run": "function", "Alias": "type", "Config": "type", "Derived": "type",
			"Mode": "type", "Store": "type", "LIMIT": "value", "NAME": "value"}},
		{"java", "src/main/java/app/a/A.java", strings.Join([]string{
			"package app.a;", "public class A { void method() {} class Inner {} }", "interface Port {}", "enum Mode { X }",
			"record Point(int x) {}", "@interface Marker {}", "@Entity", "class Row {}",
		}, "\n") + "\n", map[string]string{"Marker": "type", "Mode": "type", "Point": "type", "Port": "type"}},
		{"kotlin", "src/main/kotlin/app/a/A.kt", strings.Join([]string{
			"package app.a", "class Screen", "interface Port", "object Registry", "fun topLevel() {}", "fun String.shout() = uppercase()",
			"val LIMIT = 3", "var counter = 0", "@Composable", "fun Annotated() {}",
		}, "\n") + "\n", map[string]string{"Port": "type", "Registry": "type", "Screen": "type", "shout": "function", "topLevel": "function",
			"LIMIT": "value"}},
		{"csharp", "src/App/A.cs", strings.Join([]string{
			"namespace App;", "public class Service { void Method() {} class Inner {} }", "internal interface IPort {}", "public enum Mode { X }",
			"public record Point(int X);", "public struct Size {}",
			"public static class StringExtensions { public static string Shout(this string s) => s; }",
		}, "\n") + "\n", map[string]string{"IPort": "type", "Mode": "type", "Point": "type", "Service": "type", "Size": "type"}},
		{"swift", "Sources/App/A.swift", strings.Join([]string{
			"func topLevel() {}", "class Screen {}", "struct Point {}", "enum Mode { case x }", "protocol Port {}", "actor Worker {}",
			"typealias Alias = Int", "extension Point { func helper() {} }", "@main", "struct Entry { static func main() {} }",
		}, "\n") + "\n", map[string]string{"topLevel": "function", "Alias": "type", "Mode": "type", "Point": "type", "Port": "type",
			"Screen": "type", "Worker": "type"}},
		{"dart", "lib/src/a.dart", strings.Join([]string{
			"class Screen {}", "mixin Logging {}", "enum Mode { x }", "typedef Callback = void Function();",
			"extension StringShout on String { String shout() => toUpperCase(); }", "void topLevel() {}", "String _private() => '';",
			"@pragma('vm:entry-point')", "void entry() {}",
		}, "\n") + "\n", map[string]string{"Callback": "type", "Logging": "type", "Mode": "type", "Screen": "type", "_private": "function",
			"topLevel": "function"}},
		{"c", "src/a.c", strings.Join([]string{
			"int run(void) { return 0; }", "static int helper(int x) { return x; }", "int declared_only(void);", "struct config { int x; };",
			"enum mode { MODE_A };", "typedef struct _tagged { int x; } tagged_t;", "typedef enum _kind { KIND_A } kind_t;",
		}, "\n") + "\n", map[string]string{"helper": "function", "run": "function", "config": "type", "mode": "type"}},
		{"cpp", "src/a.cpp", strings.Join([]string{
			"int run() { return 0; }", "class Widget { public: void method() {} void other(); };", "void Widget::other() {}", "namespace app {",
			"struct Point { int x; };", "int inside() { return 1; }", "}",
		}, "\n") + "\n", map[string]string{"inside": "function", "run": "function", "Point": "type", "Widget": "type"}},
	}
	covered := map[string]bool{}
	for _, tc := range cases {
		covered[tc.entry] = true
	}
	for _, language := range conventions.Default().Languages {
		if !covered[language.Name] {
			t.Errorf("language %s has no declarations case", language.Name)
		}
	}
	for _, tc := range cases {
		for form, content := range map[string]string{
			"plain": tc.content, "crlf": strings.ReplaceAll(tc.content, "\n", "\r\n"), "bom": "\ufeff" + tc.content,
		} {
			var want []string
			for name, kind := range tc.want {
				want = append(want, fmt.Sprintf("%s %s %s", tc.path, name, kind))
			}
			slices.Sort(want)
			var got []string
			for _, a := range additionsOf(t, nil, map[string]string{tc.path: content}) {
				got = append(got, strings.Join(strings.Fields(a)[:3], " "))
			}
			if !slices.Equal(got, want) {
				t.Errorf("%s, %s: additions =\n%q\nwant\n%q", tc.entry, form, got, want)
			}
		}
	}
}

func TestBuildSaysWhereAnAddedNameIsUsed(t *testing.T) {
	lonely := map[string]string{"src/a.ts": "export function lonely() {}\n"}
	with := func(files map[string]string, more map[string]string) map[string]string {
		tree := map[string]string{}
		for p, c := range files {
			tree[p] = c
		}
		for p, c := range more {
			tree[p] = c
		}
		return tree
	}
	unused := []string{"src/a.ts lonely function used=false tests=false"}
	used := []string{"src/a.ts lonely function used=true tests=false"}
	cases := []struct {
		name      string
		base, now map[string]string
		changes   []snapshot.Change
		want      []string
	}{
		{"nothing uses it", nil, lonely, nil, unused},
		{"another file", nil, with(lonely, map[string]string{"src/b.ts": "import { lonely } from \"./a.ts\";\nlonely();\n"}), nil, used},
		{"its own file", nil, map[string]string{"src/a.ts": "function lonely() {}\nlonely();\n"}, nil, used},
		{"other code", nil, with(lonely, map[string]string{"scripts/x.ts": "lonely();\n"}), nil, used},
		{"test code alone", nil, with(lonely, map[string]string{"tests/a.test.ts": "lonely();\n"}), nil,
			[]string{"src/a.ts lonely function used=false tests=true"}},
		{"a lockfile", nil, with(lonely, map[string]string{"package.json": `{"dependencies":{}}`, "package-lock.json": `{"lonely":1}`}), nil, unused},
		{"a generated file", nil, with(lonely, map[string]string{"src/client.ts": "// Code generated by x. DO NOT EDIT.\nlonely();\n"}), nil, used},
		{"two files declare it", nil, map[string]string{"src/a.ts": "export function twin() {}\n", "src/b.ts": "export function twin() {}\n"}, nil,
			[]string{"src/a.ts twin function used=true tests=false", "src/b.ts twin function used=true tests=false"}},
		{"a modified file", map[string]string{"src/a.ts": "export function old() {}\n"},
			map[string]string{"src/a.ts": "export function old() {}\nexport function fresh() {}\n"}, nil,
			[]string{"src/a.ts fresh function used=false tests=false"}},
		{"moved", map[string]string{"src/a.ts": "export function moved() {}\n", "src/b.ts": "export const b = 1;\n"},
			map[string]string{"src/a.ts": "export const a = 1;\n", "src/b.ts": "export const b = 1;\nexport function moved() {}\n"}, nil, nil},
		{"a renamed file", lonely, map[string]string{"src/z.ts": lonely["src/a.ts"]}, []snapshot.Change{{Path: "src/z.ts", From: "src/a.ts"}}, nil},
		{"a syntax error beside it", nil, map[string]string{"src/a.ts": "export function lonely() {}\nconst x = (\n"}, nil, unused},
		{"an unchanged file", with(lonely, map[string]string{"src/b.ts": "export const b = 1;\n"}),
			with(lonely, map[string]string{"src/b.ts": "export const b = 2;\n"}), nil, nil},
		{"names of form", nil, map[string]string{"src/a.ts": "export function main() {}\nfunction x() {}\nexport function $lonely() {}\n"}, nil, nil},
		{"skipped folders", nil, map[string]string{
			"src/generated/a.ts": "export function a1() {}\n", "samples/a.ts": "export function a2() {}\n",
			"src/Acme.Examples.Web/a.ts": "export function a3() {}\n", "src/OrderUnitTests/a.ts": "export function a4() {}\n",
		}, nil, nil},
		{"test code inside a product file", nil, map[string]string{"src/a.rs": "pub fn probe() {}\n#[cfg(test)]\nmod tests {\n    #[test]\n    fn t() { super::probe(); }\n}\n"}, nil,
			[]string{"src/a.rs probe function used=false tests=true"}},
		{"skipped files", nil, map[string]string{"src/types.d.ts": "export function a1(): void;\n", "src/api.gen.ts": "export function a2() {}\n"}, nil, nil},
		{"not product", nil, map[string]string{"tests/helper.ts": "export function a1() {}\n", "scripts/tool.ts": "export function a2() {}\n"}, nil, nil},
	}
	for _, tc := range cases {
		if got := additionsOf(t, tc.base, tc.now, tc.changes...); !slices.Equal(got, tc.want) {
			t.Errorf("%s: additions = %q, want %q", tc.name, got, tc.want)
		}
	}
	missing := snapshottest.Files(map[string]string{"src/a.ts": "export function old() {}\n"},
		map[string]string{"src/a.ts": "export function old() {}\nexport function fresh() {}\n"})
	for i := range missing.Before {
		missing.Before[i].Content = nil
	}
	if got := snapshot.Build(missing, conventions.Default(), snapshottest.Readings(t)).Additions; len(got) != 0 {
		t.Errorf("a modified file whose base content is missing: additions = %+v, want none", got)
	}
}
