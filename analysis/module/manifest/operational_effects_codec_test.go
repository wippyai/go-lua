package manifest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/module/signature"
)

func TestOperationalEffectsWireFieldsMirrorSignatureFields(t *testing.T) {
	signatureFields := sliceFieldNames(reflect.TypeOf(signature.OperationalEffects{}))
	wireFields := sliceFieldNames(reflect.TypeOf(operationalEffectsWire{}))

	for field := range signatureFields {
		if _, ok := wireFields[field]; !ok {
			t.Fatalf("signature.OperationalEffects.%s has no operationalEffectsWire field", field)
		}
	}
	for field := range wireFields {
		if _, ok := signatureFields[field]; !ok {
			t.Fatalf("operationalEffectsWire.%s has no signature.OperationalEffects field", field)
		}
	}
}

func TestOperationalEffectsWireLaneRegistryCoversEveryWireField(t *testing.T) {
	wireFields := sliceFieldNames(reflect.TypeOf(operationalEffectsWire{}))
	registered := make(map[string]struct{})
	for _, lane := range operationalEffectsWireLanes {
		if lane.fieldName == "" {
			t.Fatal("operational effects wire lane with empty field name")
		}
		if lane.canonicalize == nil {
			t.Fatalf("operational effects wire lane %s has nil canonicalizer", lane.fieldName)
		}
		if _, ok := registered[lane.fieldName]; ok {
			t.Fatalf("operational effects wire lane %s registered more than once", lane.fieldName)
		}
		if _, ok := wireFields[lane.fieldName]; !ok {
			t.Fatalf("operational effects wire lane references missing field %s", lane.fieldName)
		}
		registered[lane.fieldName] = struct{}{}
	}
	for field := range wireFields {
		if _, ok := registered[field]; !ok {
			t.Fatalf("operationalEffectsWire.%s has no registered wire lane", field)
		}
	}
}

func TestCanonicalizeOperationalEffectsWireUsesLaneRegistry(t *testing.T) {
	file := parseOperationalEffectsCodecSource(t)
	fn := requireManifestFuncDecl(t, file, "canonicalizeOperationalEffectsWire")
	if !manifestFuncUsesIdent(fn, "operationalEffectsWireLanes") {
		t.Fatal("canonicalizeOperationalEffectsWire must iterate operationalEffectsWireLanes")
	}
	if field := firstSelectedManifestField(fn, sliceFieldNames(reflect.TypeOf(operationalEffectsWire{}))); field != "" {
		t.Fatalf("canonicalizeOperationalEffectsWire selects field %s directly; use operationalEffectsWireLanes", field)
	}
}

func sliceFieldNames(typ reflect.Type) map[string]struct{} {
	out := make(map[string]struct{})
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if field.Type.Kind() == reflect.Slice {
			out[field.Name] = struct{}{}
		}
	}
	return out
}

func parseOperationalEffectsCodecSource(t *testing.T) *ast.File {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "operational_effects_codec.go", nil, 0)
	if err != nil {
		t.Fatalf("parse operational_effects_codec.go: %v", err)
	}
	return file
}

func requireManifestFuncDecl(t *testing.T, file *ast.File, name string) *ast.FuncDecl {
	t.Helper()
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Name.Name == name {
			return fn
		}
	}
	t.Fatalf("function %s not found", name)
	return nil
}

func manifestFuncUsesIdent(fn *ast.FuncDecl, name string) bool {
	found := false
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		ident, ok := node.(*ast.Ident)
		if ok && ident.Name == name {
			found = true
			return false
		}
		return true
	})
	return found
}

func firstSelectedManifestField(fn *ast.FuncDecl, fields map[string]struct{}) string {
	var found string
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		sel, ok := node.(*ast.SelectorExpr)
		if ok {
			if _, isField := fields[sel.Sel.Name]; isField {
				found = sel.Sel.Name
				return false
			}
		}
		return true
	})
	return found
}
