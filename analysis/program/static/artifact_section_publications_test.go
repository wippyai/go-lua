package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestArtifactPublicationsDecoderRetainsAssignPairAndTarget(t *testing.T) {
	decoded := decodeStaticArtifactInputForTest(t, publicationFixture(t))
	if len(decoded.Publications.Type) != 1 {
		t.Fatalf("decoded publication count = %d, want 1", len(decoded.Publications.Type))
	}
	row := decoded.Publications.Type[0]
	if row.Assign != keyspace.MakeTerm(keyspace.FamilyAssign, 1) || row.Pair != 0 ||
		row.Target != keyspace.MakeTerm(keyspace.FamilyTypeRef, 1) {
		t.Fatalf("decoded publication row = %+v", row)
	}
}
