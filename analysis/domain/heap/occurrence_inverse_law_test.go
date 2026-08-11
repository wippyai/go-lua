package heap

import (
	"testing"

	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/keyspace"
	proglink "github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	programlower "github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

// TestHeapOccurrenceInversesRoundTripFirstAndLast proves that the inverse
// APIs consume one supplied owner occurrence. The test intentionally does not
// use KeyAt or IndexAccessAt to discover the selected result.
func TestHeapOccurrenceInversesRoundTripFirstAndLast(t *testing.T) {
	linked, schema, shard, p := heapOccurrenceInverseFixture(t, "heap_occurrence_inverse")

	var first, last Key
	for index := 0; index < schema.KeyCount(); index++ {
		candidate, ok := schema.KeyAt(index)
		if !ok || candidate.Kind() != RootAllocation {
			continue
		}
		if !first.Valid() {
			first = candidate
		}
		last = candidate
	}
	if !first.Valid() || !last.Valid() {
		t.Fatal("fixture omitted Program allocation roots")
	}
	for name, want := range map[string]Key{"first": first, "last": last} {
		shardOfKey, term, kind, ok := want.ProgramAllocation()
		if !ok || shardOfKey != shard || kind == AllocationInvalid {
			t.Fatalf("%s allocation origin = %v/%v/%v/%v", name, shardOfKey, term, kind, ok)
		}
		got, gotOK := schema.KeyForProgramAllocation(shard, p, term)
		if !gotOK || got != want {
			t.Fatalf("%s inverse = %#v/%v, want %#v/true", name, got, gotOK, want)
		}
	}

	geometry := p.Flow().AccessGeometry().IndexAccesses()
	var firstRead, lastRead keyspace.Term
	for index := 0; index < geometry.Reads().Count(); index++ {
		term, ok := geometry.Reads().At(index)
		if !ok {
			t.Fatal("read occurrence")
		}
		if firstRead == 0 {
			firstRead = term
		}
		lastRead = term
	}
	if firstRead == 0 || lastRead == 0 {
		t.Fatal("fixture omitted indexed reads")
	}
	for name, term := range map[string]keyspace.Term{"first": firstRead, "last": lastRead} {
		access, ok := schema.IndexAccessFor(shard, p, term)
		if !ok {
			t.Fatalf("%s access inverse rejected", name)
		}
		sealed, sealedOK := schema.IndexAccessGeometry(access)
		base, key, lens, geometryOK := geometry.Reads().Get(term)
		if !sealedOK || !geometryOK || sealed.ReadTerm != term || sealed.WriteTerm != 0 || sealed.Base != base || sealed.KeyTerm != key || sealed.Lens != lens || sealed.Position != -1 || sealed.Values != 0 {
			t.Fatalf("%s access geometry = %#v/%v, want read %v/%v/%v", name, sealed, sealedOK, base, key, lens)
		}
	}

	if schema.Link() != linked {
		t.Fatal("fixture schema lost Link owner")
	}
}

func TestHeapOccurrenceInversesRejectForeignSplicesAndMalformedTerms(t *testing.T) {
	linked, schema, shard, p := heapOccurrenceInverseFixture(t, "heap_occurrence_inverse_local")
	foreign, foreignSchema, foreignShard, foreignProgram := heapOccurrenceInverseFixture(t, "heap_occurrence_inverse_foreign")
	_ = foreignSchema

	var allocationTerm, readTerm keyspace.Term
	for index := 0; index < schema.KeyCount(); index++ {
		key, ok := schema.KeyAt(index)
		if !ok {
			continue
		}
		if _, term, _, sourceOK := key.ProgramAllocation(); sourceOK {
			allocationTerm = term
			break
		}
	}
	if allocationTerm == 0 {
		t.Fatal("allocation term")
	}
	reads := p.Flow().AccessGeometry().IndexAccesses().Reads()
	readTerm, _ = reads.At(0)
	if readTerm == 0 {
		t.Fatal("read term")
	}

	// The same ordinal from an equivalent Link and a Program from that Link
	// cannot be spliced into this schema. A local Shard with a foreign Program
	// is rejected by the mounted Program pointer fence as well.
	if foreignShard == shard || linked == foreign {
		t.Fatal("fixture did not create independent Link/Shard authorities")
	}
	if _, ok := schema.KeyForProgramAllocation(foreignShard, foreignProgram, allocationTerm); ok {
		t.Fatal("foreign Shard crossed allocation inverse")
	}
	if _, ok := schema.KeyForProgramAllocation(shard, foreignProgram, allocationTerm); ok {
		t.Fatal("foreign Program crossed allocation inverse")
	}
	if _, ok := schema.IndexAccessFor(foreignShard, foreignProgram, readTerm); ok {
		t.Fatal("foreign Shard crossed access inverse")
	}
	if _, ok := schema.IndexAccessFor(shard, foreignProgram, readTerm); ok {
		t.Fatal("foreign Program crossed access inverse")
	}

	for _, term := range []keyspace.Term{0, keyspace.MakeTerm(keyspace.FamilyBody, 1), keyspace.MakeTerm(keyspace.FamilyWrite, 1)} {
		if _, ok := schema.KeyForProgramAllocation(shard, p, term); ok {
			t.Fatalf("malformed/dead allocation term %v was admitted", term)
		}
	}
	for _, term := range []keyspace.Term{0, keyspace.MakeTerm(keyspace.FamilyBody, 1), keyspace.MakeTerm(keyspace.FamilyTable, 1)} {
		if _, ok := schema.IndexAccessFor(shard, p, term); ok {
			t.Fatalf("malformed/dead access term %v was admitted", term)
		}
	}
}

func TestHeapOccurrenceInversesWarmQueriesAllocateNothing(t *testing.T) {
	_, schema, shard, p := heapOccurrenceInverseFixture(t, "heap_occurrence_inverse_alloc")
	var allocationTerm, readTerm keyspace.Term
	for index := 0; index < schema.KeyCount(); index++ {
		key, ok := schema.KeyAt(index)
		if !ok {
			continue
		}
		if _, term, _, sourceOK := key.ProgramAllocation(); sourceOK {
			allocationTerm = term
			break
		}
	}
	readTerm, _ = p.Flow().AccessGeometry().IndexAccesses().Reads().At(0)
	if allocationTerm == 0 || readTerm == 0 {
		t.Fatal("fixture omitted inverse operands")
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		if _, ok := schema.KeyForProgramAllocation(shard, p, allocationTerm); !ok {
			t.Fatal("allocation inverse")
		}
		if _, ok := schema.IndexAccessFor(shard, p, readTerm); !ok {
			t.Fatal("access inverse")
		}
	}); allocations != 0 {
		t.Fatalf("warm occurrence inverses allocated %v times", allocations)
	}
}

func heapOccurrenceInverseFixture(t testing.TB, name string) (*proglink.Link, Schema, linkproject.Shard, *program.Program) {
	t.Helper()
	p, err := programlower.Lower(programlower.Source{Name: name + ".lua", Text: []byte(`
local t = {}
local k = 1
t[1] = 2
local a = t[1]
return t, a
`)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := proglink.Seal(&proglink.Spec{Target: contract, Modules: []linkproject.Module{{Name: name, Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	schema, ok := Seal(linked)
	if !ok {
		t.Fatal("Heap Seal")
	}
	shard, ok := linked.Project().Mounts().At(0)
	if !ok {
		t.Fatal("fixture shard")
	}
	return linked, schema, shard, p
}
