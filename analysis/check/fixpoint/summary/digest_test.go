package summary

import (
	"math/rand"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/escape"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestNormalizedPayloadDigestSeparatesUnequalSemanticContent(t *testing.T) {
	reg := standard.Registry()
	types := typevalue.NewCache()
	typed := types.FromTypeWithWitness(reg, typ.NewArray(typ.String))

	productVariants := []product.Value{
		typed,
		identityvalue.WithExact(reg, typed, identity.LuaFunction(1)),
		identityvalue.WithExact(reg, typed, identity.LuaFunction(2)),
		product.Set(reg, typed, escape.Key, escape.Fresh()),
		product.Set(reg, typed, evidence.Key, evidence.GradualTop().WithOrigin(evidence.Origin{Kind: evidence.OriginSource, ID: 1})),
		product.Set(reg, typed, evidence.Key, evidence.GradualTop().WithOrigin(evidence.Origin{Kind: evidence.OriginSource, ID: 2})),
		product.Set(reg, typed, assertion.Key, assertion.Runtime()),
		product.Set(reg, typed, variantorigin.Key, variantorigin.Singleton(1, 1)),
		product.Set(reg, typed, variantorigin.Key, variantorigin.Singleton(1, 2)),
		product.Set(reg, typed, runtimekind.Key, runtimekind.Top().Without(runtimekind.Function)),
		product.Set(reg, typed, runtimekind.Key, runtimekind.Singleton(runtimekind.Table)),
		product.WithPresence(reg, typed, presence.Absent()),
		types.FromTypeWithWitness(reg, typ.NewArray(typ.Number)),
	}
	for i := range productVariants {
		for j := i + 1; j < len(productVariants); j++ {
			assertUnequalNormalizedSummariesHaveDifferentDigests(t, reg,
				Summary{Returns: []product.Value{productVariants[i]}},
				Summary{Returns: []product.Value{productVariants[j]}},
			)
		}
	}

	// Exercise combinations as well as individual axes. The fixed seed makes
	// this a reproducible property test rather than a source of flaky coverage.
	rng := rand.New(rand.NewSource(1))
	for range 256 {
		left := typed
		right := typed
		for range 4 {
			left = productVariants[rng.Intn(len(productVariants))]
			right = productVariants[rng.Intn(len(productVariants))]
		}
		assertUnequalNormalizedSummariesHaveDifferentDigests(t, reg,
			Summary{Returns: []product.Value{left}},
			Summary{Returns: []product.Value{right}},
		)
	}

	id := testTableIdentity(1, 1)
	heapKS := keyspace.New()
	plain := Summary{HeapKeySpace: heapKS, HeapTableObjects: map[identity.ID]heapidentity.TableObject{
		id: heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: typed}),
	}}
	prefix := Summary{HeapKeySpace: heapKS, HeapTableObjects: map[identity.ID]heapidentity.TableObject{
		id: heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: typed, PrefixStableShape: true}),
	}}
	stable := Summary{HeapKeySpace: heapKS, HeapTableObjects: map[identity.ID]heapidentity.TableObject{
		id: heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: typed, StableShape: true}),
	}}
	assertUnequalNormalizedSummariesHaveDifferentDigests(t, reg, plain, prefix)
	assertUnequalNormalizedSummariesHaveDifferentDigests(t, reg, plain, stable)
}

func assertUnequalNormalizedSummariesHaveDifferentDigests(t *testing.T, reg *axis.Registry, left, right Summary) {
	t.Helper()
	left = Normalize(reg, left)
	right = Normalize(reg, right)
	if EqualNormalized(reg, left, right) {
		return
	}
	if leftDigest, rightDigest := NormalizedPayloadDigest(reg, left), NormalizedPayloadDigest(reg, right); leftDigest == rightDigest {
		t.Fatalf("unequal normalized summaries share digest %d:\nleft:  %#v\nright: %#v", leftDigest, left, right)
	}
}
