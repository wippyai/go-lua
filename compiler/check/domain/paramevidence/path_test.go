package paramevidence

import (
	"testing"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

func TestPathEvidence_WrapsFieldSegments(t *testing.T) {
	got := PathEvidence([]constraint.Segment{
		{Kind: constraint.SegmentField, Name: "meta"},
		{Kind: constraint.SegmentIndexString, Name: "id"},
	}, typ.String)

	want := typ.NewRecord().
		ReadonlyField("meta", typ.NewRecord().ReadonlyField("id", typ.String).Build()).
		Build()
	if !typ.TypeEquals(got, want) {
		t.Fatalf("PathEvidence() = %v, want %v", got, want)
	}
}

func TestPathEvidence_ReadContractAdmitsPreciseMutableCallerShape(t *testing.T) {
	contract := PathEvidence([]constraint.Segment{
		{Kind: constraint.SegmentField, Name: "created_at"},
	}, typ.Unknown)
	caller := typ.NewRecord().
		Field("created_at", typ.NewInterface("Time", nil)).
		Field("pid", typ.String).
		Build()

	if !subtype.IsSubtype(caller, contract) {
		t.Fatalf("precise caller record %v should satisfy readonly path contract %v", caller, contract)
	}
}

func TestPathEvidence_RejectsNumericSegment(t *testing.T) {
	got := PathEvidence([]constraint.Segment{{Kind: constraint.SegmentIndexInt, Index: 1}}, typ.String)
	if got != nil {
		t.Fatalf("PathEvidence() = %v, want nil", got)
	}
}

func TestIteratorAndMapElementEvidence(t *testing.T) {
	if got := IndexedIteratorEvidence(1, typ.String); !typ.TypeEquals(got, typ.NewArray(typ.String)) {
		t.Fatalf("IndexedIteratorEvidence() = %v, want string[]", got)
	}
	if got := KeyedIteratorEvidence(0, typ.String); !typ.TypeEquals(got, typ.NewReadonlyMap(typ.String, typ.Any)) {
		t.Fatalf("KeyedIteratorEvidence(key) = %v, want readonly {[string]: any}", got)
	}
	if got := KeyedIteratorEvidence(1, typ.Integer); !typ.TypeEquals(got, typ.NewReadonlyMap(typ.Any, typ.Integer)) {
		t.Fatalf("KeyedIteratorEvidence(value) = %v, want readonly {[any]: integer}", got)
	}
	if !subtype.IsSubtype(typ.NewMap(typ.String, typ.Integer), KeyedIteratorEvidence(1, typ.Number)) {
		t.Fatal("mutable {[string]: integer} should satisfy readonly value-iteration evidence {[any]: number}")
	}
	if subtype.IsSubtype(KeyedIteratorEvidence(1, typ.Number), typ.NewMap(typ.String, typ.Number)) {
		t.Fatal("readonly keyed-iteration evidence must not satisfy mutable map contract")
	}
	if got := KeyedIteratorEvidence(0, typ.Any); got != nil {
		t.Fatalf("KeyedIteratorEvidence(any key) = %v, want nil until read-only iterable contracts exist", got)
	}
	if got := KeyedIteratorEvidence(1, typ.Unknown); got != nil {
		t.Fatalf("KeyedIteratorEvidence(unknown value) = %v, want nil until read-only iterable contracts exist", got)
	}
	if got := MapElementEvidence(typ.String, typ.Number); !typ.TypeEquals(got, typ.NewMap(typ.String, typ.Number)) {
		t.Fatalf("MapElementEvidence() = %v, want {[string]: number}", got)
	}
}

func TestProvenanceRouteContractTargets_IdentityAlias(t *testing.T) {
	source := constraint.NewPath(1, "source")
	contract := DemandFromPathType([]constraint.Segment{{Kind: constraint.SegmentField, Name: "id"}}, typ.String)

	targets := ProvenanceRouteContractTargets(flow.ProvenanceRoute{
		Kind:   flow.ProvenanceRouteIdentityAlias,
		Source: source,
	}, contract)

	requireRouteTarget(t, targets, source, typ.NewRecord().ReadonlyField("id", typ.String).Build())
}

func TestProvenanceRouteContractTargets_IndexedIteratorComposesRemainder(t *testing.T) {
	source := constraint.NewPath(1, "records")

	targets := ProvenanceRouteContractTargets(flow.ProvenanceRoute{
		Kind:     flow.ProvenanceRouteIndexedIterator,
		Source:   source,
		VarIndex: 1,
		Remainder: []constraint.Segment{
			{Kind: constraint.SegmentField, Name: "payload"},
		},
	}, DemandFromPathType([]constraint.Segment{{Kind: constraint.SegmentField, Name: "id"}}, typ.String))

	wantElem := typ.NewRecord().
		ReadonlyField("payload", typ.NewRecord().ReadonlyField("id", typ.String).Build()).
		Build()
	requireRouteTarget(t, targets, source, typ.NewArray(wantElem))
}

func TestProvenanceRouteContractTargets_KeyedIteratorValue(t *testing.T) {
	source := constraint.NewPath(1, "records")

	targets := ProvenanceRouteContractTargets(flow.ProvenanceRoute{
		Kind:     flow.ProvenanceRouteKeyedIterator,
		Source:   source,
		VarIndex: 1,
	}, DemandFromType(typ.Number))

	requireRouteTarget(t, targets, source, typ.NewReadonlyMap(typ.Any, typ.Number))
}

func TestProvenanceRouteContractTargets_AppendElementFieldSourceField(t *testing.T) {
	source := constraint.NewPath(1, "out")

	targets := ProvenanceRouteContractTargets(flow.ProvenanceRoute{
		Kind:   flow.ProvenanceRouteAppendElementField,
		Source: source,
		SourceField: []constraint.Segment{
			{Kind: constraint.SegmentField, Name: "items"},
		},
		FieldRemainder: []constraint.Segment{
			{Kind: constraint.SegmentField, Name: "payload"},
		},
	}, DemandFromPathType([]constraint.Segment{{Kind: constraint.SegmentField, Name: "id"}}, typ.String))

	wantElem := typ.NewRecord().
		ReadonlyField("items", typ.NewRecord().
			ReadonlyField("payload", typ.NewRecord().ReadonlyField("id", typ.String).Build()).
			Build()).
		Build()
	requireRouteTarget(t, targets, source, typ.NewArray(wantElem))
}

func TestProvenanceRouteContractTargets_AppendElementFieldRelativeSource(t *testing.T) {
	source := constraint.NewPath(1, "out")

	targets := ProvenanceRouteContractTargets(flow.ProvenanceRoute{
		Kind:   flow.ProvenanceRouteAppendElementField,
		Source: source,
		FieldRemainder: []constraint.Segment{
			{Kind: constraint.SegmentField, Name: "payload"},
		},
	}, DemandFromPathType([]constraint.Segment{{Kind: constraint.SegmentField, Name: "id"}}, typ.String))

	requireRouteTarget(t, targets, source.Field("payload"), typ.NewRecord().ReadonlyField("id", typ.String).Build())
}

func TestProvenanceRouteContractClosure_ChainsIteratorAndAliasRoutes(t *testing.T) {
	item := constraint.NewPath(2, "item").Field("id")
	array := constraint.NewPath(3, "records")
	param := constraint.NewPath(4, "arg")
	contract := DemandFromType(typ.String)

	targets := ProvenanceRouteContractClosure(item, contract, func(path constraint.Path) []flow.ProvenanceRoute {
		switch {
		case path.Equal(item):
			return []flow.ProvenanceRoute{{
				Kind:     flow.ProvenanceRouteIndexedIterator,
				Source:   array,
				VarIndex: 1,
				Remainder: []constraint.Segment{
					{Kind: constraint.SegmentField, Name: "id"},
				},
			}}
		case path.Equal(array):
			return []flow.ProvenanceRoute{{
				Kind:   flow.ProvenanceRouteIdentityAlias,
				Source: param,
			}}
		default:
			return nil
		}
	})

	requireRouteTargetAt(t, targets, item, typ.String)
	wantArray := typ.NewArray(typ.NewRecord().ReadonlyField("id", typ.String).Build())
	requireRouteTargetAt(t, targets, array, wantArray)
	requireRouteTargetAt(t, targets, param, wantArray)
}

func TestProvenanceRouteContractClosure_JoinsSameSourcePathDemands(t *testing.T) {
	item := constraint.NewPath(2, "item")
	array := constraint.NewPath(3, "records")
	param := constraint.NewPath(4, "arg")
	contract := DemandFromType(typ.String)

	targets := ProvenanceRouteContractClosure(item, contract, func(path constraint.Path) []flow.ProvenanceRoute {
		switch {
		case path.Equal(item):
			return []flow.ProvenanceRoute{
				{
					Kind:     flow.ProvenanceRouteIndexedIterator,
					Source:   array,
					VarIndex: 1,
					Remainder: []constraint.Segment{
						{Kind: constraint.SegmentField, Name: "id"},
					},
				},
				{
					Kind:     flow.ProvenanceRouteIndexedIterator,
					Source:   array,
					VarIndex: 1,
					Remainder: []constraint.Segment{
						{Kind: constraint.SegmentField, Name: "name"},
					},
				},
			}
		case path.Equal(array):
			return []flow.ProvenanceRoute{{
				Kind:   flow.ProvenanceRouteIdentityAlias,
				Source: param,
			}}
		default:
			return nil
		}
	})

	wantArray := typ.NewArray(typ.NewRecord().
		ReadonlyField("id", typ.String).
		ReadonlyField("name", typ.String).
		Build())
	requireRouteTargetAt(t, targets, item, typ.String)
	requireRouteTargetAt(t, targets, array, wantArray)
	requireRouteTargetAt(t, targets, param, wantArray)
}

func requireRouteTarget(t *testing.T, targets []ProvenanceRouteContractTarget, path constraint.Path, projected typ.Type) {
	t.Helper()
	if len(targets) != 1 {
		t.Fatalf("route targets got %d, want 1: %#v", len(targets), targets)
	}
	if !targets[0].Path.Equal(path) {
		t.Fatalf("route target path = %#v, want %#v", targets[0].Path, path)
	}
	if got := targets[0].Contract.ProjectValue(); !typ.TypeEquals(got, projected) {
		t.Fatalf("route target contract = %v, want %v", got, projected)
	}
}

func requireRouteTargetAt(t *testing.T, targets []ProvenanceRouteContractTarget, path constraint.Path, projected typ.Type) {
	t.Helper()
	for _, target := range targets {
		if !target.Path.Equal(path) {
			continue
		}
		if got := target.Contract.ProjectValue(); !typ.TypeEquals(got, projected) {
			t.Fatalf("route target %s contract = %v, want %v", path.Key(), got, projected)
		}
		return
	}
	t.Fatalf("route target %s missing from %#v", path.Key(), targets)
}
