package engine

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestDiagnosticFamilyRegistryIsComplete(t *testing.T) {
	if got, want := len(diagnosticFamilies), int(DiagnosticFamilyDeadAssignment)+1; got != want {
		t.Fatalf("registry has %d rows, want %d family IDs", got, want)
	}
	codes := make(map[string]DiagnosticFamilyID, len(diagnosticFamilies))
	prefixes := make(map[string]DiagnosticFamilyID, len(diagnosticFamilies))
	for index, family := range diagnosticFamilies {
		id := DiagnosticFamilyID(index)
		if family.Code == "" || family.KeyPrefix == "" || family.Narrate == nil {
			t.Errorf("family %d has an incomplete registry row: %+v", id, family)
		}
		if !strings.HasSuffix(family.KeyPrefix, "/") {
			t.Errorf("family %d prefix %q has no trailing slash", id, family.KeyPrefix)
		}
		if prior, duplicate := codes[family.Code]; duplicate {
			t.Errorf("families %d and %d share code %q", prior, id, family.Code)
		}
		if prior, duplicate := prefixes[family.KeyPrefix]; duplicate {
			t.Errorf("families %d and %d share prefix %q", prior, id, family.KeyPrefix)
		}
		codes[family.Code], prefixes[family.KeyPrefix] = id, id
		if gotID, gotFamily, _, found := lookupDiagnosticFamily(family.KeyPrefix + "guard"); !found || gotID != id || gotFamily.Code != family.Code {
			t.Errorf("lookup of family %d returned %d/%q/%v", id, gotID, gotFamily.Code, found)
		}
	}
	if got := len(RegisteredDiagnosticFamilies()); got != len(diagnosticFamilies) {
		t.Fatalf("complete membership set has %d families, want %d", got, len(diagnosticFamilies))
	}
}

func TestDiagnosticFamilyLookupPreservesTransportAndFallback(t *testing.T) {
	tests := []struct {
		key       string
		code      string
		operation string
	}{
		{"type.assignment/op-1", "type.assignment", "op-1"},
		{"type.call.direct.argument_type/op-2/argument-00000000", "type.call.direct.argument_type", "op-2"},
		{"type.operator.concat_operand/op-3/value-00000000", "type.operator.concat_operand", "op-3"},
		{"claim/unproven/op-4", "lint.claim.unproven", "op-4"},
		{"child/aabb/claim/unproven/op-5", "lint.claim.unproven", "op-5"},
		{"child/aabb/custom/family/op-6", "lint.custom.family.op-6", "family"},
	}
	for _, test := range tests {
		if got := diagnosticCode(test.key); got != test.code {
			t.Errorf("diagnosticCode(%q) = %q, want %q", test.key, got, test.code)
		}
		if got := diagnosticOperationName(test.key); got != test.operation {
			t.Errorf("diagnosticOperationName(%q) = %q, want %q", test.key, got, test.operation)
		}
	}
}

// TestDiagnosticFamilyPrefixesAreRegistryOwned is the displacement guard: a
// production source file may consume a family ID or registry accessor, but may
// not restate any registered key-prefix literal.
func TestDiagnosticFamilyPrefixesAreRegistryOwned(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate engine source")
	}
	engineDir := filepath.Dir(filename)
	roots := []string{
		engineDir,
		filepath.Join(engineDir, "..", "lint"),
		filepath.Join(engineDir, "..", "fixpoint", "front"),
	}
	forbidden := make(map[string]bool, len(diagnosticFamilies))
	for _, family := range diagnosticFamilies {
		forbidden[family.KeyPrefix] = true
	}
	for _, root := range roots {
		files, err := filepath.Glob(filepath.Join(root, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range files {
			base := filepath.Base(path)
			if strings.HasSuffix(base, "_test.go") || base == "diagnostic_family.go" {
				continue
			}
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				t.Errorf("parse %s: %v", path, err)
				continue
			}
			ast.Inspect(file, func(node ast.Node) bool {
				literal, ok := node.(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					return true
				}
				value, err := strconv.Unquote(literal.Value)
				if err == nil && forbidden[value] {
					t.Errorf("%s restates diagnostic family prefix %q; use its DiagnosticFamilyID", path, value)
				}
				return true
			})
		}
	}
}
