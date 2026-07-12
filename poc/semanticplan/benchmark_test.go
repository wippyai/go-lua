package semanticplan

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

var benchmarkState state.State
var benchmarkRows []BoundRow
var benchmarkPlan Plan

func benchmarkFixture(b *testing.B) (pathFixture, factapply.FactsNodeTransferConfig, transfer.NodeContext, state.State) {
	b.Helper()
	fixture := newPathFixture()
	reg := standard.Registry()
	value := typevalue.LiteralString(reg, "value")
	config := factapply.FactsNodeTransferConfig{
		Facts: factflow.NewFacts(fixture.input), Sources: fixedSources{value: value, ok: true}, Visibility: fixture.resolver,
	}
	ctx := transfer.NodeContext{Graph: fixture.graph, Registry: reg, Point: fixture.point, Node: fixture.graph.Node(fixture.point)}
	base := state.Domain(reg).Bottom().WriteValue(reg, key.SymbolValue(fixture.target.Symbol), product.Top())
	return fixture, config, ctx, base
}

func BenchmarkPathAssignmentPlanBuild(b *testing.B) {
	fixture := newPathFixture()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var err error
		benchmarkPlan, err = CompilePathAssignments(fixture.input)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPathAssignmentConcrete(b *testing.B) {
	fixture, config, ctx, base := benchmarkFixture(b)
	oracle := factapply.NewFactsNodeTransfer(config)
	plan, err := CompilePathAssignments(newPathFixture().input)
	if err != nil {
		b.Fatal(err)
	}
	config.Facts = factflow.Facts{}
	planned := plan.BindConcrete(config)
	op, _ := plan.Operation(ctx.Point)
	transformer := DefaultPathAssignmentRegistry().Lift(op)
	b.Run("oracle", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			benchmarkState = oracle(ctx, base)
		}
	})
	b.Run("plan-kernel", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			benchmarkState = planned(ctx, base)
		}
	})
	b.Run("symbolic-kernel", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var ok bool
			benchmarkState, ok = transformer.Execute(ctx, fixture.resolver, config, base)
			if !ok {
				b.Fatal("operation rejected")
			}
		}
	})
}

func BenchmarkPathAssignmentSymbolicTerms(b *testing.B) {
	fixture := newPathFixture()
	plan, err := CompilePathAssignments(fixture.input)
	if err != nil {
		b.Fatal(err)
	}
	op, _ := plan.Operation(fixture.point)
	registry := DefaultPathAssignmentRegistry()
	transformer := registry.Lift(op)
	reg := standard.Registry()
	bindings := Bindings{
		Roots: map[symbol.ID]pathdom.Path{
			fixture.target.Symbol:     pathdom.NewPath(301, "caller-target"),
			fixture.sourcePath.Symbol: pathdom.NewPath(302, "caller-source"),
		},
		Values: map[pathdom.PathKey]product.Value{fixture.sourcePath.Key(): typevalue.LiteralString(reg, "bound")},
	}
	b.Run("lift", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			transformer = registry.Lift(op)
		}
	})
	b.Run("substitute-terms", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var ok bool
			benchmarkRows, ok = transformer.SubstituteTerms(bindings)
			if !ok {
				b.Fatal("term substitution failed")
			}
		}
	})
}
