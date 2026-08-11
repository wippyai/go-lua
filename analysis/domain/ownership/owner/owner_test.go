package owner

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/domain/ownership"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestDeclareAcceptsEmptyOwnershipDutyUniverse(t *testing.T) {
	source := emptyLink(t)
	heapSchema, heapOK := heap.Seal(source)
	schema, ok := ownership.NewSchema(source, heapSchema)
	if !heapOK || !ok || !schema.Valid() || schema.CoordinateCount() != 0 {
		t.Fatalf("empty Ownership schema = %v/%d, want valid empty universe", ok, schema.CoordinateCount())
	}
	composition := engine.NewComposition()
	owner, ok := Declare(composition, semanticKey(t, source), schema)
	if !ok || owner == nil || !owner.Schema().Valid() || owner.Schema().CoordinateCount() != 0 {
		t.Fatal("declare empty Ownership factor")
	}
}

func semanticKey(t testing.TB, source *link.Link) engine.SemanticKey {
	t.Helper()
	id := source.ContentID()
	var semanticID [32]byte
	copy(semanticID[:], id[:])
	key, ok := engine.NewSemanticKey(semanticID, 1)
	if !ok {
		t.Fatal("semantic key")
	}
	return key
}

func emptyLink(t testing.TB) *link.Link {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: "empty_ownership_owner.lua"})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	source, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "empty", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	return source
}
