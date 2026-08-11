package values_test

import (
	"testing"

	typedomain "github.com/wippyai/go-lua/analysis/domain/type"
	"github.com/wippyai/go-lua/analysis/domain/type/factor/internal/carrier"
	"github.com/wippyai/go-lua/analysis/domain/type/factor/internal/origin"
	"github.com/wippyai/go-lua/analysis/domain/type/factor/internal/values"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/link"
	programlower "github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestEquationScalarizesFixedCarrierInputs(t *testing.T) {
	source, shard, p := linked(t, `return 1, 2`)
	returned, ok := p.ReturnAt(0)
	if !ok {
		t.Fatal("Return absent")
	}
	valuesTerm, ok := p.ReturnValues(returned)
	if !ok {
		t.Fatal("Return Values absent")
	}
	result, ok := source.ValueOf(shard, valuesTerm)
	if !ok {
		t.Fatal("Values result unavailable")
	}
	equations, ok := values.Build(source)
	if !ok {
		t.Fatal("Values Build")
	}
	equation, ok := find(equations, result)
	if !ok || equation.FixedCount() != 2 {
		t.Fatalf("equation=%#v/%v fixed=%d", equation, ok, equation.FixedCount())
	}
	first, ok := equation.FixedAt(0)
	if !ok {
		t.Fatal("fixed input absent")
	}
	second, ok := equation.FixedAt(1)
	if !ok {
		t.Fatal("second fixed input absent")
	}

	table, err := typedomain.NewTable(nil)
	if err != nil {
		t.Fatal(err)
	}
	integer, err := table.DeriveClosed(typ.LiteralInt(1))
	if err != nil {
		t.Fatal(err)
	}
	secondInteger, err := table.DeriveClosed(typ.LiteralInt(2))
	if err != nil {
		t.Fatal(err)
	}
	table.Seal()
	universe, ok := origin.Build(source)
	if !ok {
		t.Fatal("origin universe")
	}
	fixed, ok := table.Closed(integer)
	if !ok {
		t.Fatal("fixed Pack")
	}
	secondPack, ok := table.Closed(secondInteger)
	if !ok {
		t.Fatal("second Pack")
	}
	empty, ok := table.Closed()
	if !ok {
		t.Fatal("empty Pack")
	}
	var seen [2]bool
	reads := 0
	got, ok := equation.Evaluate(table, universe, empty, func(index int, key link.Value) (carrier.Value, bool) {
		if index < 0 || index >= len(seen) || seen[index] {
			t.Fatalf("group read index=%d seen=%v", index, seen)
		}
		seen[index] = true
		reads++
		switch key {
		case first:
			return mustCarrier(t, table, universe, fixed), true
		case second:
			return mustCarrier(t, table, universe, secondPack), true
		default:
			return carrier.Value{}, false
		}
	}, make([]typedomain.Pack, equation.FixedCount()))
	if !ok {
		t.Fatal("evaluate")
	}
	if reads != 2 || !seen[0] || !seen[1] {
		t.Fatalf("group reads=%d seen=%v", reads, seen)
	}
	pack, ok := got.Data()
	if !ok {
		t.Fatal("finite Values carrier")
	}
	if witnesses, ok := got.Origins(); !ok || witnesses.Count() != 0 {
		t.Fatalf("Values propagated origins=%d/%v", witnesses.Count(), ok)
	}
	mode := pack.Modes()
	if len(mode) != 1 || mode[0].Kind() != typedomain.ModeClosed || mode[0].ClosedLen() != 2 {
		t.Fatalf("assembled modes=%#v", mode)
	}
	for index, want := range []typedomain.Handle{integer, secondInteger} {
		actual, ok := mode[0].ClosedAt(index)
		if !ok || actual != want {
			t.Fatalf("Pack[%d]=%v/%v, want %v", index, actual, ok, want)
		}
	}
}

func TestEquationRejectsTooSmallScratch(t *testing.T) {
	source, shard, p := linked(t, `return 1, 2`)
	returned, _ := p.ReturnAt(0)
	valuesTerm, _ := p.ReturnValues(returned)
	result, _ := source.ValueOf(shard, valuesTerm)
	equations, _ := values.Build(source)
	equation, ok := find(equations, result)
	if !ok || equation.FixedCount() != 2 {
		t.Fatal("two-fixed equation absent")
	}
	table, err := typedomain.NewTable(nil)
	if err != nil {
		t.Fatal(err)
	}
	table.Seal()
	universe, ok := origin.Build(source)
	if !ok {
		t.Fatal("origin universe")
	}
	empty, ok := table.Closed()
	if !ok {
		t.Fatal("empty Pack")
	}
	if _, ok := equation.Evaluate(table, universe, empty, func(int, link.Value) (carrier.Value, bool) {
		return carrier.Value{}, true
	}, nil); ok {
		t.Fatal("undersized scratch accepted")
	}
}

func mustCarrier(t testing.TB, table *typedomain.Table, universe *origin.Universe, pack typedomain.Pack) carrier.Value {
	t.Helper()
	value, ok := carrier.New(table, universe, pack, origin.Empty())
	if !ok {
		t.Fatal("carrier")
	}
	return value
}

func find(set *values.Equations, wanted link.Value) (values.Equation, bool) {
	for index := 0; index < set.Count(); index++ {
		equation, ok := set.At(index)
		if ok && equation.Result() == wanted {
			return equation, true
		}
	}
	return values.Equation{}, false
}

func linked(t testing.TB, text string) (*link.Link, link.Shard, *program.Program) {
	t.Helper()
	p, err := programlower.Lower(programlower.Source{Name: "values.lua", Text: []byte(text)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	source, err := link.Seal(&link.Spec{Target: contract, Modules: []link.Module{{Name: "values", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	shard, ok := source.ShardAt(0)
	if !ok {
		t.Fatal("missing shard")
	}
	return source, shard, p
}
