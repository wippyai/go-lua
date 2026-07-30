package pathevidence

import (
	"strconv"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// benchLane builds a refinement-heavy lane whose value-lane keys all sit under a
// single resolver root, the worst case for subtree invalidation (every entry is
// tested against the prefix).
func benchLane(b *testing.B, n int) (Lane, *keyspace.KeySpace, pathdom.PathKey) {
	b.Helper()
	reg := standard.Registry()
	ks := keyspace.New()
	present := product.Top()
	l := Lane{}
	for i := 0; i < n; i++ {
		raw := pathdom.PathKey("sym1@1.table.field" + strconv.Itoa(i))
		key, ok := ks.FromPathKey(raw)
		if !ok {
			b.Fatalf("FromPathKey(%q) failed", raw)
		}
		l, _ = l.WritePathKey(reg, key, present)
	}
	return l, ks, pathdom.PathKey("sym1@1.table")
}

// BenchmarkInvalidateSubtreeStructuralKey guards the value-lane subtree
// invalidation hot path: keys compare as comparable keyspace.Key structs via
// ks.HasPrefix without per-entry string reparse.
func BenchmarkInvalidateSubtreeStructuralKey(b *testing.B) {
	l, ks, seed := benchLane(b, 256)
	prefixKeys := structuralPrefixKeys(ks, []pathdom.PathKey{seed})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		deletePathKeySubtrees(ks, l.refinements, prefixKeys)
	}
}
