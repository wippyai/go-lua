package acceptance_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func TestSourceValuesContextualDotNames(t *testing.T) {
	p := parseBindLower(t, `local t = {}
return
t.type,
t.interface,
t.readonly,
t.as,
t.asserts,
t.is`)
	names := []string{"type", "interface", "readonly", "as", "asserts", "is"}
	lenses := p.Flow().Authored().Access().Exact()
	if lenses.Count() != len(names) {
		t.Fatalf("contextual dot Lens count = %d, want %d", lenses.Count(), len(names))
	}
	for index, name := range names {
		lens, ok := lenses.At(index)
		if !ok {
			t.Fatalf("contextual dot LensExact[%d] is absent", index)
		}
		_, _, keySource, fieldKind, ok := lenses.Get(lens)
		_, _, exact, exactOK := p.Source().Keys().Name(keySource)
		if !ok || fieldKind != flowkind.FieldName || !exactOK || exact == 0 {
			t.Fatalf("contextual dot Lens[%d] = source %v kind %v exact %v/%v ok %v", index, keySource, fieldKind, exact, exactOK, ok)
		}
		_, text, sourceExact, ok := p.Source().Keys().Name(keySource)
		if !ok || text != name || sourceExact != exact {
			t.Fatalf("contextual dot Name[%d] = %q/%v/%v, want %q/%v", index, text, sourceExact, ok, name, exact)
		}
		line := index + 3
		valuesLawSpan(t, p, lens, line, 1, line, len(name)+2)
		valuesLawSpan(t, p, keySource, line, 3, line, len(name)+2)
	}
}

func TestSourceValuesContextualConstructorNames(t *testing.T) {
	p := parseBindLower(t, `return {
ident = 1,
type = 2,
interface = 3,
readonly = 4,
as = 5,
asserts = 6,
is = 7,
keyof = 8,
extends = 9;
}`)
	names := []string{"ident", "type", "interface", "readonly", "as", "asserts", "is", "keyof", "extends"}
	table := valuesLawReturnedTable(t, p)
	for index, name := range names {
		field, ok := p.Flow().Authored().Tables().FieldAt(table, index)
		if !ok {
			t.Fatalf("contextual constructor TableField[%d] is absent", index)
		}
		_, keySource, values, fieldKind, ok := p.Flow().Authored().Fields().Get(field)
		_, _, exact, exactOK := p.Source().Keys().Name(keySource)
		if !ok || fieldKind != flowkind.FieldName || !exactOK || exact == 0 {
			t.Fatalf("contextual constructor TableField[%d] = key %v values %v kind %v exact %v/%v ok %v", index, keySource, values, fieldKind, exact, exactOK, ok)
		}
		_, text, sourceExact, ok := p.Source().Keys().Name(keySource)
		if !ok || text != name || sourceExact != exact {
			t.Fatalf("contextual constructor Name[%d] = %q/%v/%v, want %q/%v", index, text, sourceExact, ok, name, exact)
		}
		value := valueAt(t, p, values, 0)
		if literal, _, number, ok := p.Source().Literals().Integers().At(int(keyspace.TermOrdinal(value)) - 1); !ok || literal != value || number != int64(index+1) {
			t.Fatalf("contextual constructor value[%d] = %d/%v, want %d", index, number, ok, index+1)
		}
		line := index + 2
		valuesLawSpan(t, p, field, line, 1, line, len(name)+4)
		valuesLawSpan(t, p, keySource, line, 1, line, len(name))
	}
}

func TestSourceValuesLiteralOccurrencesRemainDistinct(t *testing.T) {
	p := parseBindLower(t, `return nil, false, true, 42, 1.5, "value", 42`)
	entry, ok := p.Flow().Body().Entry()
	if !ok {
		t.Fatal("Program has no Entry")
	}
	returned, ok := p.Source().Order().BodyAt(entry, 0)
	if !ok {
		t.Fatal("Entry has no Return")
	}
	_, values, ok := p.Flow().Authored().Control().Returns().Get(returned)
	if !ok || valuesTail(t, p, values) != 0 {
		t.Fatalf("Return Values = %v/%v tail %v, want closed", values, ok, valuesTail(t, p, values))
	}
	if width, ok := p.Flow().Authored().Values().Len(values); !ok || width != 7 {
		t.Fatalf("literal Values width = %d/%v, want 7", width, ok)
	}
	literals := p.Source().Literals()
	if literals.Nils().Count() != 1 || literals.Bools().Count() != 2 || literals.Integers().Count() != 2 || literals.Floats().Count() != 1 || literals.Strings().Count() != 1 {
		t.Fatalf("literal families Nil/Bool/Integer/Float/String = %d/%d/%d/%d/%d, want 1/2/2/1/1", literals.Nils().Count(), literals.Bools().Count(), literals.Integers().Count(), literals.Floats().Count(), literals.Strings().Count())
	}
	first, _, _, _ := literals.Integers().At(0)
	second, _, _, _ := literals.Integers().At(1)
	if first == second {
		t.Fatal("equal authored integer occurrences were shared")
	}
	if literal, _, number, ok := literals.Integers().At(int(keyspace.TermOrdinal(first)) - 1); !ok || literal != first || number != 42 {
		t.Fatalf("first Integer = %d/%v, want 42", number, ok)
	}
	valuesLawSpan(t, p, first, 1, 26, 1, 27)
	valuesLawSpan(t, p, second, 1, 44, 1, 45)
	if next, ok := p.Flow().Ports().Finish(first); !ok || next == 0 {
		t.Fatalf("first Integer has no sealed successor: %v/%v", next, ok)
	}
}

func TestSourceValuesDotLensHasNoKeyEvaluation(t *testing.T) {
	p := parseBindLower(t, "local t = {}; return t.name")
	flow := p.Flow()
	lenses := flow.Authored().Access().Exact()
	lens, ok := lenses.At(0)
	if !ok {
		t.Fatal("dot source has no LensExact")
	}
	entry, ok := p.Flow().Body().Entry()
	if !ok {
		t.Fatal("Program has no Entry")
	}
	owner, base, keySource, fieldKind, ok := lenses.Get(lens)
	nameOwner, name, key, keyOK := p.Source().Keys().Name(keySource)
	if !ok || owner != entry || base == 0 || fieldKind != flowkind.FieldName || !keyOK || key == 0 {
		t.Fatalf("dot Lens = owner %v base %v source %v kind %v key %v/%v ok %v", owner, base, keySource, fieldKind, key, keyOK, ok)
	}
	if nameOwner != entry || name != "name" {
		t.Fatalf("dot Name = owner %v name %q key %v", nameOwner, name, key)
	}
	if value, ok := flow.Ports().Entry(lens); !ok || value != base {
		t.Fatalf("dot Lens base entry = %v/%v, want %v", value, ok, base)
	}
	valuesLawSpan(t, p, lens, 1, 22, 1, 27)
	valuesLawSpan(t, p, keySource, 1, 24, 1, 27)
}

func TestSourceValuesBracketLensesKeepDistinctEvaluationPaths(t *testing.T) {
	p := parseBindLower(t, "local t, k = {}, 3; return t[1], t[k]")
	flow := p.Flow()
	exactLenses := flow.Authored().Access().Exact()
	dynamicLenses := flow.Authored().Access().Dynamic()
	exact, exactOK := exactLenses.At(0)
	dynamic, dynamicOK := dynamicLenses.At(0)
	if !exactOK || !dynamicOK {
		t.Fatalf("bracket lenses exact/dynamic = %v/%v, want both", exactOK, dynamicOK)
	}
	_, exactBase, exactSource, exactKind, exactOK := exactLenses.Get(exact)
	exactKey, exactKeyOK := exactLiteralKey(t, p, exactSource)
	if !exactOK || exactKind != flowkind.FieldExact || exactBase == 0 || exactSource == 0 || !exactKeyOK || exactKey == 0 {
		t.Fatalf("exact bracket Lens = base %v source %v kind %v key %v/%v ok %v", exactBase, exactSource, exactKind, exactKey, exactKeyOK, exactOK)
	}
	if base, ok := flow.Ports().Entry(exact); !ok || base != exactBase {
		t.Fatalf("exact Lens base entry = %v/%v, want %v", base, ok, exactBase)
	}
	_, dynamicBase, dynamicSource, dynamicOK := dynamicLenses.Get(dynamic)
	if !dynamicOK || dynamicBase == 0 || dynamicSource == 0 {
		t.Fatalf("dynamic bracket Lens = base %v source %v ok %v", dynamicBase, dynamicSource, dynamicOK)
	}
	if base, ok := flow.Ports().Entry(dynamic); !ok || base != dynamicBase {
		t.Fatalf("dynamic Lens base entry = %v/%v, want %v", base, ok, dynamicBase)
	}
	if next := unconditionalSuccessor(t, p, exactBase); next != exactSource {
		t.Fatalf("exact bracket base successor = %v, want key occurrence %v", next, exactSource)
	}
	if next := unconditionalSuccessor(t, p, exactSource); next != exact {
		t.Fatalf("exact bracket key successor = %v, want Lens %v", next, exact)
	}
	if next := unconditionalSuccessor(t, p, dynamicBase); next != dynamicSource {
		t.Fatalf("dynamic bracket base successor = %v, want key Read %v", next, dynamicSource)
	}
	if next := unconditionalSuccessor(t, p, dynamicSource); next != dynamic {
		t.Fatalf("dynamic bracket key successor = %v, want Lens %v", next, dynamic)
	}
}

func TestSourceValuesRawKeyFailuresRemainSeparate(t *testing.T) {
	p := parseBindLower(t, "return {[nil] = 1, [0 / 0] = 2, [k] = 3}")
	table := valuesLawReturnedTable(t, p)
	flow := p.Flow()
	tables := flow.Authored().Tables()
	fields := flow.Authored().Fields()
	keys := make([]keyspace.Term, 3)
	for index, want := range []flowkind.FieldKind{flowkind.FieldExact, flowkind.FieldKey, flowkind.FieldKey} {
		field, ok := tables.FieldAt(table, index)
		if !ok {
			t.Fatalf("TableFieldAtTable(%d) absent", index)
		}
		fieldTable, key, _, fieldKind, ok := fields.Get(field)
		_, normalized := exactLiteralKey(t, p, key)
		if !ok || key == 0 || fieldKind != want || normalized {
			t.Fatalf("raw key field[%d] = key %v kind %v normalized %v ok %v, want nonzero/%v/false", index, key, fieldKind, normalized, ok, want)
		}
		if _, _, _, ok := p.Source().Keys().Name(key); ok {
			t.Fatalf("raw key[%d] fabricated a static Name identity", index)
		}
		if _, _, _, ok := p.Source().Keys().List(key); ok {
			t.Fatalf("raw key[%d] fabricated a list identity", index)
		}
		body, bodyOK := tables.Get(fieldTable)
		throw, throwOK := flow.Outcomes().BodyExit(body, flowkind.OutcomeThrow)
		if !bodyOK || !throwOK || throw == 0 {
			t.Fatalf("raw key field[%d] has no Throw obligation: %v/%v", index, throw, ok)
		}
		keys[index] = key
	}
	if keys[0] == keys[1] || keys[0] == keys[2] || keys[1] == keys[2] {
		t.Fatalf("nil/NaN/dynamic source key occurrences collapsed: %v/%v/%v", keys[0], keys[1], keys[2])
	}
	if literal, _, ok := p.Source().Literals().Nils().At(int(keyspace.TermOrdinal(keys[0])) - 1); !ok || literal != keys[0] {
		t.Fatalf("nil exact key %v is not its source nil occurrence", keys[0])
	}
	if _, _, _, ok := p.Flow().Authored().Storage().Reads().Get(keys[2]); !ok {
		t.Fatalf("dynamic identifier key %v is not an evaluated Read occurrence", keys[2])
	}
}

func valuesLawReturnedTable(t *testing.T, p *program.Program) keyspace.Term {
	t.Helper()
	entry, ok := p.Flow().Body().Entry()
	if !ok {
		t.Fatal("Program has no Entry")
	}
	returned, ok := p.Source().Order().BodyAt(entry, 0)
	if !ok {
		t.Fatal("Entry has no Return")
	}
	_, values, ok := p.Flow().Authored().Control().Returns().Get(returned)
	if !ok {
		t.Fatal("Return has no Values")
	}
	table := valueAt(t, p, values, 0)
	if _, ok := p.Flow().Authored().Tables().Get(table); !ok {
		t.Fatalf("return value %v is not a Table", table)
	}
	return table
}

func valuesLawSpan(t *testing.T, p *program.Program, term keyspace.Term, startLine, startColumn, endLine, endColumn int) {
	t.Helper()
	got, ok := p.Source().Identity().Span(term)
	want := source.Span{File: "fixture.lua", StartLine: uint32(startLine), StartCol: uint32(startColumn), EndLine: uint32(endLine), EndCol: uint32(endColumn)}
	if !ok || got != want {
		t.Fatalf("term %v span = %#v/%v, want %#v", term, got, ok, want)
	}
}
