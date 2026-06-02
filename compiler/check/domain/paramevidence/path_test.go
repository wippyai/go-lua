package paramevidence

import (
	"testing"

	"github.com/wippyai/go-lua/types/constraint"
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
