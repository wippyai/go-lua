package flow

import (
	programflow "github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"testing"
)

func TestRowsAdmissionRequiresDenseTermsAndRetainsValueRange(t *testing.T) {
	rows := Rows{}
	var counts [keyspace.FamilyCount]uint32
	counts[keyspace.FamilyBody] = 1
	counts[keyspace.FamilyValues] = 1
	counts[keyspace.FamilyInteger] = 1
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	value := keyspace.MakeTerm(keyspace.FamilyInteger, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	if err := rows.AdmitValues(counts, values, body, []keyspace.Term{value}, 0); err != nil {
		t.Fatalf("AdmitValues: %v", err)
	}
	if err := rows.AdmitValues(counts, keyspace.MakeTerm(keyspace.FamilyValues, 1), body, nil, 0); err == nil {
		t.Fatal("duplicate Values term was admitted")
	}
	row, ok := rows.ValueAt(0)
	if !ok || row.Owner != body || row.Fixed != (programflow.Range{Start: 0, End: 1}) {
		t.Fatalf("stored Values row = %#v/%v", row, ok)
	}
}
