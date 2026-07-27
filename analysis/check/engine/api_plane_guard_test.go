package engine_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

func TestCollapsedAPIPlanesStayCollapsed(t *testing.T) {
	root := corpusRepositoryRoot(t)
	forbiddenFunctions := map[string]bool{
		"NarrowByOriginView":                             true,
		"TypeFromOriginView":                             true,
		"ProjectOriginView":                              true,
		"NarrowOriginByPathView":                         true,
		"NarrowOriginByPathTypeView":                     true,
		"NarrowVariantByOrigin":                          true,
		"NarrowVariantByOriginView":                      true,
		"TypeFromVariantOrigin":                          true,
		"TypeFromVariantOriginView":                      true,
		"OriginCasesForTypeView":                         true,
		"CheckWithImports":                               true,
		"CheckWithImportsAndResolver":                    true,
		"CheckWithImportsResolverAndGlobals":             true,
		"CheckWithImportsResolverAndGlobalsAndRelations": true,
		"LowerFunction":                                  true,
		"LowerFunctionWithOptions":                       true,
		"LowerWithResolver":                              true,
		"LowerWithResolverAndOptions":                    true,
		"LowerFunctionWithResolver":                      true,
		"LowerFunctionWithResolverAndOptions":            true,
		"BuiltinTopMarker":                               true,
		"IsBuiltinTopMarker":                             true,
	}
	forbiddenTypes := map[string]bool{
		"caseSelection":       true,
		"variantCases":        true,
		"SpansView":           true,
		"BoundarySpansView":   true,
		"UnitNamespace":       true,
		"DeepElementOf":       true,
		"StringUnpackValue":   true,
		"SelectCaseOfParam":   true,
		"SelectResultOfCases": true,
		"VariadicTransform":   true,
		"TypePredicate":       true,
	}
	forbiddenFiles := map[string]bool{
		"domain/value/internal/typegraph/path.go":          true,
		"lua/internal/typegraph/path.go":                   true,
		"type/internal/graph/path.go":                      true,
		"type/table/top_marker.go":                         true,
		"domain/effect/dispatch/reserved.go":               true,
		"domain/value/variant/origin_view_test.go":         true,
		"lua/branchcond/check_kind_exhaustiveness_test.go": true,
	}
	err := filepath.WalkDir(filepath.Join(root, "analysis"), func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, relativeErr := filepath.Rel(filepath.Join(root, "analysis"), path)
		if relativeErr != nil {
			return relativeErr
		}
		if forbiddenFiles[filepath.ToSlash(relative)] {
			t.Errorf("%s resurrects a displaced API plane", path)
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		for _, declaration := range file.Decls {
			switch declaration := declaration.(type) {
			case *ast.FuncDecl:
				if forbiddenFunctions[declaration.Name.Name] {
					t.Errorf("%s resurrects collapsed function %s", path, declaration.Name.Name)
				}
			case *ast.GenDecl:
				for _, spec := range declaration.Specs {
					if named, ok := spec.(*ast.TypeSpec); ok && forbiddenTypes[named.Name.Name] {
						t.Errorf("%s resurrects collapsed type %s", path, named.Name.Name)
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
