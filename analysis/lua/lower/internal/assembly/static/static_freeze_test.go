package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programsource "github.com/wippyai/go-lua/analysis/program/source"
)

func TestStaticFreezeRejectsMissingPreimageAndMismatchedCounts(t *testing.T) {
	rows := &staticRows{}
	if err := staticRowsCountsMatch(rows, [keyspace.FamilyCount]uint32{}); err != nil {
		t.Fatal(err)
	}
	if _, err := rows.freeze(programsource.Preimage{}, [keyspace.FamilyCount]uint32{}); err == nil {
		t.Fatal("freeze accepted a missing Source preimage")
	}
	if _, err := resolveStaticPath(programsource.Keys{}, nil); err == nil {
		t.Fatal("resolveStaticPath accepted an empty path")
	}
}
