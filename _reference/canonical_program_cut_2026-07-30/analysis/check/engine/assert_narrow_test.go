package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/shapefact"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestAssertPublishesNarrowedMemberOnlyAfterItsCall(t *testing.T) {
	member := typetable.NewRecord().OptField("member", typ.String).Build()
	encoded, ok := shapefact.EncodeTarget(member)
	if !ok {
		t.Fatal("encode declared member witness")
	}
	partition, err := equation.PartitionFromClosuresWithGuards(nil, equation.OutputClosure{Values: []equation.Fact{{
		Key: "declared-type/path/box/entry", Value: encoded,
	}}})
	if err != nil {
		t.Fatalf("partition: %v", err)
	}
	operation := equation.BoundEquation{Target: equation.Coordinate{Body: equation.BodyID{1}, Name: "assert-call"}}
	facts := assertedPathNarrowingFacts(operation, directCallOperands{assertedPath: []byte("path/box.member")}, partition)
	if len(facts) != 1 || facts[0].Key != "assertion-value/path/box.member/assert-call" {
		t.Fatalf("assert facts = %#v", facts)
	}
	narrowed, decoded := shapefact.DecodeTarget(facts[0].Value)
	if !decoded || !typ.TypeEquals(narrowed, typ.String) {
		t.Fatalf("asserted member value = %v / %q, want string", narrowed, facts[0].Value)
	}
	if value, found := latestValue([]byte("path/box.member"), partition); found || value != nil {
		t.Fatalf("pre-assert partition already has member value %q", value)
	}
}

func TestAssertNarrowingDoesNotEscapeItsGuardedPartition(t *testing.T) {
	guard := equation.Guard{Body: equation.BodyID{1}, Encoding: []byte("front/branch/guard/true")}
	partition, err := equation.PartitionFromClosuresWithGuards(nil, equation.OutputClosure{Values: []equation.Fact{{
		Key: "assertion-value/path/box.member/assert-call", Value: []byte("scalar/string/\"ready\""), Guards: []equation.Guard{guard},
	}}})
	if err != nil {
		t.Fatalf("partition: %v", err)
	}
	if _, found := latestValue([]byte("path/box.member"), partition); found {
		t.Fatal("guarded assertion value escaped the post-join partition")
	}
}

func TestAssertNarrowingExpiresAtLaterWrite(t *testing.T) {
	partition, err := equation.PartitionFromClosuresWithGuards(nil, equation.OutputClosure{Values: []equation.Fact{
		{Key: "assertion-value/path/box.member/op-00000002", Value: []byte("scalar/string/\"ready\"")},
		{Key: "value/path/box.member/op-00000003", Value: []byte("scalar/nil")},
	}})
	if err != nil {
		t.Fatalf("partition: %v", err)
	}
	if value, found := assertionNarrowedValue([]byte("path/box.member"), "", partition); found || value != nil {
		t.Fatalf("stale assertion value = %q / %v", value, found)
	}
}
