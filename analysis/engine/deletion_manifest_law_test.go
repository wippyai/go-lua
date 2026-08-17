package engine

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// deletionManifestFiles is the compile/receipt spine: every file listed here is
// deleted whole at the receipt flash cut. The list is shrink-only. An entry may
// be removed only once the file is gone from disk; adding an entry is a
// deliberate edit of this constant.
//
// The manifest names files in the engine flat root only; engineSourceFiles
// skips directories, so a surviving package under analysis/engine is out of
// scope by construction. analysis/engine/rows is such a package: it holds
// the ProgramArtifact declaration surface that outlives the cut, and
// artifact_receipt.go keeps only the lowering that dies with the receipt.
var deletionManifestFiles = []string{
	"activation_candidate_issuer.go",
	"artifact_receipt.go",
	"receipt_observation.go",
	"receipt_query_admission.go",
	"receipt_rule_admission.go",
	"receipt_solver.go",
	"schema_query_receipt.go",
	"schema_surface_receipt.go",
	"semantic_directory.go",
	"solver_compiler.go",
	"structural_schedule_certificate.go",
	"structural_witness.go",
}

// deletionReplacementFiles carry the typed result readers that the cut replaces
// rather than deletes outright. Their declarations are the public result API
// until Snapshot.Query supersedes them, so referencing them from any engine
// file is legal. The list exists so their deletion is tracked with the cut.
var deletionReplacementFiles = []string{
	"state_receipt.go",
}

// pinnedRuntimeCompileEdge is one surviving solver-side reference into a
// declaration owned by the deletion manifest. Every edge here dissolves at the
// cut; the set is shrink-only.
type pinnedRuntimeCompileEdge struct {
	from       string
	identifier string
	target     string
	note       string
}

var pinnedRuntimeCompileEdges = []pinnedRuntimeCompileEdge{
	{
		from:       "composition.go",
		identifier: "solverCompiler",
		target:     "solver_compiler.go",
		note:       "Solver.compiler holds the compile entry point that runtime_executor.go invokes",
	},
	{
		from:       "rule_surface.go",
		identifier: "mountedSelectedSurfaceAnchor",
		target:     "solver_compiler.go",
		note:       "RuleReadSurface.anchor and RuleWriteSurface.anchor carry the assembly-owned anchor",
	},
	{
		from:       "rule_surface.go",
		identifier: "bindingSummarySurfaceReceipt",
		target:     "schema_surface_receipt.go",
		note:       "ruleSummaryMapping.receipt is typed by the compile-side summary surface interface",
	},
	{
		from:       "runtime_selected_overlay.go",
		identifier: "validAcceptedActivations",
		target:     "solver_compiler.go",
		note:       "overlay acceptance reads the compiler-owned activation predicate",
	},
	{
		from:       "state_receipt.go",
		identifier: "ReceiptQuery",
		target:     "solver_compiler.go",
		note:       "typed result readers project the compiled query until Snapshot.Query replaces them",
	},
	{
		from:       "state_receipt.go",
		identifier: "ReceiptObservation",
		target:     "receipt_observation.go",
		note:       "typed result readers project the compiled observation until Snapshot.Query replaces them",
	},
}

func TestDeletionManifestPinsRuntimeCompileEdges(t *testing.T) {
	manifest := manifestFileSet(t)
	declared, declaredFiles := manifestDeclarations(t, manifest)
	pinned := make(map[string]pinnedRuntimeCompileEdge, len(pinnedRuntimeCompileEdges))
	for _, edge := range pinnedRuntimeCompileEdges {
		key := edge.from + " -> " + edge.identifier
		if _, duplicate := pinned[key]; duplicate {
			t.Fatalf("duplicate pinned edge %s", key)
		}
		pinned[key] = edge
	}

	observed := map[string]bool{}
	for _, name := range engineSourceFiles(t) {
		if manifest[name] {
			continue
		}
		file := parseEngineFile(t, name)
		for _, reference := range packageLevelReferences(file, declared) {
			key := name + " -> " + reference
			observed[key] = true
			edge, ok := pinned[key]
			if !ok {
				t.Errorf("unpinned runtime->compile edge %s (declared in %s); dissolve it or pin it in pinnedRuntimeCompileEdges", key, declaredFiles[reference])
				continue
			}
			if edge.target != declaredFiles[reference] {
				t.Errorf("pinned edge %s records target %s but %s is declared in %s", key, edge.target, reference, declaredFiles[reference])
			}
		}
	}

	for key := range pinned {
		if !observed[key] {
			t.Errorf("stale pinned edge %s no longer exists; the list is shrink-only, so remove it", key)
		}
	}
}

func TestDeletionManifestIsShrinkOnly(t *testing.T) {
	root := engineRoot(t)
	seen := map[string]bool{}
	for _, name := range append(append([]string{}, deletionManifestFiles...), deletionReplacementFiles...) {
		if seen[name] {
			t.Errorf("duplicate manifest entry %s", name)
		}
		seen[name] = true
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Errorf("manifest entry %s is not on disk; remove the entry in the same change that deletes the file: %v", name, err)
		}
	}
	if !sort.StringsAreSorted(deletionManifestFiles) {
		t.Errorf("deletionManifestFiles must stay sorted for reviewable shrinkage")
	}
	for _, name := range deletionReplacementFiles {
		for _, manifestName := range deletionManifestFiles {
			if name == manifestName {
				t.Errorf("%s is listed as both a deletion and a replacement file", name)
			}
		}
	}
}

func engineRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("engine root: %v", err)
	}
	return root
}

func manifestFileSet(t *testing.T) map[string]bool {
	t.Helper()
	manifest := make(map[string]bool, len(deletionManifestFiles))
	for _, name := range deletionManifestFiles {
		manifest[name] = true
	}
	return manifest
}

func engineSourceFiles(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(engineRoot(t))
	if err != nil {
		t.Fatalf("read engine root: %v", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func parseEngineFile(t *testing.T, name string) *ast.File {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(engineRoot(t), name), nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return file
}

// manifestDeclarations indexes every top-level declaration owned by the
// deletion manifest, including method names, so a reference from a surviving
// file is visible as a name lookup.
func manifestDeclarations(t *testing.T, manifest map[string]bool) (map[string]bool, map[string]string) {
	t.Helper()
	declared := map[string]bool{}
	files := map[string]string{}
	for _, name := range engineSourceFiles(t) {
		if !manifest[name] {
			continue
		}
		file := parseEngineFile(t, name)
		for _, decl := range file.Decls {
			switch typed := decl.(type) {
			case *ast.FuncDecl:
				if typed.Recv != nil {
					continue
				}
				declared[typed.Name.Name] = true
				files[typed.Name.Name] = name
			case *ast.GenDecl:
				for _, spec := range typed.Specs {
					switch specific := spec.(type) {
					case *ast.TypeSpec:
						declared[specific.Name.Name] = true
						files[specific.Name.Name] = name
					case *ast.ValueSpec:
						for _, ident := range specific.Names {
							declared[ident.Name] = true
							files[ident.Name] = name
						}
					}
				}
			}
		}
	}
	delete(declared, "_")
	delete(files, "_")
	return declared, files
}

// packageLevelReferences returns the sorted set of manifest-declared names that
// this file uses as package-level identifiers. Selector fields, struct and
// interface member names, composite-literal field keys, and every binding
// occurrence are excluded so only true package-scope uses are reported.
func packageLevelReferences(file *ast.File, declared map[string]bool) []string {
	bound := boundIdentifiers(file)
	found := map[string]bool{}
	ast.Inspect(file, func(node ast.Node) bool {
		ident, ok := node.(*ast.Ident)
		if !ok || bound[ident] || !declared[ident.Name] {
			return true
		}
		found[ident.Name] = true
		return true
	})
	names := make([]string, 0, len(found))
	for name := range found {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func boundIdentifiers(file *ast.File) map[*ast.Ident]bool {
	bound := map[*ast.Ident]bool{}
	markNames := func(list *ast.FieldList) {
		if list == nil {
			return
		}
		for _, field := range list.List {
			for _, ident := range field.Names {
				bound[ident] = true
			}
		}
	}
	ast.Inspect(file, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.SelectorExpr:
			bound[typed.Sel] = true
		case *ast.StructType:
			markNames(typed.Fields)
		case *ast.InterfaceType:
			markNames(typed.Methods)
		case *ast.FuncType:
			markNames(typed.TypeParams)
			markNames(typed.Params)
			markNames(typed.Results)
		case *ast.FuncDecl:
			bound[typed.Name] = true
			markNames(typed.Recv)
		case *ast.TypeSpec:
			bound[typed.Name] = true
			markNames(typed.TypeParams)
		case *ast.ValueSpec:
			for _, ident := range typed.Names {
				bound[ident] = true
			}
		case *ast.AssignStmt:
			if typed.Tok != token.DEFINE {
				return true
			}
			for _, expr := range typed.Lhs {
				if ident, ok := expr.(*ast.Ident); ok {
					bound[ident] = true
				}
			}
		case *ast.LabeledStmt:
			bound[typed.Label] = true
		case *ast.CompositeLit:
			if !structuralCompositeLiteral(typed.Type) {
				return true
			}
			for _, elt := range typed.Elts {
				pair, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				if ident, ok := pair.Key.(*ast.Ident); ok {
					bound[ident] = true
				}
			}
		}
		return true
	})
	return bound
}

// structuralCompositeLiteral reports whether the literal keys are struct field
// names rather than map or slice index expressions.
func structuralCompositeLiteral(expr ast.Expr) bool {
	switch typed := expr.(type) {
	case nil:
		return true
	case *ast.Ident, *ast.SelectorExpr, *ast.StructType:
		return true
	case *ast.IndexExpr:
		return structuralCompositeLiteral(typed.X)
	case *ast.IndexListExpr:
		return structuralCompositeLiteral(typed.X)
	case *ast.StarExpr:
		return structuralCompositeLiteral(typed.X)
	default:
		return false
	}
}
