package parse_test

import (
	"slices"
	"testing"

	"github.com/julianzh0/plimsoll/internal/parse"
	"github.com/julianzh0/plimsoll/internal/parse/parsetest"
)

func TestNodesAreTheTreeInTextOrder(t *testing.T) {
	parser := parsetest.Parser(t)
	content := []byte("package a\n\nfunc run() {\n\tif x {\n\t}\n}\n")
	tree, err := parser.Nodes("go", content)
	if err != nil || len(tree.Nodes) == 0 {
		t.Fatalf("Nodes(go) = %d nodes, %v; want some and no error", len(tree.Nodes), err)
	}
	kind := func(n parse.Node) string { return tree.Kinds[n.Kind] }
	if root := tree.Nodes[0]; kind(root) != "source_file" || root.Depth != 0 || root.Start != 0 || int(root.End) != len(content) {
		t.Errorf("Nodes(go) root = %s %+v, want source_file at depth 0 over the whole content", kind(root), root)
	}
	var name, token, function bool
	for i, n := range tree.Nodes {
		if i > 0 && (n.Depth == 0 || n.Depth > tree.Nodes[i-1].Depth+1 || n.Start < tree.Nodes[i-1].Start || int(n.End) > len(content)) {
			t.Errorf("Nodes(go) node %d = %+v after %+v, want text order, one level at a time, inside the content", i, n, tree.Nodes[i-1])
		}
		name = name || n.Name && kind(n) == "identifier" && string(content[n.Start:n.End]) == "run"
		token = token || !n.Named && kind(n) == "if"
		function = function || n.Named && kind(n) == "function_declaration" && n.Depth == 1
	}
	if !name || !token || !function {
		t.Errorf("Nodes(go) found name %v, if token %v, function %v; want all three", name, token, function)
	}
}

func TestNodesFlagWhatTheGrammarCouldNotRead(t *testing.T) {
	tree, err := parsetest.Parser(t).Nodes("go", []byte("package a\n\nfunc ( {\n"))
	if err != nil || !slices.ContainsFunc(tree.Nodes, func(n parse.Node) bool { return n.Error }) {
		t.Errorf("Nodes(go, broken text) = %d nodes, %v; want a node flagged as an error", len(tree.Nodes), err)
	}
}

func TestNodesFlagAMissingToken(t *testing.T) {
	tree, err := parsetest.Parser(t).Nodes("go", []byte("package a\nfunc f() { x := 1\n"))
	if err != nil || !slices.ContainsFunc(tree.Nodes, func(n parse.Node) bool { return n.Error }) {
		t.Errorf("Nodes(go, a function without its closing brace) = %d nodes, %v; want a node flagged as an error", len(tree.Nodes), err)
	}
}

func TestEveryKindHasOneIndex(t *testing.T) {
	content := []byte("package a\nfunc f(x int) { g(x)\nfor { break }\n}\n")
	tree, err := parsetest.Parser(t).Nodes("go", content)
	if err != nil {
		t.Fatalf("Nodes(go) error = %v", err)
	}
	for _, n := range tree.Nodes {
		if first := slices.Index(tree.Kinds, tree.Kinds[n.Kind]); int(n.Kind) != first {
			t.Errorf("Nodes(go) node of kind %q has index %d, want %d, the first of that name", tree.Kinds[n.Kind], n.Kind, first)
		}
	}
}

func TestAWalkReadsWhatReadReadsAndKeepsNoMemory(t *testing.T) {
	parser := newParser(t)
	if _, err := parser.Read("go", "(identifier) @x", goVars(nearTheCap)); err != nil {
		t.Fatalf("Read(go, %d var lines) error = %v, want none", nearTheCap, err)
	}
	if _, err := parser.Nodes("go", goVars(nearTheCap)); err != nil {
		t.Errorf("Nodes(go, %d var lines) error = %v, want none: a file Read reads is walked too", nearTheCap, err)
	}
	if _, err := parser.Nodes("go", goVars(aLargeWalk)); err != nil {
		t.Fatalf("Nodes(go, %d var lines) error = %v, want none", aLargeWalk, err)
	}
	if _, err := parser.Read("go", "(identifier) @x", goVars(nearTheCap)); err != nil {
		t.Errorf("Read(go, %d var lines) after a walk of %d error = %v, want none: a walk keeps no memory", nearTheCap, aLargeWalk, err)
	}
}
