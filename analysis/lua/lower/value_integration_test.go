package lower_test

import (
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/compiler/parse/numparse"
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

// These are representatives of semantic boundary classes, not a randomized
// sample: each input has a distinct parser spelling or IEEE/Lua equality
// result that the Program must retain.  Lua identifiers are ASCII by the
// frozen surface grammar; Unicode is tested where the grammar permits it, as
// string payload bytes.
func TestSourceIdentifierPayloadBoundaryClasses(t *testing.T) {
	long := "a" + strings.Repeat("z9_", 1024)
	want := []string{"a", "_", "a0", "_9", long}
	p := parseBindLower(t, "return "+strings.Join(want, ", "))
	flowView := p.Flow()
	reads := flowView.Authored().Storage().Reads()
	cells := flowView.Authored().Storage().Cells()
	if reads.ImplicitCount() != len(want) {
		t.Fatalf("ImplicitReadCount = %d, want %d", reads.ImplicitCount(), len(want))
	}
	for index, name := range want {
		read, ok := reads.ImplicitAt(index)
		if !ok {
			t.Fatalf("ImplicitReadAt(%d) missing", index)
		}
		_, cell, _, ok := reads.Get(read)
		if !ok || cell == 0 {
			t.Fatalf("implicit Read(%d) = source %v ok %v", index, cell, ok)
		}
		cellKind, _, key, cellOK := cells.Get(cell)
		value, keyOK := p.Source().Keys().Exact(key)
		if !cellOK || cellKind != flow.CellGlobal || !keyOK || value.String != name {
			t.Fatalf("Global(%d) = kind %v value %#v/%v, want %q/true", index, cellKind, value, keyOK, name)
		}
		assertExactPayload(t, p, key, keyspace.LiteralValue{Kind: keyspace.LiteralString, String: name})
	}
}

func TestSourceStringPayloadBoundaryClassesPreserveExactTableAndLensKeys(t *testing.T) {
	long := strings.Repeat("0123456789abcdef", 256)
	cases := []struct {
		name, literal, value string
	}{
		{name: "double-quoted", literal: `"plain"`, value: "plain"},
		{name: "escaped-controls", literal: `"line\n\tquote\"slash\\"`, value: "line\n\tquote\"slash\\"},
		{name: "decimal-nul-and-byte", literal: `"NUL\000tail\255"`, value: string([]byte{'N', 'U', 'L', 0, 't', 'a', 'i', 'l', 255})},
		{name: "unicode", literal: `"π界🙂"`, value: "π界🙂"},
		{name: "long-bracket", literal: "[=[" + long + "]=]", value: long},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			p := parseBindLower(t, exactKeySource(test.literal))
			lensSource, lensKey, fieldSource, fieldKey := oneExactTableAndLensKey(t, p)
			assertStringTerm(t, p, lensSource, test.value)
			assertStringTerm(t, p, fieldSource, test.value)
			if lensKey != fieldKey {
				t.Fatalf("lens/table exact keys = %v/%v, want one canonical key", lensKey, fieldKey)
			}
			assertExactPayload(t, p, lensKey, keyspace.LiteralValue{Kind: keyspace.LiteralString, String: test.value})
		})
	}
}

func TestSourceEqualStringSpellingsShareExactTableAndLensKeys(t *testing.T) {
	p := parseBindLower(t, `
local t = {}
t["A"] = true
t["\065"] = true
t[ [[A]] ] = true
return { ["A"] = true, ["\065"] = true, [ [[A]] ] = true }
`)
	flowView := p.Flow()
	lenses := flowView.Authored().Access().Exact()
	fields := flowView.Authored().Fields()
	tables := flowView.Authored().Tables()
	if lenses.Count() != 3 {
		t.Fatalf("LensExactCount = %d, want 3", lenses.Count())
	}
	table, ok := tables.At(1)
	if !ok || fields.Count() != 3 {
		t.Fatalf("result Table/fields = %v/%v, want one three-field table", table, ok)
	}
	var canonical keyspace.Key
	for index := 0; index < 3; index++ {
		lens, _ := lenses.At(index)
		_, _, lensSource, lensKind, lensOK := lenses.Get(lens)
		lensKey, lensKeyOK := exactLiteralKey(t, p, lensSource)
		field, fieldOK := tables.FieldAt(table, index)
		_, fieldSource, _, fieldKind, rowOK := fields.Get(field)
		fieldKey, fieldKeyOK := exactLiteralKey(t, p, fieldSource)
		if !lensOK || !fieldOK || !rowOK || !lensKeyOK || !fieldKeyOK || lensKind != kind.FieldExact || fieldKind != kind.FieldExact || lensKey == 0 || lensKey != fieldKey {
			t.Fatalf("spelling %d keys = lens %v/%v field %v/%v/%v", index, lensKey, lensOK, fieldKey, fieldOK, rowOK)
		}
		if index == 0 {
			canonical = lensKey
		} else if lensKey != canonical {
			t.Fatalf("spelling %d key = %v, want canonical %v", index, lensKey, canonical)
		}
		assertExactPayload(t, p, lensKey, keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "A"})
	}
}

func TestSourceNumericPayloadBoundaryClassesPreserveExactTableAndLensKeys(t *testing.T) {
	type sourceKind uint8
	const (
		sourceInteger sourceKind = iota + 1
		sourceFloat
		sourceNegatedFloat
	)
	cases := []struct {
		name    string
		literal string
		source  sourceKind
		integer int64
		float   float64
		want    keyspace.LiteralValue
	}{
		{name: "integer-zero", literal: "0", source: sourceInteger, want: keyspace.LiteralValue{Kind: keyspace.LiteralInteger}},
		{name: "leading-zero-integer", literal: "001", source: sourceInteger, integer: 1, want: keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: 1}},
		{name: "maximum-int64", literal: "9223372036854775807", source: sourceInteger, integer: math.MaxInt64, want: keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: math.MaxInt64}},
		{name: "maximum-int64-hex", literal: "0x7fffffffffffffff", source: sourceInteger, integer: math.MaxInt64, want: keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: math.MaxInt64}},
		{name: "positive-zero-float", literal: "0.0", source: sourceFloat, want: keyspace.LiteralValue{Kind: keyspace.LiteralInteger}},
		{name: "negative-zero-float", literal: "-0.0", source: sourceNegatedFloat, want: keyspace.LiteralValue{Kind: keyspace.LiteralInteger}},
		{name: "integral-exponent", literal: "1e0", source: sourceFloat, float: 1, want: keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: 1}},
		{name: "fraction", literal: "1.5", source: sourceFloat, float: 1.5, want: keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(1.5)}},
		{name: "smallest-subnormal", literal: "5e-324", source: sourceFloat, float: math.SmallestNonzeroFloat64, want: keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(math.SmallestNonzeroFloat64)}},
		{name: "largest-finite", literal: "1.7976931348623157e308", source: sourceFloat, float: math.MaxFloat64, want: keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(math.MaxFloat64)}},
		{name: "rounded-above-safe-integer", literal: "9007199254740993.0", source: sourceFloat, float: 9007199254740992, want: keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: 9007199254740992}},
		{name: "minimum-int64-through-float-boundary", literal: "-9223372036854775808", source: sourceNegatedFloat, float: 9223372036854775808, want: keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: math.MinInt64}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			p := parseBindLower(t, exactKeySource(test.literal))
			lensSource, lensKey, fieldSource, fieldKey := oneExactTableAndLensKey(t, p)
			assertNumericSource(t, p, lensSource, uint8(test.source), test.integer, test.float)
			assertNumericSource(t, p, fieldSource, uint8(test.source), test.integer, test.float)
			if lensKey != fieldKey {
				t.Fatalf("lens/table exact keys = %v/%v, want one canonical key", lensKey, fieldKey)
			}
			assertExactPayload(t, p, lensKey, test.want)
		})
	}
}

func TestSourceLuaNumericEqualitiesShareExactTableAndLensKeys(t *testing.T) {
	p := parseBindLower(t, `
local t = {}
t[0] = true
t[-0.0] = true
t[0.0] = true
t[1] = true
t[1.0] = true
t[1e0] = true
t[9007199254740992.0] = true
t[9007199254740993.0] = true
return {
	[0] = true, [-0.0] = true, [0.0] = true,
	[1] = true, [1.0] = true, [1e0] = true,
	[9007199254740992.0] = true, [9007199254740993.0] = true,
}
`)
	flowView := p.Flow()
	lenses := flowView.Authored().Access().Exact()
	fields := flowView.Authored().Fields()
	tables := flowView.Authored().Tables()
	if lenses.Count() != 8 {
		t.Fatalf("LensExactCount = %d, want 8", lenses.Count())
	}
	table, ok := tables.At(1)
	if !ok || fields.Count() != 8 {
		t.Fatalf("result Table/fields = %v/%v, want one eight-field table", table, ok)
	}
	keys := make([]keyspace.Key, 8)
	for index := range keys {
		lens, _ := lenses.At(index)
		_, _, lensSource, lensKind, lensOK := lenses.Get(lens)
		lensKey, lensKeyOK := exactLiteralKey(t, p, lensSource)
		field, fieldOK := tables.FieldAt(table, index)
		_, fieldSource, _, fieldKind, fieldRowOK := fields.Get(field)
		fieldKey, fieldKeyOK := exactLiteralKey(t, p, fieldSource)
		if !lensOK || !fieldOK || !fieldRowOK || !lensKeyOK || !fieldKeyOK || lensKind != kind.FieldExact || fieldKind != kind.FieldExact || lensKey == 0 || lensKey != fieldKey {
			t.Fatalf("numeric spelling %d keys = lens %v/%v field %v/%v/%v", index, lensKey, lensOK, fieldKey, fieldOK, fieldRowOK)
		}
		keys[index] = lensKey
	}
	for _, group := range [][]int{{0, 1, 2}, {3, 4, 5}, {6, 7}} {
		for _, index := range group[1:] {
			if keys[index] != keys[group[0]] {
				t.Fatalf("Lua-equal keys %v = %v/%v, want one canonical exact key", group, keys[group[0]], keys[index])
			}
		}
	}
	assertExactPayload(t, p, keys[0], keyspace.LiteralValue{Kind: keyspace.LiteralInteger})
	assertExactPayload(t, p, keys[3], keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: 1})
	assertExactPayload(t, p, keys[6], keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: 9007199254740992})
}

func exactKeySource(literal string) string {
	return "local target = {}\ntarget[ " + literal + " ] = true\nreturn { [ " + literal + " ] = true }"
}

func oneExactTableAndLensKey(t *testing.T, p *program.Program) (keyspace.Term, keyspace.Key, keyspace.Term, keyspace.Key) {
	t.Helper()
	authored := p.Flow().Authored()
	tables, lenses, fields := authored.Tables(), authored.Access().Exact(), authored.Fields()
	if tables.Count() != 2 || lenses.Count() != 1 || fields.Count() != 1 {
		t.Fatalf("Tables/LensExact/TableFields = %d/%d/%d, want 2/1/1", tables.Count(), lenses.Count(), fields.Count())
	}
	lens, _ := lenses.At(0)
	_, _, lensSource, lensKind, lensOK := lenses.Get(lens)
	lensKey, lensKeyOK := exactLiteralKey(t, p, lensSource)
	table, _ := tables.At(1)
	field, fieldOK := tables.FieldAt(table, 0)
	_, fieldSource, _, fieldKind, fieldRowOK := fields.Get(field)
	fieldKey, fieldKeyOK := exactLiteralKey(t, p, fieldSource)
	if !lensOK || !fieldOK || !fieldRowOK || !lensKeyOK || !fieldKeyOK || lensKind != kind.FieldExact || fieldKind != kind.FieldExact || lensSource == 0 || fieldSource == 0 || lensKey == 0 || fieldKey == 0 {
		t.Fatalf("exact lens/table payloads = %v/%v/%v and %v/%v/%v", lensSource, lensKind, lensKey, fieldSource, fieldKind, fieldKey)
	}
	return lensSource, lensKey, fieldSource, fieldKey
}

func assertStringTerm(t *testing.T, p *program.Program, term keyspace.Term, want string) {
	t.Helper()
	gotTerm, _, got, ok := p.Source().Literals().Strings().At(int(keyspace.TermOrdinal(term) - 1))
	if !ok || gotTerm != term || got != want {
		t.Fatalf("String(%v) = %q/%v, want %q/true", term, got, ok, want)
	}
}

func assertNumericSource(t *testing.T, p *program.Program, term keyspace.Term, sourceKind uint8, integer int64, float float64) {
	t.Helper()
	switch sourceKind {
	case 1:
		gotTerm, _, got, ok := p.Source().Literals().Integers().At(int(keyspace.TermOrdinal(term) - 1))
		if !ok || gotTerm != term || got != integer {
			t.Fatalf("Integer(%v) = %d/%v, want %d/true", term, got, ok, integer)
		}
	case 2:
		gotTerm, _, bits, ok := p.Source().Literals().Floats().At(int(keyspace.TermOrdinal(term) - 1))
		if !ok || gotTerm != term || bits != math.Float64bits(float) {
			t.Fatalf("Float(%v) = %x/%v, want %x/true", term, bits, ok, math.Float64bits(float))
		}
	case 3:
		_, op, operand, ok := p.Flow().Authored().Operators().Unaries().Get(term)
		if !ok || op != kind.UnaryNeg || operand == 0 {
			t.Fatalf("Unary(%v) = op %v operand %v ok %v, want negated float", term, op, operand, ok)
		}
		gotTerm, _, bits, floatOK := p.Source().Literals().Floats().At(int(keyspace.TermOrdinal(operand) - 1))
		if !floatOK || gotTerm != operand || bits != math.Float64bits(float) {
			t.Fatalf("negated Float(%v) = %x/%v, want %x/true", operand, bits, floatOK, math.Float64bits(float))
		}
	default:
		t.Fatalf("unknown numeric source kind %d", sourceKind)
	}
}

func exactLiteralKey(t *testing.T, p *program.Program, term keyspace.Term) (keyspace.Key, bool) {
	t.Helper()
	sourceView := p.Source()
	if keyspace.TermFamily(term) == keyspace.FamilyKey {
		if _, _, key, ok := sourceView.Keys().Name(term); ok {
			return key, true
		}
		_, _, key, ok := sourceView.Keys().List(term)
		return key, ok
	}
	var raw keyspace.LiteralValue
	switch keyspace.TermFamily(term) {
	case keyspace.FamilyBool:
		got, _, value, ok := sourceView.Literals().Bools().At(int(keyspace.TermOrdinal(term) - 1))
		if !ok || got != term {
			return 0, false
		}
		raw = keyspace.LiteralValue{Kind: keyspace.LiteralBool, Bool: value}
	case keyspace.FamilyInteger:
		got, _, value, ok := sourceView.Literals().Integers().At(int(keyspace.TermOrdinal(term) - 1))
		if !ok || got != term {
			return 0, false
		}
		raw = keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: value}
	case keyspace.FamilyFloat:
		got, _, bits, ok := sourceView.Literals().Floats().At(int(keyspace.TermOrdinal(term) - 1))
		if !ok || got != term {
			return 0, false
		}
		raw = keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: bits}
	case keyspace.FamilyString:
		got, _, value, ok := sourceView.Literals().Strings().At(int(keyspace.TermOrdinal(term) - 1))
		if !ok || got != term {
			return 0, false
		}
		raw = keyspace.LiteralValue{Kind: keyspace.LiteralString, String: value}
	case keyspace.FamilyUnary:
		_, op, operand, ok := p.Flow().Authored().Operators().Unaries().Get(term)
		if !ok || op != kind.UnaryNeg {
			return 0, false
		}
		switch keyspace.TermFamily(operand) {
		case keyspace.FamilyInteger:
			got, _, value, integerOK := sourceView.Literals().Integers().At(int(keyspace.TermOrdinal(operand) - 1))
			if !integerOK || got != operand || value == math.MinInt64 {
				return 0, false
			}
			raw = keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: -value}
		case keyspace.FamilyFloat:
			got, _, bits, floatOK := sourceView.Literals().Floats().At(int(keyspace.TermOrdinal(operand) - 1))
			if !floatOK || got != operand {
				return 0, false
			}
			raw = keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(-math.Float64frombits(bits))}
		default:
			return 0, false
		}
	default:
		return 0, false
	}
	return sourceView.Keys().Find(raw)
}

func assertExactPayload(t *testing.T, p *program.Program, key keyspace.Key, want keyspace.LiteralValue) {
	t.Helper()
	got, ok := p.Source().Keys().Exact(key)
	if !ok || got != want {
		t.Fatalf("ExactKey(%v) = %#v/%v, want %#v/true", key, got, ok, want)
	}
}

// TestSourceUnaryCallOperandsAreOnceAndCausal is a source-only law for every
// executable unary spelling.  A call is deliberately used as each operand so
// a duplicate operand schedule would mint a second, observable Call identity.
func TestSourceUnaryCallOperandsAreOnceAndCausal(t *testing.T) {
	tests := []struct {
		name string
		op   kind.UnaryOp
	}{
		{name: "neg", op: kind.UnaryNeg},
		{name: "not", op: kind.UnaryNot},
		{name: "len", op: kind.UnaryLen},
		{name: "bit-not", op: kind.UnaryBitNot},
	}
	p := parseBindLower(t, `
local function neg() return -negOperand() end
local function logical() return not notOperand() end
local function length() return #lengthOperand() end
local function bitwise() return ~bitwiseOperand() end
`)
	flow := p.Flow()
	unaries := flow.Authored().Operators().Unaries()
	calls := flow.Authored().Calls()
	if got := unaries.Count(); got != len(tests) {
		t.Fatalf("UnaryCount = %d, want %d", got, len(tests))
	}
	if got := calls.Count(); got != len(tests) {
		t.Fatalf("CallCount = %d, want one once-evaluated operand per unary", got)
	}

	seenCalls := make(map[keyspace.Term]struct{}, len(tests))
	for index, want := range tests {
		unary, ok := unaries.At(index)
		if !ok {
			t.Fatalf("missing Unary %d", index)
		}
		owner, op, operand, ok := unaries.Get(unary)
		if !ok || owner == 0 || op != want.op || operand == 0 {
			t.Fatalf("Unary %q = owner %v op %v operand %v ok %v", want.name, owner, op, operand, ok)
		}
		call, ok := calls.At(index)
		if !ok || call != operand {
			t.Fatalf("Unary %q operand = %v; CallAt(%d) = %v/%v, want one authored Call", want.name, operand, index, call, ok)
		}
		if _, duplicate := seenCalls[call]; duplicate {
			t.Fatalf("Unary %q reused Call %v from another authored operand", want.name, call)
		}
		seenCalls[call] = struct{}{}

		operandEntry, ok := flow.Ports().Entry(unary)
		if !ok || operandEntry == 0 {
			t.Fatalf("Unary %q has no exact operand entry", want.name)
		}
		callEntry, ok := flow.Ports().Entry(call)
		if !ok || operandEntry != callEntry {
			t.Fatalf("Unary %q operand entry = %v; Call entry = %v/%v", want.name, operandEntry, callEntry, ok)
		}
		if next, ok := flow.Ports().Finish(call); !ok || next != unary {
			t.Fatalf("Unary %q Call successor = %v/%v, want Unary %v", want.name, next, ok, unary)
		}

		returned := returnOwnedBy(t, p, owner)
		_, values, ok := flow.Authored().Control().Returns().Get(returned)
		if !ok {
			t.Fatalf("Unary %q Return %v has no Values", want.name, returned)
		}
		if result := valueAt(t, p, values, 0); result != unary {
			t.Fatalf("Unary %q Return value = %v, want Unary %v", want.name, result, unary)
		}
		if next, ok := flow.Ports().Finish(unary); !ok || next != values {
			t.Fatalf("Unary %q successor = %v/%v, want parent Return Values %v", want.name, next, ok, values)
		}
	}
}

// TestSourceValueClaimCallOperandsAreOnceScalarAndCausal proves all three
// claim spellings share one direct, erased wrapper protocol. A raw Call is a
// deliberately open producer in list position, so the fixed parent Values
// relation also proves each claim scalar-adjusts its operand.
func TestSourceValueClaimCallOperandsAreOnceScalarAndCausal(t *testing.T) {
	p := parseBindLower(t, `
type Number = number
local function asClaim() return asOperand() as Number end
local function colonClaim() return colonOperand() :: Number end
local function nonNilClaim() return nonNilOperand()! end
`)
	flow := p.Flow()
	calls := flow.Authored().Calls()
	claims := flow.Authored().Claims()
	if calls.Count() != 3 || claims.Count() != 3 {
		t.Fatalf("Calls/ValueClaims = %d/%d, want 3/3", calls.Count(), claims.Count())
	}
	wantKinds := []kind.ValueClaimKind{
		kind.ValueClaimTypeAs,
		kind.ValueClaimTypeColonColon,
		kind.ValueClaimNonNil,
	}
	for index, wantKind := range wantKinds {
		call, _ := calls.At(index)
		claim, _ := claims.At(index)
		owner, operand, claimKind, ok := claims.Get(claim)
		target, targetOK := p.Static().Operands().Claims().Target(claim)
		if !ok || owner == 0 || operand != call || claimKind != wantKind {
			t.Fatalf("ValueClaim %d = owner %v operand %v target %v/%v kind %v ok %v", index, owner, operand, target, targetOK, claimKind, ok)
		}
		if wantKind == kind.ValueClaimNonNil {
			if targetOK || target != 0 {
				t.Fatalf("NonNil ValueClaim target = %v, want absent", target)
			}
		} else if !targetOK || target == 0 {
			t.Fatalf("typed ValueClaim %d lacks exact static target", index)
		}
		entry, ok := flow.Ports().Entry(claim)
		callEntry, callOK := flow.Ports().Entry(call)
		if !ok || !callOK || entry == 0 || entry != callEntry {
			t.Fatalf("ValueClaim %d operand entry = %v/%v; Call entry = %v/%v", index, entry, ok, callEntry, callOK)
		}
		if next, ok := flow.Ports().Finish(call); !ok || next != claim {
			t.Fatalf("Call %d successor = %v/%v, want ValueClaim %v", index, next, ok, claim)
		}
		returned := returnOwnedBy(t, p, owner)
		_, values, ok := flow.Authored().Control().Returns().Get(returned)
		if !ok || valueAt(t, p, values, 0) != claim || valuesTail(t, p, values) != 0 {
			t.Fatalf("ValueClaim %d Return Values = %v/%v tail %v, want fixed claim %v", index, values, ok, valuesTail(t, p, values), claim)
		}
		if next, ok := flow.Ports().Finish(claim); !ok || next != values {
			t.Fatalf("ValueClaim %d successor = %v/%v, want parent Return Values %v", index, next, ok, values)
		}
	}
}

// TestSourceValueClaimsKeepFalseAndNilOnTheNormalPath makes the absence of a
// runtime proof/guard observable: false and definitely nil both retain their
// direct normal continuation. Nil may be diagnosed later, but lowering does
// not prune it or fabricate a Throw/Outcome at the claim.
func TestSourceValueClaimsKeepFalseAndNilOnTheNormalPath(t *testing.T) {
	p := parseBindLower(t, `
local function claims()
  return false!, nil!, false as boolean, nil :: nil
end
`)
	flow := p.Flow()
	claims := flow.Authored().Claims()
	if claims.Count() != 4 {
		t.Fatalf("ValueClaimCount = %d, want 4", claims.Count())
	}
	for index, want := range []struct {
		kind kind.ValueClaimKind
		nil  bool
	}{
		{kind: kind.ValueClaimNonNil},
		{kind: kind.ValueClaimNonNil, nil: true},
		{kind: kind.ValueClaimTypeAs},
		{kind: kind.ValueClaimTypeColonColon, nil: true},
	} {
		claim, _ := claims.At(index)
		owner, operand, claimKind, ok := claims.Get(claim)
		target, targetOK := p.Static().Operands().Claims().Target(claim)
		if !ok || owner == 0 || claimKind != want.kind {
			t.Fatalf("ValueClaim %d = owner %v operand %v target %v/%v kind %v ok %v", index, owner, operand, target, targetOK, claimKind, ok)
		}
		if want.kind == kind.ValueClaimNonNil && (targetOK || target != 0) {
			t.Fatalf("NonNil ValueClaim %d has target %v", index, target)
		}
		if want.kind != kind.ValueClaimNonNil && (!targetOK || target == 0) {
			t.Fatalf("typed ValueClaim %d lacks target", index)
		}
		if want.nil {
			if literal, _, literalOK := p.Source().Literals().Nils().At(int(keyspace.TermOrdinal(operand)) - 1); !literalOK || literal != operand {
				t.Fatalf("ValueClaim %d operand %v is not Nil", index, operand)
			}
		} else if literal, _, value, literalOK := p.Source().Literals().Bools().At(int(keyspace.TermOrdinal(operand)) - 1); !literalOK || literal != operand || value {
			t.Fatalf("ValueClaim %d operand %v is not false Bool", index, operand)
		}
		next, normal := flow.Ports().Finish(claim)
		if !normal || next == 0 {
			t.Fatalf("ValueClaim %d has no normal successor", index)
		}
		if _, isOutcome := flow.Outcomes().Get(next); isOutcome {
			t.Fatalf("ValueClaim %d normal successor %v is an Outcome", index, next)
		}
	}
}

// TestSourceValueClaimsScalarAdjustVarargOperands makes the second open
// producer explicit. Even though ... can remain a final open Values tail,
// every claim consumes exactly one scalar and the enclosing return remains a
// fixed three-value pack.
func TestSourceValueClaimsScalarAdjustVarargOperands(t *testing.T) {
	p := parseBindLower(t, `
local function claims(...)
  return (...) as number, (...) :: number, (...)!
end
`)
	flow := p.Flow()
	varargs := flow.Authored().Storage().Varargs()
	claims := flow.Authored().Claims()
	if varargs.Count() != 3 || claims.Count() != 3 {
		t.Fatalf("Varargs/ValueClaims = %d/%d, want 3/3", varargs.Count(), claims.Count())
	}
	returned, ok := flow.Authored().Control().Returns().At(0)
	if !ok {
		t.Fatal("missing return")
	}
	_, values, ok := flow.Authored().Control().Returns().Get(returned)
	if !ok || valuesTail(t, p, values) != 0 {
		t.Fatalf("claim return Values = %v/%v tail %v, want fixed", values, ok, valuesTail(t, p, values))
	}
	if fixed, ok := flow.Authored().Values().Len(values); !ok || fixed != 3 {
		t.Fatalf("claim return fixed length = %d/%v, want 3", fixed, ok)
	}
	for index, wantKind := range []kind.ValueClaimKind{
		kind.ValueClaimTypeAs,
		kind.ValueClaimTypeColonColon,
		kind.ValueClaimNonNil,
	} {
		claim, _ := claims.At(index)
		_, operand, claimKind, ok := claims.Get(claim)
		target, targetOK := p.Static().Operands().Claims().Target(claim)
		if !ok || claimKind != wantKind {
			t.Fatalf("ValueClaim %d = operand %v target %v/%v kind %v ok %v", index, operand, target, targetOK, claimKind, ok)
		}
		if _, _, varargOK := varargs.Get(operand); !varargOK {
			t.Fatalf("ValueClaim %d operand %v is not Vararg", index, operand)
		}
		if wantKind == kind.ValueClaimNonNil {
			if targetOK || target != 0 {
				t.Fatalf("NonNil ValueClaim %d target = %v, want absent", index, target)
			}
		} else if !targetOK || target == 0 {
			t.Fatalf("typed ValueClaim %d lacks target", index)
		}
		if result := valueAt(t, p, values, index); result != claim {
			t.Fatalf("return fixed value %d = %v, want ValueClaim %v", index, result, claim)
		}
		if next, ok := flow.Ports().Finish(operand); !ok || next != claim {
			t.Fatalf("Vararg %d successor = %v/%v, want ValueClaim %v", index, next, ok, claim)
		}
	}
}

// TestSourceValueClaimSpansPreserveAuthoredExpression proves the claims retain
// the exact source extent of each spelling. In particular, postfix ! owns its
// authored token rather than inheriting only the operand's end position.
func TestSourceValueClaimSpansPreserveAuthoredExpression(t *testing.T) {
	p := parseBindLower(t, `return false!, false as boolean, false :: boolean`)
	wants := []source.Span{
		{File: "fixture.lua", StartLine: 1, StartCol: 8, EndLine: 1, EndCol: 13},
		{File: "fixture.lua", StartLine: 1, StartCol: 16, EndLine: 1, EndCol: 31},
		{File: "fixture.lua", StartLine: 1, StartCol: 34, EndLine: 1, EndCol: 49},
	}
	claims := p.Flow().Authored().Claims()
	if got := claims.Count(); got != len(wants) {
		t.Fatalf("ValueClaimCount = %d, want %d", got, len(wants))
	}
	for index, want := range wants {
		claim, ok := claims.At(index)
		if !ok {
			t.Fatalf("missing ValueClaim %d", index)
		}
		if span, ok := p.Source().Identity().Span(claim); !ok || span != want {
			t.Fatalf("Span(ValueClaim %d) = %#v/%v, want %#v", index, span, ok, want)
		}
	}
}

func returnOwnedBy(t *testing.T, p *program.Program, owner keyspace.Term) keyspace.Term {
	t.Helper()
	returns := p.Flow().Authored().Control().Returns()
	var found keyspace.Term
	for index := 0; index < returns.Count(); index++ {
		returned, ok := returns.At(index)
		if !ok {
			continue
		}
		returnOwner, _, ok := returns.Get(returned)
		if !ok || returnOwner != owner {
			continue
		}
		if found != 0 {
			t.Fatalf("Body %v has multiple Returns %v and %v", owner, found, returned)
		}
		found = returned
	}
	if found == 0 {
		t.Fatalf("Body %v has no Return", owner)
	}
	return found
}

// valuesSourceCases is the values vertical's complete atomic source witness
// set. Each case is consumed directly by the exact source-to-Program law.
var valuesSourceCases = []sourceCase{
	{ID: "values.case.nil", Form: "NilExpr", Source: "local x = nil", Line: 1},
	{ID: "values.case.false", Form: "FalseExpr", Source: "local x = false", Line: 1},
	{ID: "values.case.true", Form: "TrueExpr", Source: "local x = true", Line: 1},
	{ID: "values.case.number.integer", Form: "NumberExpr", Source: "local x = 1", Line: 1},
	{ID: "values.case.number.float", Form: "NumberExpr", Source: "local x = 1.5", Line: 1},
	{ID: "values.case.string", Form: "StringExpr", Source: "local x = 's'", Line: 1},
	{ID: "values.case.vararg.open", Form: "Comma3Expr", Source: "local function f(...)\n  return ...\nend", Line: 2},
	{ID: "values.case.vararg.scalar", Form: "Comma3Expr", Source: "local function f(...)\n  return (...)\nend", Line: 2},
	{ID: "values.case.identifier.read", Form: "IdentExpr", Source: "local x = 1\nlocal y = x", Line: 2},
	{ID: "values.case.attr.dot", Form: "AttrGetExpr", Source: "local t = { x = 1 }\nlocal y = t.x", Line: 2},
	{ID: "values.case.attr.index-exact", Form: "AttrGetExpr", Source: "local t = {}\nlocal y = t[1]", Line: 2},
	{ID: "values.case.attr.index-dynamic", Form: "AttrGetExpr", Source: "local t = {}\nlocal k = 1\nlocal y = t[k]", Line: 3},
	{ID: "values.case.assignment", Form: "AssignStmt", Source: "local t = {}\nlocal first = 1\nlocal second = 2\nt[first], t[second] = 10, 20", Line: 4},
	{ID: "values.case.values.return-list", Form: "ReturnStmt", Source: "return 1, 2", Line: 1},
	{ID: "values.case.table", Form: "TableExpr", Source: "local t = {}", Line: 1},
	{ID: "values.case.table-field.name", Form: "Field", Source: "local t = {\n  x = 1,\n}", Line: 2},
	{ID: "values.case.table-field.exact", Form: "Field", Source: "local t = {\n  [1] = 2,\n}", Line: 2},
	{ID: "values.case.table-field.dynamic", Form: "Field", Source: "local k = 1\nlocal t = {\n  [k] = 2,\n}", Line: 3},
	{ID: "values.case.table-field.list-scalar-final", Form: "Field", Source: "local t = {\n  1,\n}", Line: 2},
	{ID: "values.case.table-field.list", Form: "Field", Source: "local function f(...)\n  local t = {\n    ...,\n  }\nend", Line: 3},
	{ID: "values.case.table-field.list-prefix", Form: "Field", Source: "local function f(...)\n  local t = {\n    ...,\n    1,\n  }\nend", Line: 3},
}

// TestValuesSourceCasesHaveExactProgramWitnesses is the source-to-Program
// witness for every atomic values case.  Each arm starts with the parsed AST
// occurrence, derives its expected semantic discriminant from that occurrence,
// and then follows only typed Program relations from the matching source span.
func TestValuesSourceCasesHaveExactProgramWitnesses(t *testing.T) {
	for _, sourceCase := range valuesSourceCases {
		sourceCase := sourceCase
		t.Run(sourceCase.ID, func(t *testing.T) {
			statements, err := parse.ParseString(sourceCase.Source, "values-witness.lua")
			if err != nil {
				t.Fatalf("parse %s: %v", sourceCase.ID, err)
			}
			target := valuesASTTarget(t, statements, sourceCase.Form, sourceCase.Line)
			binding := bind.BindChunk(statements)
			if binding == nil {
				t.Fatal("binder returned nil result")
			}
			p := parseBindLower(t, sourceCase.Source)
			assertValuesCase(t, p, binding, statements, sourceCase, target)
		})
	}
}

func assertValuesCase(t *testing.T, p *program.Program, binding *bind.Result, statements []ast.Stmt, sourceCase sourceCase, target ast.PositionHolder) {
	t.Helper()
	switch sourceCase.ID {
	case "values.case.nil":
		if _, ok := target.(*ast.NilExpr); !ok {
			t.Fatalf("parsed target = %T, want *ast.NilExpr", target)
		}
		term := valuesNilAt(t, p, target)
		if literal, owner, ok := p.Source().Literals().Nils().At(int(keyspace.TermOrdinal(term)) - 1); !ok || literal != term || owner == 0 {
			t.Fatalf("exact Nil owner = %v/%v", owner, ok)
		}
	case "values.case.false", "values.case.true":
		_, ok := target.(*ast.FalseExpr)
		want := false
		if !ok {
			if _, trueOK := target.(*ast.TrueExpr); !trueOK {
				t.Fatalf("parsed target = %T, want boolean literal", target)
			}
			want = true
		}
		term := valuesBoolAt(t, p, target)
		literal, owner, got, boolOK := p.Source().Literals().Bools().At(int(keyspace.TermOrdinal(term)) - 1)
		if !boolOK || literal != term || owner == 0 || got != want {
			t.Fatalf("Bool = owner %v payload %v/%v, want owned %v", owner, got, boolOK, want)
		}
	case "values.case.number.integer", "values.case.number.float":
		expr, ok := target.(*ast.NumberExpr)
		if !ok {
			t.Fatalf("parsed target = %T, want *ast.NumberExpr", target)
		}
		integer, integerOK := numparse.ParseIntegerLiteral(expr.Value)
		if integerOK {
			term := valuesIntegerAt(t, p, target)
			literal, owner, got, ok := p.Source().Literals().Integers().At(int(keyspace.TermOrdinal(term)) - 1)
			if !ok || literal != term || owner == 0 || got != integer {
				t.Fatalf("Integer = owner %v payload %d/%v, want owned %d", owner, got, ok, integer)
			}
			return
		}
		_, floating, parsed := numparse.ParseNumberLiteral(expr.Value)
		if !parsed {
			t.Fatalf("parser accepted unparseable numeric spelling %q", expr.Value)
		}
		term := valuesFloatAt(t, p, target)
		literal, owner, gotBits, ok := p.Source().Literals().Floats().At(int(keyspace.TermOrdinal(term)) - 1)
		got := math.Float64frombits(gotBits)
		if !ok || literal != term || owner == 0 || got != floating {
			t.Fatalf("Float = owner %v payload %g/%v, want owned %g", owner, got, ok, floating)
		}
	case "values.case.string":
		expr, ok := target.(*ast.StringExpr)
		if !ok {
			t.Fatalf("parsed target = %T, want *ast.StringExpr", target)
		}
		term := valuesStringAt(t, p, target)
		literal, owner, got, ok := p.Source().Literals().Strings().At(int(keyspace.TermOrdinal(term)) - 1)
		if !ok || literal != term || owner == 0 || got != expr.Value {
			t.Fatalf("String = owner %v payload %q/%v, want owned %q", owner, got, ok, expr.Value)
		}
	case "values.case.vararg.open", "values.case.vararg.scalar":
		expr, ok := target.(*ast.Comma3Expr)
		if !ok {
			t.Fatalf("parsed target = %T, want *ast.Comma3Expr", target)
		}
		vararg := valuesVarargAt(t, p, target)
		if _, cell, ok := p.Flow().Authored().Storage().Varargs().Get(vararg); !ok || cell == 0 {
			t.Fatalf("Vararg(%v) has no function vararg Cell", vararg)
		}
		if expr.AdjustRet != (sourceCase.ID == "values.case.vararg.scalar") {
			t.Fatalf("parsed AdjustRet = %v contradicts source-case %s", expr.AdjustRet, sourceCase.ID)
		}
		ret := valuesReturnAt(t, p, valuesASTTarget(t, statements, "ReturnStmt", target.Line()))
		_, values, ok := p.Flow().Authored().Control().Returns().Get(ret)
		if !ok {
			t.Fatal("vararg return lacks Values")
		}
		fixed, fixedOK := p.Flow().Authored().Values().Len(values)
		_, tail, valuesOK := p.Flow().Authored().Values().Get(values)
		if !fixedOK || !valuesOK {
			t.Fatal("vararg return Values relation is missing")
		}
		if expr.AdjustRet {
			if fixed != 1 || tail != 0 || valueAt(t, p, values, 0) != vararg {
				t.Fatalf("scalar vararg Values = fixed %d tail %v, want one fixed exact occurrence", fixed, tail)
			}
		} else if fixed != 0 || tail != vararg {
			t.Fatalf("open vararg Values = fixed %d tail %v, want exact open occurrence %v", fixed, tail, vararg)
		}
	case "values.case.identifier.read":
		expr, ok := target.(*ast.IdentExpr)
		if !ok {
			t.Fatalf("parsed target = %T, want *ast.IdentExpr", target)
		}
		if symbol, ok := binding.SymbolOf(expr); !ok || symbol == 0 {
			t.Fatal("binder did not select a declaration for the authored identifier")
		}
		if owner, source, _, ok := p.Flow().Authored().Storage().Reads().Get(valuesReadAt(t, p, target)); !ok || owner == 0 || source == 0 {
			t.Fatalf("exact identifier Read = owner %v source %v ok %v", owner, source, ok)
		}
	case "values.case.attr.dot", "values.case.attr.index-exact", "values.case.attr.index-dynamic":
		expr, ok := target.(*ast.AttrGetExpr)
		if !ok {
			t.Fatalf("parsed target = %T, want *ast.AttrGetExpr", target)
		}
		read := valuesReadAt(t, p, target)
		readOwner, lens, _, ok := p.Flow().Authored().Storage().Reads().Get(read)
		if !ok || readOwner == 0 {
			t.Fatalf("exact attribute read %v is not Read", read)
		}
		want := fieldKindForAttr(expr)
		var lensOwner, base, keySource keyspace.Term
		var gotKind flowkind.FieldKind
		if want == flowkind.FieldKey {
			lensOwner, base, keySource, ok = p.Flow().Authored().Access().Dynamic().Get(lens)
			gotKind = flowkind.FieldKey
		} else {
			lensOwner, base, keySource, gotKind, ok = p.Flow().Authored().Access().Exact().Get(lens)
		}
		if !ok || lensOwner == 0 || base == 0 {
			t.Fatalf("attribute Read source = Lens(%v) = owner %v base %v kind %v ok %v", lens, lensOwner, base, gotKind, ok)
		}
		if gotKind != want {
			t.Fatalf("Lens kind = %v, want AST-derived %v", gotKind, want)
		}
		if want == flowkind.FieldName {
			name := ast.KeyName(expr.Key)
			if name == "" {
				t.Fatalf("dot key = %T, has no authored spelling", expr.Key)
			}
			_, got, _, nameOK := p.Source().Keys().Name(keySource)
			if !nameOK || got != name {
				t.Fatalf("dot Lens key = Name(%v) = %q/%v, want static %q", keySource, got, nameOK, name)
			}
		}
		if want != flowkind.FieldName && keySource == 0 {
			t.Fatal("bracket Lens lacks its evaluated key term")
		}
	case "values.case.assignment":
		stmt, ok := target.(*ast.AssignStmt)
		if !ok {
			t.Fatalf("parsed target = %T, want *ast.AssignStmt", target)
		}
		assign := valuesAssignAt(t, p, target)
		assigns := p.Flow().Authored().Storage().Assigns()
		if owner, _, ok := assigns.Get(assign); !ok || owner == 0 {
			t.Fatalf("Assign owner = %v/%v", owner, ok)
		}
		if fixed, ok := assigns.WriteCount(assign); !ok || fixed != len(stmt.Lhs) {
			t.Fatalf("Assign write width = %d/%v, want %d", fixed, ok, len(stmt.Lhs))
		}
		_, values, ok := assigns.Get(assign)
		fixed, fixedOK := assigns.WriteCount(assign)
		if !ok || !fixedOK || values == 0 || fixed != len(stmt.Lhs) {
			t.Fatalf("AssignValues = %v/%d/%v, want one scalarized slot per target", values, fixed, ok)
		}
		for index := range stmt.Lhs {
			write, ok := assigns.WriteAt(assign, index)
			if !ok {
				t.Fatalf("missing Write %d", index)
			}
			if parent, target, ok := p.Flow().Authored().Storage().Writes().Get(write); !ok || parent != assign || target == 0 {
				t.Fatalf("Write %d = parent %v target %v ok %v", index, parent, target, ok)
			}
		}
	case "values.case.values.return-list":
		stmt, ok := target.(*ast.ReturnStmt)
		if !ok {
			t.Fatalf("parsed target = %T, want *ast.ReturnStmt", target)
		}
		ret := valuesReturnAt(t, p, target)
		owner, values, returnOK := p.Flow().Authored().Control().Returns().Get(ret)
		if !returnOK || owner == 0 {
			t.Fatalf("Return owner = %v/%v", owner, returnOK)
		}
		if fixed, ok := p.Flow().Authored().Values().Len(values); !ok || fixed != len(stmt.Exprs) {
			t.Fatalf("Return Values fixed width = %d/%v, want %d", fixed, ok, len(stmt.Exprs))
		}
	case "values.case.table":
		expr, ok := target.(*ast.TableExpr)
		if !ok {
			t.Fatalf("parsed target = %T, want *ast.TableExpr", target)
		}
		table := valuesTableAt(t, p, target)
		if owner, ok := p.Flow().Authored().Tables().Get(table); !ok || owner == 0 {
			t.Fatalf("exact Table allocation owner = %v/%v", owner, ok)
		}
		if len(expr.Fields) == 0 {
			if _, ok := p.Flow().Authored().Tables().FieldAt(table, 0); ok {
				t.Fatal("empty source table retained a TableField")
			}
			if finish, ok := p.Flow().Ports().Finish(table); !ok || finish == 0 {
				t.Fatal("empty Table lacks its completion frontier")
			}
		}
	case "values.case.table-field.name", "values.case.table-field.exact", "values.case.table-field.dynamic", "values.case.table-field.list-scalar-final", "values.case.table-field.list", "values.case.table-field.list-prefix":
		field, final, ok := valuesFieldTarget(target, statements)
		if !ok {
			t.Fatal("target Field did not belong to an authored table constructor")
		}
		fieldTerm := valuesTableFieldAt(t, p, target)
		parent, key, values, fieldKind, ok := p.Flow().Authored().Fields().Get(fieldTerm)
		if !ok || parent == 0 || values == 0 {
			t.Fatalf("TableField = parent %v key %v values %v kind %v ok %v", parent, key, values, fieldKind, ok)
		}
		wantKind := fieldKindForField(field)
		if fieldKind != wantKind {
			t.Fatalf("TableField kind = %v, want AST-derived %v", fieldKind, wantKind)
		}
		if wantKind == flowkind.FieldName && key == 0 {
			t.Fatal("named TableField lacks exact name key")
		}
		if wantKind != flowkind.FieldName && wantKind != flowkind.FieldList && key == 0 {
			t.Fatal("bracket TableField lacks evaluated key")
		}
		_, finalOpen, ok := p.Flow().Authored().Fields().Values(fieldTerm)
		wantOpen := final && ast.CanProduceMultipleValues(field.Value)
		if !ok || finalOpen != wantOpen {
			t.Fatalf("TableField final-open = %v/%v, want %v", finalOpen, ok, wantOpen)
		}
	default:
		t.Fatalf("values SourceCase %s has no direct semantic witness", sourceCase.ID)
	}
}

func fieldKindForAttr(expr *ast.AttrGetExpr) flowkind.FieldKind {
	if expr.KeySyntax == ast.AttrKeyDot {
		return flowkind.FieldName
	}
	if scalarKeyExpr(expr.Key) {
		return flowkind.FieldExact
	}
	return flowkind.FieldKey
}

func fieldKindForField(field *ast.Field) flowkind.FieldKind {
	if field.Key == nil {
		return flowkind.FieldList
	}
	if field.KeySyntax == ast.AttrKeyDot {
		return flowkind.FieldName
	}
	if scalarKeyExpr(field.Key) {
		return flowkind.FieldExact
	}
	return flowkind.FieldKey
}

func scalarKeyExpr(expr ast.Expr) bool {
	switch expr.(type) {
	case *ast.NilExpr, *ast.FalseExpr, *ast.TrueExpr, *ast.NumberExpr, *ast.StringExpr:
		return true
	default:
		return false
	}
}

func valuesNilAt(t *testing.T, p *program.Program, target ast.PositionHolder) keyspace.Term {
	t.Helper()
	var found keyspace.Term
	nils := p.Source().Literals().Nils()
	for index := 0; index < nils.Count(); index++ {
		term, _, ok := nils.At(index)
		if !ok {
			t.Fatalf("NilAt(%d) failed", index)
		}
		if valuesSameSpan(p, term, target) {
			if found != 0 {
				t.Fatal("multiple exact Nil terms")
			}
			found = term
		}
	}
	if found == 0 {
		t.Fatal("no Nil term at exact AST span")
	}
	return found
}

func valuesBoolAt(t *testing.T, p *program.Program, target ast.PositionHolder) keyspace.Term {
	t.Helper()
	var found keyspace.Term
	bools := p.Source().Literals().Bools()
	for index := 0; index < bools.Count(); index++ {
		term, _, _, ok := bools.At(index)
		if !ok {
			t.Fatalf("BoolAt(%d) failed", index)
		}
		if valuesSameSpan(p, term, target) {
			if found != 0 {
				t.Fatal("multiple exact Bool terms")
			}
			found = term
		}
	}
	if found == 0 {
		t.Fatal("no Bool term at exact AST span")
	}
	return found
}

func valuesIntegerAt(t *testing.T, p *program.Program, target ast.PositionHolder) keyspace.Term {
	t.Helper()
	var found keyspace.Term
	integers := p.Source().Literals().Integers()
	for index := 0; index < integers.Count(); index++ {
		term, _, _, ok := integers.At(index)
		if !ok {
			t.Fatalf("IntegerAt(%d) failed", index)
		}
		if valuesSameSpan(p, term, target) {
			if found != 0 {
				t.Fatal("multiple exact Integer terms")
			}
			found = term
		}
	}
	if found == 0 {
		t.Fatal("no Integer term at exact AST span")
	}
	return found
}

func valuesFloatAt(t *testing.T, p *program.Program, target ast.PositionHolder) keyspace.Term {
	t.Helper()
	var found keyspace.Term
	floats := p.Source().Literals().Floats()
	for index := 0; index < floats.Count(); index++ {
		term, _, _, ok := floats.At(index)
		if !ok {
			t.Fatalf("FloatAt(%d) failed", index)
		}
		if valuesSameSpan(p, term, target) {
			if found != 0 {
				t.Fatal("multiple exact Float terms")
			}
			found = term
		}
	}
	if found == 0 {
		t.Fatal("no Float term at exact AST span")
	}
	return found
}

func valuesStringAt(t *testing.T, p *program.Program, target ast.PositionHolder) keyspace.Term {
	t.Helper()
	var found keyspace.Term
	strings := p.Source().Literals().Strings()
	for index := 0; index < strings.Count(); index++ {
		term, _, _, ok := strings.At(index)
		if !ok {
			t.Fatalf("StringAt(%d) failed", index)
		}
		if valuesSameSpan(p, term, target) {
			if found != 0 {
				t.Fatal("multiple exact String terms")
			}
			found = term
		}
	}
	if found == 0 {
		t.Fatal("no String term at exact AST span")
	}
	return found
}

func valuesVarargAt(t *testing.T, p *program.Program, target ast.PositionHolder) keyspace.Term {
	t.Helper()
	var found keyspace.Term
	varargs := p.Flow().Authored().Storage().Varargs()
	for index := 0; index < varargs.Count(); index++ {
		term, ok := varargs.At(index)
		if !ok {
			t.Fatalf("VarargAt(%d) failed", index)
		}
		if valuesSameSpan(p, term, target) {
			if found != 0 {
				t.Fatal("multiple exact Vararg terms")
			}
			found = term
		}
	}
	if found == 0 {
		t.Fatal("no Vararg term at exact AST span")
	}
	return found
}

func valuesReadAt(t *testing.T, p *program.Program, target ast.PositionHolder) keyspace.Term {
	t.Helper()
	var found keyspace.Term
	reads := p.Flow().Authored().Storage().Reads()
	for index := 0; index < reads.Count(); index++ {
		term, ok := reads.At(index)
		if !ok {
			t.Fatalf("ReadAt(%d) failed", index)
		}
		if valuesSameSpan(p, term, target) {
			if found != 0 {
				t.Fatal("multiple exact Read terms")
			}
			found = term
		}
	}
	if found == 0 {
		t.Fatal("no Read term at exact AST span")
	}
	return found
}

func valuesAssignAt(t *testing.T, p *program.Program, target ast.PositionHolder) keyspace.Term {
	t.Helper()
	var found keyspace.Term
	assigns := p.Flow().Authored().Storage().Assigns()
	for index := 0; index < assigns.Count(); index++ {
		term, ok := assigns.At(index)
		if !ok {
			t.Fatalf("AssignAt(%d) failed", index)
		}
		if valuesSameSpan(p, term, target) {
			if found != 0 {
				t.Fatal("multiple exact Assign terms")
			}
			found = term
		}
	}
	if found == 0 {
		t.Fatal("no Assign term at exact AST span")
	}
	return found
}

func valuesReturnAt(t *testing.T, p *program.Program, target ast.PositionHolder) keyspace.Term {
	t.Helper()
	var found keyspace.Term
	returns := p.Flow().Authored().Control().Returns()
	for index := 0; index < returns.Count(); index++ {
		term, ok := returns.At(index)
		if !ok {
			t.Fatalf("ReturnAt(%d) failed", index)
		}
		if valuesSameSpan(p, term, target) {
			if found != 0 {
				t.Fatal("multiple exact Return terms")
			}
			found = term
		}
	}
	if found == 0 {
		t.Fatal("no Return term at exact AST span")
	}
	return found
}

func valuesTableAt(t *testing.T, p *program.Program, target ast.PositionHolder) keyspace.Term {
	t.Helper()
	var found keyspace.Term
	tables := p.Flow().Authored().Tables()
	for index := 0; index < tables.Count(); index++ {
		term, ok := tables.At(index)
		if !ok {
			t.Fatalf("TableAt(%d) failed", index)
		}
		if valuesSameSpan(p, term, target) {
			if found != 0 {
				t.Fatal("multiple exact Table terms")
			}
			found = term
		}
	}
	if found == 0 {
		t.Fatal("no Table term at exact AST span")
	}
	return found
}

func valuesTableFieldAt(t *testing.T, p *program.Program, target ast.PositionHolder) keyspace.Term {
	t.Helper()
	var found keyspace.Term
	fields := p.Flow().Authored().Fields()
	for index := 0; index < fields.Count(); index++ {
		term, ok := fields.At(index)
		if !ok {
			t.Fatalf("TableFieldAt(%d) failed", index)
		}
		if valuesSameSpan(p, term, target) {
			if found != 0 {
				t.Fatal("multiple exact TableField terms")
			}
			found = term
		}
	}
	if found == 0 {
		t.Fatal("no TableField term at exact AST span")
	}
	return found
}

func valuesSameSpan(p *program.Program, term keyspace.Term, target ast.PositionHolder) bool {
	span, ok := p.Source().Identity().Span(term)
	want := ast.SpanOf(target)
	return ok && span.StartLine == uint32(want.StartLine) && span.StartCol == uint32(want.StartCol) && span.EndLine == uint32(want.EndLine) && span.EndCol == uint32(want.EndCol)
}

func valuesASTTarget(t *testing.T, statements []ast.Stmt, form string, line int) ast.PositionHolder {
	t.Helper()
	var matches []ast.PositionHolder
	valuesWalkStatements(statements, func(node ast.PositionHolder, nodeForm string) {
		if nodeForm == form && node.Line() == line {
			matches = append(matches, node)
		}
	})
	if len(matches) != 1 {
		t.Fatalf("AST target %s at line %d has %d matches, want exactly one", form, line, len(matches))
	}
	return matches[0]
}

func valuesFieldTarget(target ast.PositionHolder, statements []ast.Stmt) (*ast.Field, bool, bool) {
	want, ok := target.(*ast.Field)
	if !ok {
		return nil, false, false
	}
	var final bool
	var seen int
	valuesWalkTableFields(statements, func(field *ast.Field, isFinal bool) {
		if field == want {
			seen++
			final = isFinal
		}
	})
	return want, final, seen == 1
}

func valuesWalkStatements(statements []ast.Stmt, visit func(ast.PositionHolder, string)) {
	for _, stmt := range statements {
		valuesWalkStmt(stmt, visit)
	}
}

func valuesWalkStmt(stmt ast.Stmt, visit func(ast.PositionHolder, string)) {
	switch current := stmt.(type) {
	case *ast.AssignStmt:
		visit(current, "AssignStmt")
		for _, expr := range current.Lhs {
			valuesWalkExpr(expr, visit)
		}
		for _, expr := range current.Rhs {
			valuesWalkExpr(expr, visit)
		}
	case *ast.LocalAssignStmt:
		for _, expr := range current.Exprs {
			valuesWalkExpr(expr, visit)
		}
	case *ast.FuncCallStmt:
		valuesWalkExpr(current.Expr, visit)
	case *ast.DoBlockStmt:
		valuesWalkStatements(current.Stmts, visit)
	case *ast.WhileStmt:
		valuesWalkExpr(current.Condition, visit)
		valuesWalkStatements(current.Stmts, visit)
	case *ast.RepeatStmt:
		valuesWalkStatements(current.Stmts, visit)
		valuesWalkExpr(current.Condition, visit)
	case *ast.IfStmt:
		valuesWalkExpr(current.Condition, visit)
		valuesWalkStatements(current.Then, visit)
		valuesWalkStatements(current.Else, visit)
	case *ast.NumberForStmt:
		valuesWalkExpr(current.Init, visit)
		valuesWalkExpr(current.Limit, visit)
		valuesWalkExpr(current.Step, visit)
		valuesWalkStatements(current.Stmts, visit)
	case *ast.GenericForStmt:
		for _, expr := range current.Exprs {
			valuesWalkExpr(expr, visit)
		}
		valuesWalkStatements(current.Stmts, visit)
	case *ast.FuncDefStmt:
		valuesWalkExpr(current.Name.Func, visit)
		valuesWalkExpr(current.Name.Receiver, visit)
		valuesWalkExpr(current.Func, visit)
	case *ast.ReturnStmt:
		visit(current, "ReturnStmt")
		for _, expr := range current.Exprs {
			valuesWalkExpr(expr, visit)
		}
	}
}

func valuesWalkExpr(expr ast.Expr, visit func(ast.PositionHolder, string)) {
	if expr == nil {
		return
	}
	switch current := expr.(type) {
	case *ast.NilExpr:
		visit(current, "NilExpr")
	case *ast.FalseExpr:
		visit(current, "FalseExpr")
	case *ast.TrueExpr:
		visit(current, "TrueExpr")
	case *ast.NumberExpr:
		visit(current, "NumberExpr")
	case *ast.StringExpr:
		visit(current, "StringExpr")
	case *ast.Comma3Expr:
		visit(current, "Comma3Expr")
	case *ast.IdentExpr:
		visit(current, "IdentExpr")
	case *ast.AttrGetExpr:
		visit(current, "AttrGetExpr")
		valuesWalkExpr(current.Object, visit)
		valuesWalkExpr(current.Key, visit)
	case *ast.TableExpr:
		visit(current, "TableExpr")
		for _, field := range current.Fields {
			valuesWalkField(field, visit)
		}
	case *ast.FuncCallExpr:
		valuesWalkExpr(current.Func, visit)
		valuesWalkExpr(current.Receiver, visit)
		for _, arg := range current.Args {
			valuesWalkExpr(arg, visit)
		}
	case *ast.LogicalOpExpr:
		valuesWalkExpr(current.Lhs, visit)
		valuesWalkExpr(current.Rhs, visit)
	case *ast.RelationalOpExpr:
		valuesWalkExpr(current.Lhs, visit)
		valuesWalkExpr(current.Rhs, visit)
	case *ast.StringConcatOpExpr:
		valuesWalkExpr(current.Lhs, visit)
		valuesWalkExpr(current.Rhs, visit)
	case *ast.ArithmeticOpExpr:
		valuesWalkExpr(current.Lhs, visit)
		valuesWalkExpr(current.Rhs, visit)
	case *ast.UnaryMinusOpExpr:
		valuesWalkExpr(current.Expr, visit)
	case *ast.UnaryNotOpExpr:
		valuesWalkExpr(current.Expr, visit)
	case *ast.UnaryLenOpExpr:
		valuesWalkExpr(current.Expr, visit)
	case *ast.UnaryBNotOpExpr:
		valuesWalkExpr(current.Expr, visit)
	case *ast.FunctionExpr:
		valuesWalkStatements(current.Stmts, visit)
	case *ast.CastExpr:
		valuesWalkExpr(current.Expr, visit)
	}
}

func valuesWalkField(field *ast.Field, visit func(ast.PositionHolder, string)) {
	if field == nil {
		return
	}
	visit(field, "Field")
	valuesWalkExpr(field.Key, visit)
	valuesWalkExpr(field.Value, visit)
}

func valuesWalkTableFields(statements []ast.Stmt, visit func(*ast.Field, bool)) {
	var walkStmt func(ast.Stmt)
	var walkExpr func(ast.Expr)
	walkStmt = func(stmt ast.Stmt) {
		switch current := stmt.(type) {
		case *ast.AssignStmt:
			for _, expr := range current.Lhs {
				walkExpr(expr)
			}
			for _, expr := range current.Rhs {
				walkExpr(expr)
			}
		case *ast.LocalAssignStmt:
			for _, expr := range current.Exprs {
				walkExpr(expr)
			}
		case *ast.FuncCallStmt:
			walkExpr(current.Expr)
		case *ast.DoBlockStmt:
			for _, child := range current.Stmts {
				walkStmt(child)
			}
		case *ast.WhileStmt:
			walkExpr(current.Condition)
			for _, child := range current.Stmts {
				walkStmt(child)
			}
		case *ast.RepeatStmt:
			for _, child := range current.Stmts {
				walkStmt(child)
			}
			walkExpr(current.Condition)
		case *ast.IfStmt:
			walkExpr(current.Condition)
			for _, child := range current.Then {
				walkStmt(child)
			}
			for _, child := range current.Else {
				walkStmt(child)
			}
		case *ast.NumberForStmt:
			walkExpr(current.Init)
			walkExpr(current.Limit)
			walkExpr(current.Step)
			for _, child := range current.Stmts {
				walkStmt(child)
			}
		case *ast.GenericForStmt:
			for _, expr := range current.Exprs {
				walkExpr(expr)
			}
			for _, child := range current.Stmts {
				walkStmt(child)
			}
		case *ast.FuncDefStmt:
			walkExpr(current.Name.Func)
			walkExpr(current.Name.Receiver)
			walkExpr(current.Func)
		case *ast.ReturnStmt:
			for _, expr := range current.Exprs {
				walkExpr(expr)
			}
		}
	}
	walkExpr = func(expr ast.Expr) {
		if expr == nil {
			return
		}
		switch current := expr.(type) {
		case *ast.AttrGetExpr:
			walkExpr(current.Object)
			walkExpr(current.Key)
		case *ast.TableExpr:
			for index, field := range current.Fields {
				visit(field, index == len(current.Fields)-1)
				walkExpr(field.Key)
				walkExpr(field.Value)
			}
		case *ast.FuncCallExpr:
			walkExpr(current.Func)
			walkExpr(current.Receiver)
			for _, arg := range current.Args {
				walkExpr(arg)
			}
		case *ast.LogicalOpExpr:
			walkExpr(current.Lhs)
			walkExpr(current.Rhs)
		case *ast.RelationalOpExpr:
			walkExpr(current.Lhs)
			walkExpr(current.Rhs)
		case *ast.StringConcatOpExpr:
			walkExpr(current.Lhs)
			walkExpr(current.Rhs)
		case *ast.ArithmeticOpExpr:
			walkExpr(current.Lhs)
			walkExpr(current.Rhs)
		case *ast.UnaryMinusOpExpr:
			walkExpr(current.Expr)
		case *ast.UnaryNotOpExpr:
			walkExpr(current.Expr)
		case *ast.UnaryLenOpExpr:
			walkExpr(current.Expr)
		case *ast.UnaryBNotOpExpr:
			walkExpr(current.Expr)
		case *ast.FunctionExpr:
			for _, child := range current.Stmts {
				walkStmt(child)
			}
		case *ast.CastExpr:
			walkExpr(current.Expr)
		}
	}
	for _, stmt := range statements {
		walkStmt(stmt)
	}
}
