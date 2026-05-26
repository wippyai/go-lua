package paramevidence

import (
	"testing"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

func TestMergePathCallExpectation_ReadEvidenceUsesReadonlyFields(t *testing.T) {
	segments := []constraint.Segment{{Kind: constraint.SegmentField, Name: "created_at"}}
	got := MergePathCallExpectation(nil, segments, typ.NewInterface("Time", nil), true)
	rec, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("expected record evidence, got %T %v", got, got)
	}
	field := rec.GetField("created_at")
	if field == nil || !field.Readonly {
		t.Fatalf("read path evidence field = %#v, want readonly field", field)
	}

	caller := typ.NewRecord().
		Field("created_at", typ.NewInterface("Time", nil)).
		Field("pid", typ.String).
		Build()
	if !subtype.IsSubtype(caller, got) {
		t.Fatalf("precise mutable caller %v should satisfy readonly path expectation %v", caller, got)
	}
}

func TestMergeExpectedAtPath_WriteEvidenceKeepsFinalFieldMutable(t *testing.T) {
	segments := []constraint.Segment{
		{Kind: constraint.SegmentField, Name: "session"},
		{Kind: constraint.SegmentField, Name: "last_activity"},
	}
	got := MergeExpectedAtPath(nil, segments, typ.String, true, PathAccessWrite)
	rec, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("expected record evidence, got %T %v", got, got)
	}
	outer := rec.GetField("session")
	if outer == nil || !outer.Readonly {
		t.Fatalf("intermediate field = %#v, want readonly container read", outer)
	}
	inner, ok := outer.Type.(*typ.Record)
	if !ok {
		t.Fatalf("inner field type = %T %v, want record", outer.Type, outer.Type)
	}
	last := inner.GetField("last_activity")
	if last == nil || last.Readonly {
		t.Fatalf("final write field = %#v, want mutable write slot", last)
	}
}

func TestMergeExpectedAtPath_WriteEvidenceDominatesPriorReadEvidence(t *testing.T) {
	segments := []constraint.Segment{{Kind: constraint.SegmentField, Name: "terminating"}}
	read := MergePathCallExpectation(nil, segments, typ.Boolean, true)
	got := MergeExpectedAtPath(read, segments, typ.Boolean, true, PathAccessWrite)
	rec, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("expected record evidence, got %T %v", got, got)
	}
	field := rec.GetField("terminating")
	if field == nil || field.Readonly {
		t.Fatalf("write after read field = %#v, want mutable", field)
	}
}

func TestMergeCallExpectation_TreatsAnyAsTop(t *testing.T) {
	suite := typ.NewRecord().Field("name", typ.String).Build()

	got := MergeCallExpectation(typ.Any, suite, true)
	if !typ.TypeEquals(got, typ.Any) {
		t.Fatalf("MergeCallExpectation(any, Suite) = %v, want any", got)
	}
}

func TestMergeCallExpectation_DoesNotErasePreciseRecursiveArrayWithAnyArray(t *testing.T) {
	precise := typ.NewRecursive("Inferred", func(typ.Type) typ.Type {
		return typ.NewArray(typ.String)
	})
	expected := typ.NewArray(typ.Any)

	got := MergeCallExpectation(precise, expected, true)
	if typ.TypeEquals(got, expected) {
		t.Fatalf("MergeCallExpectation(precise recursive array, any[]) erased evidence: %v", got)
	}
	if !subtype.IsSubtype(precise, got) {
		t.Fatalf("merged type %v must still admit precise evidence %v", got, precise)
	}
}

func TestMergeCallExpectation_ParamDominatesCompatibleBodyEvidence(t *testing.T) {
	headerMap := typ.NewMap(typ.String, typ.String)
	bodyHeaderEvidence := typ.NewRecord().
		SetOpen(true).
		OptField("Accept", typ.String).
		Build()
	old := typ.NewRecord().
		SetOpen(true).
		OptField("headers", typ.NewUnion(headerMap, bodyHeaderEvidence)).
		OptField("stream", typ.Unknown).
		Build()
	expected := typ.NewRecord().
		Field("headers", headerMap).
		OptField("stream", typ.Boolean).
		Build()

	got := MergeCallExpectation(old, expected, true)
	if !typ.TypeEquals(got, expected) {
		t.Fatalf("MergeCallExpectation(body evidence, expected param) = %v, want %v", got, expected)
	}
}

func TestWidenArrayElementAtPath_RecursiveProductsConverge(t *testing.T) {
	left := typ.NewRecursive("Resource", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("close", typ.Func().Param("self", self).Build()).
			Field("next", typ.NewOptional(self)).
			Build()
	})
	right := typ.NewRecursive("Resource", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("close", typ.Func().Param("self", self).Build()).
			Field("next", typ.NewOptional(self)).
			Build()
	})

	got := WidenArrayElementAtPath(typ.NewArray(left), nil, right)
	arr, ok := got.(*typ.Array)
	if !ok {
		t.Fatalf("WidenArrayElementAtPath(array, recursive) = %T %[1]v, want array", got)
	}
	if _, ok := arr.Element.(*typ.Union); ok {
		t.Fatalf("recursive array element widened to raw union: %v", arr.Element)
	}
	if !value.SameConvergedFact(arr.Element, left) {
		t.Fatalf("recursive array element = %v, want converged resource product", arr.Element)
	}

	again := WidenArrayElementAtPath(got, nil, right)
	if !value.SameConvergedFact(again, got) {
		t.Fatalf("recursive array element widening is not idempotent: first=%v second=%v", got, again)
	}
}
