package assembly

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func TestModuleRootRejectsFutureCellAliasAtOwnerBoundary(t *testing.T) {
	c := New("module-future-cell.lua", 1, bind.GlobalCensus{})
	future := keyspace.MakeTerm(keyspace.FamilyCell, 2)
	if c.SetImportAlias(keyspace.MakeTerm(keyspace.FamilyImport, 1), future) {
		t.Fatal("future Cell alias unexpectedly accepted")
	}
	if c.Body(source.Span{}) != 0 {
		t.Fatal("future alias rejection did not terminalize Collector")
	}
}

func TestModuleRootReservedImportCannotPopulateAnotherOrdinal(t *testing.T) {
	c := New("module-reserved.lua", 2, bind.GlobalCensus{})
	if c.SetImportAlias(keyspace.MakeTerm(keyspace.FamilyImport, 2), keyspace.MakeTerm(keyspace.FamilyCell, 1)) {
		t.Fatal("unfilled reserved Import accepted an alias")
	}
	if c.Body(source.Span{}) != 0 {
		t.Fatal("reserved Import rejection did not terminalize Collector")
	}
	outside := New("module-outside.lua", 1, bind.GlobalCensus{})
	if outside.SetImportAlias(keyspace.MakeTerm(keyspace.FamilyImport, 2), keyspace.MakeTerm(keyspace.FamilyCell, 1)) {
		t.Fatal("Import beyond census accepted an alias")
	}
}

func TestCollectorModuleObservationFillsReservedSlotsOutOfOrder(t *testing.T) {
	c := New("module.lua", 3, bind.GlobalCensus{})
	span := func(line uint32) source.Span {
		return source.Span{File: "module.lua", StartLine: line, StartCol: 1, EndLine: line, EndCol: 8}
	}
	body := c.Body(span(1))
	makeCall := func(line uint32) Term {
		request := c.String(span(line), body, "pkg")
		values := c.Values(span(line), body, []Term{request}, 0)
		return c.DeclareCall(span(line), body, request, 0, values)
	}
	call1, call2, call3 := makeCall(10), makeCall(20), makeCall(30)
	if c.Import(2, span(30), call3) != keyspace.MakeTerm(keyspace.FamilyImport, 3) ||
		c.Import(0, span(10), call1) != keyspace.MakeTerm(keyspace.FamilyImport, 1) ||
		c.Import(1, span(20), call2) != keyspace.MakeTerm(keyspace.FamilyImport, 2) {
		t.Fatal("reserved Import slots did not retain census order")
	}
	if c.Import(0, span(11), call1) != 0 {
		t.Fatal("duplicate reserved Import was accepted")
	}
}

func TestModuleRootRejectsEmptyStringRequestBeforeExactAdmission(t *testing.T) {
	const name = "module-empty-request.lua"
	c := New(name, 1, bind.GlobalCensus{})
	span := source.Span{File: name, StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 8}
	body := c.Body(span)
	request := c.String(span, body, "")
	values := c.Values(span, body, []Term{request}, 0)
	call := c.DeclareCall(span, body, request, 0, values)
	if body == 0 || request == 0 || values == 0 || call == 0 {
		t.Fatal("empty request construction failed")
	}
	if got := c.Import(0, span, call); got != 0 {
		t.Fatalf("empty Module Import = %v, want rejection", got)
	}
	if c.Body(span) != 0 {
		t.Fatal("empty Module request rejection did not terminalize")
	}
}
