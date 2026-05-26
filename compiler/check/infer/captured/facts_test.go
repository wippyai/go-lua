package captured

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestFieldFactsFromEvidence_JoinsCapturedFieldAssignments(t *testing.T) {
	const sym cfg.SymbolID = 7

	got := FieldFactsFromEvidence([]api.CapturedFieldEvidence{
		{Target: sym, Field: "name", ValueType: typ.String, Point: 1},
		{Target: sym, Field: "name", ValueType: typ.Number, Point: 2},
	}, nil)

	field := got[sym]["name"]
	want := typ.NewUnion(typ.String, typ.Number)
	if !typ.TypeEquals(field, want) {
		t.Fatalf("field fact = %v, want %v", field, want)
	}
}

func TestFieldFactsFromEvidence_ReplacesUnsolvedFunctionSeed(t *testing.T) {
	const sym cfg.SymbolID = 7
	seed := typ.Func().Build()
	solved := typ.Func().Param("self", typ.Any).Returns(typ.Number).Build()

	got := FieldFactsFromEvidence([]api.CapturedFieldEvidence{
		{Target: sym, Field: "get_x", ValueType: seed, Point: 1},
		{Target: sym, Field: "get_x", ValueType: solved, Point: 2},
	}, nil)

	field := got[sym]["get_x"]
	if !typ.TypeEquals(field, solved) {
		t.Fatalf("captured field fact = %v, want %v", field, solved)
	}
}

func TestFieldFactsFromEvidence_NestsMultiSegmentTargetPath(t *testing.T) {
	const sym cfg.SymbolID = 7
	fn := typ.Func().Returns(typ.String).Build()

	got := FieldFactsFromEvidence([]api.CapturedFieldEvidence{{
		Target: sym,
		Field:  "repo",
		TargetPath: constraint.Path{
			Symbol: sym,
			Segments: []constraint.Segment{
				{Kind: constraint.SegmentField, Name: "repo"},
				{Kind: constraint.SegmentField, Name: "list"},
			},
		},
		ValueType: fn,
		Point:     1,
	}}, nil)

	repo, ok := got[sym]["repo"].(*typ.Record)
	if !ok {
		t.Fatalf("repo fact = %T/%v, want record", got[sym]["repo"], got[sym]["repo"])
	}
	list := repo.GetField("list")
	if list == nil || !typ.TypeEquals(list.Type, fn) {
		t.Fatalf("repo.list = %v, want %v", list, fn)
	}
}

func TestFieldFactsFromAssignmentsAtPoint_NestsLoweredPath(t *testing.T) {
	const sym cfg.SymbolID = 7
	fn := typ.Func().Returns(typ.String).Build()
	inputs := &flow.Inputs{
		Assignments: []flow.UnifiedAssignment{{
			Point: cfg.Point(3),
			TargetPath: constraint.Path{
				Symbol: sym,
				Segments: []constraint.Segment{
					{Kind: constraint.SegmentField, Name: "repo"},
					{Kind: constraint.SegmentField, Name: "list"},
				},
			},
			Type: fn,
		}},
	}

	got := FieldFactsFromAssignmentsAtPoint(inputs, map[cfg.SymbolID]bool{sym: true}, cfg.Point(3))

	repo, ok := got[sym]["repo"].(*typ.Record)
	if !ok {
		t.Fatalf("repo fact = %T/%v, want record", got[sym]["repo"], got[sym]["repo"])
	}
	list := repo.GetField("list")
	if list == nil || !typ.TypeEquals(list.Type, fn) {
		t.Fatalf("repo.list = %v, want %v", list, fn)
	}
}

func TestFieldFactsFromAssignmentsAtPoint_RejectsFutureLoweredPath(t *testing.T) {
	const sym cfg.SymbolID = 7
	inputs := &flow.Inputs{
		Assignments: []flow.UnifiedAssignment{{
			Point:      cfg.Point(5),
			TargetPath: constraint.Path{Symbol: sym, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "name"}}},
			Type:       typ.String,
		}},
	}

	got := FieldFactsFromAssignmentsAtPoint(inputs, map[cfg.SymbolID]bool{sym: true}, cfg.Point(3))
	if len(got) != 0 {
		t.Fatalf("future captured field facts = %v, want none", got)
	}
}

func TestPromotedFieldNamesAtPoint_DominatingConcreteAssignment(t *testing.T) {
	const sym cfg.SymbolID = 7
	concrete := typ.NewRecord().Field("get", typ.Func().Build()).Build()
	inputs := &flow.Inputs{
		Assignments: []flow.UnifiedAssignment{{
			Point:      cfg.Point(2),
			TargetPath: constraint.Path{Symbol: sym, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "client"}}},
			Type:       concrete,
		}},
	}

	defPoint := cfg.Point(5)
	dominates := func(a, b cfg.Point) bool { return a == cfg.Point(2) && b == defPoint }

	got := PromotedFieldNamesAtPoint(inputs, map[cfg.SymbolID]bool{sym: true}, defPoint, dominates)
	if !got[sym]["client"] {
		t.Fatalf("expected client promoted to definite, got %v", got)
	}
}

func TestPromotedFieldNamesAtPoint_NonDominatingAssignmentNotPromoted(t *testing.T) {
	const sym cfg.SymbolID = 7
	concrete := typ.NewRecord().Field("get", typ.Func().Build()).Build()
	inputs := &flow.Inputs{
		Assignments: []flow.UnifiedAssignment{{
			Point:      cfg.Point(2),
			TargetPath: constraint.Path{Symbol: sym, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "client"}}},
			Type:       concrete,
		}},
	}

	defPoint := cfg.Point(5)
	dominates := func(a, b cfg.Point) bool { return false }

	got := PromotedFieldNamesAtPoint(inputs, map[cfg.SymbolID]bool{sym: true}, defPoint, dominates)
	if got[sym]["client"] {
		t.Fatalf("expected non-dominating assignment to stay optional, got %v", got)
	}
}

func TestPromotedFieldNamesAtPoint_NilAssignmentNotPromoted(t *testing.T) {
	const sym cfg.SymbolID = 7
	inputs := &flow.Inputs{
		Assignments: []flow.UnifiedAssignment{{
			Point:      cfg.Point(2),
			TargetPath: constraint.Path{Symbol: sym, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "client"}}},
			Type:       typ.Nil,
		}},
	}

	defPoint := cfg.Point(5)
	dominates := func(a, b cfg.Point) bool { return true }

	got := PromotedFieldNamesAtPoint(inputs, map[cfg.SymbolID]bool{sym: true}, defPoint, dominates)
	if got[sym]["client"] {
		t.Fatalf("expected nil assignment not to promote, got %v", got)
	}
}

func TestPromotedFieldNamesAtPoint_OptionalAssignmentNotPromoted(t *testing.T) {
	const sym cfg.SymbolID = 7
	optional := typ.NewOptional(typ.NewRecord().Build())
	inputs := &flow.Inputs{
		Assignments: []flow.UnifiedAssignment{{
			Point:      cfg.Point(2),
			TargetPath: constraint.Path{Symbol: sym, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "client"}}},
			Type:       optional,
		}},
	}

	defPoint := cfg.Point(5)
	dominates := func(a, b cfg.Point) bool { return true }

	got := PromotedFieldNamesAtPoint(inputs, map[cfg.SymbolID]bool{sym: true}, defPoint, dominates)
	if got[sym]["client"] {
		t.Fatalf("expected optional assignment not to promote, got %v", got)
	}
}

func TestContainerMutationsFromEvidence_WidensKeyAndValue(t *testing.T) {
	const sym cfg.SymbolID = 8

	got := ContainerMutationsFromEvidence([]api.CapturedContainerEvidence{{
		Target: sym,
		Kind:   api.ContainerMutationMapElement,
		Segments: []constraint.Segment{{
			Kind: constraint.SegmentField,
			Name: "items",
		}},
		KeyType:   typ.LiteralString("k"),
		ValueType: typ.LiteralString("v"),
		Point:     3,
	}}, MutatorTypeObservers{})

	mutations := got[sym]
	if len(mutations) != 1 {
		t.Fatalf("mutations = %#v, want one", mutations)
	}
	if !typ.TypeEquals(mutations[0].KeyType.ProjectValue(), typ.String) || !typ.TypeEquals(mutations[0].ValueType.ProjectValue(), typ.String) {
		t.Fatalf("mutation key/value = %v/%v, want string/string", mutations[0].KeyType.ProjectValue(), mutations[0].ValueType.ProjectValue())
	}
}

func TestContainerMutationsFromEvidence_UsesLoweredMapMutatorEvidence(t *testing.T) {
	const sym cfg.SymbolID = 8
	valueType := typ.NewRecord().Field("last_activity", typ.String).Build()

	got := ContainerMutationsFromEvidence([]api.CapturedContainerEvidence{{
		Target: sym,
		Kind:   api.ContainerMutationMapElement,
		Segments: []constraint.Segment{{
			Kind: constraint.SegmentField,
			Name: "sessions",
		}},
		KeyType:   typ.String,
		ValueMode: flow.MapMutationValueUpdate,
		ValueType: valueType,
	}}, MutatorTypeObservers{})

	mutations := got[sym]
	if len(mutations) != 1 {
		t.Fatalf("mutations = %#v, want one", mutations)
	}
	if !typ.TypeEquals(mutations[0].KeyType.ProjectValue(), typ.String) ||
		mutations[0].ValueMode != flow.MapMutationValueUpdate ||
		!typ.TypeEquals(mutations[0].ValueType.ProjectValue(), valueType) {
		t.Fatalf("mutation key/mode/value = %v/%v/%v, want string/update/%v", mutations[0].KeyType.ProjectValue(), mutations[0].ValueMode, mutations[0].ValueType.ProjectValue(), valueType)
	}
}

func TestContainerMutationsFromEvidence_UsesSolvedKeyAndValueObservers(t *testing.T) {
	const sym cfg.SymbolID = 8
	keyPath := constraint.NewPath(12, "suite")
	valuePath := constraint.NewPath(13, "entry")
	valueType := typ.NewRecord().Field("id", typ.String).Build()

	got := ContainerMutationsFromEvidence([]api.CapturedContainerEvidence{{
		Point:     7,
		Target:    sym,
		Kind:      api.ContainerMutationTableElement,
		Segments:  []constraint.Segment{{Kind: constraint.SegmentField, Name: "groups"}},
		KeyPath:   keyPath,
		ValuePath: valuePath,
	}}, MutatorTypeObservers{
		Key: func(point cfg.Point, path constraint.Path, static typ.Type) typ.Type {
			if point != 7 || !path.Equal(keyPath) {
				t.Fatalf("unexpected key observer input point=%v path=%v", point, path)
			}
			return typ.LiteralString("core")
		},
		Value: func(point cfg.Point, path constraint.Path, static typ.Type, template flow.ValueTemplate) typ.Type {
			if point != 7 || !path.Equal(valuePath) {
				t.Fatalf("unexpected value observer input point=%v path=%v", point, path)
			}
			return valueType
		},
	})

	mutations := got[sym]
	if len(mutations) != 1 {
		t.Fatalf("mutations = %#v, want one", mutations)
	}
	if !typ.TypeEquals(mutations[0].KeyType.ProjectValue(), typ.String) || !typ.TypeEquals(mutations[0].ValueType.ProjectValue(), valueType) {
		t.Fatalf("mutation key/value = %v/%v, want string/%v", mutations[0].KeyType.ProjectValue(), mutations[0].ValueType.ProjectValue(), valueType)
	}
}
