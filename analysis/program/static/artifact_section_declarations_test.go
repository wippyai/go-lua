package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestArtifactDeclarationsDecoderRetainsTypedRowsAndMembers(t *testing.T) {
	decoded := decodeStaticArtifactInputForTest(t, declarationFixture(t))
	if len(decoded.Declarations.Alias) != 1 || len(decoded.Declarations.TypeParam) != 1 || len(decoded.Declarations.Interface) != 1 {
		t.Fatalf("decoded declaration counts = aliases:%d params:%d interfaces:%d",
			len(decoded.Declarations.Alias), len(decoded.Declarations.TypeParam), len(decoded.Declarations.Interface))
	}
	alias := decoded.Declarations.Alias[0]
	if alias.Owner != keyspace.MakeTerm(keyspace.FamilyBody, 1) ||
		alias.Target != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1) ||
		len(alias.Params) != 1 || alias.Params[0] != keyspace.MakeTerm(keyspace.FamilyTypeParam, 1) {
		t.Fatalf("decoded alias = %+v", alias)
	}
	if got := decoded.Declarations.Interface[0].Members; len(got) != 2 ||
		got[0].Kind != InterfaceField || got[0].Field != keyspace.MakeTerm(keyspace.FamilyTypeField, 1) ||
		got[1].Kind != InterfaceMethod || got[1].Signature != keyspace.MakeTerm(keyspace.FamilyTypeFunction, 1) {
		t.Fatalf("decoded interface members = %+v", got)
	}
}
