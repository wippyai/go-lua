package lower_test

import (
	"testing"

	"github.com/wippyai/go-lua/program/flow/kind"
)

func TestFlowValuesKeepOnlyFinalUnparenthesizedExpressionOpen(t *testing.T) {
	open := parseBindLower(t, "\nlocal function source() return 1, 2 end\nreturn source(), source()")
	returned := entrySource(t, open, 1)
	_, values, returnOK := open.Flow().Authored().Control().Returns().Get(returned)
	if !returnOK {
		t.Fatal("open Return has no Values")
	}
	if fixed, ok := open.Flow().Authored().Values().Len(values); !ok || fixed != 1 || valuesTail(t, open, values) == 0 {
		t.Fatalf("open Return Values fixed=%d/%v tail=%v", fixed, ok, valuesTail(t, open, values))
	}
	scalar := parseBindLower(t, "\nlocal function source() return 1, 2 end\nreturn (source())")
	returned = entrySource(t, scalar, 1)
	_, values, returnOK = scalar.Flow().Authored().Control().Returns().Get(returned)
	if !returnOK || valuesTail(t, scalar, values) != 0 {
		t.Fatalf("parenthesized Return Values = %v/%v tail=%v", values, returnOK, valuesTail(t, scalar, values))
	}
	if fixed, ok := scalar.Flow().Authored().Values().Len(values); !ok || fixed != 1 {
		t.Fatalf("parenthesized Return fixed Values=%d/%v", fixed, ok)
	}
}

func TestFlowValuesKeepCallArgumentsAndAssignmentWidths(t *testing.T) {
	p := parseBindLower(t, "\nlocal function source() return 1, 2 end\nlocal a, b = source(), source()\na, b = source(), source()")
	bind := entrySource(t, p, 1)
	assign := entrySource(t, p, 2)
	_, bindValues, bindOK := p.Flow().Authored().Storage().Binds().Get(bind)
	if !bindOK {
		t.Fatal("missing Bind Values")
	}
	if fixed, ok := p.Flow().Authored().Values().Len(bindValues); !ok || fixed != 1 || valuesTail(t, p, bindValues) == 0 {
		t.Fatalf("Bind Values fixed=%d/%v tail=%v", fixed, ok, valuesTail(t, p, bindValues))
	}
	_, assignValues, assignOK := p.Flow().Authored().Storage().Assigns().Get(assign)
	if !assignOK {
		t.Fatal("missing Assign Values")
	}
	if fixed, ok := p.Flow().Authored().Values().Len(assignValues); !ok || fixed != 1 || valuesTail(t, p, assignValues) == 0 {
		t.Fatalf("Assign Values fixed=%d/%v tail=%v", fixed, ok, valuesTail(t, p, assignValues))
	}
	if count, ok := p.Flow().Authored().Storage().Assigns().WriteCount(assign); !ok || count != 2 {
		t.Fatalf("Assign WriteCount=%d/%v, want two", count, ok)
	}
}

func TestFlowTableFieldsKeepTheirOwnExpansionLaw(t *testing.T) {
	p := parseBindLower(t, "\nlocal function source() return 1, 2 end\nreturn {source(), name = source(), [source()] = source()}")
	returned := entrySource(t, p, 1)
	_, values, returnOK := p.Flow().Authored().Control().Returns().Get(returned)
	if !returnOK {
		t.Fatal("missing Return Values")
	}
	table := valueAt(t, p, values, 0)
	tables := p.Flow().Authored().Tables()
	fields := p.Flow().Authored().Fields()
	for _, expected := range []kind.FieldKind{kind.FieldList, kind.FieldName, kind.FieldKey} {
		field, ok := tables.FieldAt(table, int(expected-kind.FieldList))
		if !ok {
			t.Fatalf("missing Table Field kind %v", expected)
		}
		_, _, fieldValues, fieldKind, rowOK := fields.Get(field)
		if !rowOK || fieldKind != expected || fieldValues == 0 {
			t.Fatalf("Table Field = values %v kind %v ok %v, want %v", fieldValues, fieldKind, rowOK, expected)
		}
		if _, finalOpen, ok := fields.Values(field); !ok || finalOpen {
			t.Fatalf("nonfinal Table Field final-open=%v/%v", finalOpen, ok)
		}
		if fixed, ok := p.Flow().Authored().Values().Len(fieldValues); !ok || fixed != 1 || valuesTail(t, p, fieldValues) != 0 {
			t.Fatalf("Table Field Values fixed=%d/%v tail=%v", fixed, ok, valuesTail(t, p, fieldValues))
		}
	}
	final := parseBindLower(t, "local function source() return 1, 2 end; return {source()}")
	returned = entrySource(t, final, 1)
	_, values, _ = final.Flow().Authored().Control().Returns().Get(returned)
	table = valueAt(t, final, values, 0)
	field, _ := final.Flow().Authored().Tables().FieldAt(table, 0)
	fieldValues, finalOpen, fieldOK := final.Flow().Authored().Fields().Values(field)
	if !fieldOK || !finalOpen || valuesTail(t, final, fieldValues) == 0 {
		t.Fatalf("final list Field Values=%v open=%v ok=%v", fieldValues, finalOpen, fieldOK)
	}
}

func TestFlowLoopHeadersRetainTheirValuesShape(t *testing.T) {
	numeric := parseBindLower(t, "for i = start(), stop(), step() do end")
	numericLoop, _ := numeric.Flow().Authored().Control().Loops().At(0)
	_, _, numericKind, numericHeader, numericOK := numeric.Flow().Authored().Control().Loops().Get(numericLoop)
	if !numericOK || numericKind != kind.LoopNumericFor {
		t.Fatalf("numeric Loop = kind %v/%v", numericKind, numericOK)
	}
	if fixed, ok := numeric.Flow().Authored().Values().Len(numericHeader); !ok || fixed != 3 || valuesTail(t, numeric, numericHeader) != 0 {
		t.Fatalf("numeric header fixed=%d/%v tail=%v", fixed, ok, valuesTail(t, numeric, numericHeader))
	}
	generic := parseBindLower(t, "for value in generator(), state() do end")
	genericLoop, _ := generic.Flow().Authored().Control().Loops().At(0)
	_, _, genericKind, genericHeader, genericOK := generic.Flow().Authored().Control().Loops().Get(genericLoop)
	if !genericOK || genericKind != kind.LoopGenericFor {
		t.Fatalf("generic Loop = kind %v/%v", genericKind, genericOK)
	}
	if fixed, ok := generic.Flow().Authored().Values().Len(genericHeader); !ok || fixed != 1 || valuesTail(t, generic, genericHeader) == 0 {
		t.Fatalf("generic header fixed=%d/%v tail=%v", fixed, ok, valuesTail(t, generic, genericHeader))
	}
}
