package callboundary

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

func TestProjectPathRefinementValueUsesCanonicalBoundaryRule(t *testing.T) {
	reg := standard.Registry()
	for _, value := range []product.Value{product.Bottom(reg), product.Top()} {
		if _, ok := ProjectPathRefinementValue(reg, value); ok {
			t.Fatalf("non-useful refinement value %#v projected", value)
		}
	}
	value := typevalue.FromType(reg, typ.String)
	got, ok := ProjectPathRefinementValue(reg, value)
	if !ok || !product.Equal(reg, got, product.ProjectBoundary(reg, value)) {
		t.Fatalf("boundary refinement projection = %#v/%v", got, ok)
	}
}

func TestNormalReturnFactLaneRegistryCoversEveryStorageField(t *testing.T) {
	typ := reflect.TypeOf(NormalReturnFacts{})
	registeredFields := make(map[string]NormalReturnFactLaneID)
	registeredIDs := make(map[NormalReturnFactLaneID]string)
	for _, lane := range NormalReturnFactLanes() {
		if lane.ID() == "" {
			t.Fatalf("normal-return fact lane with empty ID for field %s", lane.FieldName())
		}
		if previous, ok := registeredIDs[lane.ID()]; ok {
			t.Fatalf("normal-return fact lane ID %q registered for both %s and %s", lane.ID(), previous, lane.FieldName())
		}
		registeredIDs[lane.ID()] = lane.FieldName()
		field, ok := typ.FieldByName(lane.FieldName())
		if !ok {
			t.Fatalf("normal-return fact lane %q references missing field %s", lane.ID(), lane.FieldName())
		}
		if field.Type.Kind() != reflect.Slice {
			t.Fatalf("normal-return fact lane %q references non-slice field %s", lane.ID(), lane.FieldName())
		}
		registeredFields[lane.FieldName()] = lane.ID()
	}
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if field.Type.Kind() != reflect.Slice {
			continue
		}
		if _, ok := registeredFields[field.Name]; !ok {
			t.Fatalf("NormalReturnFacts.%s has no registered lane owner", field.Name)
		}
	}
}

func TestNormalReturnFactsEmptyAndAppendCoverEveryLane(t *testing.T) {
	typ := reflect.TypeOf(NormalReturnFacts{})
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if field.Type.Kind() != reflect.Slice {
			continue
		}
		fact := normalReturnFactsWithOneElement(t, field.Name)
		if fact.Empty() {
			t.Fatalf("NormalReturnFacts.Empty ignored lane %s", field.Name)
		}
		appended := NormalReturnFacts{}.Append(fact)
		got := reflect.ValueOf(appended).FieldByName(field.Name)
		if got.Len() != 1 {
			t.Fatalf("NormalReturnFacts.Append ignored lane %s: len=%d", field.Name, got.Len())
		}
	}
}

func TestBindNormalReturnFactLanesUsesStorageOrder(t *testing.T) {
	handlers := normalReturnFactLaneTestHandlers()
	bindings := BindNormalReturnFactLanes("test", handlers, func(v int) bool { return v > 0 })
	storage := NormalReturnFactLanes()
	if len(bindings) != len(storage) {
		t.Fatalf("bindings len = %d, want %d", len(bindings), len(storage))
	}
	for i, binding := range bindings {
		if binding.ID != storage[i].ID() {
			t.Fatalf("binding[%d].ID = %q, want storage lane %q", i, binding.ID, storage[i].ID())
		}
		if binding.Storage.ID() != storage[i].ID() {
			t.Fatalf("binding[%d].Storage = %q, want %q", i, binding.Storage.ID(), storage[i].ID())
		}
		if binding.Value != i+1 {
			t.Fatalf("binding[%d].Value = %d, want %d", i, binding.Value, i+1)
		}
	}
}

func TestBindNormalReturnFactLanesRejectsMissingHandler(t *testing.T) {
	handlers := normalReturnFactLaneTestHandlers()
	delete(handlers, NormalReturnFactLanes()[0].ID())
	mustPanic(t, func() {
		_ = BindNormalReturnFactLanes("test", handlers, func(v int) bool { return v > 0 })
	})
}

func TestBindNormalReturnFactLanesRejectsInvalidHandler(t *testing.T) {
	handlers := normalReturnFactLaneTestHandlers()
	handlers[NormalReturnFactLanes()[0].ID()] = 0
	mustPanic(t, func() {
		_ = BindNormalReturnFactLanes("test", handlers, func(v int) bool { return v > 0 })
	})
}

func TestBindNormalReturnFactLanesRejectsOrphanHandler(t *testing.T) {
	handlers := normalReturnFactLaneTestHandlers()
	handlers[NormalReturnFactLaneID("orphan")] = 1
	mustPanic(t, func() {
		_ = BindNormalReturnFactLanes("test", handlers, func(v int) bool { return v > 0 })
	})
}

func TestNormalReturnFactsHotOperationsUseLaneRegistry(t *testing.T) {
	file := parseNormalReturnFactsSource(t)
	fields := normalReturnFactsStorageFields(t)

	empty := requireFuncDecl(t, file, "Empty")
	if !funcUsesIdent(empty, "normalReturnFactLanes") {
		t.Fatalf("NormalReturnFacts.Empty must iterate normalReturnFactLanes")
	}
	if field := firstSelectedStorageField(empty, fields); field != "" {
		t.Fatalf("NormalReturnFacts.Empty selects storage field %s directly; use lane registry", field)
	}

	appendNonEmpty := requireFuncDecl(t, file, "appendNonEmptyNormalReturnFacts")
	if !funcUsesIdent(appendNonEmpty, "normalReturnFactLanes") {
		t.Fatalf("appendNonEmptyNormalReturnFacts must iterate normalReturnFactLanes")
	}
	if field := firstSelectedStorageField(appendNonEmpty, fields); field != "" {
		t.Fatalf("appendNonEmptyNormalReturnFacts selects storage field %s directly; use lane registry", field)
	}
}

func normalReturnFactLaneTestHandlers() map[NormalReturnFactLaneID]int {
	handlers := make(map[NormalReturnFactLaneID]int)
	for i, lane := range NormalReturnFactLanes() {
		handlers[lane.ID()] = i + 1
	}
	return handlers
}

func mustPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	fn()
}

func TestNormalReturnFactsAppendFastPathsEmptySides(t *testing.T) {
	facts := NormalReturnFacts{
		PathRefinements: []PathValueFact{{Path: pathdom.NewPlaceholder(0)}},
	}

	allocs := testing.AllocsPerRun(1000, func() {
		_ = NormalReturnFacts{}.Append(facts)
		_ = facts.Append(NormalReturnFacts{})
	})
	if allocs != 0 {
		t.Fatalf("empty-side append allocations = %.2f, want 0", allocs)
	}
}

func TestNormalReturnFactsAppendDoesNotMutateLeftInputBackingArray(t *testing.T) {
	leftSlice := make([]PathValueFact, 1, 2)
	leftSlice[0] = PathValueFact{Path: pathdom.NewPlaceholder(0)}
	left := NormalReturnFacts{PathRefinements: leftSlice}
	right := NormalReturnFacts{PathRefinements: []PathValueFact{{Path: pathdom.NewPlaceholder(1)}}}

	got := left.Append(right)
	got.PathRefinements[0] = PathValueFact{Path: pathdom.NewPlaceholder(9)}

	if !left.PathRefinements[0].Path.Equal(pathdom.NewPlaceholder(0)) {
		t.Fatalf("left input was mutated through append result: %#v", left.PathRefinements)
	}
	if len(got.PathRefinements) != 2 || !got.PathRefinements[1].Path.Equal(pathdom.NewPlaceholder(1)) {
		t.Fatalf("append result = %#v, want left then right facts", got.PathRefinements)
	}
}

func TestNormalReturnFactsAppendNonEmptyAllocatesOnlyJoinedLane(t *testing.T) {
	left := NormalReturnFacts{PathRefinements: []PathValueFact{{Path: pathdom.Path{Root: "left"}}}}
	right := NormalReturnFacts{PathRefinements: []PathValueFact{{Path: pathdom.Path{Root: "right"}}}}

	allocs := testing.AllocsPerRun(1000, func() {
		_ = left.Append(right)
	})
	if allocs > 1 {
		t.Fatalf("non-empty append allocations = %.2f, want <= 1 joined lane allocation", allocs)
	}
}

func TestNormalReturnFactsFilterPathsIsLaneOwned(t *testing.T) {
	match := pathdom.Path{Root: "ret[0]"}
	other := pathdom.Path{Root: "ret[1]"}
	in := NormalReturnFacts{
		PathRefinements:        []PathValueFact{{Path: match}, {Path: other}},
		PersistentPathWrites:   []PathValueFact{{Path: match}},
		PathStaticMembers:      []PathStaticMemberFact{{Path: match}, {Path: other}},
		PathStaticMemberDeltas: []PathStaticMemberDeltaFact{{Path: match}, {Path: other}},
		PathInvalidations:      []PathInvalidationFact{{Path: match}, {Path: other}},
		DynamicIndexFacts:      []DynamicIndexFact{{Table: match}, {Table: other}},
		KeyMemberships:         []KeyMembershipFact{{Key: other, Table: match}, {Key: other, Table: other}},
		DynamicValueKeys:       []DynamicValueKeyMembershipFact{{Container: match, Table: other}, {Container: other, Table: other}},
		DynamicAllValues:       []DynamicAllValueKeyMembershipFact{{Container: other, Table: match}, {Container: other, Table: other}},
		BranchProofs:           []BranchProof{{Path: other, Other: match}, {Path: other, Other: other}},
		ChannelSelects:         []ChannelSelectFact{{Result: match}, {Result: other}},
		FrozenTables:           []FrozenTableFact{{Target: match}, {Target: other}},
		EffectDeltas:           []EffectDelta{{Target: match}, {Target: other}},
		EscapeEvents:           []EscapeEventFact{{Target: match}, {Target: other}},
		StoreRelations:         []StoreRelationFact{{Source: other, Into: match}, {Source: other, Into: other}},
		LifecycleFacts:         []LifecycleFact{{Target: match}, {Target: other}},
		NumFloors:              []NumFloorFact{{Path: match}, {Path: other}},
		RelConstraints:         []RelConstraintFact{{A: RelOperand{Path: other}, B: RelOperand{Path: match}, C: RelOperand{Path: other}}, {A: RelOperand{Path: other}, C: RelOperand{Path: other}}},
	}

	got := in.FilterPaths(func(p pathdom.Path) bool {
		return p.Equal(match)
	})

	assertLen := func(name string, got, want int) {
		t.Helper()
		if got != want {
			t.Fatalf("%s length = %d, want %d: %#v", name, got, want, got)
		}
	}
	assertLen("PathRefinements", len(got.PathRefinements), 1)
	assertLen("PersistentPathWrites", len(got.PersistentPathWrites), 0)
	assertLen("PathStaticMembers", len(got.PathStaticMembers), 1)
	assertLen("PathStaticMemberDeltas", len(got.PathStaticMemberDeltas), 1)
	assertLen("PathInvalidations", len(got.PathInvalidations), 1)
	assertLen("DynamicIndexFacts", len(got.DynamicIndexFacts), 1)
	assertLen("KeyMemberships", len(got.KeyMemberships), 1)
	assertLen("DynamicValueKeys", len(got.DynamicValueKeys), 1)
	assertLen("DynamicAllValues", len(got.DynamicAllValues), 1)
	assertLen("BranchProofs", len(got.BranchProofs), 1)
	assertLen("ChannelSelects", len(got.ChannelSelects), 1)
	assertLen("FrozenTables", len(got.FrozenTables), 1)
	assertLen("EffectDeltas", len(got.EffectDeltas), 1)
	assertLen("EscapeEvents", len(got.EscapeEvents), 1)
	assertLen("StoreRelations", len(got.StoreRelations), 1)
	assertLen("LifecycleFacts", len(got.LifecycleFacts), 1)
	assertLen("NumFloors", len(got.NumFloors), 1)
	assertLen("RelConstraints", len(got.RelConstraints), 1)
}

func TestNormalReturnFactsDropFactsTouchingPathsIsLaneOwned(t *testing.T) {
	drop := pathdom.Path{Root: "ret[0]"}.Field("value")
	keep := pathdom.Path{Root: "ret[1]"}.Field("value")
	in := NormalReturnFacts{
		PathRefinements:        []PathValueFact{{Path: drop}, {Path: keep}},
		PersistentPathWrites:   []PathValueFact{{Path: drop}},
		PathStaticMembers:      []PathStaticMemberFact{{Path: drop}, {Path: keep}},
		PathStaticMemberDeltas: []PathStaticMemberDeltaFact{{Path: drop}, {Path: keep}},
		PathInvalidations:      []PathInvalidationFact{{Path: drop}, {Path: keep}},
		DynamicIndexFacts:      []DynamicIndexFact{{Table: drop}, {Table: keep}},
		KeyMemberships:         []KeyMembershipFact{{Key: keep, Table: drop}, {Key: keep, Table: keep}},
		DynamicValueKeys:       []DynamicValueKeyMembershipFact{{Container: drop, Table: keep}, {Container: keep, Table: keep}},
		DynamicAllValues:       []DynamicAllValueKeyMembershipFact{{Container: keep, Table: drop}, {Container: keep, Table: keep}},
		BranchProofs:           []BranchProof{{Path: keep, Other: drop}, {Path: keep, Other: keep}},
		ChannelSelects:         []ChannelSelectFact{{Result: drop}, {Result: keep}},
		FrozenTables:           []FrozenTableFact{{Target: drop}, {Target: keep}},
		EffectDeltas:           []EffectDelta{{Target: drop}, {Target: keep}},
		EscapeEvents:           []EscapeEventFact{{Target: drop}, {Target: keep}},
		StoreRelations:         []StoreRelationFact{{Source: keep, Into: drop}, {Source: keep, Into: keep}},
		LifecycleFacts:         []LifecycleFact{{Target: drop}, {Target: keep}},
		NumFloors:              []NumFloorFact{{Path: drop}, {Path: keep}},
		RelConstraints:         []RelConstraintFact{{A: RelOperand{Path: keep}, B: RelOperand{Path: drop}, C: RelOperand{Path: keep}}, {A: RelOperand{Path: keep}, C: RelOperand{Path: keep}}},
	}

	got := in.DropFactsTouchingPaths(func(p pathdom.Path) bool {
		return p.Equal(drop)
	})

	assertLen := func(name string, got, want int) {
		t.Helper()
		if got != want {
			t.Fatalf("%s length = %d, want %d", name, got, want)
		}
	}
	assertLen("PathRefinements", len(got.PathRefinements), 1)
	assertLen("PersistentPathWrites", len(got.PersistentPathWrites), 1)
	assertLen("PathStaticMembers", len(got.PathStaticMembers), 1)
	assertLen("PathStaticMemberDeltas", len(got.PathStaticMemberDeltas), 1)
	assertLen("PathInvalidations", len(got.PathInvalidations), 1)
	assertLen("DynamicIndexFacts", len(got.DynamicIndexFacts), 1)
	assertLen("KeyMemberships", len(got.KeyMemberships), 1)
	assertLen("DynamicValueKeys", len(got.DynamicValueKeys), 1)
	assertLen("DynamicAllValues", len(got.DynamicAllValues), 1)
	assertLen("BranchProofs", len(got.BranchProofs), 1)
	assertLen("ChannelSelects", len(got.ChannelSelects), 1)
	assertLen("FrozenTables", len(got.FrozenTables), 1)
	assertLen("EffectDeltas", len(got.EffectDeltas), 1)
	assertLen("EscapeEvents", len(got.EscapeEvents), 1)
	assertLen("StoreRelations", len(got.StoreRelations), 1)
	assertLen("LifecycleFacts", len(got.LifecycleFacts), 1)
	assertLen("NumFloors", len(got.NumFloors), 1)
	assertLen("RelConstraints", len(got.RelConstraints), 1)
}

func normalReturnFactsWithOneElement(t *testing.T, fieldName string) NormalReturnFacts {
	t.Helper()
	out := NormalReturnFacts{}
	value := reflect.ValueOf(&out).Elem().FieldByName(fieldName)
	if !value.IsValid() || value.Kind() != reflect.Slice {
		t.Fatalf("NormalReturnFacts.%s is not a slice lane", fieldName)
	}
	elem := reflect.New(value.Type().Elem()).Elem()
	value.Set(reflect.Append(value, elem))
	return out
}

func parseNormalReturnFactsSource(t *testing.T) *ast.File {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "normal_return_facts.go", nil, 0)
	if err != nil {
		t.Fatalf("parse normal_return_facts.go: %v", err)
	}
	return file
}

func requireFuncDecl(t *testing.T, file *ast.File, name string) *ast.FuncDecl {
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

func normalReturnFactsStorageFields(t *testing.T) map[string]struct{} {
	t.Helper()
	out := make(map[string]struct{})
	typ := reflect.TypeOf(NormalReturnFacts{})
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if field.Type.Kind() == reflect.Slice {
			out[field.Name] = struct{}{}
		}
	}
	return out
}

func firstSelectedStorageField(fn *ast.FuncDecl, fields map[string]struct{}) string {
	var found string
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		sel, ok := node.(*ast.SelectorExpr)
		if ok {
			if _, isStorageField := fields[sel.Sel.Name]; isStorageField {
				found = sel.Sel.Name
				return false
			}
		}
		return true
	})
	return found
}
