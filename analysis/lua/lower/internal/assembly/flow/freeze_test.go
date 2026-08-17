package flow

import (
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programsource "github.com/wippyai/go-lua/analysis/program/source"
	"testing"
)

func TestRowsFreezeRequiresAnOwnedSourceIdentity(t *testing.T) {
	var rows Rows
	if _, err := rows.Freeze(programsource.Preimage{}, [keyspace.FamilyCount]uint32{}); err == nil {
		t.Fatal("Freeze accepted an unavailable Source preimage")
	}
}
