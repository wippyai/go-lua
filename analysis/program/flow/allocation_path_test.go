package flow_test

import (
	"testing"

	lualower "github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestAllocationPathsUseTheSealedFlowCertificate(t *testing.T) {
	program, err := lualower.Lower(lualower.Source{
		Name: "allocation-paths.lua",
		Text: []byte("local function make() return {key = 1} end\nreturn make()"),
	})
	if err != nil {
		t.Fatal(err)
	}
	bodies := program.Flow().Body()
	entry, ok := bodies.Entry()
	if !ok {
		t.Fatal("missing Source entry Body")
	}
	flow := program.Flow()
	path, ok := flow.BodyPath(entry)
	if !ok || !path.Available() {
		t.Fatalf("BodyPath(%v) = %v/%v, want the sealed entry path", entry, path, ok)
	}
	if got, ok := flow.SemanticTermPath(entry); !ok || got != path {
		t.Fatalf("SemanticTermPath(%v) = %v/%v, want BodyPath %v", entry, got, ok, path)
	}
	atom, atomOK := flow.SemanticTermAtom(entry)
	if !atomOK || !atom.Available() || atom.ID() != path {
		t.Fatalf("SemanticTermAtom(%v) = %v/%v, want the sealed atom for %v", entry, atom, atomOK, path)
	}
	secondAtom, secondAtomOK := flow.SemanticTermAtom(entry)
	if !secondAtomOK || secondAtom != atom {
		t.Fatal("SemanticTermAtom did not replay the same sealed atom")
	}
	if _, ok := flow.SemanticTermAtom(0); ok {
		t.Fatal("SemanticTermAtom accepted the invalid term")
	}
	if _, ok := flow.BodyPath(keyspace.MakeTerm(keyspace.FamilyCall, 1)); ok {
		t.Fatal("BodyPath accepted a non-Body term")
	}
	var table keyspace.Term
	tables := flow.Authored().Tables()
	for index := 0; index < tables.Count(); index++ {
		candidate, candidateOK := tables.At(index)
		if candidateOK && flow.Executable().Contains(candidate) {
			table = candidate
			break
		}
	}
	if table == 0 {
		t.Fatal("fixture did not produce an executable table allocation")
	}
	allocationID, allocationOK := flow.AllocationID(table)
	if !allocationOK || !allocationID.Available() {
		t.Fatalf("AllocationID(%v) = %v/%v, want the owner-issued allocation identity", table, allocationID, allocationOK)
	}
	field, fieldOK := tables.FieldAt(table, 0)
	if !fieldOK {
		t.Fatal("executable table has no authored field")
	}
	fieldID, fieldIDOK := flow.AllocationFieldID(table, field)
	if !fieldIDOK || !fieldID.Available() {
		t.Fatalf("AllocationFieldID(%v,%v) = %v/%v, want the owner-issued field identity", table, field, fieldID, fieldIDOK)
	}
	if _, foreignOK := flow.AllocationFieldID(table, keyspace.MakeTerm(keyspace.FamilyTableField, keyspace.TermOrdinal(field)+1)); foreignOK {
		t.Fatal("AllocationFieldID accepted a field outside the table relation")
	}
}
