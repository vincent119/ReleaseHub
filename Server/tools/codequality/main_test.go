package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestFunctionNestingTracksDeepestBranch(t *testing.T) {
	function := parseFunction(t, `package sample
func nested() {
	if true {
		for {
			if true {}
		}
	}
}`)

	if depth := functionNesting(function.Body); depth != 3 {
		t.Fatalf("expected nesting depth 3, got %d", depth)
	}
}

func TestAllowsReadsNarrowRuleDirective(t *testing.T) {
	function := parseFunction(t, `package sample
// codequality:allow-parameters Generated contract requires this signature.
func generatedContract(a, b, c, d, e string) {}`)

	if !allows(function, "parameters") {
		t.Fatal("expected parameters rule to be allowed")
	}
	if allows(function, "lines") {
		t.Fatal("did not expect unrelated rule to be allowed")
	}
}

func TestInspectorRejectsTypeWithTooManyFieldDependencies(t *testing.T) {
	file, fileset := parseFileWithSet(t, `package sample
type oversized struct { a A; b B; c C; d D; e E; f F; g G; h H; i I; j J; k K }`)
	checker := &inspector{fileset: fileset}
	checker.inspectFile("sample.go", file)
	if len(checker.violations) != 1 {
		t.Fatalf("expected one dependency violation, got %d", len(checker.violations))
	}
}

func parseFunction(t *testing.T, source string) *ast.FuncDecl {
	t.Helper()
	file := parseFile(t, source)
	return file.Decls[0].(*ast.FuncDecl)
}

func parseFile(t *testing.T, source string) *ast.File {
	t.Helper()
	file, _ := parseFileWithSet(t, source)
	return file
}

func parseFileWithSet(t *testing.T, source string) (*ast.File, *token.FileSet) {
	t.Helper()
	fileset := token.NewFileSet()
	file, err := parser.ParseFile(fileset, "sample.go", source, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse source: %v", err)
	}
	return file, fileset
}
