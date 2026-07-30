package subtype

import (
	"strconv"
	"testing"

	"github.com/wippyai/go-lua/analysis/type/typ"
)

// BenchmarkSubtypeDeepAliasChain measures IsSubtype against a linear chain of
// N aliases wrapping a mismatched terminal type, so no level can short-circuit
// via identity or equality and checkCore must peel every Alias layer.
//
// The chain is built with raw &typ.Alias{...} literals rather than
// typ.NewAlias, matching the construction TestSubtypeTraversesDeepProductsExactly
// already uses to stress un-flattened chains: it is the shape that exposed the
// O(n^2) defect (UnaliasedTarget re-walking from the head on every recursion
// level before the fix landed). A chain built through typ.NewAlias resolves
// its unaliased target eagerly and was never quadratic; this benchmark targets
// the un-cached path specifically so a regression there is caught.
func BenchmarkSubtypeDeepAliasChain(b *testing.B) {
	for _, n := range []int{1000, 4000, 8000} {
		n := n
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			var chain typ.Type = typ.String
			for i := 0; i < n; i++ {
				chain = &typ.Alias{Name: "A", Target: chain}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if IsSubtype(chain, typ.Number) {
					b.Fatal("string alias chain should not be a subtype of number")
				}
			}
		})
	}
}
