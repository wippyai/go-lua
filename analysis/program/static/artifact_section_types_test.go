package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestArtifactTypesDecoderRetainsTypedForestRows(t *testing.T) {
	decoded := decodeStaticArtifactInputForTest(t, staticTypeDenominatorInput(t))
	if len(decoded.Types.Primitive) != 20 || len(decoded.Types.Literal) != 1 ||
		len(decoded.Types.Optional) != 1 || len(decoded.Types.Union) != 1 ||
		len(decoded.Types.Intersection) != 1 || len(decoded.Types.Generic) != 1 ||
		len(decoded.Types.Array) != 1 || len(decoded.Types.Map) != 1 ||
		len(decoded.Types.Record) != 1 || len(decoded.Types.Field) != 1 {
		t.Fatalf("decoded type counts = primitive:%d literal:%d optional:%d union:%d intersection:%d generic:%d array:%d map:%d record:%d field:%d",
			len(decoded.Types.Primitive), len(decoded.Types.Literal), len(decoded.Types.Optional),
			len(decoded.Types.Union), len(decoded.Types.Intersection), len(decoded.Types.Generic),
			len(decoded.Types.Array), len(decoded.Types.Map), len(decoded.Types.Record), len(decoded.Types.Field))
	}
	if decoded.Types.Union[0].Members[0] != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 2) ||
		decoded.Types.Generic[0].Base != keyspace.MakeTerm(keyspace.FamilyTypeRef, 1) ||
		decoded.Types.Array[0].Element != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 7) ||
		decoded.Types.Array[0].ReadOnly || decoded.Types.Record[0].ReadOnly {
		t.Fatalf("decoded typed forest rows = unions:%+v generic:%+v array:%+v record:%+v",
			decoded.Types.Union, decoded.Types.Generic, decoded.Types.Array, decoded.Types.Record)
	}
}
