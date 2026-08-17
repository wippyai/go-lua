package assembly

import (
	"math"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestAssemblySourceMintRejectsReservedFamiliesAndInvalidExactValues(t *testing.T) {
	c := newAssemblyCollector()
	if got := c.mint(keyspace.FamilyOutcome, assemblyTestSpan()); got != 0 || c.err == nil {
		t.Fatal("mint accepted the derived Outcome family")
	}
	if validRawExactCandidate(keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(math.NaN())}) {
		t.Fatal("NaN was accepted as an exact-key candidate")
	}
}
