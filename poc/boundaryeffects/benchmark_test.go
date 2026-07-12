package boundaryeffects

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

var benchmarkEffects Result
var benchmarkOracle transfer.Result
var benchmarkBindings PackedBindings

func BenchmarkGuardedBoundaryEffects(b *testing.B) {
	f := makeFixture()
	bound := bindCase(b, f, 1)
	reg := standard.Registry()
	domain := state.Domain(reg)
	entry := entryState(reg, bound, domain.Bottom(), typevalue.LiteralString(reg, "same"), typevalue.LiteralString(reg, "other"))
	config := Config{Registry: reg, Resolver: bound.resolver, Entry: entry}

	b.Run("current-body-solve", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			benchmarkOracle = oracle(f, bound, reg, entry, nil)
		}
	})
	b.Run("precomputed-boundary-effects", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var err error
			benchmarkEffects, err = bound.bound.Execute(config)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("packed-root-binding", func(b *testing.B) {
		left, right, output := bound.bound.paths[0].path.RootOnly(), bound.bound.paths[1].path.RootOnly(), bound.bound.paths[2].path.RootOnly()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var err error
			benchmarkBindings, err = PackBindings(RootBinding{Left, left}, RootBinding{Right, right}, RootBinding{Output, output})
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}
