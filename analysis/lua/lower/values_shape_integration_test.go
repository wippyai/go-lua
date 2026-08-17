package lower_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestFlowValuesRetainFixedAndOpenSequenceMembers(t *testing.T) {
	for _, sample := range []struct {
		name      string
		input     string
		root      int
		wantFixed int
		wantOpen  bool
	}{
		{"empty", "return", 0, 0, false},
		{"scalar", "return 1", 0, 1, false},
		{"fixed", "return 1, false, \"three\"", 0, 3, false},
		{"open-call", "local function f() return 1, 2 end; return 1, f()", 1, 1, true},
	} {
		t.Run(sample.name, func(t *testing.T) {
			p := parseBindLower(t, sample.input)
			returned := entrySource(t, p, sample.root)
			_, values, ok := p.Flow().Authored().Control().Returns().Get(returned)
			if !ok {
				t.Fatal("missing Return Values")
			}
			fixed, fixedOK := p.Flow().Authored().Values().Len(values)
			_, tail, valuesOK := p.Flow().Authored().Values().Get(values)
			if !fixedOK || !valuesOK || fixed != sample.wantFixed || (tail != 0) != sample.wantOpen {
				t.Fatalf("Values = fixed %d/%v tail %v/%v, want %d/open=%v", fixed, fixedOK, tail, valuesOK, sample.wantFixed, sample.wantOpen)
			}
			for index := 0; index < fixed; index++ {
				if value, ok := p.Flow().Authored().Values().Member(values, index); !ok || value == 0 {
					t.Fatalf("Values member[%d] = %v/%v", index, value, ok)
				}
			}
		})
	}
}

func longIntegerReturnSource(length int) string {
	var input strings.Builder
	input.WriteString("return ")
	for index := 0; index < length; index++ {
		if index != 0 {
			input.WriteString(", ")
		}
		input.WriteString(strconv.Itoa(index))
	}
	return input.String()
}

func TestFlowValuesScaleKeepsSingleExactOwner(t *testing.T) {
	const length = 512
	p := parseBindLower(t, longIntegerReturnSource(length))
	returned := entrySource(t, p, 0)
	_, values, ok := p.Flow().Authored().Control().Returns().Get(returned)
	if !ok {
		t.Fatal("missing Return Values")
	}
	if fixed, ok := p.Flow().Authored().Values().Len(values); !ok || fixed != length || valuesTail(t, p, values) != 0 {
		t.Fatalf("scaled Values = fixed %d/%v tail %v, want %d fixed", fixed, ok, valuesTail(t, p, values), length)
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		sequenceValueSink, sequenceValueOK = p.Flow().Authored().Values().Member(values, length-1)
	}); allocations != 0 {
		t.Fatalf("Values.Member allocations = %v, want 0", allocations)
	}
}

func TestFlowAssignsKeepWriteOrderAndTargets(t *testing.T) {
	p := parseBindLower(t, "a()[b()], c()[d()] = f(), g()")
	assign := entrySource(t, p, 0)
	assigns := p.Flow().Authored().Storage().Assigns()
	writes := p.Flow().Authored().Storage().Writes()
	_, values, assignOK := assigns.Get(assign)
	if !assignOK {
		t.Fatal("missing Assign")
	}
	if count, ok := assigns.WriteCount(assign); !ok || count != 2 {
		t.Fatalf("Assign writes = %d/%v, want two", count, ok)
	}
	first, _ := assigns.WriteAt(assign, 0)
	second, _ := assigns.WriteAt(assign, 1)
	_, firstTarget, firstOK := writes.Get(first)
	_, secondTarget, secondOK := writes.Get(second)
	if !firstOK || !secondOK || firstTarget == secondTarget {
		t.Fatalf("Write targets = %v/%v %v/%v", firstTarget, firstOK, secondTarget, secondOK)
	}
	for _, target := range []keyspace.Term{firstTarget, secondTarget} {
		_, base, key, lensOK := p.Flow().Authored().Access().Dynamic().Get(target)
		if !lensOK || base == 0 || key == 0 {
			t.Fatalf("Write target Lens = base %v key %v ok %v", base, key, lensOK)
		}
	}
	if fixed, ok := p.Flow().Authored().Values().Len(values); !ok || fixed != 1 || valuesTail(t, p, values) == 0 {
		t.Fatalf("Assign Values fixed=%d/%v tail=%v", fixed, ok, valuesTail(t, p, values))
	}
}

func TestFlowTablesKeepFieldOrderAndFinalOpenTail(t *testing.T) {
	p := parseBindLower(t, "local function f() return 1, 2 end; return {a(), b(), [c()] = d(), f()}")
	returned := entrySource(t, p, 1)
	_, values, returnOK := p.Flow().Authored().Control().Returns().Get(returned)
	if !returnOK {
		t.Fatal("missing Return")
	}
	table := valueAt(t, p, values, 0)
	tables := p.Flow().Authored().Tables()
	fields := p.Flow().Authored().Fields()
	if count, ok := tables.FieldCount(table); !ok || count != 4 {
		t.Fatalf("Table field count=%d/%v, want four", count, ok)
	}
	for index, expected := range []kind.FieldKind{kind.FieldList, kind.FieldList, kind.FieldKey, kind.FieldList} {
		field, _ := tables.FieldAt(table, index)
		_, _, fieldValues, fieldKind, rowOK := fields.Get(field)
		if !rowOK || fieldKind != expected {
			t.Fatalf("Table Field[%d] kind=%v/%v, want %v", index, fieldKind, rowOK, expected)
		}
		_, finalOpen, valuesOK := fields.Values(field)
		if !valuesOK || finalOpen != (index == 3) {
			t.Fatalf("Table Field[%d] final-open=%v/%v", index, finalOpen, valuesOK)
		}
		if index == 3 && valuesTail(t, p, fieldValues) == 0 {
			t.Fatal("final Table Field lost open call tail")
		}
	}
}

var (
	sequenceValueSink keyspace.Term
	sequenceValueOK   bool
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
	for index, expected := range []kind.FieldKind{kind.FieldList, kind.FieldName, kind.FieldKey} {
		field, ok := tables.FieldAt(table, index)
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
