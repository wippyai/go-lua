package lower_test

import (
	"math"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/flow"
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
)

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
