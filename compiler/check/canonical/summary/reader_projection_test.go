package summary

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestReaderProjectsSnapshotSummaryCells(t *testing.T) {
	ref := FuncRef{GraphID: 7}
	reader := NewReader(nil, nil, map[FuncRef]Summary{
		ref: {
			Returns: []product.AbstractValue{product.FromType(typ.String)},
			Params:  paramevidence.Contracts{1: paramevidence.DemandFromType(typ.Number)},
			Relations: flow.ReturnRelationsOfErrorReturns([]flow.ReturnCorrelation{{
				ValueIndex: 0,
				ErrorIndex: 1,
			}}),
		},
	})

	returnValues := reader.ReturnValues(ref)
	if len(returnValues) != 1 || !typ.TypeEquals(returnValues[0].ProjectValue(), typ.String) {
		t.Fatalf("ReturnValues = %#v, want string slot", returnValues)
	}
	returnValues[0] = product.FromType(typ.Boolean)
	if again := reader.ReturnValues(ref); !typ.TypeEquals(again[0].ProjectValue(), typ.String) {
		t.Fatalf("ReturnValues exposed mutable backing: %#v", again)
	}

	returnTypes := reader.ReturnTypes(ref)
	if len(returnTypes) != 1 || !typ.TypeEquals(returnTypes[0], typ.String) {
		t.Fatalf("ReturnTypes = %#v, want string", returnTypes)
	}
	paramTypes := reader.ParamTypes(ref)
	if len(paramTypes) != 1 || !typ.TypeEquals(paramTypes[1], typ.Number) {
		t.Fatalf("ParamTypes = %#v, want slot 1 number", paramTypes)
	}
	if !reader.ReturnRelations(ref).HasErrorReturn(flow.ReturnCorrelation{ValueIndex: 0, ErrorIndex: 1}) {
		t.Fatalf("ReturnRelations = %#v, want value/error proof", reader.ReturnRelations(ref).ErrorReturns())
	}
}

func TestReaderSnapshotOverlayOverridesExactContext(t *testing.T) {
	ref := FuncRef{GraphID: 17}
	values := EntryValues{0: product.FromType(typ.Boolean)}
	key := NewKeyWithReferenceContext(
		ref,
		flow.ReferenceContextOf(flow.CaptureCellsDomain.Bottom(), flow.FunctionRefsDomain.Bottom(), flow.ClosureRefsDomain.Bottom()),
		values,
		flow.BoundaryFactsDomain.Top(),
	)
	reader := NewReaderWithOverlay(nil, nil, map[FuncRef]Summary{
		ref: {Returns: []product.AbstractValue{product.FromType(typ.String)}},
	}, map[Key]Summary{
		key: {Returns: []product.AbstractValue{product.FromType(typ.Number)}},
	})

	got := ReturnTypes(reader.SummarizeWithKey(key))
	if len(got) != 1 || !typ.TypeEquals(got[0], typ.Number) {
		t.Fatalf("exact ReturnTypes = %#v, want number from overlay", got)
	}
	if fallback := reader.ReturnTypes(ref); len(fallback) != 1 || !typ.TypeEquals(fallback[0], typ.String) {
		t.Fatalf("fallback ReturnTypes = %#v, want string snapshot", fallback)
	}
}

func TestReaderParamNarrowsDefensiveCopy(t *testing.T) {
	ref := FuncRef{GraphID: 8}
	post := paramevidence.ReturnPostconditionsFromParamNarrows([]paramevidence.ParamNarrow{{
		Param:    0,
		Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "kind"}},
		Check:    cfg.CheckNotNil,
		EqParam:  -1,
	}})
	reader := NewReader(nil, nil, map[FuncRef]Summary{
		ref: {
			Postconditions: post,
			ParamNarrows: []paramevidence.ParamNarrow{{
				Param:    0,
				Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "kind"}},
				Check:    cfg.CheckNotNil,
			}},
		},
	})

	got := reader.ParamNarrows(ref)
	if len(got) != 1 || len(got[0].Segments) != 1 {
		t.Fatalf("ParamNarrows = %#v, want one segmented narrow", got)
	}
	got[0].Segments[0] = constraint.Segment{Kind: constraint.SegmentField, Name: "mutated"}
	again := reader.ParamNarrows(ref)
	if again[0].Segments[0].Name != "kind" {
		t.Fatalf("ParamNarrows exposed mutable backing: %#v", again)
	}

	postCopy := reader.ReturnPostconditions(ref)
	if !postCopy.HasConstraints() {
		t.Fatal("ReturnPostconditions missing snapshot proof")
	}
	mutated := postCopy.Condition()
	mutated.Disjuncts[0][0] = constraint.Truthy{Path: constraint.ParamPath(9)}
	againPost := reader.ReturnPostconditions(ref)
	if !containsConstraint(againPost.Condition().MustConstraints(), constraint.NotNil{Path: constraint.ParamPath(0).Field("kind")}) {
		t.Fatalf("ReturnPostconditions exposed mutable backing: %v", againPost.Condition())
	}
}

func TestReaderEntryValueDependenciesUseSnapshotSummary(t *testing.T) {
	dep := FuncRef{GraphID: 9}
	callee := FuncRef{GraphID: 10}
	reader := NewReader(nil, nil, map[FuncRef]Summary{
		dep: {
			CallEntryValues: CallEntryValues{
				callee: EntryValues{2: product.FromType(typ.String)},
			},
			PrototypeSelf: flow.PrototypeSelfOf([]flow.PrototypeSelfEntry{{
				Prototype: 99,
				Value:     product.FromType(typ.Boolean),
			}}),
		},
	})

	values := reader.CallEntryValues(dep, callee)
	if len(values) != 1 || !typ.TypeEquals(values[2].ProjectValue(), typ.String) {
		t.Fatalf("CallEntryValues = %#v, want slot 2 string", values)
	}
	values[2] = product.FromType(typ.Number)
	again := reader.CallEntryValues(dep, callee)
	if !typ.TypeEquals(again[2].ProjectValue(), typ.String) {
		t.Fatalf("CallEntryValues exposed mutable backing: %#v", again)
	}

	self, ok := reader.PrototypeSelf(dep).Value(99)
	if !ok || !typ.TypeEquals(self.ProjectValue(), typ.Boolean) {
		t.Fatalf("PrototypeSelf = %#v, want prototype 99 boolean", reader.PrototypeSelf(dep).Entries())
	}
}

func containsConstraint(haystack []constraint.Constraint, needle constraint.Constraint) bool {
	for _, c := range haystack {
		if c.Equals(needle) {
			return true
		}
	}
	return false
}
