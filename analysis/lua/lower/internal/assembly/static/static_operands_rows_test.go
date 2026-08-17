package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestStaticOperandRowsDistinguishClaimDeclarationAndFill(t *testing.T) {
	rows := &staticRows{}
	claim := staticTestTerm(keyspace.FamilyValueClaim, 1)
	if err := rows.ClaimDeclare(claim, claim); err != nil {
		t.Fatal(err)
	}
	if err := rows.FillClaimTarget(claim, claim, staticTestTerm(keyspace.FamilyTypePrimitive, 1)); err != nil {
		t.Fatal(err)
	}
	if err := rows.FillClaimTarget(claim, claim, staticTestTerm(keyspace.FamilyTypePrimitive, 1)); err == nil {
		t.Fatal("FillClaimTarget accepted a duplicate target")
	}
}
