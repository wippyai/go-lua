package owner

import (
	"strconv"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/escape"
	"github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestDeclareAcceptsTargetStaticEscapeUniverse(t *testing.T) {
	source := emptyLink(t)
	heapSchema, heapOK := heap.Seal(source)
	schema, ok := escape.NewSchema(source, heapSchema)
	if !heapOK || !ok || !schema.Valid() {
		t.Fatalf("target-static Escape schema = %v/%d", ok, schema.CoordinateCount())
	}
	composition := engine.NewComposition()
	owner, ok := Declare(composition, semanticKey(t, source), schema)
	if !ok || owner == nil || !owner.Schema().Valid() {
		t.Fatal("declare target-static Escape factor")
	}
}

func TestValidCoordinateCountHonorsOneBasedEscapeCoordinateLimit(t *testing.T) {
	if !validCoordinateCount(0) || !validCoordinateCount(1) || validCoordinateCount(-1) {
		t.Fatal("basic Escape coordinate range")
	}
	if strconv.IntSize <= 32 {
		return
	}
	limit := uint64(^uint32(0))
	if !validCoordinateCount(int(limit)) || validCoordinateCount(int(limit+1)) {
		t.Fatal("Escape coordinate range did not reserve zero sentinel")
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
	program, err := lower.Lower(lower.Source{Name: "empty_escape_owner.lua"})
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
