package flow

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestBuildAdmitsAuthoredRowsButRejectsDerivedFamilies(t *testing.T) {
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyBody] = 1
	if draft, err := Build(authored.Input{Counts: counts}); err != nil || draft == nil {
		t.Fatalf("Build(authored Body) = %#v/%v, want a live Draft", draft, err)
	}
	counts[keyspace.FamilyOutcome] = 1
	if draft, err := Build(authored.Input{Counts: counts}); err == nil || draft != nil {
		t.Fatalf("Build(derived Outcome) = %#v/%v, want fail-closed rejection", draft, err)
	}
}
