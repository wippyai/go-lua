package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestStaticDeclarationRowsFillAliasIdentityExactlyOnce(t *testing.T) {
	rows := &staticRows{}
	alias := staticTestTerm(keyspace.FamilyTypeAlias, 1)
	body := staticTestTerm(keyspace.FamilyBody, 1)
	if err := rows.AliasDeclare(alias, body, "Alias", staticTestCoordinate()); err != nil {
		t.Fatal(err)
	}
	if err := rows.AliasParams(alias, nil); err != nil || rows.AliasTarget(alias, staticTestTerm(keyspace.FamilyTypePrimitive, 1)) != nil {
		t.Fatalf("alias fills failed: %v", err)
	}
	if err := rows.AliasTarget(alias, staticTestTerm(keyspace.FamilyTypePrimitive, 1)); err == nil {
		t.Fatal("AliasTarget accepted a duplicate fill")
	}
}
