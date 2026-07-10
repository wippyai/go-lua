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

func TestProductTypedKeyMismatchPanicsDeterministically(t *testing.T) {
	reg := mustRegistry(t, syntheticSpec().Erase())
	wrongKey := axis.NewKey[int](syntheticKey.ID())

	mustPanicContaining(t, `product: axis "test.synthetic" has incompatible typed key type`, func() {
		_ = Get(reg, Top(), wrongKey)
	})
	mustPanicContaining(t, `product: axis "test.synthetic" has incompatible typed key type`, func() {
		_ = Set(reg, Top(), wrongKey, 1)
	})
}

func TestBottomIsCachedPerRegistry(t *testing.T) {
	regA := mustRegistry(t, syntheticSpec().Erase())
	regB := mustRegistry(t, syntheticSpec().Erase())

	bottomA1 := Bottom(regA)
	bottomA2 := Bottom(regA)
	bottomB := Bottom(regB)

	if bottomA1.n == nil || bottomA2.n == nil || bottomB.n == nil {
		t.Fatalf("Bottom(reg) must return interned nodes")
	}
	if bottomA1.n != bottomA2.n {
		t.Fatalf("Bottom(reg) should reuse the same interned node for one registry")
	}
	if bottomA1.n == bottomB.n {
		t.Fatalf("Bottom(reg) must not cross-share interned nodes between registries")
	}
}

func TestDomainStableAcrossRepeatedConstruction(t *testing.T) {
	reg := mustRegistry(t, syntheticSpec().Erase())
	top := Domain(reg).Top()
	bottom := Domain(reg).Bottom()
	domain := Domain(reg)

	if !domain.Equal(top, domain.Top()) {
		t.Fatalf("reconstructed product domain did not recognize prior top")
	}
	if !domain.Equal(bottom, domain.Bottom()) {
		t.Fatalf("reconstructed product domain did not recognize prior bottom")
	}
	if !domain.Equal(domain.Join(bottom, top), top) {
		t.Fatalf("reconstructed product domain join(bottom, top) did not produce top")
	}
}

func TestProductHashReturnsStoredNodeHash(t *testing.T) {
	hashCalls := 0
	spec := syntheticSpec()
	spec.Hash = func(v synthetic) uint64 {
		hashCalls++
		return uint64(v) + 1
	}
	reg := mustRegistry(t, spec.Erase())

	v := Set(reg, Top(), syntheticKey, syntheticLow)
	if v.n == nil {
		t.Fatalf("non-top sparse value must be interned")
	}
	hashCalls = 0
	if got := Hash(reg, v); got != v.n.hash {
		t.Fatalf("Hash = %d, want stored node hash %d", got, v.n.hash)
	}
	if hashCalls != 0 {
		t.Fatalf("Hash recomputed sparse axis hash %d times, want stored node hash fast path", hashCalls)
	}
}

func TestSchemaOrdinalsKeepExternalStableHashes(t *testing.T) {
	forward := mustRegistry(t, syntheticSpec().Erase(), secondSyntheticSpec().Erase())
	reverse := mustRegistry(t, secondSyntheticSpec().Erase(), syntheticSpec().Erase())

	forwardValue := Set(forward, Set(forward, Top(), syntheticKey, syntheticLow), secondSyntheticKey, syntheticHigh)
	reverseValue := Set(reverse, Set(reverse, Top(), syntheticKey, syntheticLow), secondSyntheticKey, syntheticHigh)
	if Hash(forward, forwardValue) != Hash(reverse, reverseValue) {
		t.Fatalf("schema ordinal assignment changed external stable hash: forward=%d reverse=%d", Hash(forward, forwardValue), Hash(reverse, reverseValue))
	}
}

func TestProductEqualRejectsHashMismatchBeforeAxisEquality(t *testing.T) {
	equalCalls := 0
	spec := syntheticSpec()
	spec.Equal = func(a, b synthetic) bool {
		equalCalls++
		return a == b
	}
	reg := mustRegistry(t, spec.Erase())

	a := Set(reg, Top(), syntheticKey, syntheticLow)
	if a.n == nil {
		t.Fatalf("non-top sparse value must be interned")
	}
	copiedSlots := append([]slot(nil), a.n.slots...)
	mismatchedHash := a.n.hash + 1
	if mismatchedHash == 0 {
		mismatchedHash = 1
	}
	b := Value{n: &node{
		reg:      a.n.reg,
		shape:    a.n.shape,
		presence: a.n.presence,
		slots:    copiedSlots,
		hash:     mismatchedHash,
	}}

	equalCalls = 0
	if Equal(reg, a, b) {
		t.Fatalf("Equal should reject nonzero hash mismatch")
	}
	if equalCalls != 0 {
		t.Fatalf("Equal called sparse axis equality %d times after hash mismatch, want fast false", equalCalls)
	}
}

func TestProductTopIdentityFastPathsAvoidAxisOperations(t *testing.T) {
	var joinCalls, meetCalls, widenCalls int
	spec := syntheticSpec()
	spec.Join = func(a, b synthetic) synthetic {
		joinCalls++
		return syntheticSpec().Join(a, b)
	}
	spec.Meet = func(a, b synthetic) synthetic {
		meetCalls++
		return syntheticSpec().Meet(a, b)
	}
	spec.Widen = func(prev, next synthetic) synthetic {
		widenCalls++
		return syntheticSpec().Widen(prev, next)
	}
	reg := mustRegistry(t, spec.Erase())

	v := Set(reg, Top(), syntheticKey, syntheticLow)
	joinCalls, meetCalls, widenCalls = 0, 0, 0

	if got := Join(reg, Top(), v); got.n != nil {
		t.Fatalf("Join(top, v) = %s, want top", formatValue(got))
	}
	if got := Join(reg, v, Top()); got.n != nil {
		t.Fatalf("Join(v, top) = %s, want top", formatValue(got))
	}
	if got := Widen(reg, Top(), v); got.n != nil {
		t.Fatalf("Widen(top, v) = %s, want top", formatValue(got))
	}
	if got := Widen(reg, v, Top()); got.n != nil {
		t.Fatalf("Widen(v, top) = %s, want top", formatValue(got))
	}
	if got := Meet(reg, Top(), v); got.n != v.n {
		t.Fatalf("Meet(top, v) = %s, want original operand %s", formatValue(got), formatValue(v))
	}
	if got := Meet(reg, v, Top()); got.n != v.n {
		t.Fatalf("Meet(v, top) = %s, want original operand %s", formatValue(got), formatValue(v))
	}

	if joinCalls != 0 || meetCalls != 0 || widenCalls != 0 {
		t.Fatalf("top identity fast paths called axis ops: join=%d meet=%d widen=%d", joinCalls, meetCalls, widenCalls)
	}
}

func TestProductTopIdentityFastPathsStillValidateOperands(t *testing.T) {
	regA := mustRegistry(t, syntheticSpec().Erase())
	regB := mustRegistry(t, syntheticSpec().Erase())
	a := Set(regA, Top(), syntheticKey, syntheticLow)

	mustPanic(t, func() {
		_ = Join(regB, Top(), a)
	})
	mustPanic(t, func() {
		_ = Join(regB, a, Top())
	})
	mustPanic(t, func() {
		_ = Widen(regB, Top(), a)
	})
	mustPanic(t, func() {
		_ = Widen(regB, a, Top())
	})
	mustPanic(t, func() {
		_ = Meet(regB, Top(), a)
	})
	mustPanic(t, func() {
		_ = Meet(regB, a, Top())
	})
}

func TestProductOrderCasesStillUseAxisOperations(t *testing.T) {
	var joinCalls, meetCalls, widenCalls int
	spec := syntheticSpec()
	spec.Join = func(a, b synthetic) synthetic {
		joinCalls++
		return syntheticSpec().Join(a, b)
	}
	spec.Meet = func(a, b synthetic) synthetic {
		meetCalls++
		return syntheticSpec().Meet(a, b)
	}
	spec.Widen = func(prev, next synthetic) synthetic {
		widenCalls++
		return syntheticSpec().Widen(prev, next)
	}
	reg := mustRegistry(t, spec.Erase())

	low := Set(reg, Top(), syntheticKey, syntheticLow)
	high := Set(reg, Top(), syntheticKey, syntheticHigh)

	if got := Join(reg, low, high); got.n != high.n {
		t.Fatalf("Join(low, high) = %s, want high operand %s", formatValue(got), formatValue(high))
	}
	if got := Meet(reg, low, high); got.n != low.n {
		t.Fatalf("Meet(low, high) = %s, want low operand %s", formatValue(got), formatValue(low))
	}
	if got := Widen(reg, high, low); got.n != high.n {
		t.Fatalf("Widen(high, low) = %s, want high operand %s", formatValue(got), formatValue(high))
	}

	if joinCalls == 0 || meetCalls == 0 || widenCalls == 0 {
		t.Fatalf("order cases bypassed axis ops: join=%d meet=%d widen=%d", joinCalls, meetCalls, widenCalls)
	}
}

func TestProductOrderCasesStillValidateOperands(t *testing.T) {
	regA := mustRegistry(t, syntheticSpec().Erase())
	regB := mustRegistry(t, syntheticSpec().Erase())
	lowA := Set(regA, Top(), syntheticKey, syntheticLow)
	highA := Set(regA, Top(), syntheticKey, syntheticHigh)

	mustPanic(t, func() {
		_ = Join(regB, lowA, highA)
	})
	mustPanic(t, func() {
		_ = Meet(regB, lowA, highA)
	})
	mustPanic(t, func() {
		_ = Widen(regB, highA, lowA)
	})
}

func TestFreshRegistryValuesStayIsolatedByPointerIdentity(t *testing.T) {
	regA := mustRegistry(t, syntheticSpec().Erase())
	regB := mustRegistry(t, syntheticSpec().Erase())

	a := Set(regA, Top(), syntheticKey, syntheticLow)
	b := Set(regB, Top(), syntheticKey, syntheticLow)

	if a.n == nil || b.n == nil {
		t.Fatalf("non-top sparse values must be interned nodes")
	}
	if a.n == b.n {
		t.Fatalf("equivalent registries must not reuse product nodes across registry pointers")
	}
	if got := Get(regA, a, syntheticKey); got != syntheticLow {
		t.Fatalf("regA value = %v, want %v", got, syntheticLow)
	}
	if got := Get(regB, b, syntheticKey); got != syntheticLow {
		t.Fatalf("regB value = %v, want %v", got, syntheticLow)
	}
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
	explicit := intern(reg, ShapeTop, presence.Top(), []slot{sparseTestSlot(reg, syntheticKey.ID(), syntheticTop)})

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

		v := intern(reg, ShapeTop, presence.Absent(), []slot{sparseTestSlot(reg, syntheticKey.ID(), syntheticLow)})

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
	if !DefinitelyPresent(present) {
		t.Fatalf("DefinitelyPresent(present) = false, want true")
	}
	if DefinitelyPresent(absent) || DefinitelyPresent(top) {
		t.Fatalf("DefinitelyPresent should only accept proven-present values")
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
	mustPanicContaining(t, "product: presence is a core lane; use PresenceOf", func() {
		_ = Get(reg, present, presence.Key)
	})
	mustPanicContaining(t, "product: presence is a core lane; use WithPresence", func() {
		_ = Set(reg, top, presence.Key, presence.Present())
	})

	explicitTop := intern(reg, ShapeTop, presence.Top(), nil)
	if !Equal(reg, explicitTop, top) || Hash(reg, explicitTop) != Hash(reg, top) {
		t.Fatalf("explicit top presence should canonicalize with top value")
	}
}

func TestWithCompatiblePresenceFrom(t *testing.T) {
	reg := mustRegistry(t)
	base := Top()
	presentSource := WithPresence(reg, Top(), presence.Present())

	got, ok := WithCompatiblePresenceFrom(reg, base, presentSource)
	if !ok || !presence.Equal(PresenceOf(got), presence.Present()) {
		t.Fatalf("WithCompatiblePresenceFrom(top, present) = %s/%v, want present/true", PresenceOf(got), ok)
	}
	matching, ok := WithCompatiblePresenceFrom(reg, got, presentSource)
	if !ok || !Equal(reg, matching, got) {
		t.Fatalf("WithCompatiblePresenceFrom(present, present) = %s/%v, want unchanged/true", formatValue(matching), ok)
	}
	absentSource := WithPresence(reg, Top(), presence.Absent())
	if got, ok := WithCompatiblePresenceFrom(reg, matching, absentSource); ok {
		t.Fatalf("WithCompatiblePresenceFrom(present, absent) = %s/true, want conflict false", formatValue(got))
	}
	if got, ok := WithCompatiblePresenceFrom(reg, base, Top()); ok {
		t.Fatalf("WithCompatiblePresenceFrom(top, unknown) = %s/true, want false", formatValue(got))
	}
	if got, ok := WithCompatiblePresenceFrom(reg, base, WithPresence(reg, Top(), presence.Bottom())); ok {
		t.Fatalf("WithCompatiblePresenceFrom(top, bottom) = %s/true, want false", formatValue(got))
	}
}

func TestProductMeetCorePresenceRefinement(t *testing.T) {
	reg := mustRegistry(t, syntheticSpec().Erase())
	top := Top()

	present := WithPresence(reg, top, presence.Present())
	absent := WithPresence(reg, top, presence.Absent())
	if got := Meet(reg, present, absent); !Equal(reg, got, Bottom(reg)) {
		t.Fatalf("present meet absent = %s, want product bottom", formatValue(got))
	}

	shaped := Set(reg, WithPresence(reg, top, presence.Maybe()), syntheticKey, syntheticLow)
	constraint := NewWithPresence(reg, ShapeTop, presence.Present())
	refined := Meet(reg, shaped, constraint)
	if got := PresenceOf(refined); !presence.Equal(got, presence.Present()) {
		t.Fatalf("presence-only meet presence = %s, want present", got)
	}
	if got := Get(reg, refined, syntheticKey); got != syntheticLow {
		t.Fatalf("presence-only meet erased sparse axis = %v, want %v", got, syntheticLow)
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
		_ = intern(valid, ShapeTop, presence.Top(), []slot{{ordinal: uint16(len(mustRuntime(valid).canonicalAxes)), value: presence.Present()}})
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

	explicitTop := intern(reg, ShapeTop, presence.Top(), []slot{sparseTestSlot(reg, syntheticKey.ID(), syntheticTop)})
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

func TestReducerMutatesOwnedWorkSlotsOnly(t *testing.T) {
	reg := mustRegistry(t, syntheticMirrorReducerSpec().Erase(), secondSyntheticSpec().Erase())

	base := Set(reg, Top(), secondSyntheticKey, syntheticLow)
	next := Set(reg, base, syntheticKey, syntheticLow)

	if got := Get(reg, base, secondSyntheticKey); got != syntheticLow {
		t.Fatalf("source slot was mutated by reducer: second axis = %v, want %v", got, syntheticLow)
	}
	if got := Get(reg, next, secondSyntheticKey); got != syntheticHigh {
		t.Fatalf("reduced value second axis = %v, want %v", got, syntheticHigh)
	}
}

func TestReducerCanUseExplicitPresenceHelpers(t *testing.T) {
	reg := mustRegistry(t, presenceHelperReducerSpec().Erase())

	v := Set(reg, Top(), syntheticKey, syntheticHigh)

	if ShapeOf(v) != ShapeBottom {
		t.Fatalf("ShapeOf = %s, want %s", ShapeOf(v), ShapeBottom)
	}
	if got := PresenceOf(v); !presence.Equal(got, presence.Absent()) {
		t.Fatalf("PresenceOf = %s, want absent", got)
	}
	if got := Get(reg, v, syntheticKey); got != syntheticLow {
		t.Fatalf("sparse slot = %v, want %v", got, syntheticLow)
	}
}

func TestReducerGenericPresenceAccessPanics(t *testing.T) {
	t.Run("Get", func(t *testing.T) {
		reg := mustRegistry(t, presenceGenericGetReducerSpec().Erase())
		mustPanicContaining(t, "product: presence is a core lane; use presence.Get", func() {
			_ = Set(reg, Top(), syntheticKey, syntheticLow)
		})
	})

	t.Run("Set", func(t *testing.T) {
		reg := mustRegistry(t, presenceGenericSetReducerSpec().Erase())
		mustPanicContaining(t, "product: presence is a core lane; use presence.Set", func() {
			_ = Set(reg, Top(), syntheticKey, syntheticLow)
		})
	})
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

func sparseTestSlot(reg *axis.Registry, id string, value any) slot {
	info, ok := mustRuntime(reg).axis(id)
	if !ok {
		panic("test: unregistered sparse axis " + id)
	}
	return slot{ordinal: info.ordinal, value: value}
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

func presenceHelperReducerSpec() axis.Spec[synthetic] {
	spec := syntheticSpec()
	spec.Reducer = func(w axis.Writer) bool {
		if !presence.Equal(presence.Get(w), presence.Top()) {
			return false
		}
		presence.Set(w, presence.Absent())
		axis.Set(w, syntheticKey, syntheticLow)
		return false
	}
	return spec
}

func presenceGenericGetReducerSpec() axis.Spec[synthetic] {
	spec := syntheticSpec()
	spec.Reducer = func(w axis.Writer) bool {
		_, _ = axis.Get(w, presence.Key)
		return false
	}
	return spec
}

func presenceGenericSetReducerSpec() axis.Spec[synthetic] {
	spec := syntheticSpec()
	spec.Reducer = func(w axis.Writer) bool {
		axis.Set(w, presence.Key, presence.Present())
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
