package program

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestValuesIdentityQueriesDoNotCreateRowsForUnknownTerms(t *testing.T) {
	published, err := Publish(rootAssembly(t, "program-values-identity-law.lua"))
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	unknown := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	if id, ok := published.ValueSubjectID(unknown); ok || id.Available() {
		t.Fatalf("ValueSubjectID(unknown) = %x/%v; want unavailable", id, ok)
	}
	if id, ok := published.ValuesOccurrenceID(unknown); ok || id.Available() {
		t.Fatalf("ValuesOccurrenceID(unknown) = %x/%v; want unavailable", id, ok)
	}
	if id, ok := published.ValuesMemberID(unknown, 0); ok || id.Available() {
		t.Fatalf("ValuesMemberID(unknown,0) = %x/%v; want unavailable", id, ok)
	}
	if id, ok := published.ValuesTailID(unknown); ok || id.Available() {
		t.Fatalf("ValuesTailID(unknown) = %x/%v; want unavailable", id, ok)
	}
	if id, ok := published.ValuesMemberID(unknown, -1); ok || id.Available() {
		t.Fatalf("ValuesMemberID(unknown,-1) = %x/%v; want unavailable", id, ok)
	}
}
