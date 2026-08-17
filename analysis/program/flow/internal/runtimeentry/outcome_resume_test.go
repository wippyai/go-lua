package runtimeentry

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func TestNormalizeOutcomeResumeRejectsUnavailableAndForeignInputs(t *testing.T) {
	var result Result
	if row, err := result.NormalizeOutcomeResume(source.View{}, nil, nil, keyspace.MakeTerm(keyspace.FamilyOutcome, 1)); err == nil || row.Available() {
		t.Fatalf("NormalizeOutcomeResume accepted unavailable inputs: row=%v err=%v", row, err)
	}
}
