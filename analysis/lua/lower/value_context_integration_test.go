package lower_test

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
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
	entry, ok := p.Source().Index().Entry()
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
	entry, ok := p.Source().Index().Entry()
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
	if next, ok := flow.Ports().Finish(exactBase); !ok || next != exactSource {
		t.Fatalf("exact bracket base successor = %v/%v, want key occurrence %v", next, ok, exactSource)
	}
	if next, ok := flow.Ports().Finish(exactSource); !ok || next != exact {
		t.Fatalf("exact bracket key successor = %v/%v, want Lens %v", next, ok, exact)
	}
	if next, ok := flow.Ports().Finish(dynamicBase); !ok || next != dynamicSource {
		t.Fatalf("dynamic bracket base successor = %v/%v, want key Read %v", next, ok, dynamicSource)
	}
	if next, ok := flow.Ports().Finish(dynamicSource); !ok || next != dynamic {
		t.Fatalf("dynamic bracket key successor = %v/%v, want Lens %v", next, ok, dynamic)
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
	entry, ok := p.Source().Index().Entry()
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

func TestNonNilValueClaimLowersBeforeMemberAccess(t *testing.T) {
	p := parseBindLower(t, `
local function member(value)
  return (value!).name
end
`)
	flow := p.Flow()
	claims := flow.Authored().Claims()
	if got := claims.Count(); got != 1 {
		t.Fatalf("ValueClaimCount = %d, want 1", got)
	}
	claim, _ := claims.At(0)
	_, operand, claimKind, ok := claims.Get(claim)
	target, targetOK := p.Static().Operands().Claims().Target(claim)
	if !ok || claimKind != kind.ValueClaimNonNil || targetOK || target != 0 {
		t.Fatalf("ValueClaim = operand %v target %v/%v kind %v ok %v, want targetless NonNil", operand, target, targetOK, claimKind, ok)
	}
	if _, _, _, ok := flow.Authored().Storage().Reads().Get(operand); !ok {
		t.Fatalf("non-nil operand %v is not the member base Read", operand)
	}

	function, _ := flow.Authored().Functions().At(0)
	_, body, _, ok := flow.Authored().Functions().Get(function)
	if !ok {
		t.Fatal("missing member Function")
	}
	returned, ok := flow.Authored().Control().Returns().At(0)
	if !ok {
		t.Fatal("missing member Return")
	}
	owner, values, ok := flow.Authored().Control().Returns().Get(returned)
	if !ok || owner != body {
		t.Fatalf("member Return = owner %v ok %v, want body %v", owner, ok, body)
	}
	result := valueAt(t, p, values, 0)
	_, lens, _, ok := flow.Authored().Storage().Reads().Get(result)
	if !ok {
		t.Fatalf("member result %v is not a Read", result)
	}
	_, base, _, _, ok := flow.Authored().Access().Exact().Get(lens)
	if !ok || base != claim {
		t.Fatalf("member Lens base = %v (ok %v), want ValueClaim %v", base, ok, claim)
	}
}

func TestNonNilValueClaimMakesCallResultScalar(t *testing.T) {
	p := parseBindLower(t, `
local function source() return 1 end
local function scalar() return source()! end
`)
	flow := p.Flow()
	claims := flow.Authored().Claims()
	if got := claims.Count(); got != 1 {
		t.Fatalf("ValueClaimCount = %d, want 1", got)
	}
	claim, _ := claims.At(0)
	_, operand, claimKind, ok := claims.Get(claim)
	target, targetOK := p.Static().Operands().Claims().Target(claim)
	if !ok || claimKind != kind.ValueClaimNonNil || targetOK || target != 0 {
		t.Fatalf("ValueClaim = operand %v target %v/%v kind %v ok %v, want targetless NonNil", operand, target, targetOK, claimKind, ok)
	}
	if _, _, _, _, ok := flow.Authored().Calls().Get(operand); !ok {
		t.Fatalf("non-nil operand %v is not a Call", operand)
	}

	functions := flow.Authored().Functions()
	returns := flow.Authored().Control().Returns()
	valuesView := flow.Authored().Values()
	for index := 0; index < functions.Count(); index++ {
		function, _ := functions.At(index)
		_, body, _, functionOK := functions.Get(function)
		if !functionOK {
			t.Fatalf("FunctionAt(%d) is not a Function", index)
		}
		for returnIndex := 0; returnIndex < returns.Count(); returnIndex++ {
			returned, returnOK := returns.At(returnIndex)
			if !returnOK {
				continue
			}
			owner, values, isReturn := returns.Get(returned)
			if !isReturn || owner != body || valuesTail(t, p, values) != 0 {
				continue
			}
			if fixed, _ := valuesView.Len(values); fixed == 1 && valueAt(t, p, values, 0) == claim {
				return
			}
		}
	}
	t.Fatalf("non-nil Call result %v was not returned as one fixed scalar", claim)
}

func TestNestedValueClaimsPreserveExactOrder(t *testing.T) {
	p := parseBindLower(t, `
type Value = number
local value = (1 :: Value)!
`)
	flow := p.Flow()
	claims := flow.Authored().Claims()
	if got := claims.Count(); got != 2 {
		t.Fatalf("ValueClaimCount = %d, want 2", got)
	}
	inner, _ := claims.At(0)
	outer, _ := claims.At(1)
	_, _, innerKind, innerOK := claims.Get(inner)
	_, outerOperand, outerKind, outerOK := claims.Get(outer)
	innerTarget, innerTargetOK := p.Static().Operands().Claims().Target(inner)
	outerTarget, outerTargetOK := p.Static().Operands().Claims().Target(outer)
	if !innerOK || innerKind != kind.ValueClaimTypeColonColon || !innerTargetOK || innerTarget == 0 {
		t.Fatalf("inner ValueClaim = target %v/%v kind %v ok %v, want typed :: claim", innerTarget, innerTargetOK, innerKind, innerOK)
	}
	if !outerOK || outerKind != kind.ValueClaimNonNil || outerTargetOK || outerTarget != 0 || outerOperand != inner {
		t.Fatalf("outer ValueClaim = operand %v target %v/%v kind %v ok %v, want NonNil(inner)", outerOperand, outerTarget, outerTargetOK, outerKind, outerOK)
	}
	if next, ok := flow.Ports().Finish(inner); !ok || next != outer {
		t.Fatalf("inner ValueClaim successor = %v/%v, want outer %v", next, ok, outer)
	}
}

func TestCastExpressionsLowerToTypedValueClaims(t *testing.T) {
	p := parseBindLower(t, `
type Value = number
local value = 1
local asValue = value as Value
local colonValue = value :: Value
local sameValue = value as typeof(value)
`)
	claims := p.Flow().Authored().Claims()
	if got := claims.Count(); got != 3 {
		t.Fatalf("ValueClaimCount = %d, want 3", got)
	}
	kinds := []kind.ValueClaimKind{kind.ValueClaimTypeAs, kind.ValueClaimTypeColonColon, kind.ValueClaimTypeAs}
	for index, wantKind := range kinds {
		claim, ok := claims.At(index)
		if !ok {
			t.Fatalf("missing ValueClaim %d", index)
		}
		owner, operand, claimKind, ok := claims.Get(claim)
		target, targetOK := p.Static().Operands().Claims().Target(claim)
		if !ok || owner == 0 || operand == 0 || !targetOK || target == 0 || claimKind != wantKind {
			t.Fatalf("ValueClaim %d = owner %v operand %v target %v/%v kind %v ok %v", index, owner, operand, target, targetOK, claimKind, ok)
		}
		if index < 2 {
			state, declaration, _, refOK := p.Static().References().Get(target)
			if !refOK || state != static.TypeRefDeclaration || declaration == 0 {
				t.Fatalf("ValueClaim %d target = state %v declaration %v ok %v", index, state, declaration, refOK)
			}
			continue
		}
		scope, _, typeOfOK := p.Static().Operators().TypeOfs().Get(target)
		if !typeOfOK || scope != claim {
			t.Fatalf("cast typeof target scope = %v/%v, want %v/true", scope, typeOfOK, claim)
		}
	}
}

func directCallTypeValue(t *testing.T, p *program.Program) (keyspace.Term, keyspace.Term) {
	t.Helper()
	calls := p.Flow().Authored().Calls()
	if calls.Count() != 1 {
		t.Fatalf("CallCount = %d, want 1", calls.Count())
	}
	call, _ := calls.At(0)
	_, callee, _, _, ok := calls.Get(call)
	if !ok {
		t.Fatal("missing Call")
	}
	if _, typeValueOK := p.Flow().Authored().TypeValues().Get(callee); !typeValueOK {
		t.Fatalf("Call callee %v is not a Flow TypeValue", callee)
	}
	target, ok := p.Static().Operands().TypeValues().Target(callee)
	if !ok {
		t.Fatalf("Call callee %v is not TypeValue", callee)
	}
	return call, target
}

func TestRuntimeTypeValuesUseOrdinaryCallsAndExactStaticTargets(t *testing.T) {
	t.Run("primitive", func(t *testing.T) {
		p := parseBindLower(t, `
local value = 1
return string(value)
`)
		call, target := directCallTypeValue(t, p)
		if primitive, ok := p.Static().Types().Primitives().Get(target); !ok || primitive != static.PrimitiveString {
			t.Fatalf("TypeValue primitive = %v/%v, want string", primitive, ok)
		}
		if got := p.Flow().Authored().Claims().Count(); got != 0 {
			t.Fatalf("ValueClaimCount = %d, want 0", got)
		}
		if got := p.Flow().Authored().Storage().Reads().Count(); got != 1 {
			t.Fatalf("ReadCount = %d, want one argument Read and no base Read", got)
		}
		_, _, _, actuals, _ := p.Flow().Authored().Calls().Get(call)
		if fixed, ok := p.Flow().Authored().Values().Len(actuals); !ok || fixed != 1 || valuesTail(t, p, actuals) != 0 {
			t.Fatalf("Call actuals = fixed %d/%v tail %v, want one fixed runtime argument", fixed, ok, valuesTail(t, p, actuals))
		}
	})

	t.Run("declaration", func(t *testing.T) {
		p := parseBindLower(t, `
type Validator = number
local value = 1
return Validator(value)
`)
		_, target := directCallTypeValue(t, p)
		state, declaration, _, ok := p.Static().References().Get(target)
		if !ok || state != static.TypeRefDeclaration || declaration == 0 {
			t.Fatalf("TypeValue declaration target = %v/%v/%v, want exact declaration TypeRef", state, declaration, ok)
		}
	})

	t.Run("external-global", func(t *testing.T) {
		p := parseBindLower(t, `
local value = 1
return Remote(value)
		`)
		if got := p.Flow().Authored().TypeValues().Count(); got != 0 {
			t.Fatalf("TypeValueCount = %d, want none for ordinary external global", got)
		}
		calls := p.Flow().Authored().Calls()
		if got := calls.Count(); got != 1 {
			t.Fatalf("CallCount = %d, want one ordinary call", got)
		}
		call, _ := calls.At(0)
		_, callee, _, _, ok := calls.Get(call)
		if !ok {
			t.Fatal("missing ordinary external call")
		}
		_, source, _, ok := p.Flow().Authored().Storage().Reads().Get(callee)
		if !ok {
			t.Fatalf("external callee %v is not an ordinary Read", callee)
		}
		_, _, key, cellOK := p.Flow().Authored().Storage().Cells().Get(source)
		literal, keyOK := p.Source().Keys().Exact(key)
		if !cellOK || !keyOK || literal.Kind != keyspace.LiteralString || literal.String != "Remote" {
			t.Fatalf("external callee source = %q/%v, want global Remote", literal.String, cellOK && keyOK)
		}
		reads := p.Flow().Authored().Storage().Reads()
		if got := reads.ImplicitCount(); got != 1 {
			t.Fatalf("ImplicitReadCount = %d, want Remote only", got)
		}
		implicit, ok := reads.ImplicitAt(0)
		if !ok || implicit != callee {
			t.Fatalf("ImplicitReadAt(0) = %v/%v, want Remote callee %v", implicit, ok, callee)
		}
	})
}

func TestRuntimeTypeMethodFormsKeepOrdinaryTopology(t *testing.T) {
	methods := []string{"is", "kind", "name", "elem", "key", "val", "inner", "ret", "fields", "variants", "params", "tparams"}
	source := "type T = number\nlocal value = 1\n"
	for _, method := range methods {
		source += "T." + method + "(value)\n"
		source += "T[\"" + method + "\"](value)\n"
		source += "T:" + method + "(value)\n"
	}
	p := parseBindLower(t, source)
	flow := p.Flow()
	calls := flow.Authored().Calls()
	typeValues := flow.Authored().TypeValues()
	if want := len(methods) * 3; calls.Count() != want || typeValues.Count() != want {
		t.Fatalf("Calls/TypeValues = %d/%d, want %d/%d", calls.Count(), typeValues.Count(), want, want)
	}
	if flow.Authored().Claims().Count() != 0 {
		t.Fatalf("type-method calls created %d ValueClaims", flow.Authored().Claims().Count())
	}

	colon := 0
	for index := 0; index < calls.Count(); index++ {
		call, _ := calls.At(index)
		_, callee, receiver, actuals, ok := calls.Get(call)
		if !ok || callee == 0 || actuals == 0 {
			t.Fatalf("Call %d = callee %v actuals %v ok %v", index, callee, actuals, ok)
		}
		if fixed, ok := flow.Authored().Values().Len(actuals); !ok || fixed != 1 || valuesTail(t, p, actuals) != 0 {
			t.Fatalf("Call %d actuals = fixed %d/%v tail %v", index, fixed, ok, valuesTail(t, p, actuals))
		}
		if n, ok := p.Static().Contracts().Calls().TypeArgumentCount(call); !ok || n != 0 {
			t.Fatalf("Call %d TypeArgs = %d/%v, want explicit empty", index, n, ok)
		}
		_, lens, _, readOK := flow.Authored().Storage().Reads().Get(callee)
		if !readOK {
			t.Fatalf("Call %d callee %v is not ordinary Read(Lens)", index, callee)
		}
		_, base, _, _, lensOK := flow.Authored().Access().Exact().Get(lens)
		if !lensOK {
			t.Fatalf("Call %d callee source %v is not Lens", index, lens)
		}
		if _, valueOK := typeValues.Get(base); !valueOK {
			t.Fatalf("Call %d Lens base %v is not the one TypeValue", index, base)
		}
		if receiver != 0 {
			colon++
			if receiver != base {
				t.Fatalf("Call %d receiver = %v, want its once-evaluated Lens base %v", index, receiver, base)
			}
		}
	}
	if colon != len(methods) {
		t.Fatalf("colon Call count = %d, want %d", colon, len(methods))
	}

	ordinary := parseBindLower(t, `
local value = "a"
return string.rep(value, 2)
`)
	if ordinary.Flow().Authored().TypeValues().Count() != 0 || ordinary.Flow().Authored().Calls().Count() != 1 {
		t.Fatalf("ordinary string.rep TypeValues/Calls = %d/%d, want 0/1", ordinary.Flow().Authored().TypeValues().Count(), ordinary.Flow().Authored().Calls().Count())
	}
}

func TestUnmarkedTypeLikeValuesRemainOrdinaryReads(t *testing.T) {
	p := parseBindLower(t, `
type T = number
local plain = T
return plain
`)
	if p.Flow().Authored().TypeValues().Count() != 0 {
		t.Fatalf("plain value position created %d TypeValues", p.Flow().Authored().TypeValues().Count())
	}
	bind, ok := p.Flow().Authored().Storage().Binds().At(0)
	if !ok {
		t.Fatal("missing plain local Bind")
	}
	_, values, ok := p.Flow().Authored().Storage().Binds().Get(bind)
	if !ok {
		t.Fatalf("Bind %v is not a plain local Bind", bind)
	}
	plain := valueAt(t, p, values, 0)
	_, source, _, ok := p.Flow().Authored().Storage().Reads().Get(plain)
	if !ok {
		t.Fatalf("plain T value %v is not an ordinary Read", plain)
	}
	_, _, key, cellOK := p.Flow().Authored().Storage().Cells().Get(source)
	literal, keyOK := p.Source().Keys().Exact(key)
	if !cellOK || !keyOK || literal.Kind != keyspace.LiteralString || literal.String != "T" {
		t.Fatalf("plain T Read source = %q/%v, want global T", literal.String, cellOK && keyOK)
	}
}

func TestValueScopeShadowsAndNonAuthoritativeStaticNamesRemainCalls(t *testing.T) {
	tests := []string{
		`type T = number\nlocal T = function(v) return v end\nreturn T(1)`,
		`type T = number\nlocal f = function(T) return T(1) end\nreturn f`,
		`type T = number\nlocal T = function(v) return v end\nlocal f = function() return T(1) end\nreturn f`,
		`local f = function() type Nested = number return Nested(1) end\nreturn f`,
		`local f = function<T>() return T(1) end\nreturn f`,
	}
	for _, source := range tests {
		source := strings.ReplaceAll(source, "\\n", "\n")
		t.Run(source, func(t *testing.T) {
			p := parseBindLower(t, source)
			if p.Flow().Authored().TypeValues().Count() != 0 {
				t.Fatalf("TypeValueCount = %d, want ordinary value Call", p.Flow().Authored().TypeValues().Count())
			}
			if p.Flow().Authored().Calls().Count() == 0 {
				t.Fatal("ordinary type-like value did not retain a Call")
			}
		})
	}
}

func TestProgramValuesPositionLaws(t *testing.T) {
	p := parseBindLower(t, `
local function many(...) return ... end
local a, b, c = 1, many()
local d, e = 1
return a, b, c, d, e
`)
	var open, closed keyspace.Term
	flow := p.Flow()
	binds := flow.Authored().Storage().Binds()
	valuesView := flow.Authored().Values()
	for index := 0; index < binds.Count(); index++ {
		bind, ok := binds.At(index)
		if !ok {
			t.Fatal("bind")
		}
		_, values, ok := binds.Get(bind)
		if !ok {
			t.Fatal("Bind")
		}
		fixed, sized := valuesView.Len(values)
		_, tail, related := valuesView.Get(values)
		if !sized || !related {
			t.Fatal("Values")
		}
		if fixed == 1 && tail != 0 {
			open = values
		}
		if fixed == 1 && tail == 0 {
			closed = values
		}
	}
	if open == 0 || closed == 0 {
		t.Fatal("missing open/closed Values")
	}
	fixed, ok := valuesView.Position(open, 0)
	if !ok || fixed.Fixed == 0 || fixed.Tail != 0 || fixed.NilFill {
		t.Fatalf("fixed = %#v/%v", fixed, ok)
	}
	tail, ok := valuesView.Position(open, 3)
	if !ok || tail.Tail == 0 || tail.TailOffset != 2 || tail.Fixed != 0 || tail.NilFill {
		t.Fatalf("tail = %#v/%v", tail, ok)
	}
	nilFill, ok := valuesView.Position(closed, 1)
	if !ok || !nilFill.NilFill || nilFill.Fixed != 0 || nilFill.Tail != 0 {
		t.Fatalf("nil = %#v/%v", nilFill, ok)
	}
}
