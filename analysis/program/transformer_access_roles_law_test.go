package program

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestTransformerAccessRolesBorrowTheEmptyFlowPlanes(t *testing.T) {
	published, err := Publish(rootAssembly(t, "transformer-access-empty.lua"))
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	input := published.TransformerInput()
	if !input.Available() {
		t.Fatal("published TransformerInput unavailable")
	}
	if input.IndexReads().Count() != 0 || input.IndexWrites().Count() != 0 {
		t.Fatal("empty Flow fixture exposed fabricated index candidates")
	}
	if _, ok := input.IndexReads().At(0); ok {
		t.Fatal("empty read plane accepted an out-of-range candidate")
	}
	if _, ok := input.IndexWrites().At(0); ok {
		t.Fatal("empty write plane accepted an out-of-range candidate")
	}
	var zero TransformerInput
	if zero.IndexReads().Count() != 0 || zero.IndexWrites().Count() != 0 {
		t.Fatal("zero TransformerInput exposed access candidates")
	}
}

func TestTransformerAccessRolesRejectMixedProgramSubproofs(t *testing.T) {
	leftPublished, err := Publish(rootAssembly(t, "transformer-access-mixed-left.lua"))
	if err != nil {
		t.Fatalf("Publish left: %v", err)
	}
	rightPublished, err := Publish(rootAssembly(t, "transformer-access-mixed-right.lua"))
	if err != nil {
		t.Fatalf("Publish right: %v", err)
	}
	left, right := leftPublished.TransformerInput(), rightPublished.TransformerInput()
	readTerm := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	read := IndexRead{
		input: left, read: readTerm,
		base:   IndexBase{input: right, access: readTerm, term: readTerm},
		lens:   IndexLens{input: right, access: readTerm, term: readTerm, keyTerm: readTerm},
		result: IndexResult{input: right, access: readTerm, term: readTerm},
	}
	if left.OwnsIndexRead(read) {
		t.Fatal("mixed read subproofs crossed Program ownership")
	}

	writeTerm := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	write := IndexWrite{
		input: left, write: writeTerm,
		base:   IndexBase{input: right, access: writeTerm, term: writeTerm},
		lens:   IndexLens{input: right, access: writeTerm, term: writeTerm, keyTerm: writeTerm},
		values: IndexValues{input: right, access: writeTerm, term: writeTerm, position: 0},
	}
	if left.OwnsIndexWrite(write) {
		t.Fatal("mixed write subproofs crossed Program ownership")
	}
}

func TestTransformerAccessRolesRejectSameOwnerSwappedSubproofs(t *testing.T) {
	published, err := Publish(rootAssembly(t, "transformer-access-swapped.lua"))
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	input := published.TransformerInput()
	first := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	second := keyspace.MakeTerm(keyspace.FamilyBody, 2)

	read := IndexRead{
		input: input, read: first,
		base:   IndexBase{input: input, access: second, term: first},
		lens:   IndexLens{input: input, access: second, term: first, keyTerm: first},
		result: IndexResult{input: input, access: first, term: first},
	}
	if read.exactlyComposedBy(input) {
		t.Fatal("same-owner read accepted subproofs from a different access row")
	}

	write := IndexWrite{
		input: input, write: first,
		base:   IndexBase{input: input, access: second, term: first},
		lens:   IndexLens{input: input, access: second, term: first, keyTerm: first},
		values: IndexValues{input: input, access: second, term: first, position: 0},
	}
	if write.exactlyComposedBy(input) {
		t.Fatal("same-owner write accepted subproofs from a different access row")
	}
}

func TestTransformerAccessRolesRejectForeignEquivalentNestedSpan(t *testing.T) {
	leftPublished, err := Publish(rootAssembly(t, "transformer-access-equivalent-span.lua"))
	if err != nil {
		t.Fatalf("Publish left: %v", err)
	}
	rightPublished, err := Publish(rootAssembly(t, "transformer-access-equivalent-span.lua"))
	if err != nil {
		t.Fatalf("Publish right: %v", err)
	}
	left, right := leftPublished.TransformerInput(), rightPublished.TransformerInput()
	term := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	local, localOK := left.Span(term)
	foreign, foreignOK := right.Span(term)
	if !localOK || !foreignOK || !local.Equal(foreign) {
		t.Fatal("equivalent replay did not produce equivalent nested spans")
	}
	if exactOptionalSpan(left, foreign, term) || exactSpan(left, foreign, term) {
		t.Fatal("foreign equivalent nested span crossed exact ownership")
	}
}
