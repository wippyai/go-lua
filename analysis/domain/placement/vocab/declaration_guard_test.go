package vocab_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// TestCanonicalVocabularyOwnsEscapeAndPlacementEnums prevents the parallel
// spellings displaced by this package from growing back. Compatibility aliases
// are permitted; new scalar type definitions are not.
func TestCanonicalVocabularyOwnsEscapeAndPlacementEnums(t *testing.T) {
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller did not locate declaration guard")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(here), "../../../.."))
	vocabDir := filepath.ToSlash(filepath.Join("analysis", "domain", "placement", "vocab"))

	// These scalar enums are distinct axes, not alternate spellings of an
	// allocation disposition. Keeping the allowlist exact makes additions fail
	// until an architect classifies them.
	allowed := map[string]bool{
		"analysis/diagnostic/types.go:LabelPlacement":       true, // diagnostic label geometry
		"analysis/domain/placement/license.go:LicenseKind":  true, // proof-record coordinate
		"analysis/domain/placement/license.go:LicenseState": true, // proof evidence state
		"analysis/domain/value/axis/escape/escape.go:Value": true, // fresh/escaped value-product lattice
	}

	var violations []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if entry.IsDir() {
			switch {
			case rel == ".git", rel == "vendor", rel == "testdata", rel == ".claude", rel == "__legacy":
				return filepath.SkipDir
			case rel == vocabDir:
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				continue
			}
			for _, spec := range gen.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok || typeSpec.Assign.IsValid() || !scalarUnderlying(typeSpec.Type) {
					continue
				}
				name := typeSpec.Name.Name
				semanticName := strings.ToLower(name)
				semanticPackage := strings.ToLower(file.Name.Name)
				if !strings.Contains(semanticName, "escape") &&
					!strings.Contains(semanticName, "placement") &&
					!strings.Contains(semanticPackage, "escape") &&
					semanticPackage != "placement" {
					continue
				}
				key := rel + ":" + name
				if !allowed[key] {
					violations = append(violations, key)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan enum declarations: %v", err)
	}
	sort.Strings(violations)
	if len(violations) != 0 {
		t.Fatalf("escape/placement scalar enum declarations must live in %s; use a type alias or a wire-boundary converter:\n%s",
			vocabDir, strings.Join(violations, "\n"))
	}
}

func scalarUnderlying(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	if !ok {
		return false
	}
	switch ident.Name {
	case "byte", "rune", "string",
		"int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64":
		return true
	default:
		return false
	}
}
