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
// SchemaBinding directory by semantic key afterward. The Link-local record
// keeps no owner or capability of its own, and the rule table's projection
// reaches that one directory rather than caching its answers.
func TestProgramBindingUsesCanonicalCapabilityDirectory(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("program binding source location unavailable")
	}
	path := filepath.Join(filepath.Dir(current), "domain", "composite", "program_binding.go")
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	file, err := parser.ParseFile(token.NewFileSet(), path, source, 0)
	if err != nil {
		t.Fatal(err)
	}
	registryPath := filepath.Join(filepath.Dir(current), "domain", "composite", "rule_registry.go")
	registrySource, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	registryFile, err := parser.ParseFile(token.NewFileSet(), registryPath, registrySource, 0)
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
			if !ok || typeSpec.Name.Name != "ProgramBinding" {
				continue
			}
			binding, _ = typeSpec.Type.(*ast.StructType)
		}
	}
	if binding == nil {
		t.Fatal("ProgramBinding struct not found")
	}
	for _, field := range binding.Fields.List {
		for _, name := range field.Names {
			if strings.HasSuffix(name.Name, "Capability") {
				t.Errorf("ProgramBinding retains copied capability field %q", name.Name)
			}
			switch name.Name {
			case "call", "heap", "pack", "effect":
				t.Errorf("ProgramBinding retains unused cached owner field %q", name.Name)
			}
		}
	}

	if capabilityStores(file) != 0 {
		t.Error("ProgramBinding stores RuleSlotCapability; use SchemaBinding's canonical directory")
	}

	hasVocabulary := false
	bindingRuleSlotCalls := 0
	ast.Inspect(registryFile, func(node ast.Node) bool {
		if field, ok := node.(*ast.Field); ok && len(field.Names) == 1 && field.Names[0].Name == "roles" {
			hasVocabulary = true
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if selector, ok := call.Fun.(*ast.SelectorExpr); ok && selector.Sel.Name == "BindingRuleSlot" {
			bindingRuleSlotCalls++
		}
		return true
	})
	if !hasVocabulary {
		t.Error("the rule registry does not retain the resolved semantic role vocabulary")
	}
	if bindingRuleSlotCalls == 0 {
		t.Error("the rule registry never resolves capabilities through engine.BindingRuleSlot")
	}
	// The registry may hold a capability only for the pre-seal pairing pass,
	// which is a local of one function, never a retained field of a record.
	if stored := capabilityStores(registryFile); stored != 0 {
		t.Errorf("the rule registry retains %d RuleSlotCapability field(s); the sealed directory is the sole authority", stored)
	}
}

// capabilityStores counts retained per-role capability storage: a struct field
// or a map whose value is a RuleSlotCapability.
func capabilityStores(file *ast.File) int {
	stored := 0
	ast.Inspect(file, func(node ast.Node) bool {
		if mapType, ok := node.(*ast.MapType); ok {
			if selector, ok := mapType.Value.(*ast.SelectorExpr); ok && selector.Sel.Name == "RuleSlotCapability" {
				stored++
			}
		}
		structType, ok := node.(*ast.StructType)
		if !ok {
			return true
		}
		for _, field := range structType.Fields.List {
			if capabilityTyped(field.Type) {
				stored++
			}
		}
		return true
	})
	return stored
}

func capabilityTyped(expr ast.Expr) bool {
	switch typed := expr.(type) {
	case *ast.SelectorExpr:
		return typed.Sel.Name == "RuleSlotCapability"
	case *ast.ArrayType:
		return capabilityTyped(typed.Elt)
	case *ast.StarExpr:
		return capabilityTyped(typed.X)
	case *ast.MapType:
		return capabilityTyped(typed.Value)
	default:
		return false
	}
}
