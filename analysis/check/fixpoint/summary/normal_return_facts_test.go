package summary

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestNormalReturnSummaryLaneRegistryCoversStorageLanes(t *testing.T) {
	storage := callboundary.NormalReturnFactLanes()
	if len(normalReturnSummaryLanes) != len(storage) {
		t.Fatalf("summary normal-return lanes = %d, want storage lane count %d", len(normalReturnSummaryLanes), len(storage))
	}
	for _, lane := range normalReturnSummaryLanes {
		if !normalReturnSummaryLaneValid(lane.Value) {
			t.Fatal("summary normal-return lane has incomplete behavior")
		}
	}
}

func TestNormalReturnFactsSummaryOperationsUseLaneRegistry(t *testing.T) {
	file := parseNormalReturnFactsSource(t)
	fields := normalReturnFactsStorageFields(t)

	for _, name := range []string{"normalizeNormalReturnFactsWith", "CloneNormalReturnFacts"} {
		fn := requireFuncDecl(t, file, name)
		if !funcUsesIdent(fn, "normalReturnSummaryLanes") {
			t.Fatalf("%s must iterate normalReturnSummaryLanes", name)
		}
		if field := firstSelectedStorageField(fn, fields); field != "" {
			t.Fatalf("%s selects storage field %s directly; use normalReturnSummaryLanes", name, field)
		}
	}
}

func TestNormalReturnFactsNormalizeKeepsConcreteCapturedPathBoundaryFacts(t *testing.T) {
	reg := mustRegistry(t)
	placeholder := pathdom.NewPlaceholder(0).Field("field")
	returnSlot := pathdom.Path{Root: "ret[0]"}.Field("field")
	concrete := pathdom.NewPath(symbol.ID(10), "arg").Field("field")
	value := presentProduct(reg)

	got := Normalize(reg, Summary{NormalReturnFacts: callboundary.NormalReturnFacts{
		PathRefinements: []callboundary.PathValueFact{
			{Path: concrete, Value: value},
			{Path: placeholder, Value: value},
		},
		PersistentPathWrites: []callboundary.PathValueFact{
			{Path: concrete, Value: value},
			{Path: placeholder, Value: value},
		},
		PathStaticMembers: []callboundary.PathStaticMemberFact{
			{Path: concrete, Value: value},
			{Path: placeholder, Value: value},
			{Path: returnSlot, Value: value},
		},
		PathInvalidations: []callboundary.PathInvalidationFact{
			{Path: concrete},
			{Path: placeholder},
		},
		DynamicIndexFacts: []callboundary.DynamicIndexFact{
			{Table: concrete, Site: "caller.dynamic.ignored", Value: dynamicindex.Fact{KeyPresence: presence.Present()}},
			{Table: placeholder, Site: "caller.dynamic.1", Value: dynamicindex.Fact{KeyPresence: presence.Present()}},
		},
		BranchProofs: []callboundary.BranchProof{
			{Kind: pathevidence.BranchProofPathPresence, Path: concrete, Presence: presence.Present()},
			{Kind: pathevidence.BranchProofPathPresence, Path: placeholder, Presence: presence.Present()},
			{Kind: pathevidence.BranchProofPathEqual, Path: placeholder, Other: concrete},
			{Kind: pathevidence.BranchProofPathEqual, Path: placeholder, Other: pathdom.NewPlaceholder(1)},
		},
		ChannelSelects: []callboundary.ChannelSelectFact{
			{Select: channelselectfact.ID("select-concrete"), Kind: channelselectfact.FactReceive, Result: concrete, Index: 0},
			{Select: channelselectfact.ID("select-placeholder"), Kind: channelselectfact.FactReceive, Result: placeholder, Index: 0},
		},
		FrozenTables: []callboundary.FrozenTableFact{
			{Target: concrete},
			{Target: placeholder},
		},
		EffectDeltas: []callboundary.EffectDelta{
			{Target: concrete, Site: "caller.effect.ignored", Kind: effectdelta.Mutation, Value: effectdelta.Value{Before: value, After: value, Change: effectdelta.ChangeChanged}},
			{Target: placeholder, Site: "caller.effect.1", Kind: effectdelta.Mutation, Value: effectdelta.Value{Before: value, After: value, Change: effectdelta.ChangeChanged}},
		},
		EscapeEvents: []callboundary.EscapeEventFact{
			{Target: concrete, Kind: callboundary.EscapeEventSend, Recursive: true},
			{Target: placeholder, Kind: callboundary.EscapeEventSend, Recursive: true},
		},
		StoreRelations: []callboundary.StoreRelationFact{
			{Source: concrete, Into: placeholder},
			{Source: placeholder, Into: concrete},
			{Source: placeholder, Into: pathdom.NewPlaceholder(1)},
		},
		LifecycleFacts: []callboundary.LifecycleFact{
			{Target: concrete, Kind: callboundary.LifecycleAcquire, Protocol: typestate.Protocol("transaction"), To: typestate.State("open")},
			{Target: placeholder, Kind: callboundary.LifecycleAcquire, Protocol: typestate.Protocol("transaction"), To: typestate.State("open")},
		},
		NumFloors: []callboundary.NumFloorFact{
			{Path: concrete, Floor: 1},
			{Path: placeholder, Floor: 1},
		},
		RelConstraints: []callboundary.RelConstraintFact{
			{
				CoA: 1,
				A:   callboundary.RelOperand{Path: concrete},
				C:   callboundary.RelOperand{Path: placeholder, IsLength: true},
			},
			{
				CoA: 1,
				A:   callboundary.RelOperand{Path: placeholder},
				C:   callboundary.RelOperand{Path: placeholder, IsLength: true},
			},
		},
	}})

	facts := got.NormalReturnFacts
	if len(facts.PathRefinements) != 1 || !facts.PathRefinements[0].Path.Equal(placeholder) {
		t.Fatalf("PathRefinements = %#v, want only placeholder fact", facts.PathRefinements)
	}
	if len(facts.PersistentPathWrites) != 1 || !facts.PersistentPathWrites[0].Path.Equal(concrete) {
		t.Fatalf("PersistentPathWrites = %#v, want only concrete captured-path write", facts.PersistentPathWrites)
	}
	if len(facts.PathStaticMembers) != 3 ||
		findPathStaticMember(facts.PathStaticMembers, concrete) == nil ||
		findPathStaticMember(facts.PathStaticMembers, placeholder) == nil ||
		findPathStaticMember(facts.PathStaticMembers, returnSlot) == nil {
		t.Fatalf("PathStaticMembers = %#v, want concrete captured, placeholder, and return-slot facts", facts.PathStaticMembers)
	}
	if len(facts.PathInvalidations) != 2 ||
		findPathInvalidation(facts.PathInvalidations, concrete) == nil ||
		findPathInvalidation(facts.PathInvalidations, placeholder) == nil {
		t.Fatalf("PathInvalidations = %#v, want placeholder and concrete captured-path facts", facts.PathInvalidations)
	}
	var sawPlaceholderDynamic bool
	var sawConcreteDynamic bool
	for _, fact := range facts.DynamicIndexFacts {
		if fact.Table.Equal(placeholder) && fact.Site == "caller.dynamic.1" {
			sawPlaceholderDynamic = true
		}
		if fact.Table.Equal(concrete) && fact.Site == "caller.dynamic.ignored" {
			sawConcreteDynamic = true
		}
	}
	if !sawPlaceholderDynamic || !sawConcreteDynamic {
		t.Fatalf("DynamicIndexFacts = %#v, want placeholder and concrete captured-path facts", facts.DynamicIndexFacts)
	}
	if len(facts.BranchProofs) != 2 {
		t.Fatalf("BranchProofs = %#v, want placeholder presence and equality proofs", facts.BranchProofs)
	}
	if len(facts.ChannelSelects) != 1 || facts.ChannelSelects[0].Select != "select-placeholder" {
		t.Fatalf("ChannelSelects = %#v, want only placeholder result fact", facts.ChannelSelects)
	}
	if len(facts.FrozenTables) != 1 || !facts.FrozenTables[0].Target.Equal(placeholder) {
		t.Fatalf("FrozenTables = %#v, want only placeholder fact", facts.FrozenTables)
	}
	if len(facts.EffectDeltas) != 1 || facts.EffectDeltas[0].Site != "caller.effect.1" {
		t.Fatalf("EffectDeltas = %#v, want stable caller site placeholder fact", facts.EffectDeltas)
	}
	if len(facts.EscapeEvents) != 1 || !facts.EscapeEvents[0].Target.Equal(placeholder) {
		t.Fatalf("EscapeEvents = %#v, want only placeholder fact", facts.EscapeEvents)
	}
	if len(facts.StoreRelations) != 1 ||
		!facts.StoreRelations[0].Source.Equal(placeholder) ||
		!facts.StoreRelations[0].Into.Equal(pathdom.NewPlaceholder(1)) {
		t.Fatalf("StoreRelations = %#v, want only placeholder pair", facts.StoreRelations)
	}
	if len(facts.LifecycleFacts) != 2 ||
		findLifecycleFact(facts.LifecycleFacts, concrete) == nil ||
		findLifecycleFact(facts.LifecycleFacts, placeholder) == nil {
		t.Fatalf("LifecycleFacts = %#v, want placeholder and concrete captured-path facts", facts.LifecycleFacts)
	}
	if len(facts.NumFloors) != 1 || !facts.NumFloors[0].Path.Equal(placeholder) || facts.NumFloors[0].Floor != 1 {
		t.Fatalf("NumFloors = %#v, want only placeholder floor fact", facts.NumFloors)
	}
	if len(facts.RelConstraints) != 1 || !facts.RelConstraints[0].A.Path.Equal(placeholder) ||
		!facts.RelConstraints[0].C.Path.Equal(placeholder) || !facts.RelConstraints[0].C.IsLength {
		t.Fatalf("RelConstraints = %#v, want only placeholder relation fact", facts.RelConstraints)
	}
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
	typ := reflect.TypeOf(callboundary.NormalReturnFacts{})
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

func TestNormalReturnFactsLifecycleKeyDistinguishesFinalStateSets(t *testing.T) {
	reg := mustRegistry(t)
	target := pathdom.NewPlaceholder(0)
	left := Summary{NormalReturnFacts: callboundary.NormalReturnFacts{
		LifecycleFacts: []callboundary.LifecycleFact{{
			Target:   target,
			Kind:     callboundary.LifecycleAcquire,
			Protocol: typestate.Protocol("transaction"),
			To:       typestate.State("active"),
			Obligation: typestate.Obligation{
				Finals: typestate.NewFinalStates(typestate.State("committed"), typestate.State("rolled_back")),
			},
		}},
	}}
	right := Summary{NormalReturnFacts: callboundary.NormalReturnFacts{
		LifecycleFacts: []callboundary.LifecycleFact{{
			Target:   target,
			Kind:     callboundary.LifecycleAcquire,
			Protocol: typestate.Protocol("transaction"),
			To:       typestate.State("active"),
			Obligation: typestate.Obligation{
				Finals: typestate.NewFinalStates(typestate.State("committed")),
			},
		}},
	}}

	joined := Join(reg, left, right).NormalReturnFacts
	if len(joined.LifecycleFacts) != 0 {
		t.Fatalf("LifecycleFacts = %#v, want no must-fact when final sets differ", joined.LifecycleFacts)
	}
}

func TestNormalReturnFactsNormalizeDropsBottomDynamicAndEffectFacts(t *testing.T) {
	reg := mustRegistry(t)
	placeholder := pathdom.NewPlaceholder(0).Field("items")
	value := presentProduct(reg)
	keptDynamic := callboundary.DynamicIndexFact{
		Table: placeholder,
		Site:  "caller.dynamic.kept",
		Value: dynamicindex.Fact{
			KeyPresence: presence.Present(),
			KeyValue:    value,
			Value:       value,
			Admission:   dynamicindex.AdmissionAdmitted,
		},
	}
	keptEffect := callboundary.EffectDelta{
		Target: placeholder,
		Site:   "caller.effect.kept",
		Kind:   effectdelta.Mutation,
		Value: effectdelta.Value{
			Before: value,
			After:  value,
			Change: effectdelta.ChangeChanged,
		},
	}

	got := Normalize(reg, Summary{NormalReturnFacts: callboundary.NormalReturnFacts{
		DynamicIndexFacts: []callboundary.DynamicIndexFact{
			{Table: placeholder, Site: "caller.dynamic.bottom", Value: dynamicindex.Bottom(reg)},
			keptDynamic,
		},
		EffectDeltas: []callboundary.EffectDelta{
			{Target: placeholder, Site: "caller.effect.bottom", Kind: effectdelta.Mutation, Value: effectdelta.Bottom(reg)},
			keptEffect,
		},
	}})

	facts := got.NormalReturnFacts
	if len(facts.DynamicIndexFacts) != 1 || !dynamicIndexFactEqual(reg, facts.DynamicIndexFacts[0], keptDynamic) {
		t.Fatalf("DynamicIndexFacts = %#v, want exactly %#v after bottom pruning", facts.DynamicIndexFacts, keptDynamic)
	}
	if len(facts.EffectDeltas) != 1 || !effectDeltaEqual(reg, facts.EffectDeltas[0], keptEffect) {
		t.Fatalf("EffectDeltas = %#v, want exactly %#v after bottom pruning", facts.EffectDeltas, keptEffect)
	}
}

func TestNormalReturnFactsSparseNormalizeAndCloneKeepsOnlyActiveLanes(t *testing.T) {
	reg := mustRegistry(t)
	input := callboundary.NormalReturnFacts{
		NumFloors: []callboundary.NumFloorFact{{
			Path:  pathdom.NewPlaceholder(0).Field("index"),
			Floor: 1,
		}},
	}

	got := CloneNormalReturnFacts(normalizeNormalReturnFacts(reg, input))

	for _, lane := range callboundary.NormalReturnFactLanes() {
		gotLen := lane.Len(got)
		if lane.ID() == callboundary.LaneNumFloors {
			if gotLen != 1 {
				t.Fatalf("%s length = %d, want 1", lane.FieldName(), gotLen)
			}
			continue
		}
		if gotLen != 0 {
			t.Fatalf("%s length = %d, want empty sparse lane", lane.FieldName(), gotLen)
		}
	}
	if got.NumFloors[0].Floor != 1 || !got.NumFloors[0].Path.Equal(pathdom.NewPlaceholder(0).Field("index")) {
		t.Fatalf("NumFloors = %#v, want original floor evidence", got.NumFloors)
	}
}

func BenchmarkSparseNormalReturnFactsNormalizeClone(b *testing.B) {
	reg := axis.NewRegistry()
	input := callboundary.NormalReturnFacts{
		NumFloors: []callboundary.NumFloorFact{{
			Path:  pathdom.NewPlaceholder(0).Field("index"),
			Floor: 1,
		}},
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		got := CloneNormalReturnFacts(normalizeNormalReturnFacts(reg, input))
		if len(got.NumFloors) != 1 {
			b.Fatalf("NumFloors length = %d, want 1", len(got.NumFloors))
		}
	}
}

func TestNormalReturnFactsCloneIsolatesPayload(t *testing.T) {
	reg := mustRegistry(t)
	original := Summary{NormalReturnFacts: callboundary.NormalReturnFacts{
		PathRefinements:      []callboundary.PathValueFact{{Path: pathdom.NewPlaceholder(0).Field("value"), Value: presentProduct(reg)}},
		PersistentPathWrites: []callboundary.PathValueFact{{Path: pathdom.NewPath(symbol.ID(91003), "captured"), Value: presentProduct(reg)}},
		PathStaticMembers: []callboundary.PathStaticMemberFact{{
			Path:  pathdom.NewPlaceholder(0).Field("member"),
			Value: presentProduct(reg),
		}},
		PathInvalidations: []callboundary.PathInvalidationFact{{
			Path: pathdom.NewPlaceholder(0).Field("invalidate"),
		}},
		DynamicIndexFacts: []callboundary.DynamicIndexFact{{
			Table: pathdom.NewPlaceholder(0),
			Site:  "caller.dynamic.clone",
			Value: dynamicindex.Fact{
				KeyPresence: presence.Present(),
				KeyValue:    presentProduct(reg),
				Value:       presentProduct(reg),
				Admission:   dynamicindex.AdmissionAdmitted,
			},
		}},
		BranchProofs: []callboundary.BranchProof{{
			Kind:     pathevidence.BranchProofPathPresence,
			Path:     pathdom.NewPlaceholder(0).Field("ok"),
			Presence: presence.Present(),
		}},
		ChannelSelects: []callboundary.ChannelSelectFact{{
			Select: channelselectfact.ID("select.clone"),
			Kind:   channelselectfact.FactReceive,
			Result: pathdom.NewPlaceholder(0).Field("received"),
			Index:  1,
		}},
		FrozenTables: []callboundary.FrozenTableFact{{
			Target: pathdom.NewPlaceholder(0).Field("frozen"),
		}},
		EffectDeltas: []callboundary.EffectDelta{{
			Target: pathdom.NewPlaceholder(0).Field("effect"),
			Site:   "caller.effect.clone",
			Kind:   effectdelta.Mutation,
			Value: effectdelta.Value{
				Before: presentProduct(reg),
				After:  presentProduct(reg),
				Change: effectdelta.ChangeChanged,
			},
		}},
		EscapeEvents: []callboundary.EscapeEventFact{{
			Target:    pathdom.NewPlaceholder(0).Field("escape"),
			Kind:      callboundary.EscapeEventSend,
			Recursive: true,
		}},
		StoreRelations: []callboundary.StoreRelationFact{{
			Source: pathdom.NewPlaceholder(0).Field("stored"),
			Into:   pathdom.NewPlaceholder(1),
		}},
		LifecycleFacts: []callboundary.LifecycleFact{{
			Target:   pathdom.NewPlaceholder(0).Field("resource"),
			Kind:     callboundary.LifecycleTransition,
			Protocol: typestate.Protocol("transaction"),
			From:     typestate.State("open"),
			To:       typestate.State("closed"),
		}},
		NumFloors: []callboundary.NumFloorFact{{
			Path:  pathdom.NewPlaceholder(0).Field("index"),
			Floor: 1,
		}},
		RelConstraints: []callboundary.RelConstraintFact{{
			CoA: 1,
			A:   callboundary.RelOperand{Path: pathdom.NewPlaceholder(0).Field("i")},
			CoB: 1,
			B:   callboundary.RelOperand{Path: pathdom.NewPlaceholder(1)},
			C:   callboundary.RelOperand{Path: pathdom.NewPlaceholder(0), IsLength: true},
		}},
	}}

	cloned := original.Clone()
	cloned.NormalReturnFacts.PathRefinements[0].Path = pathdom.NewPlaceholder(1)
	cloned.NormalReturnFacts.PersistentPathWrites[0].Path = pathdom.NewPath(symbol.ID(91004), "changed")
	cloned.NormalReturnFacts.PathStaticMembers[0].Path = pathdom.NewPlaceholder(1)
	cloned.NormalReturnFacts.PathInvalidations[0].Path = pathdom.NewPlaceholder(1)
	cloned.NormalReturnFacts.DynamicIndexFacts[0].Site = "caller.dynamic.changed"
	cloned.NormalReturnFacts.BranchProofs[0].Presence = presence.Absent()
	cloned.NormalReturnFacts.ChannelSelects[0].Select = channelselectfact.ID("select.changed")
	cloned.NormalReturnFacts.StoreRelations[0].Source = pathdom.NewPlaceholder(2)
	cloned.NormalReturnFacts.FrozenTables[0].Target = pathdom.NewPlaceholder(1)
	cloned.NormalReturnFacts.EffectDeltas[0].Site = "caller.effect.changed"
	cloned.NormalReturnFacts.EscapeEvents[0].Target = pathdom.NewPlaceholder(1)
	cloned.NormalReturnFacts.LifecycleFacts[0].Target = pathdom.NewPlaceholder(1)
	cloned.NormalReturnFacts.NumFloors[0].Path = pathdom.NewPlaceholder(1)
	cloned.NormalReturnFacts.RelConstraints[0].A.Path = pathdom.NewPlaceholder(2)

	if !original.NormalReturnFacts.PathRefinements[0].Path.Equal(pathdom.NewPlaceholder(0).Field("value")) {
		t.Fatalf("mutating cloned path refinement changed original")
	}
	if !original.NormalReturnFacts.PersistentPathWrites[0].Path.Equal(pathdom.NewPath(symbol.ID(91003), "captured")) {
		t.Fatalf("mutating cloned persistent path write changed original")
	}
	if !original.NormalReturnFacts.PathStaticMembers[0].Path.Equal(pathdom.NewPlaceholder(0).Field("member")) {
		t.Fatalf("mutating cloned path static member changed original")
	}
	if !original.NormalReturnFacts.PathInvalidations[0].Path.Equal(pathdom.NewPlaceholder(0).Field("invalidate")) {
		t.Fatalf("mutating cloned path invalidation changed original")
	}
	if original.NormalReturnFacts.DynamicIndexFacts[0].Site != "caller.dynamic.clone" {
		t.Fatalf("mutating cloned dynamic index changed original")
	}
	if !presence.Equal(original.NormalReturnFacts.BranchProofs[0].Presence, presence.Present()) {
		t.Fatalf("mutating cloned branch proof changed original")
	}
	if original.NormalReturnFacts.ChannelSelects[0].Select != channelselectfact.ID("select.clone") {
		t.Fatalf("mutating cloned channel select changed original")
	}
	if !original.NormalReturnFacts.FrozenTables[0].Target.Equal(pathdom.NewPlaceholder(0).Field("frozen")) {
		t.Fatalf("mutating cloned frozen table fact changed original")
	}
	if original.NormalReturnFacts.EffectDeltas[0].Site != "caller.effect.clone" {
		t.Fatalf("mutating cloned effect delta changed original")
	}
	if !original.NormalReturnFacts.EscapeEvents[0].Target.Equal(pathdom.NewPlaceholder(0).Field("escape")) {
		t.Fatalf("mutating cloned escape event changed original")
	}
	if !original.NormalReturnFacts.StoreRelations[0].Source.Equal(pathdom.NewPlaceholder(0).Field("stored")) {
		t.Fatalf("mutating cloned store relation changed original")
	}
	if !original.NormalReturnFacts.LifecycleFacts[0].Target.Equal(pathdom.NewPlaceholder(0).Field("resource")) {
		t.Fatalf("mutating cloned lifecycle fact changed original")
	}
	if !original.NormalReturnFacts.NumFloors[0].Path.Equal(pathdom.NewPlaceholder(0).Field("index")) {
		t.Fatalf("mutating cloned num-floor fact changed original")
	}
	if !original.NormalReturnFacts.RelConstraints[0].A.Path.Equal(pathdom.NewPlaceholder(0).Field("i")) {
		t.Fatalf("mutating cloned relational constraint fact changed original")
	}
}

func TestNormalReturnFactsJoinUsesStateLaneSemantics(t *testing.T) {
	reg := mustRegistry(t)
	p0 := pathdom.NewPlaceholder(0)
	leftOnly := p0.Field("left")
	commonPath := p0.Field("common")
	commonPersistentPath := pathdom.NewPath(symbol.ID(91001), "captured")
	leftPersistentPath := pathdom.NewPath(symbol.ID(91002), "left_captured")
	storeInto := p0.Field("container")
	leftValue := presentProduct(reg)
	rightValue := absentProduct(reg)

	left := Summary{NormalReturnFacts: callboundary.NormalReturnFacts{
		PathRefinements: []callboundary.PathValueFact{
			{Path: commonPath, Value: leftValue},
			{Path: leftOnly, Value: leftValue},
		},
		PersistentPathWrites: []callboundary.PathValueFact{
			{Path: commonPersistentPath, Value: leftValue},
			{Path: leftPersistentPath, Value: leftValue},
		},
		PathStaticMembers: []callboundary.PathStaticMemberFact{
			{Path: commonPath, Value: leftValue},
			{Path: leftOnly, Value: leftValue},
		},
		DynamicIndexFacts: []callboundary.DynamicIndexFact{
			{Table: p0, Site: "caller.dynamic.common", Value: dynamicindex.Fact{KeyPresence: presence.Present(), KeyValue: leftValue, Value: leftValue, Admission: dynamicindex.AdmissionAdmitted}},
			{Table: p0, Site: "caller.dynamic.left", Value: dynamicindex.Fact{KeyPresence: presence.Present(), KeyValue: leftValue, Value: leftValue, Admission: dynamicindex.AdmissionAdmitted}},
		},
		BranchProofs: []callboundary.BranchProof{
			{Kind: pathevidence.BranchProofPathPresence, Path: commonPath, Presence: presence.Present()},
			{Kind: pathevidence.BranchProofPathPresence, Path: leftOnly, Presence: presence.Present()},
		},
		ChannelSelects: []callboundary.ChannelSelectFact{
			{Select: channelselectfact.ID("select-common"), Kind: channelselectfact.FactSelect, Result: commonPath, Index: 0},
			{Select: channelselectfact.ID("select-left"), Kind: channelselectfact.FactReceive, Case: leftOnly, Index: 1},
		},
		FrozenTables: []callboundary.FrozenTableFact{
			{Target: commonPath},
			{Target: leftOnly},
		},
		EffectDeltas: []callboundary.EffectDelta{
			{Target: commonPath, Site: "caller.effect.common", Kind: effectdelta.Mutation, Value: effectdelta.Value{Before: leftValue, After: leftValue, Change: effectdelta.ChangeChanged}},
			{Target: leftOnly, Site: "caller.effect.left", Kind: effectdelta.Mutation, Value: effectdelta.Value{Before: leftValue, After: leftValue, Change: effectdelta.ChangeChanged}},
		},
		EscapeEvents: []callboundary.EscapeEventFact{
			{Target: commonPath, Kind: callboundary.EscapeEventBorrow},
			{Target: leftOnly, Kind: callboundary.EscapeEventRetain},
		},
		StoreRelations: []callboundary.StoreRelationFact{
			{Source: commonPath, Into: storeInto},
			{Source: leftOnly, Into: storeInto},
		},
		LifecycleFacts: []callboundary.LifecycleFact{
			{Target: commonPath, Kind: callboundary.LifecycleTransition, Protocol: typestate.Protocol("transaction"), From: typestate.State("open"), To: typestate.State("closed")},
			{Target: leftOnly, Kind: callboundary.LifecycleAcquire, Protocol: typestate.Protocol("transaction"), To: typestate.State("open")},
		},
		NumFloors: []callboundary.NumFloorFact{
			{Path: commonPath, Floor: 2},
			{Path: leftOnly, Floor: 1},
		},
		RelConstraints: []callboundary.RelConstraintFact{
			{
				CoA: 1,
				A:   callboundary.RelOperand{Path: commonPath},
				CoB: 1,
				B:   callboundary.RelOperand{Path: p0.Field("shared")},
				C:   callboundary.RelOperand{Path: p0, IsLength: true},
			},
			{
				CoA: 1,
				A:   callboundary.RelOperand{Path: leftOnly},
				C:   callboundary.RelOperand{Path: p0, IsLength: true},
			},
		},
	}}
	right := Summary{NormalReturnFacts: callboundary.NormalReturnFacts{
		PathRefinements:      []callboundary.PathValueFact{{Path: commonPath, Value: rightValue}},
		PersistentPathWrites: []callboundary.PathValueFact{{Path: commonPersistentPath, Value: rightValue}},
		PathStaticMembers:    []callboundary.PathStaticMemberFact{{Path: commonPath, Value: rightValue}},
		DynamicIndexFacts: []callboundary.DynamicIndexFact{{
			Table: p0,
			Site:  "caller.dynamic.common",
			Value: dynamicindex.Fact{
				KeyPresence: presence.Absent(),
				KeyValue:    rightValue,
				Value:       rightValue,
				Admission:   dynamicindex.AdmissionRejected,
			},
		}},
		BranchProofs: []callboundary.BranchProof{
			{Kind: pathevidence.BranchProofPathPresence, Path: commonPath, Presence: presence.Present()},
			{Kind: pathevidence.BranchProofPathPresence, Path: p0.Field("right"), Presence: presence.Present()},
		},
		ChannelSelects: []callboundary.ChannelSelectFact{
			{Select: channelselectfact.ID("select-common"), Kind: channelselectfact.FactSelect, Result: commonPath, Index: 0},
			{Select: channelselectfact.ID("select-right"), Kind: channelselectfact.FactCase, Case: p0.Field("right"), Index: 2},
		},
		FrozenTables: []callboundary.FrozenTableFact{
			{Target: commonPath},
			{Target: p0.Field("right")},
		},
		EffectDeltas: []callboundary.EffectDelta{{
			Target: commonPath,
			Site:   "caller.effect.common",
			Kind:   effectdelta.Mutation,
			Value:  effectdelta.Value{Before: rightValue, After: rightValue, Change: effectdelta.ChangeNone},
		}},
		EscapeEvents: []callboundary.EscapeEventFact{{
			Target: commonPath,
			Kind:   callboundary.EscapeEventSend,
		}},
		StoreRelations: []callboundary.StoreRelationFact{
			{Source: commonPath, Into: storeInto},
			{Source: p0.Field("right"), Into: storeInto},
		},
		LifecycleFacts: []callboundary.LifecycleFact{
			{Target: commonPath, Kind: callboundary.LifecycleTransition, Protocol: typestate.Protocol("transaction"), From: typestate.State("open"), To: typestate.State("closed")},
			{Target: p0.Field("right"), Kind: callboundary.LifecycleAcquire, Protocol: typestate.Protocol("transaction"), To: typestate.State("open")},
		},
		NumFloors: []callboundary.NumFloorFact{
			{Path: commonPath, Floor: 4},
			{Path: p0.Field("right"), Floor: 9},
		},
		RelConstraints: []callboundary.RelConstraintFact{
			{
				CoA: 1,
				A:   callboundary.RelOperand{Path: p0.Field("shared")},
				CoB: 1,
				B:   callboundary.RelOperand{Path: commonPath},
				C:   callboundary.RelOperand{Path: p0, IsLength: true},
			},
			{
				CoA: 1,
				A:   callboundary.RelOperand{Path: p0.Field("right")},
				C:   callboundary.RelOperand{Path: p0, IsLength: true},
			},
		},
	}}

	got := Join(reg, left, right).NormalReturnFacts
	if len(got.PathRefinements) != 2 {
		t.Fatalf("PathRefinements = %#v, want common joined and left-only retained", got.PathRefinements)
	}
	if common := findPathRefinement(got.PathRefinements, commonPath); common == nil ||
		!presence.Equal(product.PresenceOf(common.Value), presence.Maybe()) {
		t.Fatalf("common path refinement did not join to maybe: %#v", common)
	}
	if len(got.PersistentPathWrites) != 1 || !got.PersistentPathWrites[0].Path.Equal(commonPersistentPath) ||
		!presence.Equal(product.PresenceOf(got.PersistentPathWrites[0].Value), presence.Maybe()) {
		t.Fatalf("PersistentPathWrites = %#v, want only common joined must write", got.PersistentPathWrites)
	}
	if len(got.PathStaticMembers) != 1 || !got.PathStaticMembers[0].Path.Equal(commonPath) ||
		!presence.Equal(product.PresenceOf(got.PathStaticMembers[0].Value), presence.Maybe()) {
		t.Fatalf("PathStaticMembers = %#v, want only common joined must fact", got.PathStaticMembers)
	}
	if len(got.DynamicIndexFacts) != 2 {
		t.Fatalf("DynamicIndexFacts = %#v, want common joined and left-only retained", got.DynamicIndexFacts)
	}
	if common := findDynamicIndexFact(got.DynamicIndexFacts, "caller.dynamic.common"); common == nil ||
		!presence.Equal(common.Value.KeyPresence, presence.Maybe()) ||
		common.Value.Admission != dynamicindex.AdmissionUnknown {
		t.Fatalf("common dynamic index fact did not pointwise join: %#v", common)
	}
	if leftOnlyFact := findDynamicIndexFact(got.DynamicIndexFacts, "caller.dynamic.left"); leftOnlyFact == nil ||
		!dynamicIndexFactEqual(reg, *leftOnlyFact, left.NormalReturnFacts.DynamicIndexFacts[1]) {
		t.Fatalf("left-only dynamic index fact = %#v, want original fact", leftOnlyFact)
	}
	if len(got.BranchProofs) != 1 || !got.BranchProofs[0].Path.Equal(commonPath) {
		t.Fatalf("BranchProofs = %#v, want only common must proof", got.BranchProofs)
	}
	if len(got.ChannelSelects) != 1 || got.ChannelSelects[0].Select != "select-common" {
		t.Fatalf("ChannelSelects = %#v, want only common must fact", got.ChannelSelects)
	}
	if len(got.FrozenTables) != 1 || !got.FrozenTables[0].Target.Equal(commonPath) {
		t.Fatalf("FrozenTables = %#v, want only common must fact", got.FrozenTables)
	}
	if len(got.EffectDeltas) != 2 {
		t.Fatalf("EffectDeltas = %#v, want common joined and left-only retained", got.EffectDeltas)
	}
	if common := findEffectDelta(got.EffectDeltas, "caller.effect.common"); common == nil ||
		!presence.Equal(product.PresenceOf(common.Value.Before), presence.Maybe()) ||
		common.Value.Change != effectdelta.ChangeUnknown {
		t.Fatalf("common effect delta did not pointwise join: %#v", common)
	}
	if leftOnlyDelta := findEffectDelta(got.EffectDeltas, "caller.effect.left"); leftOnlyDelta == nil ||
		!effectDeltaEqual(reg, *leftOnlyDelta, left.NormalReturnFacts.EffectDeltas[1]) {
		t.Fatalf("left-only effect delta = %#v, want original delta", leftOnlyDelta)
	}
	if len(got.EscapeEvents) != 2 {
		t.Fatalf("EscapeEvents = %#v, want common strengthened and left-only retained", got.EscapeEvents)
	}
	if common := findEscapeEvent(got.EscapeEvents, commonPath, false); common == nil ||
		common.Kind != callboundary.EscapeEventSend {
		t.Fatalf("common escape event did not strengthen to send: %#v", common)
	}
	if leftOnlyEvent := findEscapeEvent(got.EscapeEvents, leftOnly, false); leftOnlyEvent == nil ||
		leftOnlyEvent.Kind != callboundary.EscapeEventRetain {
		t.Fatalf("left-only escape event = %#v, want original event", leftOnlyEvent)
	}
	if len(got.StoreRelations) != 1 ||
		!got.StoreRelations[0].Source.Equal(commonPath) ||
		!got.StoreRelations[0].Into.Equal(storeInto) {
		t.Fatalf("StoreRelations = %#v, want only common must relation", got.StoreRelations)
	}
	if len(got.LifecycleFacts) != 1 ||
		!got.LifecycleFacts[0].Target.Equal(commonPath) ||
		got.LifecycleFacts[0].Kind != callboundary.LifecycleTransition {
		t.Fatalf("LifecycleFacts = %#v, want only common must lifecycle transition", got.LifecycleFacts)
	}
	if len(got.NumFloors) != 1 || !got.NumFloors[0].Path.Equal(commonPath) || got.NumFloors[0].Floor != 2 {
		t.Fatalf("NumFloors = %#v, want common floor with weaker lower bound 2", got.NumFloors)
	}
	if len(got.RelConstraints) != 1 ||
		!got.RelConstraints[0].A.Path.Equal(commonPath) ||
		!got.RelConstraints[0].B.Path.Equal(p0.Field("shared")) ||
		!got.RelConstraints[0].C.Path.Equal(p0) ||
		!got.RelConstraints[0].C.IsLength {
		t.Fatalf("RelConstraints = %#v, want only common must relation", got.RelConstraints)
	}

	widened := Widen(reg, left, right).NormalReturnFacts
	if leftOnlyFact := findDynamicIndexFact(widened.DynamicIndexFacts, "caller.dynamic.left"); leftOnlyFact == nil ||
		!dynamicIndexFactEqual(reg, *leftOnlyFact, left.NormalReturnFacts.DynamicIndexFacts[1]) {
		t.Fatalf("widen left-only dynamic index fact = %#v, want original fact", leftOnlyFact)
	}
	if leftOnlyDelta := findEffectDelta(widened.EffectDeltas, "caller.effect.left"); leftOnlyDelta == nil ||
		!effectDeltaEqual(reg, *leftOnlyDelta, left.NormalReturnFacts.EffectDeltas[1]) {
		t.Fatalf("widen left-only effect delta = %#v, want original delta", leftOnlyDelta)
	}
	if common := findEscapeEvent(widened.EscapeEvents, commonPath, false); common == nil ||
		common.Kind != callboundary.EscapeEventSend {
		t.Fatalf("widen common escape event = %#v, want strengthened event", common)
	}
	if len(widened.FrozenTables) != 1 || !widened.FrozenTables[0].Target.Equal(commonPath) {
		t.Fatalf("widen FrozenTables = %#v, want only common must fact", widened.FrozenTables)
	}
	if len(widened.StoreRelations) != 1 ||
		!widened.StoreRelations[0].Source.Equal(commonPath) ||
		!widened.StoreRelations[0].Into.Equal(storeInto) {
		t.Fatalf("widen StoreRelations = %#v, want only common must relation", widened.StoreRelations)
	}
	if len(widened.NumFloors) != 1 || !widened.NumFloors[0].Path.Equal(commonPath) || widened.NumFloors[0].Floor != 2 {
		t.Fatalf("widen NumFloors = %#v, want common floor with weaker lower bound 2", widened.NumFloors)
	}
	if len(widened.RelConstraints) != 1 ||
		!widened.RelConstraints[0].A.Path.Equal(commonPath) ||
		!widened.RelConstraints[0].B.Path.Equal(p0.Field("shared")) ||
		!widened.RelConstraints[0].C.Path.Equal(p0) ||
		!widened.RelConstraints[0].C.IsLength {
		t.Fatalf("widen RelConstraints = %#v, want only common must relation", widened.RelConstraints)
	}
}

func TestNormalReturnFactsEqualAndLessOrEqAccountForLane(t *testing.T) {
	reg := mustRegistry(t)
	p0 := pathdom.NewPlaceholder(0)
	left := Summary{NormalReturnFacts: callboundary.NormalReturnFacts{
		PathRefinements: []callboundary.PathValueFact{{Path: p0, Value: presentProduct(reg)}},
		BranchProofs:    []callboundary.BranchProof{{Kind: pathevidence.BranchProofPathPresence, Path: p0, Presence: presence.Present()}},
		EscapeEvents:    []callboundary.EscapeEventFact{{Target: p0, Kind: callboundary.EscapeEventBorrow}},
		StoreRelations:  []callboundary.StoreRelationFact{{Source: p0, Into: pathdom.NewPlaceholder(1)}},
		NumFloors:       []callboundary.NumFloorFact{{Path: p0, Floor: 2}},
		RelConstraints:  []callboundary.RelConstraintFact{{CoA: 1, A: callboundary.RelOperand{Path: p0}, C: callboundary.RelOperand{Path: p0, IsLength: true}}},
	}}
	right := Summary{NormalReturnFacts: callboundary.NormalReturnFacts{
		PathRefinements: []callboundary.PathValueFact{{Path: p0, Value: product.Top()}},
		EscapeEvents:    []callboundary.EscapeEventFact{{Target: p0, Kind: callboundary.EscapeEventSend}},
		NumFloors:       []callboundary.NumFloorFact{{Path: p0, Floor: 1}},
	}}

	if Equal(reg, left, right) {
		t.Fatalf("Equal ignored normal return facts")
	}
	if !LessOrEq(reg, left, right) {
		t.Fatalf("LessOrEq should account for product weakening and must-proof removal")
	}
	if LessOrEq(reg, right, left) {
		t.Fatalf("LessOrEq should reject stronger product and extra must proof")
	}
	if !Equal(reg, left, Normalize(reg, left)) {
		t.Fatalf("Equal should compare normalized normal return facts")
	}

	frozen := Summary{NormalReturnFacts: callboundary.NormalReturnFacts{
		FrozenTables: []callboundary.FrozenTableFact{{Target: p0}},
	}}
	if frozen.Clone().NormalReturnFacts.FrozenTables == nil || Normalize(reg, frozen).NormalReturnFacts.FrozenTables == nil {
		t.Fatalf("FrozenTables should make normal return facts non-empty")
	}
	withFrozen := Summary{
		Returns:           []product.Value{presentProduct(reg)},
		NormalReturnFacts: frozen.NormalReturnFacts,
	}
	withoutFrozen := Summary{Returns: []product.Value{presentProduct(reg)}}
	if !LessOrEq(reg, withFrozen, withoutFrozen) {
		t.Fatalf("frozen table proof should be <= empty proof set")
	}
	if LessOrEq(reg, withoutFrozen, withFrozen) {
		t.Fatalf("empty proof set should not be <= frozen table proof")
	}

	withFloor := Summary{
		Returns: []product.Value{presentProduct(reg)},
		NormalReturnFacts: callboundary.NormalReturnFacts{
			NumFloors: []callboundary.NumFloorFact{{Path: p0, Floor: 1}},
		},
	}
	withoutFloor := Summary{Returns: []product.Value{presentProduct(reg)}}
	if !LessOrEq(reg, withFloor, withoutFloor) {
		t.Fatalf("numeric floor proof should be <= empty proof set")
	}
	if LessOrEq(reg, withoutFloor, withFloor) {
		t.Fatalf("empty proof set should not be <= numeric floor proof")
	}

	withRelation := Summary{
		Returns: []product.Value{presentProduct(reg)},
		NormalReturnFacts: callboundary.NormalReturnFacts{
			RelConstraints: []callboundary.RelConstraintFact{{CoA: 1, A: callboundary.RelOperand{Path: p0}, C: callboundary.RelOperand{Path: p0, IsLength: true}}},
		},
	}
	withoutRelation := Summary{Returns: []product.Value{presentProduct(reg)}}
	if !LessOrEq(reg, withRelation, withoutRelation) {
		t.Fatalf("relational proof should be <= empty proof set")
	}
	if LessOrEq(reg, withoutRelation, withRelation) {
		t.Fatalf("empty proof set should not be <= relational proof")
	}
}

func TestNormalReturnFactsEscapeEventsCompressRecursiveParents(t *testing.T) {
	reg := mustRegistry(t)
	p0 := pathdom.NewPlaceholder(0)
	child := p0.Field("child")
	grandchild := child.Field("leaf")
	sibling := p0.Field("sibling")

	got := Normalize(reg, Summary{NormalReturnFacts: callboundary.NormalReturnFacts{
		EscapeEvents: []callboundary.EscapeEventFact{
			{Target: child, Kind: callboundary.EscapeEventBorrow},
			{Target: child, Kind: callboundary.EscapeEventStore},
			{Target: grandchild, Kind: callboundary.EscapeEventSend},
			{Target: p0, Kind: callboundary.EscapeEventSend, Recursive: true},
			{Target: sibling, Kind: callboundary.EscapeEventOpaque},
			{Target: child.Field("stronger"), Kind: callboundary.EscapeEventOpaque},
		},
	}}).NormalReturnFacts.EscapeEvents

	if len(got) != 3 {
		t.Fatalf("EscapeEvents = %#v, want recursive parent plus stronger descendants", got)
	}
	if parent := findEscapeEvent(got, p0, true); parent == nil || parent.Kind != callboundary.EscapeEventSend {
		t.Fatalf("parent recursive event = %#v, want send", parent)
	}
	if childEvent := findEscapeEvent(got, child, false); childEvent != nil {
		t.Fatalf("non-recursive child send/store should be compressed by recursive parent: %#v", childEvent)
	}
	if grandchildEvent := findEscapeEvent(got, grandchild, false); grandchildEvent != nil {
		t.Fatalf("grandchild send should be compressed by recursive parent: %#v", grandchildEvent)
	}
	if siblingEvent := findEscapeEvent(got, sibling, false); siblingEvent == nil ||
		siblingEvent.Kind != callboundary.EscapeEventOpaque {
		t.Fatalf("stronger sibling escape should remain distinct: %#v", siblingEvent)
	}

	childSend := Summary{NormalReturnFacts: callboundary.NormalReturnFacts{
		EscapeEvents: []callboundary.EscapeEventFact{{Target: child, Kind: callboundary.EscapeEventSend}},
	}}
	parentRecursiveSend := Summary{NormalReturnFacts: callboundary.NormalReturnFacts{
		EscapeEvents: []callboundary.EscapeEventFact{{Target: p0, Kind: callboundary.EscapeEventSend, Recursive: true}},
	}}
	if !LessOrEq(reg, childSend, parentRecursiveSend) {
		t.Fatalf("recursive parent send should dominate child send")
	}
}

func TestNormalReturnFactsEscapeEventsKeepNonRecursiveChildrenDistinct(t *testing.T) {
	reg := mustRegistry(t)
	p0 := pathdom.NewPlaceholder(0)
	parent := p0.Field("parent")
	child := parent.Field("child")
	sibling := parent.Field("sibling")

	got := Normalize(reg, Summary{NormalReturnFacts: callboundary.NormalReturnFacts{
		EscapeEvents: []callboundary.EscapeEventFact{
			{Target: parent, Kind: callboundary.EscapeEventSend},
			{Target: child, Kind: callboundary.EscapeEventSend},
			{Target: sibling, Kind: callboundary.EscapeEventSend},
		},
	}}).NormalReturnFacts.EscapeEvents

	if len(got) != 3 {
		t.Fatalf("EscapeEvents = %#v, want parent, child, and sibling preserved", got)
	}
	if findEscapeEvent(got, parent, false) == nil ||
		findEscapeEvent(got, child, false) == nil ||
		findEscapeEvent(got, sibling, false) == nil {
		t.Fatalf("non-recursive escape facts must not imply children or siblings: %#v", got)
	}

	childSend := Summary{NormalReturnFacts: callboundary.NormalReturnFacts{
		EscapeEvents: []callboundary.EscapeEventFact{{Target: child, Kind: callboundary.EscapeEventSend}},
	}}
	parentSend := Summary{NormalReturnFacts: callboundary.NormalReturnFacts{
		EscapeEvents: []callboundary.EscapeEventFact{{Target: parent, Kind: callboundary.EscapeEventSend}},
	}}
	siblingSend := Summary{NormalReturnFacts: callboundary.NormalReturnFacts{
		EscapeEvents: []callboundary.EscapeEventFact{{Target: sibling, Kind: callboundary.EscapeEventSend}},
	}}
	if LessOrEq(reg, childSend, parentSend) {
		t.Fatalf("non-recursive parent send should not dominate child send")
	}
	if LessOrEq(reg, childSend, siblingSend) {
		t.Fatalf("child send should not imply sibling send")
	}
}

func TestNormalReturnFactsPathInvalidationsCompressParentEvidence(t *testing.T) {
	reg := mustRegistry(t)
	p0 := pathdom.NewPlaceholder(0)
	parent := p0.Field("cache")
	child := parent.Field("entry")
	grandchild := child.Field("value")
	sibling := p0.Field("cache2")
	concrete := pathdom.NewPath(symbol.ID(99), "arg").Field("cache")

	got := Normalize(reg, Summary{NormalReturnFacts: callboundary.NormalReturnFacts{
		PathInvalidations: []callboundary.PathInvalidationFact{
			{Path: child},
			{Path: concrete},
			{Path: grandchild},
			{Path: sibling},
			{Path: parent},
			{Path: child},
		},
	}}).NormalReturnFacts.PathInvalidations

	if len(got) != 3 {
		t.Fatalf("PathInvalidations = %#v, want parent, sibling, and concrete captured path", got)
	}
	if findPathInvalidation(got, parent) == nil {
		t.Fatalf("PathInvalidations = %#v, want parent invalidation", got)
	}
	if findPathInvalidation(got, child) != nil || findPathInvalidation(got, grandchild) != nil {
		t.Fatalf("child invalidations should be compressed by parent: %#v", got)
	}
	if findPathInvalidation(got, sibling) == nil {
		t.Fatalf("PathInvalidations = %#v, want distinct sibling invalidation", got)
	}
	if findPathInvalidation(got, concrete) == nil {
		t.Fatalf("PathInvalidations = %#v, want concrete captured-path invalidation", got)
	}

	parentOnly := Summary{NormalReturnFacts: callboundary.NormalReturnFacts{
		PathInvalidations: []callboundary.PathInvalidationFact{{Path: parent}},
	}}
	childOnly := Summary{NormalReturnFacts: callboundary.NormalReturnFacts{
		PathInvalidations: []callboundary.PathInvalidationFact{{Path: child}},
	}}
	siblingOnly := Summary{NormalReturnFacts: callboundary.NormalReturnFacts{
		PathInvalidations: []callboundary.PathInvalidationFact{{Path: sibling}},
	}}

	joined := Join(reg, childOnly, parentOnly).NormalReturnFacts.PathInvalidations
	if len(joined) != 1 || !joined[0].Path.Equal(parent) {
		t.Fatalf("Join(child,parent) = %#v, want parent only", joined)
	}
	widened := Widen(reg, childOnly, Summary{NormalReturnFacts: callboundary.NormalReturnFacts{
		PathInvalidations: []callboundary.PathInvalidationFact{{Path: parent}, {Path: sibling}},
	}}).NormalReturnFacts.PathInvalidations
	if len(widened) != 2 || findPathInvalidation(widened, parent) == nil || findPathInvalidation(widened, sibling) == nil {
		t.Fatalf("Widen(child,parent+sibling) = %#v, want parent and sibling", widened)
	}
	if !Equal(reg, parentOnly, Summary{NormalReturnFacts: callboundary.NormalReturnFacts{
		PathInvalidations: []callboundary.PathInvalidationFact{{Path: parent}, {Path: child}},
	}}) {
		t.Fatalf("Equal should compare normalized parent-dominated invalidations")
	}
	if !LessOrEq(reg, childOnly, parentOnly) {
		t.Fatalf("child invalidation should be <= parent invalidation")
	}
	if LessOrEq(reg, parentOnly, childOnly) {
		t.Fatalf("parent invalidation should not be <= child invalidation")
	}
	if LessOrEq(reg, siblingOnly, parentOnly) {
		t.Fatalf("sibling invalidation should not be <= parent invalidation")
	}
}

func findPathRefinement(facts []callboundary.PathValueFact, path pathdom.Path) *callboundary.PathValueFact {
	for i := range facts {
		if facts[i].Path.Equal(path) {
			return &facts[i]
		}
	}
	return nil
}

func findPathStaticMember(facts []callboundary.PathStaticMemberFact, path pathdom.Path) *callboundary.PathStaticMemberFact {
	for i := range facts {
		if facts[i].Path.Equal(path) {
			return &facts[i]
		}
	}
	return nil
}

func findPathInvalidation(facts []callboundary.PathInvalidationFact, path pathdom.Path) *callboundary.PathInvalidationFact {
	for i := range facts {
		if facts[i].Path.Equal(path) {
			return &facts[i]
		}
	}
	return nil
}

func findLifecycleFact(facts []callboundary.LifecycleFact, path pathdom.Path) *callboundary.LifecycleFact {
	for i := range facts {
		if facts[i].Target.Equal(path) {
			return &facts[i]
		}
	}
	return nil
}

func findDynamicIndexFact(facts []callboundary.DynamicIndexFact, site string) *callboundary.DynamicIndexFact {
	for i := range facts {
		if string(facts[i].Site) == site {
			return &facts[i]
		}
	}
	return nil
}

func findEffectDelta(deltas []callboundary.EffectDelta, site string) *callboundary.EffectDelta {
	for i := range deltas {
		if string(deltas[i].Site) == site {
			return &deltas[i]
		}
	}
	return nil
}

func findEscapeEvent(events []callboundary.EscapeEventFact, target pathdom.Path, recursive bool) *callboundary.EscapeEventFact {
	for i := range events {
		if events[i].Target.Equal(target) && events[i].Recursive == recursive {
			return &events[i]
		}
	}
	return nil
}

func presentProduct(reg *axis.Registry) product.Value {
	return product.NewWithPresence(reg, product.ShapeTop, presence.Present())
}

func absentProduct(reg *axis.Registry) product.Value {
	return product.NewWithPresence(reg, product.ShapeTop, presence.Absent())
}
