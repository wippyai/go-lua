package product

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	latticelaws "github.com/wippyai/go-lua/analysis/test/laws/lattice"
)

func TestProductLaws(t *testing.T) {
	reg := mustRegistry(t, syntheticSpec().Erase(), secondSyntheticSpec().Erase())
	d := Domain(reg)
	suite := latticelaws.LawSuite[Value]{
		Name:   "value.product(generic)",
		Domain: d,
		Sample: productSample(reg, d.Bottom(), d.Top()),
		Format: formatValue,
	}
	suite.Run(t)
}

func TestRegistryWithAxesFrozenAndStable(t *testing.T) {
	reg := mustRegistry(t, syntheticSpec().Erase(), secondSyntheticSpec().Erase())
	want := []string{syntheticKey.ID(), secondSyntheticKey.ID()}
	if got := registrySpecIDs(reg); !slices.Equal(got, want) {
		t.Fatalf("RegistryWithAxes axes = %v, want %v", got, want)
	}
	if !reg.Frozen() {
		t.Fatalf("RegistryWithAxes must return a frozen registry")
	}
	if _, ok := reg.LookupErased(presence.Key.ID()); ok {
		t.Fatalf("presence must be core, not registered as a sparse axis")
	}
}

func TestRegistryWithAxesRejectsDuplicateIDs(t *testing.T) {
	if _, err := RegistryWithAxes(syntheticSpec().Erase(), syntheticSpec().Erase()); err == nil {
		t.Fatalf("duplicate caller axis ID should fail")
	}
}

func TestValidateRegistryRejectsNil(t *testing.T) {
	if err := ValidateRegistry(nil); err == nil || !strings.Contains(err.Error(), "registry is required") {
		t.Fatalf("ValidateRegistry(nil) error = %v, want registry-required error", err)
	}
}

func TestProductValuesAreScopedToRegistryIdentity(t *testing.T) {
	regA := mustRegistry(t, syntheticSpec().Erase())
	regB := mustRegistry(t, syntheticHighMeetSpec().Erase())

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
	if _, err := RegistryWithAxes(spec); err == nil || !strings.Contains(err.Error(), `product: sparse axis "test.synthetic.no_meet" must define Meet`) {
		t.Fatalf("RegistryWithAxes(no-meet) error = %v, want product no-meet error", err)
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
	reg := mustRegistry(t, syntheticSpec().Erase())
	top := Top()
	explicit := intern(reg, ShapeTop, presence.Top(), []slot{{key: syntheticKey.ID(), value: syntheticTop}})

	if explicit.n != nil {
		t.Fatalf("explicit top slot should canonicalize to the nil top node, got %s", formatValue(explicit))
	}
	if !Equal(reg, explicit, top) {
		t.Fatalf("explicit top slot must equal omitted slot")
	}
	if Hash(reg, explicit) != Hash(reg, top) {
		t.Fatalf("explicit top slot hash differs from omitted slot")
	}

	setTop := Set(reg, top, syntheticKey, syntheticTop)
	if !Equal(reg, setTop, top) || Hash(reg, setTop) != Hash(reg, top) {
		t.Fatalf("Set(top axis) did not preserve omitted-slot canonical form")
	}
}

func TestReducePresenceShapeNormalizesCoreLanes(t *testing.T) {
	reg := mustRegistry(t)
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
		reg := mustRegistry(t, syntheticSpec().Erase())

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
		reg := mustRegistry(t, syntheticTopReducerSpec().Erase())

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
	reg := mustRegistry(t)
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

func TestProductMeetCorePresenceRefinement(t *testing.T) {
	reg := mustRegistry(t)
	top := Top()

	present := WithPresence(reg, top, presence.Present())
	absent := WithPresence(reg, top, presence.Absent())
	if got := Meet(reg, present, absent); !Equal(reg, got, Bottom(reg)) {
		t.Fatalf("present meet absent = %s, want product bottom", formatValue(got))
	}
}

func TestPresenceCannotBeSparseProductAxis(t *testing.T) {
	reg := axis.NewRegistry()
	axis.Register(reg, presence.Spec())
	reg.Freeze()

	if _, err := RegistryWithAxes(presence.Spec().Erase()); err == nil {
		t.Fatalf("RegistryWithAxes should reject presence as a sparse axis")
	}
	mustPanic(t, func() {
		_ = Domain(reg)
	})
	mustPanic(t, func() {
		valid := mustRegistry(t)
		_ = intern(valid, ShapeTop, presence.Top(), []slot{{key: presence.Key.ID(), value: presence.Present()}})
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

func TestProductOperationsRequireRegistry(t *testing.T) {
	mustPanicContaining(t, "registry is required", func() {
		_ = Bottom(nil)
	})
	mustPanicContaining(t, "registry is required", func() {
		_ = Domain(nil)
	})
	mustPanicContaining(t, "registry is required", func() {
		_ = Equal(nil, Top(), Top())
	})
}

func TestProductMeetUsesCustomSparseAxis(t *testing.T) {
	reg := mustRegistry(t, syntheticSpec().Erase())
	d := Domain(reg)

	low := Set(reg, d.Top(), syntheticKey, syntheticLow)
	high := Set(reg, d.Top(), syntheticKey, syntheticHigh)
	got := d.Meet(low, high)
	if gotValue := Get(reg, got, syntheticKey); gotValue != syntheticLow {
		t.Fatalf("custom sparse meet = %v, want %v", gotValue, syntheticLow)
	}
}

func TestSyntheticAxisParticipatesThroughRegistry(t *testing.T) {
	reg := mustRegistry(t, syntheticSpec().Erase())
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
	reg := mustRegistry(t, syntheticMirrorReducerSpec().Erase(), secondSyntheticSpec().Erase())

	v := Set(reg, Top(), syntheticKey, syntheticLow)
	if got := Get(reg, v, syntheticKey); got != syntheticLow {
		t.Fatalf("source axis = %v, want %v", got, syntheticLow)
	}
	if got := Get(reg, v, secondSyntheticKey); got != syntheticHigh {
		t.Fatalf("reducer mirror axis = %v, want %v", got, syntheticHigh)
	}
}

func mustRegistry(t *testing.T, specs ...axis.ErasedSpec) *axis.Registry {
	t.Helper()
	reg, err := RegistryWithAxes(specs...)
	if err != nil {
		t.Fatalf("RegistryWithAxes() error = %v", err)
	}
	return reg
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

func productSample(reg *axis.Registry, bottom, top Value) []Value {
	present := WithPresence(reg, top, presence.Present())
	absent := WithPresence(reg, top, presence.Absent())
	low := Set(reg, top, syntheticKey, syntheticLow)
	high := Set(reg, top, syntheticKey, syntheticHigh)
	mirror := Set(reg, low, secondSyntheticKey, syntheticHigh)
	combo := WithPresence(reg, high, presence.Present())
	presenceBottom := WithPresence(reg, top, presence.Bottom())

	return []Value{
		bottom,
		top,
		present,
		absent,
		low,
		high,
		mirror,
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

func mustPanicContaining(t *testing.T, want string, f func()) {
	t.Helper()
	defer func() {
		got := recover()
		if got == nil {
			t.Fatalf("expected panic containing %q", want)
		}
		if !strings.Contains(fmt.Sprint(got), want) {
			t.Fatalf("panic = %v, want substring %q", got, want)
		}
	}()
	f()
}
