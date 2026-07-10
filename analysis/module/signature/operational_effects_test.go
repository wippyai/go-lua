package signature

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"testing"
)

func TestOperationalEffectLaneRegistryCoversEveryField(t *testing.T) {
	typ := reflect.TypeOf(OperationalEffects{})
	registered := make(map[string]struct{})
	for _, lane := range operationalEffectLanes {
		if lane.fieldName == "" {
			t.Fatal("operational effect lane with empty field name")
		}
		if _, ok := registered[lane.fieldName]; ok {
			t.Fatalf("operational effect lane %s registered more than once", lane.fieldName)
		}
		field, ok := typ.FieldByName(lane.fieldName)
		if !ok {
			t.Fatalf("operational effect lane references missing field %s", lane.fieldName)
		}
		if !operationalEffectsLaneFieldKind(field.Type.Kind()) {
			t.Fatalf("operational effect lane %s references unsupported field kind %s", lane.fieldName, field.Type.Kind())
		}
		registered[lane.fieldName] = struct{}{}
	}
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if !operationalEffectsLaneFieldKind(field.Type.Kind()) {
			continue
		}
		if _, ok := registered[field.Name]; !ok {
			t.Fatalf("OperationalEffects.%s has no registered lane owner", field.Name)
		}
	}
}

func TestOperationalEffectsHotOperationsUseLaneRegistry(t *testing.T) {
	file := parseOperationalEffectsSource(t)
	fields := operationalEffectsLaneFields(t)

	for _, name := range []string{"IsEmpty", "Clone", "Equals"} {
		fn := requireOperationalEffectsFuncDecl(t, file, name)
		if !funcUsesIdent(fn, "operationalEffectLanes") {
			t.Fatalf("OperationalEffects.%s must iterate operationalEffectLanes", name)
		}
		if field := firstSelectedOperationalEffectsField(fn, fields); field != "" {
			t.Fatalf("OperationalEffects.%s selects field %s directly; use operationalEffectLanes", name, field)
		}
	}
}

func TestSubstituteOperationalTypesUsesLaneRegistry(t *testing.T) {
	file := parseOperationalEffectsFile(t, "operational_effects_subst.go")
	fn := requireOperationalEffectsFuncDecl(t, file, "SubstituteOperationalTypes")
	if !funcUsesIdent(fn, "operationalEffectLanes") {
		t.Fatal("SubstituteOperationalTypes must iterate operationalEffectLanes")
	}
	if field := firstSelectedOperationalEffectsField(fn, operationalEffectsLaneFields(t)); field != "" {
		t.Fatalf("SubstituteOperationalTypes selects field %s directly; use operationalEffectLanes", field)
	}
}

func operationalEffectsLaneFieldKind(kind reflect.Kind) bool {
	return kind == reflect.Bool || kind == reflect.Slice
}

func TestOperationalEffectLaneRegistryOwnsTypeSubstitution(t *testing.T) {
	wantTyped := map[string]struct{}{
		"NormalReturnTypeRefinements": {},
		"PathPresenceImplications":    {},
		"PathStaticMembers":           {},
		"PathStaticMemberDeltas":      {},
		"DynamicIndexFacts":           {},
		"ReturnAllocationTemplates":   {},
	}
	for _, lane := range operationalEffectLanes {
		_, want := wantTyped[lane.fieldName]
		got := lane.substituteTypes != nil
		if got != want {
			t.Fatalf("operational effect lane %s substituteTypes present = %v, want %v", lane.fieldName, got, want)
		}
		delete(wantTyped, lane.fieldName)
	}
	for field := range wantTyped {
		t.Fatalf("typed operational effect field %s has no lane owner", field)
	}
}

func parseOperationalEffectsSource(t *testing.T) *ast.File {
	return parseOperationalEffectsFile(t, "operational_effects.go")
}

func parseOperationalEffectsFile(t *testing.T, name string) *ast.File {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return file
}

func requireOperationalEffectsFuncDecl(t *testing.T, file *ast.File, name string) *ast.FuncDecl {
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

func funcUsesIdent(fn *ast.FuncDecl, name string) bool {
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

func operationalEffectsLaneFields(t *testing.T) map[string]struct{} {
	t.Helper()
	out := make(map[string]struct{})
	typ := reflect.TypeOf(OperationalEffects{})
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if operationalEffectsLaneFieldKind(field.Type.Kind()) {
			out[field.Name] = struct{}{}
		}
	}
	return out
}

func firstSelectedOperationalEffectsField(fn *ast.FuncDecl, fields map[string]struct{}) string {
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
