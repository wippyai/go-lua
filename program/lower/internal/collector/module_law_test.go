package collector

import (
	"errors"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

func moduleCounts(imports, calls, cells uint32) [keyspace.FamilyCount]uint32 {
	var counts [keyspace.FamilyCount]uint32
	counts[keyspace.FamilyImport] = imports
	counts[keyspace.FamilyCall] = calls
	counts[keyspace.FamilyCell] = cells
	counts[keyspace.FamilyString] = imports
	return counts
}

func TestModuleRowsUseCensusSlotsRatherThanVisitOrder(t *testing.T) {
	var rows moduleRows
	rows.init(3)
	call1 := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	call2 := keyspace.MakeTerm(keyspace.FamilyCall, 2)
	call3 := keyspace.MakeTerm(keyspace.FamilyCall, 3)

	// Visit order is deliberately reversed. The canonical Import identity is
	// still its declared census slot, not the order in which observations land.
	if got, err := rows.declare(2, call3, keyspace.MakeTerm(keyspace.FamilyString, 3)); err != nil || got != keyspace.MakeTerm(keyspace.FamilyImport, 3) {
		t.Fatalf("declare slot 2 = %v/%v", got, err)
	}
	if got, err := rows.declare(0, call1, keyspace.MakeTerm(keyspace.FamilyString, 1)); err != nil || got != keyspace.MakeTerm(keyspace.FamilyImport, 1) {
		t.Fatalf("declare slot 0 = %v/%v", got, err)
	}
	if got, err := rows.declare(1, call2, keyspace.MakeTerm(keyspace.FamilyString, 2)); err != nil || got != keyspace.MakeTerm(keyspace.FamilyImport, 2) {
		t.Fatalf("declare slot 1 = %v/%v", got, err)
	}

	input, err := rows.freeze(moduleCounts(3, 3, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(input.Imports) != 3 {
		t.Fatalf("Import rows = %d, want 3", len(input.Imports))
	}
	for index, row := range input.Imports {
		wantTerm := keyspace.MakeTerm(keyspace.FamilyImport, uint32(index+1))
		wantCall := keyspace.MakeTerm(keyspace.FamilyCall, uint32(index+1))
		if row.Term != wantTerm || row.Call != wantCall || row.Alias != 0 || row.Request != keyspace.MakeTerm(keyspace.FamilyString, uint32(index+1)) || row.Key != 0 {
			t.Fatalf("Import[%d] = %#v, want authored term/call and zero derived fields", index, row)
		}
	}
}

func TestModuleRowsRejectDuplicateMissingAndOutOfRangeSlots(t *testing.T) {
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	var rows moduleRows
	rows.init(2)
	if _, err := rows.declare(0, call, keyspace.MakeTerm(keyspace.FamilyString, 1)); err != nil {
		t.Fatal(err)
	}
	if _, err := rows.declare(0, call, keyspace.MakeTerm(keyspace.FamilyString, 1)); !errors.Is(err, errModuleSlotDuplicate) {
		t.Fatalf("duplicate slot error = %v, want %v", err, errModuleSlotDuplicate)
	}
	if _, err := rows.declare(-1, call, keyspace.MakeTerm(keyspace.FamilyString, 1)); !errors.Is(err, errModuleSlotRange) {
		t.Fatalf("negative slot error = %v, want %v", err, errModuleSlotRange)
	}
	if _, err := rows.declare(2, call, keyspace.MakeTerm(keyspace.FamilyString, 1)); !errors.Is(err, errModuleSlotRange) {
		t.Fatalf("past-end slot error = %v, want %v", err, errModuleSlotRange)
	}
	if _, err := rows.freeze(moduleCounts(2, 1, 0)); !errors.Is(err, errModuleSlotMissing) {
		t.Fatalf("incomplete freeze error = %v, want %v", err, errModuleSlotMissing)
	}
}

func TestModuleRowsAliasIsSingleWriteAndZeroIsAuthoredAbsence(t *testing.T) {
	c := New("module-alias.lua", 2, bind.GlobalCensus{})
	c.counts[keyspace.FamilyCall] = 1
	c.counts[keyspace.FamilyCell] = 1
	importTerm, err := c.module.declare(1, keyspace.MakeTerm(keyspace.FamilyCall, 1), keyspace.MakeTerm(keyspace.FamilyString, 1))
	if err != nil {
		t.Fatal(err)
	}
	if !c.Module().SetImportAlias(importTerm, 0) {
		t.Fatalf("explicit zero alias failed: %v", c.err)
	}
	if c.Module().SetImportAlias(importTerm, 0) || !errors.Is(c.err, errModuleAliasSet) {
		t.Fatalf("second zero alias error = %v, want %v", c.err, errModuleAliasSet)
	}

	missing := New("module-missing.lua", 1, bind.GlobalCensus{})
	missing.counts[keyspace.FamilyCell] = 1
	if missing.Module().SetImportAlias(keyspace.MakeTerm(keyspace.FamilyImport, 1), keyspace.MakeTerm(keyspace.FamilyCell, 1)) ||
		!errors.Is(missing.err, errModuleSlotMissing) {
		t.Fatalf("alias before Import error = %v, want %v", missing.err, errModuleSlotMissing)
	}

	invalid := New("module-invalid.lua", 1, bind.GlobalCensus{})
	invalid.counts[keyspace.FamilyCall] = 1
	invalidTerm, err := invalid.module.declare(0, keyspace.MakeTerm(keyspace.FamilyCall, 1), keyspace.MakeTerm(keyspace.FamilyString, 1))
	if err != nil {
		t.Fatal(err)
	}
	if invalid.Module().SetImportAlias(invalidTerm, keyspace.MakeTerm(keyspace.FamilyFunction, 1)) ||
		!errors.Is(invalid.err, errModuleAliasInvalid) {
		t.Fatal("invalid alias family unexpectedly accepted")
	}

	var rows moduleRows
	rows.init(2)
	if _, err := rows.declare(1, keyspace.MakeTerm(keyspace.FamilyCall, 1), keyspace.MakeTerm(keyspace.FamilyString, 1)); err != nil {
		t.Fatal(err)
	}
	if _, err := rows.freeze(moduleCounts(2, 1, 1)); !errors.Is(err, errModuleSlotMissing) {
		t.Fatalf("unfilled second Import error = %v, want %v", err, errModuleSlotMissing)
	}
}

func TestModuleRowsFreezeOwnsFreshInputAndExcludesDerivedKey(t *testing.T) {
	var rows moduleRows
	rows.init(1)
	if _, err := rows.declare(0, keyspace.MakeTerm(keyspace.FamilyCall, 1), keyspace.MakeTerm(keyspace.FamilyString, 1)); err != nil {
		t.Fatal(err)
	}
	rows.imports[0].Alias = keyspace.MakeTerm(keyspace.FamilyCell, 1)
	rows.aliases[0] = true
	counts := moduleCounts(1, 1, 1)
	first, err := rows.freeze(counts)
	if err != nil {
		t.Fatal(err)
	}
	first.Imports[0].Alias = 0
	first.Imports[0].Request = keyspace.MakeTerm(keyspace.FamilyString, 1)
	second, err := rows.freeze(counts)
	if err != nil {
		t.Fatal(err)
	}
	want := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	if second.Imports[0].Alias != want || second.Imports[0].Request != keyspace.MakeTerm(keyspace.FamilyString, 1) || second.Imports[0].Key != 0 {
		t.Fatalf("fresh freeze retained caller mutation: %#v", second.Imports[0])
	}
	if reflect.DeepEqual(first.Imports, second.Imports) {
		// The slices were deliberately mutated above; equality here would prove
		// that freeze accidentally returned shared storage.
		t.Fatal("fresh freeze unexpectedly shares caller-owned row state")
	}
}

func TestModuleRootRejectsFutureCellAliasAtOwnerBoundary(t *testing.T) {
	c := New("module-future-cell.lua", 1, bind.GlobalCensus{})
	c.counts[keyspace.FamilyCall] = 1
	c.counts[keyspace.FamilyCell] = 1
	importTerm, err := c.module.declare(0, keyspace.MakeTerm(keyspace.FamilyCall, 1), keyspace.MakeTerm(keyspace.FamilyString, 1))
	if err != nil {
		t.Fatal(err)
	}
	future := keyspace.MakeTerm(keyspace.FamilyCell, 2)
	if c.Module().SetImportAlias(importTerm, future) {
		t.Fatal("future Cell alias unexpectedly accepted")
	}
	if !errors.Is(c.err, errModuleAliasInvalid) {
		t.Fatalf("future Cell alias error = %v, want %v", c.err, errModuleAliasInvalid)
	}
	if len(c.module.imports) != 0 || len(c.module.aliases) != 0 {
		t.Fatalf("terminal rejection retained Module scratch: imports=%d aliases=%d", len(c.module.imports), len(c.module.aliases))
	}
}

func TestModuleRootReservedImportCannotPopulateAnotherOrdinal(t *testing.T) {
	c := New("module-reserved.lua", 2, bind.GlobalCensus{})
	c.counts[keyspace.FamilyCell] = 1
	reserved := keyspace.MakeTerm(keyspace.FamilyImport, 2)
	alias := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	if c.Module().SetImportAlias(reserved, alias) {
		t.Fatal("unfilled reserved Import accepted an alias")
	}
	if !errors.Is(c.err, errModuleSlotMissing) {
		t.Fatalf("unfilled reserved Import error = %v, want %v", c.err, errModuleSlotMissing)
	}
	if len(c.module.imports) != 0 || len(c.module.aliases) != 0 {
		t.Fatalf("terminal rejection retained Module scratch: imports=%d aliases=%d", len(c.module.imports), len(c.module.aliases))
	}

	outside := New("module-outside.lua", 1, bind.GlobalCensus{})
	outside.counts[keyspace.FamilyCell] = 1
	if outside.Module().SetImportAlias(keyspace.MakeTerm(keyspace.FamilyImport, 2), alias) {
		t.Fatal("Import beyond the reserved census accepted an alias")
	}
	if !errors.Is(outside.err, errModuleAliasInvalid) {
		t.Fatalf("outside Import error = %v, want %v", outside.err, errModuleAliasInvalid)
	}
	if len(outside.module.imports) != 0 || len(outside.module.aliases) != 0 {
		t.Fatalf("terminal rejection retained outside Module scratch: imports=%d aliases=%d", len(outside.module.imports), len(outside.module.aliases))
	}
}

func TestModuleRowsRejectCountAndForeignKeyMismatches(t *testing.T) {
	var rows moduleRows
	rows.init(1)
	if _, err := rows.declare(0, keyspace.MakeTerm(keyspace.FamilyCall, 2), keyspace.MakeTerm(keyspace.FamilyString, 1)); err != nil {
		t.Fatal(err)
	}
	if _, err := rows.freeze(moduleCounts(0, 2, 0)); !errors.Is(err, errModuleCounts) {
		t.Fatalf("Import count mismatch = %v, want %v", err, errModuleCounts)
	}
	if _, err := rows.freeze(moduleCounts(1, 1, 0)); err == nil {
		t.Fatal("out-of-range Call unexpectedly accepted")
	}

	var malformed moduleRows
	malformed.init(1)
	if err := malformed.set(0, keyspace.MakeTerm(keyspace.FamilyImport, 1), keyspace.MakeTerm(keyspace.FamilyFunction, 1), keyspace.MakeTerm(keyspace.FamilyString, 1)); err == nil {
		t.Fatal("non-Call authored relation unexpectedly accepted")
	}
	if err := malformed.set(0, keyspace.MakeTerm(keyspace.FamilyImport, 2), keyspace.MakeTerm(keyspace.FamilyCall, 1), keyspace.MakeTerm(keyspace.FamilyString, 1)); err == nil {
		t.Fatal("noncanonical Import term unexpectedly accepted")
	}
}

func TestCollectorModuleObservationFillsReservedSlotsOutOfOrder(t *testing.T) {
	c := New("module.lua", 3, bind.GlobalCensus{})
	span := func(line uint32) source.Span {
		return source.Span{File: "module.lua", StartLine: line, StartCol: 1, EndLine: line, EndCol: 8}
	}
	body := c.Source().Order().Body(span(1))
	makeCall := func(line uint32) keyspace.Term {
		request := c.Source().Literals().String(span(line), body, "pkg")
		values := c.Flow().Values().Values(span(line), body, []Term{request}, 0)
		return c.Flow().Calls().DeclareCall(span(line), body, request, 0, values)
	}
	call1 := makeCall(10)
	call2 := makeCall(20)
	call3 := makeCall(30)
	if got := c.Module().Import(2, span(30), call3); got != keyspace.MakeTerm(keyspace.FamilyImport, 3) {
		t.Fatalf("reserved Import slot 2 = %v", got)
	}
	if got := c.Module().Import(0, span(10), call1); got != keyspace.MakeTerm(keyspace.FamilyImport, 1) {
		t.Fatalf("reserved Import slot 0 = %v", got)
	}
	if got := c.Module().Import(1, span(20), call2); got != keyspace.MakeTerm(keyspace.FamilyImport, 2) {
		t.Fatalf("reserved Import slot 1 = %v", got)
	}
	if c.spans[keyspace.FamilyImport][0].StartLine != 10 || c.spans[keyspace.FamilyImport][2].StartLine != 30 {
		t.Fatalf("reserved Import spans lost census slot identity: %#v", c.spans[keyspace.FamilyImport])
	}
	if got := c.Module().Import(0, span(11), call1); got != 0 || c.err == nil {
		t.Fatal("duplicate reserved Import was not rejected and collector was not poisoned")
	}
}

func TestModuleRootRejectsEmptyStringRequestBeforeExactAdmission(t *testing.T) {
	const name = "module-empty-request.lua"
	c := New(name, 1, bind.GlobalCensus{})
	span := source.Span{File: name, StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 8}
	body := c.Source().Order().Body(span)
	request := c.Source().Literals().String(span, body, "")
	values := c.Flow().Values().Values(span, body, []Term{request}, 0)
	call := c.Flow().Calls().DeclareCall(span, body, request, 0, values)
	if body == 0 || request == 0 || values == 0 || call == 0 {
		t.Fatalf("empty request construction failed: %v", failure(c))
	}
	if got := c.Module().Import(0, span, call); got != 0 {
		t.Fatalf("empty Module Import = %v, want rejection", got)
	}
	if c.err == nil || c.err != errModuleRequestEmpty {
		t.Fatalf("empty Module Import error = %v, want %v", c.err, errModuleRequestEmpty)
	}
	if len(c.source.exact) != 0 {
		t.Fatalf("empty request entered the exact-key denominator: %#v", c.source.exact)
	}
}
