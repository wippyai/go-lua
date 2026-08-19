package acceptance_test

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	staticrefs "github.com/wippyai/go-lua/analysis/program/static/references"
	statictypes "github.com/wippyai/go-lua/analysis/program/static/types"
)

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
		if primitive, ok := p.Static().Types().Primitives().Get(target); !ok || primitive != statictypes.PrimitiveString {
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
		if !ok || state != staticrefs.Declaration || declaration == 0 {
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
