package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/shapefact"
	"github.com/wippyai/go-lua/analysis/type/ambient"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func eventSourceType() *typ.Record {
	return typetable.NewRecord().
		Field("primary", typ.Instantiate(ambient.ChannelGeneric(), typetable.NewRecord().Field("id", typ.String).Build())).
		Build()
}

func declaredTopRootPartition(t *testing.T, declared typ.Type) equation.Partition {
	t.Helper()
	encoded, ok := shapefact.EncodeTarget(declared)
	if !ok {
		t.Fatal("encode declared root witness")
	}
	partition, err := equation.PartitionFromClosuresWithGuards(nil, equation.OutputClosure{Values: []equation.Fact{
		{Key: "declared-type/path/source/entry", Value: encoded},
		{Key: "value/path/source/entry", Value: []byte("scalar/top")},
		{Key: epochFactPrefix + "path/source/entry", Value: []byte("entry")},
	}})
	if err != nil {
		t.Fatalf("partition: %v", err)
	}
	return partition
}

// TestDeclaredSurfaceSurvivesTopRootValue states the rule a declared root keeps
// while its runtime value is an honest unknown: every write to that root had to
// satisfy the declaration, so the declaration is still the surface a static
// member read descends through.
func TestDeclaredSurfaceSurvivesTopRootValue(t *testing.T) {
	partition := declaredTopRootPartition(t, eventSourceType())

	value, found := typedPathValue([]byte("path/source.primary"), partition)
	if !found {
		t.Fatal("a declared root with a Top value published no member surface")
	}
	member, decoded := shapefact.DecodeTarget(value)
	if !decoded {
		t.Fatalf("member projection %q is not a type target", value)
	}
	payload, channel := ambient.ChannelPayloadType(member)
	if !channel || payload == nil {
		t.Fatalf("source.primary projected as %v, want the declared Channel<Event>", member)
	}
	if !typ.TypeEquals(payload, typetable.NewRecord().Field("id", typ.String).Build()) {
		t.Fatalf("channel payload = %v, want the declared event record", payload)
	}
}

// TestDeclaredSurfaceYieldsToConcreteRootValue keeps the lane falsifiable: a
// root whose current value is a concrete witness is decided by that value, not
// by the declaration.
func TestDeclaredSurfaceYieldsToConcreteRootValue(t *testing.T) {
	declared, ok := shapefact.EncodeTarget(eventSourceType())
	if !ok {
		t.Fatal("encode declared root witness")
	}
	concrete, ok := shapefact.EncodeTarget(typetable.NewRecord().Field("primary", typ.String).Build())
	if !ok {
		t.Fatal("encode concrete root witness")
	}
	partition, err := equation.PartitionFromClosuresWithGuards(nil, equation.OutputClosure{Values: []equation.Fact{
		{Key: "declared-type/path/source/entry", Value: declared},
		{Key: "value/path/source/op-00000002", Value: concrete},
		{Key: epochFactPrefix + "path/source/op-00000002", Value: []byte("op-00000002")},
	}})
	if err != nil {
		t.Fatalf("partition: %v", err)
	}
	value, found := typedPathValue([]byte("path/source.primary"), partition)
	if !found {
		t.Fatal("a concrete root value published no member surface")
	}
	member, decoded := shapefact.DecodeTarget(value)
	if !decoded || !typ.TypeEquals(member, typ.String) {
		t.Fatalf("source.primary projected as %v, want the concrete value's member", member)
	}
}

// TestDeclaredSurfaceRefusesGradualDeclaration holds the boundary: an any
// declaration states no members, so it must not become the surface a member
// read is decided against.
func TestDeclaredSurfaceRefusesGradualDeclaration(t *testing.T) {
	partition := declaredTopRootPartition(t, typ.Any)
	if _, found := typedPathValue([]byte("path/source.primary"), partition); found {
		t.Fatal("an any declaration published a member surface")
	}
}

// TestTypedChannelPayloadReadsProvenChannelValue states that a term whose own
// current value is a proven Channel<X> carries that payload, which is what a
// generic wrapper's call result establishes for its consumer's receive.
func TestTypedChannelPayloadReadsProvenChannelValue(t *testing.T) {
	payload := typetable.NewRecord().Field("id", typ.String).Build()
	encoded, ok := shapefact.EncodeTarget(typ.Instantiate(ambient.ChannelGeneric(), payload))
	if !ok {
		t.Fatal("encode channel result value")
	}
	partition, err := equation.PartitionFromClosuresWithGuards(nil, equation.OutputClosure{Values: []equation.Fact{
		{Key: "value/path/typed_events/op-00000004", Value: encoded},
		{Key: epochFactPrefix + "path/typed_events/op-00000004", Value: []byte("op-00000004")},
	}})
	if err != nil {
		t.Fatalf("partition: %v", err)
	}
	resolved, found := typedChannelPayload([]byte("path/typed_events"), partition)
	if !found || resolved == nil {
		t.Fatal("a proven Channel<X> value published no payload")
	}
	if !typ.TypeEquals(resolved, payload) {
		t.Fatalf("payload = %v, want the channel's declared payload", resolved)
	}
}

// TestTypedChannelPayloadRefusesNonChannelValue keeps that read falsifiable.
func TestTypedChannelPayloadRefusesNonChannelValue(t *testing.T) {
	encoded, ok := shapefact.EncodeTarget(typetable.NewRecord().Field("id", typ.String).Build())
	if !ok {
		t.Fatal("encode record result value")
	}
	partition, err := equation.PartitionFromClosuresWithGuards(nil, equation.OutputClosure{Values: []equation.Fact{
		{Key: "value/path/record/op-00000004", Value: encoded},
		{Key: epochFactPrefix + "path/record/op-00000004", Value: []byte("op-00000004")},
	}})
	if err != nil {
		t.Fatalf("partition: %v", err)
	}
	if payload, found := typedChannelPayload([]byte("path/record"), partition); found {
		t.Fatalf("a record value published channel payload %v", payload)
	}
}
