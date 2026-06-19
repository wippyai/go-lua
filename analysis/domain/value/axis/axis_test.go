package axis_test

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/escape"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	latticelaws "github.com/wippyai/go-lua/analysis/test/laws/lattice"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestFiniteAxisSpecsLaws(t *testing.T) {
	t.Run("presence", func(t *testing.T) {
		runAxisLaws(t, "presence", presence.Spec(), []presence.Value{
			presence.Bottom(),
			presence.Present(),
			presence.Absent(),
			presence.Top(),
		})
	})

	t.Run("variantorigin", func(t *testing.T) {
		runAxisLaws(t, "variantorigin", variantorigin.Spec(), []variantorigin.Value{
			variantorigin.Bottom(),
			variantorigin.Singleton(7, 0),
			variantorigin.Singleton(7, 1),
			variantorigin.Of(7, []int{0, 1}),
			variantorigin.Singleton(8, 0),
			variantorigin.Top(),
		})
	})

	t.Run("runtimekind", func(t *testing.T) {
		numberOrString := runtimekind.Join(
			runtimekind.Singleton(runtimekind.Number),
			runtimekind.Singleton(runtimekind.String),
		)
		runAxisLaws(t, "runtimekind", runtimekind.Spec(), []runtimekind.Value{
			runtimekind.Bottom(),
			runtimekind.Singleton(runtimekind.Nil),
			runtimekind.Singleton(runtimekind.Number),
			runtimekind.Singleton(runtimekind.String),
			numberOrString,
			runtimekind.Top().Without(runtimekind.Table),
			runtimekind.Top(),
		})
	})

	t.Run("typewitness", func(t *testing.T) {
		runAxisLaws(t, "typewitness", typewitness.Spec(), []typewitness.Value{
			typewitness.Bottom(),
			typewitness.Of(typ.Number),
			typewitness.Of(typ.String),
			typewitness.Of(typ.Boolean),
			typewitness.Of(typ.LiteralString("a")),
			typewitness.Of(typ.LiteralString("b")),
			typewitness.Of(typ.MaterializeUnion([]typ.Type{typ.LiteralString("a"), typ.LiteralString("b")})),
			typewitness.Of(typ.MaterializeUnion([]typ.Type{typ.LiteralString("a"), typ.Number})),
			typewitness.Of(typ.MaterializeUnion([]typ.Type{typ.Number, typ.String})),
			typewitness.Of(typ.MaterializeUnion([]typ.Type{typ.String, typ.Boolean})),
			typewitness.Of(typ.MaterializeUnion([]typ.Type{typ.Number, typ.String, typ.Boolean})),
			typewitness.Top(),
		})
	})

	t.Run("escape", func(t *testing.T) {
		runAxisLaws(t, "escape", escape.Spec(), []escape.Value{
			escape.Bottom(),
			escape.Fresh(),
			escape.Top(),
		})
	})

	t.Run("evidence", func(t *testing.T) {
		runAxisLaws(t, "evidence", evidence.Spec(), []evidence.Value{
			evidence.Bottom(),
			evidence.GradualTop(),
			evidence.ExplicitTop(),
			evidence.Top(),
		})
	})

	t.Run("identity", func(t *testing.T) {
		runAxisLaws(t, "identity", identity.Spec(), []identity.Value{
			identity.Bottom(),
			identity.Singleton(identity.ID{Kind: "alloc", Site: "sample", Index: 1}),
			identity.Singleton(identity.ID{Kind: "alloc", Site: "sample", Index: 2}),
			identity.Top(),
		})
	})

	t.Run("assertion", func(t *testing.T) {
		runAxisLaws(t, "assertion", assertion.Spec(), []assertion.Value{
			assertion.Bottom(),
			assertion.Type(),
			assertion.Any(),
			assertion.NonNil(),
			assertion.Of(assertion.TypeClaim, assertion.NonNilClaim),
			assertion.Top(),
		})
	})
}

func TestRegistryFreezeRejectsRegistration(t *testing.T) {
	reg := axis.NewRegistry()
	axis.Register(reg, presence.Spec())
	if reg.Frozen() {
		t.Fatalf("new registry should be mutable")
	}
	reg.Freeze()
	if !reg.Frozen() {
		t.Fatalf("Freeze did not mark registry frozen")
	}
	if err := reg.RegisterErased(escape.Spec().Erase()); err == nil {
		t.Fatalf("RegisterErased after Freeze should fail")
	}
	defer func() {
		if recover() == nil {
			t.Fatalf("axis.Register after Freeze should panic")
		}
	}()
	axis.Register(reg, escape.Spec())
}

func TestRegistryRejectsDuplicateErasedIDAcrossSpecTypes(t *testing.T) {
	reg := axis.NewRegistry()
	first := registryTestSpec("test.registry.duplicate", nil).Erase()
	spoofed := registryStringSpec("test.registry.duplicate").Erase()
	if err := reg.RegisterErased(first); err != nil {
		t.Fatalf("RegisterErased(first) error = %v", err)
	}
	if err := reg.RegisterErased(spoofed); err == nil {
		t.Fatalf("RegisterErased accepted spoofed duplicate erased ID with different Go value type")
	}
}

func TestRegistrySpecsRemainDefensiveCopy(t *testing.T) {
	reg := axis.NewRegistry()
	first := registryTestSpec("test.registry.first", nil).Erase()
	second := registryTestSpec("test.registry.second", nil).Erase()
	if err := reg.RegisterErased(first); err != nil {
		t.Fatalf("RegisterErased(first) error = %v", err)
	}
	if err := reg.RegisterErased(second); err != nil {
		t.Fatalf("RegisterErased(second) error = %v", err)
	}
	reg.Freeze()

	specs := reg.Specs()
	specs[0] = second

	view := reg.SpecsView()
	if got := view.At(0).ID(); got != first.ID() {
		t.Fatalf("Specs() mutation changed registry order: first ID = %q, want %q", got, first.ID())
	}
}

func TestRegistryReducersRemainDefensiveCopy(t *testing.T) {
	var calls []string
	firstReducer := func(axis.Writer) bool {
		calls = append(calls, "first")
		return false
	}
	replacementReducer := func(axis.Writer) bool {
		calls = append(calls, "replacement")
		return false
	}
	reg := axis.NewRegistry()
	if err := reg.RegisterErased(registryTestSpec("test.registry.reducer.first", firstReducer).Erase()); err != nil {
		t.Fatalf("RegisterErased(first) error = %v", err)
	}
	reg.Freeze()

	reducers := reg.Reducers()
	reducers[0] = replacementReducer

	view := reg.ReducersView()
	view.At(0)(nil)
	if got, want := fmt.Sprint(calls), "[first]"; got != want {
		t.Fatalf("Reducers() mutation changed registry reducers: calls = %s, want %s", got, want)
	}
}

func TestRegistryViewsPreserveOrderAndLength(t *testing.T) {
	var calls []string
	firstReducer := func(axis.Writer) bool {
		calls = append(calls, "first")
		return false
	}
	secondReducer := func(axis.Writer) bool {
		calls = append(calls, "second")
		return false
	}
	reg := axis.NewRegistry()
	first := registryTestSpec("test.registry.view.first", firstReducer).Erase()
	second := registryTestSpec("test.registry.view.second", secondReducer).Erase()
	if err := reg.RegisterErased(first); err != nil {
		t.Fatalf("RegisterErased(first) error = %v", err)
	}
	if err := reg.RegisterErased(second); err != nil {
		t.Fatalf("RegisterErased(second) error = %v", err)
	}
	reg.Freeze()

	specs := reg.SpecsView()
	if got, want := specs.Len(), 2; got != want {
		t.Fatalf("SpecsView.Len() = %d, want %d", got, want)
	}
	if got, want := specs.At(0).ID(), first.ID(); got != want {
		t.Fatalf("SpecsView.At(0).ID() = %q, want %q", got, want)
	}
	if got, want := specs.At(1).ID(), second.ID(); got != want {
		t.Fatalf("SpecsView.At(1).ID() = %q, want %q", got, want)
	}

	reducers := reg.ReducersView()
	if got, want := reducers.Len(), 2; got != want {
		t.Fatalf("ReducersView.Len() = %d, want %d", got, want)
	}
	reducers.At(0)(nil)
	reducers.At(1)(nil)
	if got, want := fmt.Sprint(calls), "[first second]"; got != want {
		t.Fatalf("ReducersView order calls = %s, want %s", got, want)
	}
}

func runAxisLaws[T any](t *testing.T, name string, spec axis.Spec[T], sample []T) {
	t.Helper()
	suite := latticelaws.LawSuite[T]{
		Name:   "axis." + name,
		Domain: spec.Lattice(),
		Sample: sample,
		Format: func(v T) string {
			return fmt.Sprintf("%v", v)
		},
	}
	suite.Run(t)
}

func registryTestSpec(id string, reducer axis.Reducer) axis.Spec[int] {
	return axis.Spec[int]{
		Key:    axis.NewKey[int](id),
		Bottom: func() int { return 0 },
		Top:    func() int { return 2 },
		Equal:  func(a, b int) bool { return a == b },
		LessOrEq: func(a, b int) bool {
			return a <= b
		},
		Join: func(a, b int) int {
			if a > b {
				return a
			}
			return b
		},
		Widen: func(prev, next int) int {
			if prev > next {
				return prev
			}
			return next
		},
		Hash: func(v int) uint64 {
			return uint64(v) + 1
		},
		Reducer: reducer,
	}
}

func registryStringSpec(id string) axis.Spec[string] {
	return axis.Spec[string]{
		Key:    axis.NewKey[string](id),
		Bottom: func() string { return "" },
		Top:    func() string { return "top" },
		Equal:  func(a, b string) bool { return a == b },
		LessOrEq: func(a, b string) bool {
			return a == b || a == ""
		},
		Join: func(a, b string) string {
			if a == "" {
				return b
			}
			if b == "" || a == b {
				return a
			}
			return "top"
		},
		Widen: func(prev, next string) string {
			if prev == next {
				return prev
			}
			return "top"
		},
		Hash: func(v string) uint64 {
			return uint64(len(v)) + 1
		},
	}
}
