package captured

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/functionsymbols"
	interprocdomain "github.com/wippyai/go-lua/compiler/check/domain/interproc"
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

	field := got[sym][capturedTestFieldKey("name")].ProjectValue()
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

	field := got[sym][capturedTestFieldKey("get_x")].ProjectValue()
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

	repoType := got[sym][capturedTestFieldKey("repo")].ProjectValue()
	repo, ok := repoType.(*typ.Record)
	if !ok {
		t.Fatalf("repo fact = %T/%v, want record", repoType, repoType)
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

	got := FieldFactsFromAssignmentsAtPoint(inputs.Assignments, capturedTestSymbols(sym), cfg.Point(3))

	repoType := got[sym][capturedTestFieldKey("repo")].ProjectValue()
	repo, ok := repoType.(*typ.Record)
	if !ok {
		t.Fatalf("repo fact = %T/%v, want record", repoType, repoType)
	}
	list := repo.GetField("list")
	if list == nil || !typ.TypeEquals(list.Type, fn) {
		t.Fatalf("repo.list = %v, want %v", list, fn)
	}
}

func capturedTestFieldKey(name string) interprocdomain.FieldKey {
	key, ok := interprocdomain.FieldKeyFromName(name)
	if !ok {
		panic("empty test field key")
	}
	return key
}

func capturedTestSymbols(syms ...cfg.SymbolID) functionsymbols.Set {
	var set functionsymbols.Set
	for _, sym := range syms {
		set.Add(sym)
	}
	return set
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

	got := FieldFactsFromAssignmentsAtPoint(inputs.Assignments, capturedTestSymbols(sym), cfg.Point(3))
	if len(got) != 0 {
		t.Fatalf("future captured field facts = %v, want none", got)
	}
}

func TestPromotedFieldsAtPoint_DominatingConcreteAssignment(t *testing.T) {
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

	got := PromotedFieldsAtPoint(inputs.Assignments, capturedTestSymbols(sym), defPoint, dominates)
	if !got[sym][capturedTestFieldKey("client")] {
		t.Fatalf("expected client promoted to definite, got %v", got)
	}
}

func TestPromotedFieldsAtPoint_NonDominatingAssignmentNotPromoted(t *testing.T) {
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

	got := PromotedFieldsAtPoint(inputs.Assignments, capturedTestSymbols(sym), defPoint, dominates)
	if got[sym][capturedTestFieldKey("client")] {
		t.Fatalf("expected non-dominating assignment to stay optional, got %v", got)
	}
}

func TestPromotedFieldsAtPoint_NilAssignmentNotPromoted(t *testing.T) {
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

	got := PromotedFieldsAtPoint(inputs.Assignments, capturedTestSymbols(sym), defPoint, dominates)
	if got[sym][capturedTestFieldKey("client")] {
		t.Fatalf("expected nil assignment not to promote, got %v", got)
	}
}

func TestPromotedFieldsAtPoint_OptionalAssignmentNotPromoted(t *testing.T) {
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

	got := PromotedFieldsAtPoint(inputs.Assignments, capturedTestSymbols(sym), defPoint, dominates)
	if got[sym][capturedTestFieldKey("client")] {
		t.Fatalf("expected optional assignment not to promote, got %v", got)
	}
}
