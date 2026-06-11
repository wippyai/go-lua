package axis_test

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/escape"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/ownership"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	latticelaws "github.com/wippyai/go-lua/analysis/test/laws/lattice"
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

	t.Run("escape", func(t *testing.T) {
		runAxisLaws(t, "escape", escape.Spec(), []escape.Value{
			escape.Bottom(),
			escape.Fresh(),
			escape.Top(),
		})
	})

	t.Run("ownership", func(t *testing.T) {
		runAxisLaws(t, "ownership", ownership.Spec(), []ownership.Value{
			ownership.Bottom(),
			ownership.Unique(),
			ownership.Top(),
		})
	})

	t.Run("evidence", func(t *testing.T) {
		runAxisLaws(t, "evidence", evidence.Spec(), []evidence.Value{
			evidence.Bottom(),
			evidence.GradualTop(),
			evidence.Top(),
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
