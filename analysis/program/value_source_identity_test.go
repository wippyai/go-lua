package program

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestValueSourceIdentityUsesClosedFamilyAdmission(t *testing.T) {
	published, err := Publish(rootAssembly(t, "program-value-source-law.lua"))
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	for _, family := range []keyspace.Family{keyspace.FamilyNil, keyspace.FamilyBool, keyspace.FamilyInteger, keyspace.FamilyFloat, keyspace.FamilyString, keyspace.FamilyTypeValue, keyspace.FamilyInvalid} {
		if count := published.ValueSourceCount(family); count != 0 {
			t.Fatalf("ValueSourceCount(%v) = %d; want empty source column", family, count)
		}
		if source, span, term, ok := published.ValueSourceIDAt(family, 0); ok || source.Available() || span.Available() || term != 0 {
			t.Fatalf("ValueSourceIDAt(%v,0) = %x/%x/%v/%v; want unavailable", family, source, span, term, ok)
		}
	}
	if source, span, term, ok := published.ValueSourceIDAt(keyspace.FamilyNil, -1); ok || source.Available() || span.Available() || term != 0 {
		t.Fatalf("ValueSourceIDAt(nil,-1) = %x/%x/%v/%v; want unavailable", source, span, term, ok)
	}
}
