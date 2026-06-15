package standard

import (
	"fmt"
	"slices"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/escape"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/ownership"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	latticelaws "github.com/wippyai/go-lua/analysis/test/laws/lattice"
)

func TestRegistryBundleFrozenAndStable(t *testing.T) {
	reg := Registry()
	want := []string{
		"variantorigin",
		"identity",
		"runtimekind",
		"typewitness",
		"escape",
		"ownership",
		"placement",
		"evidence",
		"assertion",
	}
	if got := registrySpecIDs(reg); !slices.Equal(got, want) {
		t.Fatalf("Registry axes = %v, want %v", got, want)
	}
	if !reg.Frozen() {
		t.Fatalf("Registry must be frozen")
	}
	if _, ok := reg.LookupErased(presence.Key.ID()); ok {
		t.Fatalf("presence must be core, not registered as a sparse standard axis")
	}
	if spec, ok := reg.LookupErased(placement.Key.ID()); !ok || spec.ID() != placement.Key.ID() {
		t.Fatalf("placement axis missing from standard registry")
	}
}

func TestRegistryWithAxesReturnsFreshFrozenRegistry(t *testing.T) {
	fresh, err := RegistryWithAxes()
	if err != nil {
		t.Fatalf("RegistryWithAxes() error = %v", err)
	}
	if fresh == Registry() {
		t.Fatalf("RegistryWithAxes must not expose the singleton")
	}
	if !fresh.Frozen() {
		t.Fatalf("RegistryWithAxes must return a frozen registry")
	}
	if got := registrySpecIDs(fresh); !slices.Equal(got, registrySpecIDs(Registry())) {
		t.Fatalf("RegistryWithAxes axes = %v, want %v", got, registrySpecIDs(Registry()))
	}
}

func TestRegistryWithAxesAddsCustomSparseAxis(t *testing.T) {
	reg, err := RegistryWithAxes(syntheticSpec().Erase())
	if err != nil {
		t.Fatalf("RegistryWithAxes error = %v", err)
	}
	want := append(registrySpecIDs(Registry()), syntheticKey.ID())
	if got := registrySpecIDs(reg); !slices.Equal(got, want) {
		t.Fatalf("custom registry axes = %v, want %v", got, want)
	}
	if _, ok := Registry().LookupErased(syntheticKey.ID()); ok {
		t.Fatalf("custom axis mutated Registry singleton")
	}

	v := product.Set(reg, product.Top(), syntheticKey, syntheticLow)
	if got := product.Get(reg, v, syntheticKey); got != syntheticLow {
		t.Fatalf("custom sparse axis value = %v, want %v", got, syntheticLow)
	}
}

func TestRegistryWithAxesRejectsDuplicateIDs(t *testing.T) {
	if _, err := RegistryWithAxes(escape.Spec().Erase()); err == nil {
		t.Fatalf("duplicate standard axis ID should fail")
	}
	if _, err := RegistryWithAxes(syntheticSpec().Erase(), syntheticSpec().Erase()); err == nil {
		t.Fatalf("duplicate caller axis ID should fail")
	}
}

func TestStandardProductLaws(t *testing.T) {
	reg := Registry()
	d := product.Domain(reg)
	suite := latticelaws.LawSuite[product.Value]{
		Name:   "value.product(standard)",
		Domain: d,
		Sample: standardProductSample(reg, d.Bottom(), d.Top()),
		Format: formatValue,
	}
	suite.Run(t)
}

func TestRuntimeKindStoresAndSparsifiesTop(t *testing.T) {
	reg := Registry()
	tableKind := runtimekind.Singleton(runtimekind.Table)

	v := product.Set(reg, product.Top(), runtimekind.Key, tableKind)
	if got := product.Get(reg, v, runtimekind.Key); !runtimekind.Equal(got, tableKind) {
		t.Fatalf("runtimekind value = %s, want %s", got, tableKind)
	}

	setTop := product.Set(reg, v, runtimekind.Key, runtimekind.Top())
	if !product.Equal(reg, setTop, product.Top()) {
		t.Fatalf("setting runtimekind top should sparsify to product top, got %s", formatValue(setTop))
	}
}

func TestAssertionAxisStoresSparsifiesAndAffectsIdentity(t *testing.T) {
	reg := Registry()
	typeClaim := assertion.Type()
	anyClaim := assertion.Any()

	v := product.Set(reg, product.Top(), assertion.Key, typeClaim)
	if got := product.Get(reg, v, assertion.Key); !assertion.Equal(got, typeClaim) {
		t.Fatalf("assertion value = %s, want %s", got, typeClaim)
	}

	other := product.Set(reg, product.Top(), assertion.Key, anyClaim)
	if product.Equal(reg, v, other) {
		t.Fatalf("different assertion indicators should affect product equality")
	}
	if product.Hash(reg, v) == product.Hash(reg, other) {
		t.Fatalf("different assertion indicators should affect product hash")
	}

	setTop := product.Set(reg, v, assertion.Key, assertion.Top())
	if !product.Equal(reg, setTop, product.Top()) {
		t.Fatalf("setting assertion top should sparsify to product top, got %s", formatValue(setTop))
	}
}

func TestOwnershipAndPlacementStoreInStandardProduct(t *testing.T) {
	reg := Registry()
	unique := ownership.Unique()

	stack := product.Set(reg, product.Top(), ownership.Key, unique)
	stack = product.Set(reg, stack, placement.Key, placement.Stack)
	if got := product.Get(reg, stack, ownership.Key); got != unique {
		t.Fatalf("ownership value = %s, want %s", got, unique)
	}
	if got := product.Get(reg, stack, placement.Key); got != placement.Stack {
		t.Fatalf("placement value = %s, want %s", got, placement.Stack)
	}

	sharedHeap := product.Set(reg, stack, placement.Key, placement.SharedHeap)
	if product.Equal(reg, stack, sharedHeap) {
		t.Fatalf("changing placement stack -> shared-heap should affect product equality")
	}
	if product.Hash(reg, stack) == product.Hash(reg, sharedHeap) {
		t.Fatalf("changing placement stack -> shared-heap should affect product hash")
	}

	ownershipOnly := product.Set(reg, product.Top(), ownership.Key, unique)
	placementTop := product.Set(reg, stack, placement.Key, placement.Unknown)
	if got := product.Get(reg, placementTop, ownership.Key); got != unique {
		t.Fatalf("ownership after placement sparsify = %s, want %s", got, unique)
	}
	if got := product.Get(reg, placementTop, placement.Key); got != placement.Unknown {
		t.Fatalf("placement after sparsify = %s, want %s", got, placement.Unknown)
	}
	if !product.Equal(reg, placementTop, ownershipOnly) {
		t.Fatalf("setting placement unknown should sparsify to ownership-only product, got %s", formatValue(placementTop))
	}
}

func registrySpecIDs(reg *axis.Registry) []string {
	specs := reg.Specs()
	ids := make([]string, len(specs))
	for i, spec := range specs {
		ids[i] = spec.ID()
	}
	return ids
}

func standardProductSample(reg *axis.Registry, bottom, top product.Value) []product.Value {
	present := product.WithPresence(reg, top, presence.Present())
	absent := product.WithPresence(reg, top, presence.Absent())
	fresh := product.Set(reg, top, escape.Key, escape.Fresh())
	unique := product.Set(reg, top, ownership.Key, ownership.Unique())
	stack := product.Set(reg, top, placement.Key, placement.Stack)
	gradual := product.Set(reg, top, evidence.Key, evidence.GradualTop())
	explicit := product.Set(reg, top, evidence.Key, evidence.ExplicitTop())
	claimed := product.Set(reg, top, assertion.Key, assertion.Type())
	variant := product.Set(reg, top, variantorigin.Key, variantorigin.Singleton(7, 1))
	ident := product.Set(reg, top, identity.Key, identity.Singleton(identity.ID{Kind: "alloc", Site: "sample", Index: 1}))
	tableKind := product.Set(reg, top, runtimekind.Key, runtimekind.Singleton(runtimekind.Table))
	combo := product.Set(reg, present, escape.Key, escape.Fresh())
	combo = product.Set(reg, combo, evidence.Key, evidence.GradualTop())
	combo = product.Set(reg, combo, runtimekind.Key, runtimekind.Singleton(runtimekind.Function))
	presenceBottom := product.WithPresence(reg, top, presence.Bottom())

	return []product.Value{
		bottom,
		top,
		present,
		absent,
		fresh,
		unique,
		stack,
		gradual,
		explicit,
		claimed,
		variant,
		ident,
		tableKind,
		combo,
		presenceBottom,
	}
}

func formatValue(v product.Value) string {
	return fmt.Sprintf("value(hash=%d,shape=%s,presence=%s)", product.Hash(Registry(), v), product.ShapeOf(v), product.PresenceOf(v))
}

type synthetic uint8

const (
	syntheticBottom synthetic = iota
	syntheticLow
	syntheticHigh
	syntheticTop
)

var syntheticKey = axis.NewKey[synthetic]("test.standard.synthetic")

func syntheticSpec() axis.Spec[synthetic] {
	return axis.Spec[synthetic]{
		Key:    syntheticKey,
		Bottom: func() synthetic { return syntheticBottom },
		Top:    func() synthetic { return syntheticTop },
		Equal:  func(a, b synthetic) bool { return a == b },
		LessOrEq: func(a, b synthetic) bool {
			return a <= b
		},
		Join: func(a, b synthetic) synthetic {
			if a > b {
				return a
			}
			return b
		},
		Meet: func(a, b synthetic) synthetic {
			if a < b {
				return a
			}
			return b
		},
		Widen: func(prev, next synthetic) synthetic {
			if prev > next {
				return prev
			}
			return next
		},
		Hash: func(v synthetic) uint64 { return uint64(v) + 1 },
	}
}
