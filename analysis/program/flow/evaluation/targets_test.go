package evaluation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestPortsLocalCellWriteOwnershipMatrix(t *testing.T) {
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	body3 := keyspace.MakeTerm(keyspace.FamilyBody, 3)
	body4 := keyspace.MakeTerm(keyspace.FamilyBody, 4)
	body5 := keyspace.MakeTerm(keyspace.FamilyBody, 5)
	for _, test := range []struct {
		name       string
		owner      keyspace.Term
		wantReject bool
	}{
		{name: "same-body-loop-body", owner: body3},
		{name: "descendant-body", owner: body4},
		{name: "sibling-body", owner: body5, wantReject: true},
		{name: "enclosing-body", owner: body2, wantReject: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := openLocalCellWriteFixture(t, test.owner)
			defer fixture.close()
			ports, err := SealPorts(fixture.identity, fixture.view, fixture.forest,
				fixture.staticView.ContentID(), fixture.moduleFinalize.View().ContentID())
			if test.wantReject {
				if err == nil {
					t.Fatal("SealPorts accepted a local Cell write across the ownership frontier")
				}
				return
			}
			if err != nil {
				t.Fatalf("SealPorts rejected a lexically valid local Cell write: %v", err)
			}
			if got, ok := ports.Finish(keyspace.MakeTerm(keyspace.FamilyAssign, 1)); !ok || got != keyspace.MakeTerm(keyspace.FamilyWrite, 1) {
				t.Fatalf("Finish(assign) = %v/%v, want Write1", got, ok)
			}
		})
	}
}
