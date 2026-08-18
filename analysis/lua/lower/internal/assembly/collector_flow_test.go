package assembly

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestAssemblyExactLensRetainsNameKeyProvenance(t *testing.T) {
	c := newAssemblyCollector()
	body := c.Body(assemblyTestSpan())
	base := c.String(assemblyTestSpan(), body, "object")
	key := c.Name(assemblyTestSpan(), body, "field")
	lens := c.LensExact(assemblyTestSpan(), body, base, key, kind.FieldName)
	if lens == 0 {
		t.Fatal("LensExact rejected an authored Name key")
	}
}

func TestAssemblyCallCoordinatesCalleeAndActualValues(t *testing.T) {
	c := newAssemblyCollector()
	body := c.Body(assemblyTestSpan())
	callee := c.String(assemblyTestSpan(), body, "callable")
	actuals := c.Values(assemblyTestSpan(), body, []keyspace.Term{callee}, 0)
	call := c.DeclareCall(assemblyTestSpan(), body, callee, 0, actuals, "")
	if call == 0 || !c.SetCallTypeArgs(call, nil) {
		t.Fatalf("call construction failed: call=%d", call)
	}
}

func TestAssemblyReturnLinksValuesToOwningBody(t *testing.T) {
	c := newAssemblyCollector()
	body := c.Body(assemblyTestSpan())
	value := c.Integer(assemblyTestSpan(), body, 1)
	values := c.Values(assemblyTestSpan(), body, []keyspace.Term{value}, 0)
	if term := c.Return(assemblyTestSpan(), body, values); term == 0 {
		t.Fatal("Return rejected an authored Values range")
	}
}

func TestAssemblyFunctionRowsCloseAgainstNestedBody(t *testing.T) {
	c := newAssemblyCollector()
	owner := c.Body(assemblyTestSpan())
	function := c.DeclareFunction(assemblyTestSpan(), owner)
	body := c.Body(assemblyTestSpan())
	if function == 0 || body == 0 || !c.FillFunction(function, body, nil, 0, nil) {
		t.Fatalf("function construction failed: function=%d body=%d", function, body)
	}
}

func TestAssemblyGlobalOriginUsesCollectorFilename(t *testing.T) {
	span, err := globalOriginSpan("globals.lua", ast.Position{Line: 2, Column: 3, EndLine: 2, EndColumn: 7})
	if err != nil || span.File != "globals.lua" || span.StartLine != 2 || span.EndCol != 7 {
		t.Fatalf("globalOriginSpan = %#v, %v", span, err)
	}
	if _, err := globalOriginSpan("globals.lua", ast.Position{File: "other.lua"}); err == nil {
		t.Fatal("globalOriginSpan accepted a foreign source file")
	}
}

func TestAssemblyNonNilClaimHasNoStaticTarget(t *testing.T) {
	c := newAssemblyCollector()
	body := c.Body(assemblyTestSpan())
	operand := c.Integer(assemblyTestSpan(), body, 1)
	claim := c.ValueClaim(assemblyTestSpan(), body, kind.ValueClaimNonNil, operand, 0)
	if claim == 0 {
		t.Fatal("ValueClaim rejected a valid non-nil claim")
	}
}

func TestAssemblyUnaryOperatorLinksAuthoredOperand(t *testing.T) {
	c := newAssemblyCollector()
	body := c.Body(assemblyTestSpan())
	operand := c.Integer(assemblyTestSpan(), body, 1)
	term := c.Unary(assemblyTestSpan(), body, kind.UnaryNeg, operand)
	if term == 0 {
		t.Fatal("Unary rejected a valid authored operand")
	}
}

func TestAssemblyStorageRowsLinkReadsToLocalCells(t *testing.T) {
	c := newAssemblyCollector()
	body := c.Body(assemblyTestSpan())
	cell := c.Cell(assemblyTestSpan(), body, "")
	read := c.Read(assemblyTestSpan(), body, cell)
	if cell == 0 || read == 0 {
		t.Fatalf("storage rows rejected a local Cell read: cell=%d read=%d", cell, read)
	}
}

func TestAssemblyTableRowsCloseOneFieldRange(t *testing.T) {
	c := newAssemblyCollector()
	body := c.Body(assemblyTestSpan())
	table := c.DeclareTable(assemblyTestSpan(), body)
	key := c.Name(assemblyTestSpan(), body, "field")
	value := c.Integer(assemblyTestSpan(), body, 1)
	values := c.Values(assemblyTestSpan(), body, []keyspace.Term{value}, 0)
	field := c.TableField(assemblyTestSpan(), table, key, values, kind.FieldName)
	if table == 0 || field == 0 || !c.FillTable(table, []keyspace.Term{field}) {
		t.Fatalf("table construction failed: table=%d field=%d", table, field)
	}
}

func TestAssemblyValuesPreserveFixedOccurrenceOrder(t *testing.T) {
	c := newAssemblyCollector()
	body := c.Body(assemblyTestSpan())
	first := c.Integer(assemblyTestSpan(), body, 1)
	second := c.Integer(assemblyTestSpan(), body, 2)
	values := c.Values(assemblyTestSpan(), body, []keyspace.Term{first, second}, 0)
	if values == 0 {
		t.Fatal("Values rejected an authored fixed occurrence range")
	}
	row, ok := c.flow.ValueAt(0)
	if !ok || row.Owner != body || row.Fixed.Start != 0 || row.Fixed.End != 2 {
		t.Fatalf("Values row = %#v/%t, want owner/body and fixed range [0,2)", row, ok)
	}
	if got, ok := c.flow.ValueTermAt(0); !ok || got != first {
		t.Fatalf("first Values term = %d/%t, want %d", got, ok, first)
	}
	if got, ok := c.flow.ValueTermAt(1); !ok || got != second {
		t.Fatalf("second Values term = %d/%t, want %d", got, ok, second)
	}
}
