package branchcond

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
)

// TestCheckKindStaysAliasedToWIR pins CheckKind as a genuine type alias of
// wir.CheckKind (not a parallel redeclaration) and requires every wir
// CheckKind variant to have a same-named branchcond alias constant pointing
// back at it. wir owns the enum; this fails the build the moment the two
// packages' variant names or counts diverge, instead of silently misprinting
// an unrecognized kind as "cond?".
func TestCheckKindStaysAliasedToWIR(t *testing.T) {
	wirNames := wirCheckKindConstNames(t)
	aliasTargets := branchcondCheckKindAliasTargets(t)

	var missing []string
	for _, name := range wirNames {
		if _, ok := aliasTargets[name]; !ok {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	if len(missing) != 0 {
		t.Fatalf("branchcond has no CheckKind alias for wir variant(s): %v; add `Check<Name> = wir.Check<Name>`", missing)
	}

	var extra []string
	known := make(map[string]bool, len(wirNames))
	for _, name := range wirNames {
		known[name] = true
	}
	for name := range aliasTargets {
		if !known[name] {
			extra = append(extra, name)
		}
	}
	sort.Strings(extra)
	if len(extra) != 0 {
		t.Fatalf("branchcond declares CheckKind alias(es) with no wir counterpart: %v", extra)
	}

	for name, target := range aliasTargets {
		if target != name {
			t.Fatalf("branchcond.Check%s aliases wir.Check%s, want wir.Check%s", name, target, name)
		}
	}
}

// wirCheckKindConstNames extracts the CheckKind variant names from wir's
// authoritative const block, in declaration order.
func wirCheckKindConstNames(t *testing.T) []string {
	t.Helper()
	path := repoFilePath(t, "analysis", "ir", "wir", "check.go")
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse check.go: %v", err)
	}
	var names []string
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		inCheckKindBlock := false
		for _, spec := range gen.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			if ident, ok := value.Type.(*ast.Ident); ok {
				inCheckKindBlock = ident.Name == "CheckKind"
			}
			if !inCheckKindBlock {
				continue
			}
			for _, name := range value.Names {
				names = append(names, name.Name)
			}
		}
	}
	if len(names) == 0 {
		t.Fatal("no wir.CheckKind constants found")
	}
	return names
}

// branchcondCheckKindAliasTargets extracts, for each `CheckXxx = wir.CheckYyy`
// alias constant declared in branchcond, the branchcond-facing name mapped to
// the wir-facing name it points at.
func branchcondCheckKindAliasTargets(t *testing.T) map[string]string {
	t.Helper()
	path := repoFilePath(t, "analysis", "lua", "branchcond", "branch_condition.go")
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse branch_condition.go: %v", err)
	}
	targets := make(map[string]string)
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok || len(value.Names) != 1 || len(value.Values) != 1 {
				continue
			}
			sel, ok := value.Values[0].(*ast.SelectorExpr)
			if !ok {
				continue
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok || pkg.Name != "wir" {
				continue
			}
			targets[value.Names[0].Name] = sel.Sel.Name
		}
	}
	if len(targets) == 0 {
		t.Fatal("no branchcond CheckKind = wir.CheckKind alias constants found")
	}
	return targets
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
