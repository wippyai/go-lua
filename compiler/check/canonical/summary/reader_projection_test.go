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

func TestReaderSnapshotRequiresExactContextSummary(t *testing.T) {
	ref := FuncRef{GraphID: 17}
	values := EntryValues{0: product.FromType(typ.Boolean)}
	key := NewKeyWithReferenceContext(
		ref,
		flow.ReferenceContextOf(flow.CaptureCellsDomain.Bottom(), flow.FunctionRefsDomain.Bottom(), flow.ClosureRefsDomain.Bottom()),
		values,
		flow.BoundaryFactsDomain.Top(),
	)
	reader := NewReader(nil, nil, map[FuncRef]Summary{
		ref: {Returns: []product.AbstractValue{product.FromType(typ.String)}},
	})

	got := ReturnTypes(reader.SummarizeWithKey(key))
	if len(got) != 0 {
		t.Fatalf("exact ReturnTypes = %#v, want no implicit aggregate projection", got)
	}
	if aggregate := reader.ReturnTypes(ref); len(aggregate) != 1 || !typ.TypeEquals(aggregate[0], typ.String) {
		t.Fatalf("aggregate ReturnTypes = %#v, want string snapshot", aggregate)
	}
}

func TestReaderReturnPostconditionsDefensiveCopy(t *testing.T) {
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
		},
	})

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

func TestReaderEntryPublicationDependenciesUseSnapshotSummary(t *testing.T) {
	dep := FuncRef{GraphID: 9}
	callee := FuncRef{GraphID: 10}
	reader := NewReader(nil, nil, map[FuncRef]Summary{
		dep: {
			CallEntryPublication: CallEntryPublications{
				callee: {
					Values: EntryValues{2: product.FromType(typ.String)},
					Facts: flow.BoundaryFactsFromParts(flow.BoundaryFactParts{
						KeyPresence: []flow.BoundaryKeyPresenceFact{{
							Table: flow.BoundaryPath{Kind: flow.BoundaryPathParam, Index: 2},
							Key:   flow.BoundaryPath{Kind: flow.BoundaryPathParam, Index: 2, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "id"}}},
						}},
					}),
				},
			},
			PrototypeSelf: flow.PrototypeSelfOf([]flow.PrototypeSelfEntry{{
				Prototype: 99,
				Value:     product.FromType(typ.Boolean),
			}}),
		},
	})

	pub := reader.CallEntryPublication(dep, callee)
	values := pub.Values
	if len(values) != 1 || !typ.TypeEquals(values[2].ProjectValue(), typ.String) {
		t.Fatalf("CallEntryPublication.Values = %#v, want slot 2 string", values)
	}
	facts := pub.Facts
	if !facts.HasKeyPresence(flow.BoundaryKeyPresenceFact{
		Table: flow.BoundaryPath{Kind: flow.BoundaryPathParam, Index: 2},
		Key:   flow.BoundaryPath{Kind: flow.BoundaryPathParam, Index: 2, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "id"}}},
	}) {
		t.Fatalf("CallEntryPublication.Facts = %#v, want param key proof", facts.KeyPresence())
	}
	if missing := reader.CallEntryPublication(dep, FuncRef{GraphID: 99}).Facts; missing.HasProof() {
		t.Fatalf("missing CallEntryPublication.Facts = %#v, want no finite proof", missing)
	}
	values[2] = product.FromType(typ.Number)
	again := reader.CallEntryPublication(dep, callee).Values
	if !typ.TypeEquals(again[2].ProjectValue(), typ.String) {
		t.Fatalf("CallEntryPublication.Values exposed mutable backing: %#v", again)
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
