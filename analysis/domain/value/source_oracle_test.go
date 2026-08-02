package value_test

import (
	"context"
	"reflect"
	"testing"

	value "github.com/wippyai/go-lua/analysis/domain/value"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/artifact"
	"github.com/wippyai/go-lua/program/link"
	programlower "github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

type sourceLiteralScenario struct {
	name   string
	source string
	count  func(*program.Program) int
	at     func(*program.Program, int) (program.Term, bool)
	want   func(*axis.Registry) product.Value
}

func TestSourceLiteralOracle(t *testing.T) {
	const denominator = 6
	cases := []sourceLiteralScenario{
		{
			name: "nil", source: "local value = nil",
			count: (*program.Program).NilCount, at: (*program.Program).NilAt,
			want: typevalue.Nil,
		},
		{
			name: "false", source: "local value = false",
			count: (*program.Program).BoolCount, at: (*program.Program).BoolAt,
			want: func(registry *axis.Registry) product.Value { return typevalue.LiteralBool(registry, false) },
		},
		{
			name: "true", source: "local value = true",
			count: (*program.Program).BoolCount, at: (*program.Program).BoolAt,
			want: func(registry *axis.Registry) product.Value { return typevalue.LiteralBool(registry, true) },
		},
		{
			name: "integer", source: "local value = 41",
			count: (*program.Program).IntegerCount, at: (*program.Program).IntegerAt,
			want: func(registry *axis.Registry) product.Value { return typevalue.LiteralInt(registry, 41) },
		},
		{
			name: "float", source: "local value = 3.5",
			count: (*program.Program).FloatCount, at: (*program.Program).FloatAt,
			want: func(registry *axis.Registry) product.Value { return typevalue.LiteralNumber(registry, 3.5) },
		},
		{
			name: "string", source: "local value = \"literal\"",
			count: (*program.Program).StringCount, at: (*program.Program).StringAt,
			want: func(registry *axis.Registry) product.Value { return typevalue.LiteralString(registry, "literal") },
		},
	}
	if len(cases) != denominator {
		t.Fatalf("source literal oracle corpus = %d cases, want %d", len(cases), denominator)
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
			got := solveLiteral(t, project, shard, term)
			if want := scenario.want(value.Registry()); !product.Equal(value.Registry(), got, want) {
				t.Fatalf("%s source literal value differs from its typed Program payload", scenario.name)
			}
		})
	}
	t.Logf("source literal oracle: %d/%d", denominator, denominator)
}

func TestSourceLiteralOracleKeepsEqualOccurrencesDistinct(t *testing.T) {
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
		t.Fatal("install source literal oracle")
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
		t.Fatal("seal duplicate literal oracle")
	}
	state, ok := solver.Solve(context.Background(), nil)
	if !ok || state == nil {
		t.Fatal("solve duplicate literal oracle")
	}
	want := typevalue.LiteralInt(value.Registry(), 7)
	for name, query := range map[string]*engine.Query[uint64, product.Value]{"first": left, "second": right} {
		got, present := query.Read(state)
		if !present || !product.Equal(value.Registry(), got, want) {
			t.Fatalf("%s equal literal query = present %t, want its exact literal fact", name, present)
		}
	}
}

func TestSourceLiteralOracleRejectsUnsupportedTerms(t *testing.T) {
	project, shard, source := sourceProject(t, "unsupported.lua", "type Subject = string\nlocal converted = string(1)\nlocal table = {}")
	solver, err := engine.New(project)
	if err != nil {
		t.Fatal(err)
	}
	domain, ok := value.Install(solver, project)
	if !ok {
		t.Fatal("install source literal oracle")
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
			t.Fatalf("%s term %v was accepted by literal oracle", name, term)
		}
	}

	nestedProject, nestedShard, nested := sourceProject(t, "nested.lua", "local function nested() return 1 end")
	nestedSolver, err := engine.New(nestedProject)
	if err != nil {
		t.Fatal(err)
	}
	nestedDomain, ok := value.Install(nestedSolver, nestedProject)
	if !ok {
		t.Fatal("install nested source literal oracle")
	}
	nestedLiteral, ok := nested.IntegerAt(0)
	if !ok {
		t.Fatal("nested integer")
	}
	if query, accepted := nestedDomain.Query(nestedShard, nestedLiteral); accepted || query != nil {
		t.Fatal("non-Entry literal was accepted as a root query")
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
			t.Fatal("install source literal oracle")
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

func solveLiteral(t *testing.T, project *link.Link, shard link.Shard, term program.Term) product.Value {
	t.Helper()
	solver, err := engine.New(project)
	if err != nil {
		t.Fatal(err)
	}
	domain, ok := value.Install(solver, project)
	if !ok {
		t.Fatal("install source literal oracle")
	}
	query, ok := domain.Query(shard, term)
	if !ok {
		t.Fatalf("query source literal %v", term)
	}
	if !solver.Seal() {
		t.Fatal("seal source literal oracle")
	}
	state, ok := solver.Solve(context.Background(), nil)
	if !ok || state == nil {
		t.Fatal("solve source literal oracle")
	}
	got, ok := query.Read(state)
	if !ok {
		t.Fatal("read source literal oracle")
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
