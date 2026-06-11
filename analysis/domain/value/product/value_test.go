package product

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/defaults"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/escape"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/ownership"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	latticelaws "github.com/wippyai/go-lua/analysis/test/laws/lattice"
)

func TestDefaultProductLaws(t *testing.T) {
	reg := DefaultRegistry()
	if !reg.Frozen() {
		t.Fatalf("DefaultRegistry must be frozen")
	}
	if _, ok := reg.LookupErased(presence.Key.ID()); ok {
		t.Fatalf("presence must be core, not registered as a sparse default axis")
	}
	d := Domain(reg)
	suite := latticelaws.LawSuite[Value]{
		Name:   "value.product(default)",
		Domain: d,
		Sample: defaultProductSample(reg, d.Bottom(), d.Top()),
		Format: formatValue,
	}
	suite.Run(t)
}

func TestDefaultRegistryBundleFrozenAndStable(t *testing.T) {
	reg := DefaultRegistry()
	want := defaultRegistrySpecIDs()
	if got := registrySpecIDs(reg); !slices.Equal(got, want) {
		t.Fatalf("DefaultRegistry axes = %v, want %v", got, want)
	}
	if !reg.Frozen() {
		t.Fatalf("DefaultRegistry must be frozen")
	}
	if _, ok := reg.LookupErased(presence.Key.ID()); ok {
		t.Fatalf("presence must be core, not registered as a sparse default axis")
	}

	fresh, err := DefaultRegistryWithAxes()
	if err != nil {
		t.Fatalf("DefaultRegistryWithAxes() error = %v", err)
	}
	if fresh == reg {
		t.Fatalf("DefaultRegistryWithAxes must not expose the default singleton")
	}
	if !fresh.Frozen() {
		t.Fatalf("DefaultRegistryWithAxes must return a frozen registry")
	}
	if got := registrySpecIDs(fresh); !slices.Equal(got, want) {
		t.Fatalf("DefaultRegistryWithAxes axes = %v, want %v", got, want)
	}
}

func TestDefaultRegistryWithAxesAddsCustomSparseAxis(t *testing.T) {
	reg, err := DefaultRegistryWithAxes(syntheticSpec().Erase())
	if err != nil {
		t.Fatalf("DefaultRegistryWithAxes error = %v", err)
	}
	want := append(defaultRegistrySpecIDs(), syntheticKey.ID())
	if got := registrySpecIDs(reg); !slices.Equal(got, want) {
		t.Fatalf("custom registry axes = %v, want %v", got, want)
	}
	if _, ok := DefaultRegistry().LookupErased(syntheticKey.ID()); ok {
		t.Fatalf("custom axis mutated DefaultRegistry")
	}

	d := Domain(reg)
	v := Set(reg, d.Top(), syntheticKey, syntheticLow)
	if got := Get(reg, v, syntheticKey); got != syntheticLow {
		t.Fatalf("custom sparse axis value = %v, want %v", got, syntheticLow)
	}
}

func TestDefaultRegistryWithAxesRejectsDuplicateIDs(t *testing.T) {
	if _, err := DefaultRegistryWithAxes(escape.Spec().Erase()); err == nil {
		t.Fatalf("duplicate default axis ID should fail")
	}
	if _, err := DefaultRegistryWithAxes(syntheticSpec().Erase(), syntheticSpec().Erase()); err == nil {
		t.Fatalf("duplicate caller axis ID should fail")
	}
}

func TestProductValuesAreScopedToRegistryIdentity(t *testing.T) {
	regA := axis.NewRegistry()
	axis.Register(regA, syntheticSpec())
	regA.Freeze()

	regB := axis.NewRegistry()
	axis.Register(regB, syntheticHighMeetSpec())
	regB.Freeze()

	aLow := Set(regA, Top(), syntheticKey, syntheticLow)
	aHigh := Set(regA, Top(), syntheticKey, syntheticHigh)
	bLow := Set(regB, Top(), syntheticKey, syntheticLow)
	bHigh := Set(regB, Top(), syntheticKey, syntheticHigh)

	if aLow.n == nil || bLow.n == nil {
		t.Fatalf("non-top custom-axis values must be interned nodes")
	}
	if aLow.n == bLow.n {
		t.Fatalf("values from independent registries with the same axis ID must not alias")
	}
	if got := Get(regA, aLow, syntheticKey); got != syntheticLow {
		t.Fatalf("regA value = %v, want %v", got, syntheticLow)
	}
	if got := Get(regB, bLow, syntheticKey); got != syntheticLow {
		t.Fatalf("regB value = %v, want %v", got, syntheticLow)
	}
	if got := Get(regA, Meet(regA, aLow, aHigh), syntheticKey); got != syntheticLow {
		t.Fatalf("regA meet(low, high) = %v, want %v", got, syntheticLow)
	}
	if got := Get(regB, Meet(regB, bLow, bHigh), syntheticKey); got != syntheticHigh {
		t.Fatalf("regB meet(low, high) = %v, want %v", got, syntheticHigh)
	}

	mustPanic(t, func() {
		_ = Get(regB, aLow, syntheticKey)
	})
	mustPanic(t, func() {
		_ = Equal(regB, aLow, bLow)
	})
	mustPanic(t, func() {
		_ = LessOrEq(regB, aLow, bLow)
	})
	mustPanic(t, func() {
		_ = Meet(regB, aLow, bLow)
	})
	mustPanic(t, func() {
		_ = Set(regB, aLow, syntheticKey, syntheticHigh)
	})
}

func TestProductRegistryRejectsSparseAxisWithoutMeet(t *testing.T) {
	spec := noMeetSpec().Erase()
	if _, err := DefaultRegistryWithAxes(spec); err == nil || !strings.Contains(err.Error(), `product: sparse axis "test.synthetic.no_meet" must define Meet`) {
		t.Fatalf("DefaultRegistryWithAxes(no-meet) error = %v, want product no-meet error", err)
	}

	reg := axis.NewRegistry()
	if err := reg.RegisterErased(spec); err != nil {
		t.Fatalf("generic axis registry should accept no-meet spec: %v", err)
	}
	reg.Freeze()
	mustPanic(t, func() {
		_ = Domain(reg)
	})
}

func TestExplicitTopSparseSlotNormalizesToOmission(t *testing.T) {
	reg := DefaultRegistry()
	top := Top()
	explicit := intern(reg, ShapeTop, presence.Top(), []slot{{key: escape.Key.ID(), value: escape.Top()}})

	if explicit.n != nil {
		t.Fatalf("explicit top slot should canonicalize to the nil top node, got %s", formatValue(explicit))
	}
	if !Equal(reg, explicit, top) {
		t.Fatalf("explicit top slot must equal omitted slot")
	}
	if Hash(reg, explicit) != Hash(reg, top) {
		t.Fatalf("explicit top slot hash differs from omitted slot")
	}

	setTop := Set(reg, top, escape.Key, escape.Top())
	if !Equal(reg, setTop, top) || Hash(reg, setTop) != Hash(reg, top) {
		t.Fatalf("Set(top axis) did not preserve omitted-slot canonical form")
	}
}

func TestDefaultRegistryRuntimeKindStoresAndSparsifiesTop(t *testing.T) {
	reg := DefaultRegistry()
	tableKind := runtimekind.Singleton(runtimekind.Table)

	v := Set(reg, Top(), runtimekind.Key, tableKind)
	if got := Get(reg, v, runtimekind.Key); !runtimekind.Equal(got, tableKind) {
		t.Fatalf("runtimekind value = %s, want %s", got, tableKind)
	}
	if _, ok := lookupSlot(v, runtimekind.Key.ID()); !ok {
		t.Fatalf("runtimekind singleton should be stored as a sparse slot")
	}

	setTop := Set(reg, v, runtimekind.Key, runtimekind.Top())
	if !Equal(reg, setTop, Top()) {
		t.Fatalf("setting runtimekind top should sparsify to product top, got %s", formatValue(setTop))
	}

	explicitTop := intern(reg, ShapeTop, presence.Top(), []slot{{key: runtimekind.Key.ID(), value: runtimekind.Top()}})
	if !Equal(reg, explicitTop, Top()) || Hash(reg, explicitTop) != Hash(reg, Top()) {
		t.Fatalf("explicit runtimekind top slot should canonicalize to omission")
	}
}

func TestDefaultRegistryClaimAxisStoresSparsifiesAndAffectsIdentity(t *testing.T) {
	reg := DefaultRegistry()
	typeClaim := assertion.Type()
	anyClaim := assertion.Any()

	v := Set(reg, Top(), assertion.Key, typeClaim)
	if got := Get(reg, v, assertion.Key); !assertion.Equal(got, typeClaim) {
		t.Fatalf("assertion value = %s, want %s", got, typeClaim)
	}
	if _, ok := lookupSlot(v, assertion.Key.ID()); !ok {
		t.Fatalf("non-top assertion should be stored as a sparse slot")
	}

	other := Set(reg, Top(), assertion.Key, anyClaim)
	if Equal(reg, v, other) {
		t.Fatalf("different claim indicators should affect product equality")
	}
	if Hash(reg, v) == Hash(reg, other) {
		t.Fatalf("different claim indicators should affect product hash")
	}

	setTop := Set(reg, v, assertion.Key, assertion.Top())
	if !Equal(reg, setTop, Top()) {
		t.Fatalf("setting assertion top should sparsify to product top, got %s", formatValue(setTop))
	}

	explicitTop := intern(reg, ShapeTop, presence.Top(), []slot{{key: assertion.Key.ID(), value: assertion.Top()}})
	if !Equal(reg, explicitTop, Top()) || Hash(reg, explicitTop) != Hash(reg, Top()) {
		t.Fatalf("explicit assertion top slot should canonicalize to omission")
	}
}

func TestReducePresenceShapeNormalizesCoreLanes(t *testing.T) {
	reg := DefaultRegistry()
	tests := []struct {
		name         string
		shape        Shape
		presence     presence.Value
		wantShape    Shape
		wantPresence presence.Value
	}{
		{
			name:         "ShapeTopAbsent",
			shape:        ShapeTop,
			presence:     presence.Absent(),
			wantShape:    ShapeBottom,
			wantPresence: presence.Absent(),
		},
		{
			name:         "ShapeBottomPresent",
			shape:        ShapeBottom,
			presence:     presence.Present(),
			wantShape:    ShapeBottom,
			wantPresence: presence.Bottom(),
		},
		{
			name:         "ShapeBottomTop",
			shape:        ShapeBottom,
			presence:     presence.Top(),
			wantShape:    ShapeBottom,
			wantPresence: presence.Absent(),
		},
		{
			name:         "PresenceBottomDragsShape",
			shape:        ShapeTop,
			presence:     presence.Bottom(),
			wantShape:    ShapeBottom,
			wantPresence: presence.Bottom(),
		},
		{
			name:         "ShapeTopPresent",
			shape:        ShapeTop,
			presence:     presence.Present(),
			wantShape:    ShapeTop,
			wantPresence: presence.Present(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewWithPresence(reg, tt.shape, tt.presence)
			if ShapeOf(got) != tt.wantShape {
				t.Fatalf("ShapeOf = %s, want %s", ShapeOf(got), tt.wantShape)
			}
			if !presence.Equal(PresenceOf(got), tt.wantPresence) {
				t.Fatalf("PresenceOf = %s, want %s", PresenceOf(got), tt.wantPresence)
			}
		})
	}
}

func TestReducePresenceShapeSparseSlots(t *testing.T) {
	t.Run("preserves sparse slots", func(t *testing.T) {
		reg := axis.NewRegistry()
		axis.Register(reg, syntheticSpec())
		reg.Freeze()

		v := Set(reg, Top(), syntheticKey, syntheticLow)
		v = WithPresence(reg, v, presence.Absent())

		if ShapeOf(v) != ShapeBottom {
			t.Fatalf("ShapeOf = %s, want %s", ShapeOf(v), ShapeBottom)
		}
		if !presence.Equal(PresenceOf(v), presence.Absent()) {
			t.Fatalf("PresenceOf = %s, want absent", PresenceOf(v))
		}
		if got := Get(reg, v, syntheticKey); got != syntheticLow {
			t.Fatalf("sparse slot = %v, want %v", got, syntheticLow)
		}
	})

	t.Run("explicit top removes sparse slot", func(t *testing.T) {
		reg := axis.NewRegistry()
		axis.Register(reg, syntheticTopReducerSpec())
		reg.Freeze()

		v := intern(reg, ShapeTop, presence.Absent(), []slot{{key: syntheticKey.ID(), value: syntheticLow}})

		if ShapeOf(v) != ShapeBottom {
			t.Fatalf("ShapeOf = %s, want %s", ShapeOf(v), ShapeBottom)
		}
		if !presence.Equal(PresenceOf(v), presence.Absent()) {
			t.Fatalf("PresenceOf = %s, want absent", PresenceOf(v))
		}
		if got := Get(reg, v, syntheticKey); got != syntheticTop {
			t.Fatalf("sparse slot = %v, want omitted top %v", got, syntheticTop)
		}
		if v.n == nil || len(v.n.slots) != 0 {
			t.Fatalf("explicit top should omit sparse slot, got %s", formatValue(v))
		}
	})
}

func TestPresenceIsCoreLane(t *testing.T) {
	reg := DefaultRegistry()
	top := Top()
	present := WithPresence(reg, top, presence.Present())
	absent := WithPresence(reg, top, presence.Absent())

	if got := PresenceOf(present); !presence.Equal(got, presence.Present()) {
		t.Fatalf("PresenceOf = %s, want present", got)
	}
	if Equal(reg, present, absent) {
		t.Fatalf("different core presence values should not be equal")
	}
	if Hash(reg, present) == Hash(reg, absent) {
		t.Fatalf("different core presence values should contribute distinct hashes")
	}
	if joined := Join(reg, present, absent); !Equal(reg, joined, top) {
		t.Fatalf("present join absent should raise core presence to top, got %s", formatValue(joined))
	}
	mustPanic(t, func() {
		_ = Get(reg, present, presence.Key)
	})
	mustPanic(t, func() {
		_ = Set(reg, top, presence.Key, presence.Present())
	})

	explicitTop := intern(reg, ShapeTop, presence.Top(), nil)
	if !Equal(reg, explicitTop, top) || Hash(reg, explicitTop) != Hash(reg, top) {
		t.Fatalf("explicit top presence should canonicalize with top value")
	}
}

func TestProductMeetCoreAndRuntimeKindRefinements(t *testing.T) {
	reg := DefaultRegistry()
	top := Top()

	present := WithPresence(reg, top, presence.Present())
	absent := WithPresence(reg, top, presence.Absent())
	if got := Meet(reg, present, absent); !Equal(reg, got, Bottom(reg)) {
		t.Fatalf("present meet absent = %s, want product bottom", formatValue(got))
	}

	tableKind := Set(reg, top, runtimekind.Key, runtimekind.Singleton(runtimekind.Table))
	functionKind := Set(reg, top, runtimekind.Key, runtimekind.Singleton(runtimekind.Function))
	if got := Meet(reg, tableKind, functionKind); !Equal(reg, got, Bottom(reg)) {
		t.Fatalf("table meet function = %s, want product bottom", formatValue(got))
	}

	if got := Meet(reg, top, tableKind); !Equal(reg, got, tableKind) {
		t.Fatalf("top meet table = %s, want table", formatValue(got))
	}
}

func TestPresenceCannotBeSparseProductAxis(t *testing.T) {
	reg := axis.NewRegistry()
	axis.Register(reg, presence.Spec())
	reg.Freeze()

	if _, err := DefaultRegistryWithAxes(presence.Spec().Erase()); err == nil {
		t.Fatalf("DefaultRegistryWithAxes should reject presence as a sparse axis")
	}
	mustPanic(t, func() {
		_ = Domain(reg)
	})
	mustPanic(t, func() {
		_ = intern(DefaultRegistry(), ShapeTop, presence.Top(), []slot{{key: presence.Key.ID(), value: presence.Present()}})
	})
}

func TestProductRequiresFrozenRegistry(t *testing.T) {
	reg := axis.NewRegistry()
	axis.Register(reg, syntheticSpec())
	mustPanic(t, func() {
		_ = Domain(reg)
	})

	reg.Freeze()
	d := Domain(reg)
	v := Set(reg, d.Top(), syntheticKey, syntheticLow)
	mustPanic(t, func() {
		axis.Register(reg, secondSyntheticSpec())
	})
	if got := Get(reg, v, syntheticKey); got != syntheticLow {
		t.Fatalf("value changed after failed frozen registry mutation: %v", got)
	}
}

func TestProductMeetUsesCustomSparseAxis(t *testing.T) {
	reg := axis.NewRegistry()
	axis.Register(reg, syntheticSpec())
	reg.Freeze()
	d := Domain(reg)

	low := Set(reg, d.Top(), syntheticKey, syntheticLow)
	high := Set(reg, d.Top(), syntheticKey, syntheticHigh)
	got := d.Meet(low, high)
	if gotValue := Get(reg, got, syntheticKey); gotValue != syntheticLow {
		t.Fatalf("custom sparse meet = %v, want %v", gotValue, syntheticLow)
	}
}

func TestSyntheticAxisParticipatesThroughRegistry(t *testing.T) {
	reg := axis.NewRegistry()
	axis.Register(reg, syntheticSpec())
	reg.Freeze()
	d := Domain(reg)

	a := Set(reg, d.Top(), syntheticKey, syntheticLow)
	b := Set(reg, d.Top(), syntheticKey, syntheticHigh)
	joined := d.Join(a, b)
	if got := Get(reg, joined, syntheticKey); got != syntheticHigh {
		t.Fatalf("synthetic join = %v, want %v", got, syntheticHigh)
	}
	widened := d.Widen(a, b)
	if got := Get(reg, widened, syntheticKey); got != syntheticHigh {
		t.Fatalf("synthetic widen = %v, want %v", got, syntheticHigh)
	}
	if d.Equal(a, b) {
		t.Fatalf("distinct synthetic axis values should not be equal")
	}
	if Hash(reg, a) == Hash(reg, b) {
		t.Fatalf("distinct synthetic axis values should contribute distinct hashes")
	}

	explicitTop := intern(reg, ShapeTop, presence.Top(), []slot{{key: syntheticKey.ID(), value: syntheticTop}})
	if !d.Equal(explicitTop, d.Top()) || Hash(reg, explicitTop) != Hash(reg, d.Top()) {
		t.Fatalf("synthetic explicit top must canonicalize to omission")
	}
}

func TestSparseAxisReducerStillRunsWithRegistryScopedValues(t *testing.T) {
	reg := axis.NewRegistry()
	axis.Register(reg, syntheticMirrorReducerSpec())
	axis.Register(reg, secondSyntheticSpec())
	reg.Freeze()

	v := Set(reg, Top(), syntheticKey, syntheticLow)
	if got := Get(reg, v, syntheticKey); got != syntheticLow {
		t.Fatalf("source axis = %v, want %v", got, syntheticLow)
	}
	if got := Get(reg, v, secondSyntheticKey); got != syntheticHigh {
		t.Fatalf("reducer mirror axis = %v, want %v", got, syntheticHigh)
	}
}

func defaultRegistrySpecIDs() []string {
	return specIDs(defaults.SparseSpecs())
}

func registrySpecIDs(reg *axis.Registry) []string {
	return specIDs(reg.Specs())
}

func specIDs(specs []axis.ErasedSpec) []string {
	ids := make([]string, len(specs))
	for i, spec := range specs {
		ids[i] = spec.ID()
	}
	return ids
}

func defaultProductSample(reg *axis.Registry, bottom, top Value) []Value {
	present := WithPresence(reg, top, presence.Present())
	absent := WithPresence(reg, top, presence.Absent())
	fresh := Set(reg, top, escape.Key, escape.Fresh())
	unique := Set(reg, top, ownership.Key, ownership.Unique())
	gradual := Set(reg, top, evidence.Key, evidence.GradualTop())
	claimed := Set(reg, top, assertion.Key, assertion.Type())
	variant := Set(reg, top, variantorigin.Key, variantorigin.Singleton(7, 1))
	ident := Set(reg, top, identity.Key, identity.Singleton(identity.ID{Kind: "alloc", Site: "sample", Index: 1}))
	tableKind := Set(reg, top, runtimekind.Key, runtimekind.Singleton(runtimekind.Table))
	combo := Set(reg, present, escape.Key, escape.Fresh())
	combo = Set(reg, combo, evidence.Key, evidence.GradualTop())
	combo = Set(reg, combo, runtimekind.Key, runtimekind.Singleton(runtimekind.Function))
	presenceBottom := WithPresence(reg, top, presence.Bottom())

	return []Value{
		bottom,
		top,
		present,
		absent,
		fresh,
		unique,
		gradual,
		claimed,
		variant,
		ident,
		tableKind,
		combo,
		presenceBottom,
	}
}

func formatValue(v Value) string {
	if v.n == nil {
		return "Value(top)"
	}
	return fmt.Sprintf("Value(shape=%s,presence=%s,slots=%d,hash=%d)", v.n.shape, v.n.presence, len(v.n.slots), v.n.hash)
}

type synthetic uint8

const (
	syntheticBottom synthetic = iota
	syntheticLow
	syntheticHigh
	syntheticTop
)

var syntheticKey = axis.NewKey[synthetic]("test.synthetic")
var secondSyntheticKey = axis.NewKey[synthetic]("test.synthetic.second")
var noMeetKey = axis.NewKey[synthetic]("test.synthetic.no_meet")

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
		Hash: func(v synthetic) uint64 {
			return uint64(v) + 1
		},
	}
}

func secondSyntheticSpec() axis.Spec[synthetic] {
	spec := syntheticSpec()
	spec.Key = secondSyntheticKey
	return spec
}

func noMeetSpec() axis.Spec[synthetic] {
	spec := syntheticSpec()
	spec.Key = noMeetKey
	spec.Meet = nil
	return spec
}

func syntheticHighMeetSpec() axis.Spec[synthetic] {
	spec := syntheticSpec()
	spec.Meet = func(a, b synthetic) synthetic {
		if a == b {
			return a
		}
		if a == syntheticBottom || b == syntheticBottom {
			return syntheticBottom
		}
		if a == syntheticTop {
			return b
		}
		if b == syntheticTop {
			return a
		}
		return syntheticHigh
	}
	return spec
}

func syntheticTopReducerSpec() axis.Spec[synthetic] {
	spec := syntheticSpec()
	spec.Reducer = func(w axis.Writer) bool {
		axis.Set(w, syntheticKey, syntheticTop)
		return false
	}
	return spec
}

func syntheticMirrorReducerSpec() axis.Spec[synthetic] {
	spec := syntheticSpec()
	spec.Reducer = func(w axis.Writer) bool {
		source, ok := axis.Get(w, syntheticKey)
		if !ok || source != syntheticLow {
			return false
		}
		axis.Set(w, secondSyntheticKey, syntheticHigh)
		return false
	}
	return spec
}

func mustPanic(t *testing.T, f func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatalf("expected panic")
		}
	}()
	f()
}
