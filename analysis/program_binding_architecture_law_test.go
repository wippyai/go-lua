package analysis

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestProgramBindingUsesCanonicalCapabilityDirectory keeps the hot binding
// from regressing into a second per-role capability registry. Capabilities
// are short-lived during pre-seal registration and resolved from the sealed
// SchemaBinding directory by semantic key afterward.
func TestProgramBindingUsesCanonicalCapabilityDirectory(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("program binding source location unavailable")
	}
	path := filepath.Join(filepath.Dir(current), "program_binding.go")
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	file, err := parser.ParseFile(token.NewFileSet(), path, source, 0)
	if err != nil {
		t.Fatal(err)
	}

	var binding *ast.StructType
	for _, declaration := range file.Decls {
		gen, ok := declaration.(*ast.GenDecl)
		if !ok || gen.Tok.String() != "type" {
			continue
		}
		for _, spec := range gen.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || typeSpec.Name.Name != "programBinding" {
				continue
			}
			binding, _ = typeSpec.Type.(*ast.StructType)
		}
	}
	if binding == nil {
		t.Fatal("programBinding struct not found")
	}
	for _, field := range binding.Fields.List {
		for _, name := range field.Names {
			if strings.HasSuffix(name.Name, "Capability") {
				t.Errorf("programBinding retains copied capability field %q", name.Name)
			}
			switch name.Name {
			case "call", "heap", "pack", "effect":
				t.Errorf("programBinding retains unused cached owner field %q", name.Name)
			}
		}
	}

	hasVocabulary := false
	bindingRuleSlotCalls := 0
	capabilityMaps := 0
	ast.Inspect(file, func(node ast.Node) bool {
		field, ok := node.(*ast.Field)
		if ok && len(field.Names) == 1 && field.Names[0].Name == "vocabulary" {
			hasVocabulary = true
		}
		if mapType, ok := node.(*ast.MapType); ok {
			if selector, ok := mapType.Value.(*ast.SelectorExpr); ok && selector.Sel.Name == "RuleSlotCapability" {
				capabilityMaps++
			}
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if ok && selector.Sel.Name == "BindingRuleSlot" {
			bindingRuleSlotCalls++
		}
		return true
	})
	if !hasVocabulary {
		t.Error("programBinding does not retain the canonical semanticvocabulary.Bundle")
	}
	if bindingRuleSlotCalls == 0 {
		t.Error("programBinding never resolves capabilities through engine.BindingRuleSlot")
	}
	if capabilityMaps != 0 {
		t.Errorf("programBinding introduces %d RuleSlotCapability map(s); use SchemaBinding's canonical directory", capabilityMaps)
	}
}
