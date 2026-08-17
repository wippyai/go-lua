package evaluation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestSessionScalarFieldExactIsMetadata(t *testing.T) {
	fixture := pendingTableOrderFixture(t, "session-table-scalar-exact.lua")
	walker, err := New(fixture.flowView)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	scalar := pendingTerm(keyspace.FamilyInteger, 1)
	if !walker.staticLensSource(scalar, kind.FieldExact) || walker.runtimeFieldOperand(scalar, kind.FieldExact) {
		t.Fatal("ordinary Session did not classify scalar FieldExact as static metadata")
	}
	if err := walker.Start(pendingTerm(keyspace.FamilyTable, 1)); err != nil {
		t.Fatalf("Session.Start(Table1): %v", err)
	}
	for {
		if _, ok, nextErr := walker.Next(); nextErr != nil {
			t.Fatalf("Session.Next(Table1): %v", nextErr)
		} else if !ok {
			break
		}
	}
}
