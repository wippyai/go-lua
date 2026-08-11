package lower_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
)

func TestFlowValuesRetainFixedAndOpenSequenceMembers(t *testing.T) {
	for _, sample := range []struct {
		name      string
		input     string
		wantFixed int
		wantOpen  bool
	}{
		{"empty", "return", 0, false},
		{"scalar", "return 1", 1, false},
		{"fixed", "return 1, false, \"three\"", 3, false},
		{"open-call", "local function f() return 1, 2 end; return 1, f()", 1, true},
	} {
		t.Run(sample.name, func(t *testing.T) {
			p := parseBindLower(t, sample.input)
			returned := entrySource(t, p, 0)
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
