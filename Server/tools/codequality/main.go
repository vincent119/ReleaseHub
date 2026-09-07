// Command codequality enforces structural limits for production Go code.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

const (
	maxFileLines     = 300
	maxFunctionLines = 20
	maxParameters    = 4
	maxNesting       = 3
	maxDependencies  = 10
)

type violation struct {
	path    string
	line    int
	message string
}

type inspector struct {
	fileset    *token.FileSet
	violations []violation
}

func main() {
	root := flag.String("root", "./internal", "root directory to inspect")
	flag.Parse()
	violations, err := inspectRoot(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	printViolations(violations)
	if len(violations) > 0 {
		os.Exit(1)
	}
}

func inspectRoot(root string) ([]violation, error) {
	paths, err := productionGoFiles(root)
	if err != nil {
		return nil, err
	}
	result := make([]violation, 0)
	for _, path := range paths {
		violations, inspectErr := inspectFile(path)
		if inspectErr != nil {
			return nil, inspectErr
		}
		result = append(result, violations...)
	}
	sortViolations(result)
	return result, nil
}

func productionGoFiles(root string) ([]string, error) {
	paths := make([]string, 0)
	err := filepath.WalkDir(root, collectGoFile(&paths))
	sort.Strings(paths)
	return paths, err
}

func collectGoFile(paths *[]string) fs.WalkDirFunc {
	return func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if isProductionGoFile(entry) {
			*paths = append(*paths, path)
		}
		return nil
	}
}

func isProductionGoFile(entry fs.DirEntry) bool {
	if entry.IsDir() {
		return false
	}
	name := entry.Name()
	return strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") && !strings.HasSuffix(name, ".gen.go")
}

func inspectFile(path string) ([]violation, error) {
	if generated, err := isGenerated(path); err != nil || generated {
		return nil, err
	}
	fileset := token.NewFileSet()
	file, err := parser.ParseFile(fileset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	checker := &inspector{fileset: fileset}
	checker.inspectFile(path, file)
	return checker.violations, nil
}

func isGenerated(path string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for line := 0; line < 10 && scanner.Scan(); line++ {
		if strings.Contains(scanner.Text(), "Code generated") {
			return true, nil
		}
	}
	return false, scanner.Err()
}

func (i *inspector) inspectFile(path string, file *ast.File) {
	lines := i.fileset.Position(file.End()).Line
	if lines > maxFileLines {
		i.add(path, 1, fmt.Sprintf("file has %d lines; maximum is %d", lines, maxFileLines))
	}
	for _, declaration := range file.Decls {
		switch value := declaration.(type) {
		case *ast.FuncDecl:
			function := value
			i.inspectFunction(path, function)
		case *ast.GenDecl:
			i.inspectTypes(path, value)
		}
	}
}

func (i *inspector) inspectTypes(path string, declaration *ast.GenDecl) {
	for _, specification := range declaration.Specs {
		typeSpec, ok := specification.(*ast.TypeSpec)
		if !ok {
			continue
		}
		structure, ok := typeSpec.Type.(*ast.StructType)
		if ok && typeDependencyCount(structure) > maxDependencies {
			line := i.fileset.Position(typeSpec.Pos()).Line
			i.add(path, line, fmt.Sprintf("type %s has more than %d direct type dependencies", typeSpec.Name.Name, maxDependencies))
		}
	}
}

func typeDependencyCount(structure *ast.StructType) int {
	dependencies := make(map[string]struct{})
	for _, field := range structure.Fields.List {
		collectTypeDependencies(field.Type, dependencies)
	}
	return len(dependencies)
}

func collectTypeDependencies(expression ast.Expr, dependencies map[string]struct{}) {
	switch value := expression.(type) {
	case *ast.Ident:
		if !isPrimitiveType(value.Name) {
			dependencies[value.Name] = struct{}{}
		}
	case *ast.SelectorExpr:
		dependencies[typeExpressionName(value)] = struct{}{}
	case *ast.StarExpr:
		collectTypeDependencies(value.X, dependencies)
	case *ast.ArrayType:
		collectTypeDependencies(value.Elt, dependencies)
	case *ast.MapType:
		collectTypeDependencies(value.Key, dependencies)
		collectTypeDependencies(value.Value, dependencies)
	}
}

func typeExpressionName(expression *ast.SelectorExpr) string {
	owner, ok := expression.X.(*ast.Ident)
	if !ok {
		return expression.Sel.Name
	}
	return owner.Name + "." + expression.Sel.Name
}

func isPrimitiveType(value string) bool {
	return slices.Contains([]string{
		"bool", "byte", "complex64", "complex128", "error", "float32", "float64",
		"int", "int8", "int16", "int32", "int64", "rune", "string",
		"uint", "uint8", "uint16", "uint32", "uint64", "uintptr",
	}, value)
}

func (i *inspector) inspectFunction(path string, function *ast.FuncDecl) {
	start := i.fileset.Position(function.Pos()).Line
	end := i.fileset.Position(function.End()).Line
	if lines := end - start + 1; lines > maxFunctionLines {
		i.add(path, start, fmt.Sprintf("function %s has %d lines; maximum is %d", function.Name.Name, lines, maxFunctionLines))
	}
	if parameters := parameterCount(function.Type.Params); parameters > maxParameters && !allows(function, "parameters") {
		i.add(path, start, fmt.Sprintf("function %s has %d parameters; maximum is %d", function.Name.Name, parameters, maxParameters))
	}
	if depth := functionNesting(function.Body); depth > maxNesting {
		i.add(path, start, fmt.Sprintf("function %s has nesting depth %d; maximum is %d", function.Name.Name, depth, maxNesting))
	}
}

func allows(function *ast.FuncDecl, rule string) bool {
	if function.Doc == nil {
		return false
	}
	return strings.Contains(function.Doc.Text(), "codequality:allow-"+rule)
}

func parameterCount(fields *ast.FieldList) int {
	if fields == nil {
		return 0
	}
	count := 0
	for _, field := range fields.List {
		count += max(1, len(field.Names))
	}
	return count
}

func functionNesting(body *ast.BlockStmt) int {
	maximum := 0
	visitor := &nestingVisitor{maximum: &maximum}
	ast.Walk(visitor, body)
	return maximum
}

type nestingVisitor struct {
	depth   int
	maximum *int
}

func (v *nestingVisitor) Visit(node ast.Node) ast.Visitor {
	if node == nil {
		return nil
	}
	if !isNestingNode(node) {
		return v
	}
	depth := v.depth + 1
	if depth > *v.maximum {
		*v.maximum = depth
	}
	return &nestingVisitor{depth: depth, maximum: v.maximum}
}

func isNestingNode(node ast.Node) bool {
	switch node.(type) {
	case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
		return true
	default:
		return false
	}
}

func (i *inspector) add(path string, line int, message string) {
	i.violations = append(i.violations, violation{path: path, line: line, message: message})
}

func sortViolations(values []violation) {
	sort.Slice(values, func(left, right int) bool {
		if values[left].path == values[right].path {
			return values[left].line < values[right].line
		}
		return values[left].path < values[right].path
	})
}

func printViolations(values []violation) {
	for _, value := range values {
		fmt.Printf("%s:%d: %s\n", value.path, value.line, value.message)
	}
}
