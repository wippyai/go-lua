package value_test

import (
	"context"
	"math"
	"reflect"
	"testing"

	value "github.com/wippyai/go-lua/analysis/domain/value"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/registry"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/program/link"
	programlower "github.com/wippyai/go-lua/analysis/program/lower"
	"github.com/wippyai/go-lua/analysis/program/target"
)

type sourceLiteralScenario struct {
	name   string
	family string
	source string
	count  func(*program.Program) int
	at     func(*program.Program, int) (program.Term, bool)
	want   func(*axis.Registry) product.Value
}

func TestSourceLiteralSemantics(t *testing.T) {
	const (
		familyDenominator  = 5
		fixtureDenominator = 6
	)
	cases := []sourceLiteralScenario{
		{
			name: "nil", family: "nil", source: "local value = nil",
			count: (*program.Program).NilCount, at: (*program.Program).NilAt,
			want: typevalue.Nil,
		},
		{
			name: "false", family: "bool", source: "local value = false",
			count: (*program.Program).BoolCount, at: (*program.Program).BoolAt,
			want: func(registry *axis.Registry) product.Value { return typevalue.LiteralBool(registry, false) },
		},
		{
			name: "true", family: "bool", source: "local value = true",
			count: (*program.Program).BoolCount, at: (*program.Program).BoolAt,
			want: func(registry *axis.Registry) product.Value { return typevalue.LiteralBool(registry, true) },
		},
		{
			name: "integer", family: "integer", source: "local value = 41",
			count: (*program.Program).IntegerCount, at: (*program.Program).IntegerAt,
			want: func(registry *axis.Registry) product.Value { return typevalue.LiteralInt(registry, 41) },
		},
		{
			name: "float", family: "float", source: "local value = 3.5",
			count: (*program.Program).FloatCount, at: (*program.Program).FloatAt,
			want: func(registry *axis.Registry) product.Value { return typevalue.LiteralNumber(registry, 3.5) },
		},
		{
			name: "string", family: "string", source: "local value = \"literal\"",
			count: (*program.Program).StringCount, at: (*program.Program).StringAt,
			want: func(registry *axis.Registry) product.Value { return typevalue.LiteralString(registry, "literal") },
		},
	}
	if len(cases) != fixtureDenominator {
		t.Fatalf("source literal fixtures = %d, want %d", len(cases), fixtureDenominator)
	}
	families := make(map[string]struct{}, len(cases))
	for _, scenario := range cases {
		families[scenario.family] = struct{}{}
	}
	if len(families) != familyDenominator {
		t.Fatalf("source literal families = %d, want %d", len(families), familyDenominator)
	}
	for _, scenario := range cases {
		scenario := scenario
		t.Run(scenario.name, func(t *testing.T) {
			project, shard, source := sourceProject(t, scenario.name+".lua", scenario.source)
			if got := scenario.count(source); got != 1 {
				t.Fatalf("%s typed source rows = %d, want 1", scenario.name, got)
			}
			term, ok := scenario.at(source, 0)
			if !ok {
				t.Fatalf("%s literal term", scenario.name)
			}
			got := solveSourceLiteral(t, project, shard, term)
			if want := scenario.want(registry.Registry()); !product.Equal(registry.Registry(), got, want) {
				t.Fatalf("%s source literal value differs from its typed Program payload", scenario.name)
			}
		})
	}
	t.Logf("source literal families: %d/%d; fixtures: %d", familyDenominator, familyDenominator, fixtureDenominator)
}

func TestSourceLiteralSemanticsKeepsEqualOccurrencesDistinct(t *testing.T) {
	project, shard, source := sourceProject(t, "duplicates.lua", "local first = 7\nlocal second = 7")
	if got := source.IntegerCount(); got != 2 {
		t.Fatalf("integer source rows = %d, want 2", got)
	}
	first, ok := source.IntegerAt(0)
	if !ok {
		t.Fatal("first integer")
	}
	second, ok := source.IntegerAt(1)
	if !ok {
		t.Fatal("second integer")
	}
	if first == second {
		t.Fatal("equal source payloads must retain distinct Program occurrences")
	}

	solver, err := engine.New(project)
	if err != nil {
		t.Fatal(err)
	}
	domain, ok := value.Install(solver, project)
	if !ok {
		t.Fatal("install source literal semantics")
	}
	left, ok := domain.Query(shard, first)
	if !ok {
		t.Fatal("query first literal")
	}
	right, ok := domain.Query(shard, second)
	if !ok {
		t.Fatal("query second literal")
	}
	if !solver.Seal() {
		t.Fatal("seal duplicate literal semantics")
	}
	state, ok := solver.Solve(context.Background(), nil)
	if !ok || state == nil {
		t.Fatal("solve duplicate literal semantics")
	}
	want := typevalue.LiteralInt(registry.Registry(), 7)
	for name, query := range map[string]*engine.Query[uint64, product.Value]{"first": left, "second": right} {
		got, present := query.Read(state)
		if !present || !product.Equal(registry.Registry(), got, want) {
			t.Fatalf("%s equal literal query = present %t, want its exact literal fact", name, present)
		}
	}
}

func TestSourceLiteralSemanticsRejectsUnsupportedTerms(t *testing.T) {
	project, shard, source := sourceProject(t, "unsupported.lua", "type Subject = string\nlocal converted = string(1)\nlocal table = {}")
	solver, err := engine.New(project)
	if err != nil {
		t.Fatal(err)
	}
	domain, ok := value.Install(solver, project)
	if !ok {
		t.Fatal("install source literal semantics")
	}

	values, ok := source.ValuesAt(0)
	if !ok {
		t.Fatal("values pack")
	}
	cell, ok := source.CellAt(0)
	if !ok {
		t.Fatal("cell")
	}
	table, ok := source.TableAt(0)
	if !ok {
		t.Fatal("table allocation")
	}
	call, ok := source.CallAt(0)
	if !ok {
		t.Fatal("call")
	}
	typeValue, ok := source.TypeValueAt(0)
	if !ok {
		t.Fatal("runtime TypeValue")
	}
	static, ok := source.PrimitiveAt(0)
	if !ok {
		t.Fatal("static primitive")
	}
	for name, term := range map[string]program.Term{
		"pack": values, "cell": cell, "table": table, "call": call, "type value": typeValue, "static": static,
	} {
		if query, accepted := domain.Query(shard, term); accepted || query != nil {
			t.Fatalf("%s term %v was accepted by source literal semantics", name, term)
		}
	}

	nestedProject, nestedShard, nested := sourceProject(t, "nested.lua", "local function nested() return 1 end")
	nestedSolver, err := engine.New(nestedProject)
	if err != nil {
		t.Fatal(err)
	}
	nestedDomain, ok := value.Install(nestedSolver, nestedProject)
	if !ok {
		t.Fatal("install nested source literal semantics")
	}
	nestedLiteral, ok := nested.IntegerAt(0)
	if !ok {
		t.Fatal("nested integer")
	}
	if query, accepted := nestedDomain.Query(nestedShard, nestedLiteral); accepted || query != nil {
		t.Fatal("non-Entry literal was accepted as a root query")
	}
}

func TestSourceLiteralSemanticsMinusZeroStaysUnaryOutsideTranche(t *testing.T) {
	project, shard, source := sourceProject(t, "minus-zero.lua", "local value = -0.0")
	if source.FloatCount() != 1 || source.UnaryCount() != 1 {
		t.Fatalf("minus-zero Float/Unary rows = %d/%d, want 1/1", source.FloatCount(), source.UnaryCount())
	}
	unary, ok := source.UnaryAt(0)
	if !ok {
		t.Fatal("minus-zero Unary")
	}
	_, operation, operand, ok := source.Unary(unary)
	if !ok || operation != program.UnaryNeg {
		t.Fatalf("minus-zero Unary = %v/%t, want UnaryNeg", operation, ok)
	}
	_, number, ok := source.Float(operand)
	if !ok || math.Float64bits(number) != math.Float64bits(0) {
		t.Fatalf("minus-zero operand Float = %x/%t, want canonical +0", math.Float64bits(number), ok)
	}

	solver, err := engine.New(project)
	if err != nil {
		t.Fatal(err)
	}
	domain, ok := value.Install(solver, project)
	if !ok {
		t.Fatal("install source literal semantics")
	}
	if query, accepted := domain.Query(shard, unary); accepted || query != nil {
		t.Fatal("Unary negation was admitted as a source literal")
	}
	query, ok := domain.Query(shard, operand)
	if !ok {
		t.Fatal("canonical +0 operand was not admitted")
	}
	if !solver.Seal() {
		t.Fatal("seal canonical +0 literal")
	}
	state, ok := solver.Solve(context.Background(), nil)
	if !ok || state == nil {
		t.Fatal("solve canonical +0 literal")
	}
	got, present := query.Read(state)
	want := typevalue.LiteralNumber(registry.Registry(), 0)
	if !present || !product.Equal(registry.Registry(), got, want) {
		t.Fatalf("canonical +0 literal = present %t, want exact +0 fact", present)
	}
}

func TestSourceLiteralEquationCacheIgnoresUnrelatedModules(t *testing.T) {
	main := lowerSource(t, "main.lua", "local value = 1")
	unrelated := lowerSource(t, "unrelated.lua", "local other = 2")

	cacheFor := func(modules []link.Module) artifact.EquationCache {
		project := sourceLink(t, modules)
		shard := shardForProgram(t, project, main)
		term, ok := main.IntegerAt(0)
		if !ok {
			t.Fatal("main literal")
		}
		solver, err := engine.New(project)
		if err != nil {
			t.Fatal(err)
		}
		domain, ok := value.Install(solver, project)
		if !ok {
			t.Fatal("install source literal semantics")
		}
		if _, ok := domain.Query(shard, term); !ok || !solver.Seal() {
			t.Fatal("seal source literal cache")
		}
		cache, ok := solver.EquationCache(shard)
		if !ok {
			t.Fatal("source literal equation cache")
		}
		return cache
	}

	base := cacheFor([]link.Module{{Name: "main", Program: main}})
	for _, candidate := range []artifact.EquationCache{
		cacheFor([]link.Module{{Name: "main", Program: main}, {Name: "unrelated", Program: unrelated}}),
		cacheFor([]link.Module{{Name: "unrelated", Program: unrelated}, {Name: "main", Program: main}}),
	} {
		if !reflect.DeepEqual(base.Factors, candidate.Factors) || !reflect.DeepEqual(base.Rules, candidate.Rules) || !reflect.DeepEqual(base.Boundary, candidate.Boundary) {
			t.Fatal("unrelated modules changed source literal equation cache identities")
		}
	}
}

func solveSourceLiteral(t *testing.T, project *link.Link, shard link.Shard, term program.Term) product.Value {
	t.Helper()
	solver, err := engine.New(project)
	if err != nil {
		t.Fatal(err)
	}
	domain, ok := value.Install(solver, project)
	if !ok {
		t.Fatal("install source literal semantics")
	}
	query, ok := domain.Query(shard, term)
	if !ok {
		t.Fatalf("query source literal %v", term)
	}
	if !solver.Seal() {
		t.Fatal("seal source literal semantics")
	}
	state, ok := solver.Solve(context.Background(), nil)
	if !ok || state == nil {
		t.Fatal("solve source literal semantics")
	}
	got, ok := query.Read(state)
	if !ok {
		t.Fatal("read source literal semantics")
	}
	return got
}

func sourceProject(t *testing.T, name, text string) (*link.Link, link.Shard, *program.Program) {
	t.Helper()
	source := lowerSource(t, name, text)
	project := sourceLink(t, []link.Module{{Name: name, Program: source}})
	return project, shardForProgram(t, project, source), source
}

func lowerSource(t *testing.T, name, text string) *program.Program {
	t.Helper()
	source, err := programlower.Lower(programlower.Source{Name: name, Text: []byte(text)})
	if err != nil {
		t.Fatalf("lower %s: %v", name, err)
	}
	return source
}

func sourceLink(t *testing.T, modules []link.Module) *link.Link {
	t.Helper()
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	project, err := link.Seal(&link.Spec{Target: contract, Modules: modules})
	if err != nil {
		t.Fatal(err)
	}
	return project
}

func shardForProgram(t *testing.T, project *link.Link, source *program.Program) link.Shard {
	t.Helper()
	for index := 0; index < project.ShardCount(); index++ {
		shard, ok := project.ShardAt(index)
		if !ok {
			t.Fatal("project shard")
		}
		candidate, ok := project.Program(shard)
		if ok && candidate == source {
			return shard
		}
	}
	t.Fatal("source shard")
	return 0
}
