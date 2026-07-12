package functiontransformer

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

var benchmarkResult Result
var benchmarkConcrete transfer.Result
var benchmarkBound *BoundTransformer

func BenchmarkLexicalFunction(b *testing.B) {
	fixture := newFixture()
	transformer, err := Compile(fixture.input)
	if err != nil {
		b.Fatal(err)
	}
	concrete := bindFixture(b, fixture, 1)
	bound, err := transformer.Bind(concrete.bindings)
	if err != nil {
		b.Fatal(err)
	}
	registry := standard.Registry()
	entry := initialState(
		registry, concrete.resolver, fixture.graph, concrete.left, concrete.right,
		typevalue.LiteralString(registry, "left"), typevalue.LiteralString(registry, "right"),
	)
	config := Config{Registry: registry, Resolver: concrete.resolver, EntryState: entry}

	b.Run("concrete-body-solve", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			benchmarkConcrete = solveConcrete(fixture, concrete, registry, entry)
		}
	})
	b.Run("bound-row-instantiation", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var err error
			benchmarkResult, err = bound.Execute(config)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("root-binding", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			benchmarkBound, err = transformer.Bind(concrete.bindings)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

func TestBenchmarkFixtureExercisesJoin(t *testing.T) {
	fixture := newFixture()
	concrete := bindFixture(t, fixture, 1)
	registry := standard.Registry()
	entry := initialState(registry, concrete.resolver, fixture.graph, concrete.left, concrete.right,
		typevalue.LiteralString(registry, "left"), typevalue.LiteralString(registry, "right"))
	result := solveConcrete(fixture, concrete, registry, entry)
	if state.Domain(registry).Equal(result[fixture.join], state.Domain(registry).Bottom()) {
		t.Fatal("benchmark join is unreachable")
	}
}
