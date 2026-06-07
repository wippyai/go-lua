package provenance

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/domain/typepath"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestRouteLocalTypeIdentityAlias(t *testing.T) {
	source := typ.NewRecord().Field("id", typ.String).Build()
	got := RouteLocalType(flow.ProvenanceRoute{
		Kind: flow.ProvenanceRouteIdentityAlias,
	}, source, testProjectSegments)
	if !typ.TypeEquals(got, source) {
		t.Fatalf("RouteLocalType(identity) = %v, want %v", got, source)
	}
}

func TestRouteLocalTypeIndexedIteratorComposesRemainder(t *testing.T) {
	source := typ.NewArray(typ.NewRecord().Field("payload", typ.String).Build())
	got := RouteLocalType(flow.ProvenanceRoute{
		Kind:     flow.ProvenanceRouteIndexedIterator,
		VarIndex: 1,
		Remainder: []constraint.Segment{
			{Kind: constraint.SegmentField, Name: "payload"},
		},
	}, source, testProjectSegments)
	if !typ.TypeEquals(got, typ.String) {
		t.Fatalf("RouteLocalType(indexed remainder) = %v, want string", got)
	}
}

func TestRouteLocalTypeKeyedIteratorKeyAndValue(t *testing.T) {
	source := typ.NewMap(typ.String, typ.Number)
	key := RouteLocalType(flow.ProvenanceRoute{
		Kind:     flow.ProvenanceRouteKeyedIterator,
		VarIndex: 0,
	}, source, testProjectSegments)
	if !typ.TypeEquals(key, typ.String) {
		t.Fatalf("RouteLocalType(keyed key) = %v, want string", key)
	}
	value := RouteLocalType(flow.ProvenanceRoute{
		Kind:     flow.ProvenanceRouteKeyedIterator,
		VarIndex: 1,
	}, source, testProjectSegments)
	if !typ.TypeEquals(value, typ.Number) {
		t.Fatalf("RouteLocalType(keyed value) = %v, want number", value)
	}
}

func TestRouteLocalTypeRejectsUnsupportedSlots(t *testing.T) {
	source := typ.NewArray(typ.String)
	got := RouteLocalType(flow.ProvenanceRoute{
		Kind:     flow.ProvenanceRouteIndexedIterator,
		VarIndex: 0,
	}, source, testProjectSegments)
	if got != nil {
		t.Fatalf("RouteLocalType(indexed key slot) = %v, want nil", got)
	}
}

func testProjectSegments(base typ.Type, segments []constraint.Segment) typ.Type {
	return typepath.TypeAtSegments(base, segments, typepath.Options{MissingFieldAsNil: true})
}
