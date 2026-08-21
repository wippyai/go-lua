package module

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func moduleCounts(imports, calls, cells uint32) [keyspace.FamilyCount]uint32 {
	var counts [keyspace.FamilyCount]uint32
	counts[keyspace.FamilyImport] = imports
	counts[keyspace.FamilyCall] = calls
	counts[keyspace.FamilyCell] = cells
	counts[keyspace.FamilyString] = imports
	return counts
}

func TestRowsUseCensusSlotsRatherThanVisitOrder(t *testing.T) {
	rows := New(3)
	call1 := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	call2 := keyspace.MakeTerm(keyspace.FamilyCall, 2)
	call3 := keyspace.MakeTerm(keyspace.FamilyCall, 3)
	if err := rows.Set(2, keyspace.MakeTerm(keyspace.FamilyImport, 3), call3, keyspace.MakeTerm(keyspace.FamilyString, 3)); err != nil {
		t.Fatal(err)
	}
	if err := rows.Set(0, keyspace.MakeTerm(keyspace.FamilyImport, 1), call1, keyspace.MakeTerm(keyspace.FamilyString, 1)); err != nil {
		t.Fatal(err)
	}
	if err := rows.Set(1, keyspace.MakeTerm(keyspace.FamilyImport, 2), call2, keyspace.MakeTerm(keyspace.FamilyString, 2)); err != nil {
		t.Fatal(err)
	}
	input, err := rows.Freeze(moduleCounts(3, 3, 0))
	if err != nil {
		t.Fatal(err)
	}
	for index, row := range input {
		wantTerm := keyspace.MakeTerm(keyspace.FamilyImport, uint32(index+1))
		wantCall := keyspace.MakeTerm(keyspace.FamilyCall, uint32(index+1))
		if row.Term != wantTerm || row.Call != wantCall || row.Request != keyspace.MakeTerm(keyspace.FamilyString, uint32(index+1)) || row.Alias != 0 {
			t.Fatalf("Import[%d] = %#v, want census order", index, row)
		}
	}
}

func TestRowsRejectDuplicateMissingAndOutOfRangeSlots(t *testing.T) {
	rows := New(2)
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	if err := rows.Set(0, keyspace.MakeTerm(keyspace.FamilyImport, 1), call, keyspace.MakeTerm(keyspace.FamilyString, 1)); err != nil {
		t.Fatal(err)
	}
	if err := rows.Set(0, keyspace.MakeTerm(keyspace.FamilyImport, 1), call, keyspace.MakeTerm(keyspace.FamilyString, 1)); err == nil {
		t.Fatal("duplicate slot accepted")
	}
	if err := rows.Set(-1, keyspace.MakeTerm(keyspace.FamilyImport, 1), call, keyspace.MakeTerm(keyspace.FamilyString, 1)); err == nil {
		t.Fatal("negative slot accepted")
	}
	if err := rows.Set(2, keyspace.MakeTerm(keyspace.FamilyImport, 3), call, keyspace.MakeTerm(keyspace.FamilyString, 1)); err == nil {
		t.Fatal("past-end slot accepted")
	}
	if _, err := rows.Freeze(moduleCounts(2, 1, 0)); err == nil {
		t.Fatal("incomplete module freeze accepted")
	}
}

func TestRowsAliasIsSingleWriteAndZeroIsAuthoredAbsence(t *testing.T) {
	rows := New(1)
	term := keyspace.MakeTerm(keyspace.FamilyImport, 1)
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	request := keyspace.MakeTerm(keyspace.FamilyString, 1)
	if err := rows.Set(0, term, call, request); err != nil || rows.SetAlias(0, 0) != nil || rows.SetAlias(0, 0) == nil {
		t.Fatal("zero alias was not a one-shot authored absence")
	}
	missing := New(1)
	if err := missing.SetAlias(0, keyspace.MakeTerm(keyspace.FamilyCell, 1)); err == nil {
		t.Fatal("alias before Import was accepted")
	}
	invalid := New(1)
	if err := invalid.Set(0, term, call, request); err != nil {
		t.Fatal(err)
	}
	if err := invalid.SetAlias(0, keyspace.MakeTerm(keyspace.FamilyFunction, 1)); err == nil {
		t.Fatal("invalid alias family was accepted")
	}
}

func TestRowsFreezeOwnsFreshInputAndExcludesDerivedKey(t *testing.T) {
	rows := New(1)
	term := keyspace.MakeTerm(keyspace.FamilyImport, 1)
	if err := rows.Set(0, term, keyspace.MakeTerm(keyspace.FamilyCall, 1), keyspace.MakeTerm(keyspace.FamilyString, 1)); err != nil {
		t.Fatal(err)
	}
	if err := rows.SetAlias(0, keyspace.MakeTerm(keyspace.FamilyCell, 1)); err != nil {
		t.Fatal(err)
	}
	counts := moduleCounts(1, 1, 1)
	first, err := rows.Freeze(counts)
	if err != nil {
		t.Fatal(err)
	}
	first[0].Alias = 0
	second, err := rows.Freeze(counts)
	if err != nil {
		t.Fatal(err)
	}
	if second[0].Alias != keyspace.MakeTerm(keyspace.FamilyCell, 1) {
		t.Fatalf("fresh freeze retained caller mutation: %#v", second[0])
	}
}

func TestRowsRejectCountAndForeignKeyMismatches(t *testing.T) {
	rows := New(1)
	if err := rows.Set(0, keyspace.MakeTerm(keyspace.FamilyImport, 1), keyspace.MakeTerm(keyspace.FamilyCall, 2), keyspace.MakeTerm(keyspace.FamilyString, 1)); err != nil {
		t.Fatal(err)
	}
	if _, err := rows.Freeze(moduleCounts(0, 2, 0)); err == nil {
		t.Fatal("Import count mismatch accepted")
	}
	malformed := New(1)
	if err := malformed.Set(0, keyspace.MakeTerm(keyspace.FamilyImport, 1), keyspace.MakeTerm(keyspace.FamilyFunction, 1), keyspace.MakeTerm(keyspace.FamilyString, 1)); err == nil {
		t.Fatal("non-Call authored relation accepted")
	}
}
