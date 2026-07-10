package body

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
)

func TestDecomposableUseTrackerClassifiesEveryWIROpcode(t *testing.T) {
	wirOps := wirOpConstNames(t)
	classified := classifyInstructionCaseOpNames(t)

	var missing []string
	for _, op := range wirOps {
		if !classified[op] {
			missing = append(missing, op)
		}
	}
	sort.Strings(missing)
	if len(missing) != 0 {
		t.Fatalf("classifyInstruction has no explicit case for wir opcode(s): %v; add a policy decision instead of relying on the default", missing)
	}

	var extra []string
	known := make(map[string]bool, len(wirOps))
	for _, op := range wirOps {
		known[op] = true
	}
	for op := range classified {
		if !known[op] {
			extra = append(extra, op)
		}
	}
	sort.Strings(extra)
	if len(extra) != 0 {
		t.Fatalf("classifyInstruction switch references unknown wir opcode(s): %v", extra)
	}
}

func wirOpConstNames(t *testing.T) []string {
	t.Helper()
	path := repoFilePath(t, "analysis", "ir", "wir", "instruction.go")
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse instruction.go: %v", err)
	}
	var names []string
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		inOpBlock := false
		for _, spec := range gen.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			if ident, ok := value.Type.(*ast.Ident); ok {
				inOpBlock = ident.Name == "Op"
			}
			if !inOpBlock {
				continue
			}
			for _, name := range value.Names {
				names = append(names, name.Name)
			}
		}
	}
	if len(names) == 0 {
		t.Fatal("no wir.Op constants found")
	}
	return names
}

func classifyInstructionCaseOpNames(t *testing.T) map[string]bool {
	t.Helper()
	path := repoFilePath(t, "analysis", "check", "body", "decomposable_allocation.go")
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse decomposable_allocation.go: %v", err)
	}
	var sw *ast.SwitchStmt
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "classifyInstruction" {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if s, ok := n.(*ast.SwitchStmt); ok && sw == nil {
				sw = s
				return false
			}
			return true
		})
	}
	if sw == nil {
		t.Fatal("no switch statement found in classifyInstruction")
	}
	names := make(map[string]bool)
	for _, stmt := range sw.Body.List {
		clause, ok := stmt.(*ast.CaseClause)
		if !ok || clause.List == nil {
			continue
		}
		for _, expr := range clause.List {
			sel, ok := expr.(*ast.SelectorExpr)
			if !ok {
				continue
			}
			if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "wir" {
				names[sel.Sel.Name] = true
			}
		}
	}
	if len(names) == 0 {
		t.Fatal("no wir.OpXxx cases found in classifyInstruction switch")
	}
	return names
}

func repoFilePath(t *testing.T, parts ...string) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(thisFile))))
	return filepath.Join(append([]string{root}, parts...)...)
}
