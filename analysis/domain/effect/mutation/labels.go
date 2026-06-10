package mutation

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/constraint/expr"
	"github.com/wippyai/go-lua/analysis/domain/effect"
)

// Mutate indicates that a function changes a parameter's type-level shape.
type Mutate struct {
	Target      effect.ParamRef
	Transform   TypeTransform
	LengthDelta expr.Expr
}

func (Mutate) EffectLabel() {}
func (m Mutate) String() string {
	if m.LengthDelta != nil {
		return fmt.Sprintf("mutate(%s, %s, delta=%s)", m.Target, m.Transform, m.LengthDelta)
	}
	return fmt.Sprintf("mutate(%s, %s)", m.Target, m.Transform)
}
func (m Mutate) Equals(other effect.Label) bool {
	if o, ok := other.(Mutate); ok {
		return m.Target.Index == o.Target.Index &&
			transformEquals(m.Transform, o.Transform) &&
			expr.ExprEquals(m.LengthDelta, o.LengthDelta)
	}
	return false
}

// TypeTransform describes a local type change for Mutate.
type TypeTransform interface {
	transform()
	String() string
}

type ElementUnion struct {
	Source effect.ParamRef
}

func (ElementUnion) transform() {}
func (e ElementUnion) String() string {
	return fmt.Sprintf("union_elem(%s)", e.Source)
}

type ContainerElementUnion struct {
	Container effect.ParamRef
	Value     effect.ParamRef
}

func (ContainerElementUnion) transform() {}
func (c ContainerElementUnion) String() string {
	return fmt.Sprintf("union_elem(%s, %s)", c.Container, c.Value)
}

type ToArray struct {
	Element effect.ParamRef
}

func (ToArray) transform() {}
func (t ToArray) String() string {
	return fmt.Sprintf("to_array(%s)", t.Element)
}

type Unchanged struct{}

func (Unchanged) transform()     {}
func (Unchanged) String() string { return "unchanged" }

type LengthChange struct {
	Target effect.ParamRef
	Delta  int
}

func (LengthChange) EffectLabel() {}
func (l LengthChange) String() string {
	if l.Delta >= 0 {
		return fmt.Sprintf("len(%s) += %d", l.Target, l.Delta)
	}
	return fmt.Sprintf("len(%s) -= %d", l.Target, -l.Delta)
}
func (l LengthChange) Equals(other effect.Label) bool {
	if o, ok := other.(LengthChange); ok {
		return l.Target.Index == o.Target.Index && l.Delta == o.Delta
	}
	return false
}

type TableMutator struct {
	Target effect.ParamRef
	Value  effect.ParamRef
}

func (TableMutator) EffectLabel() {}
func (t TableMutator) String() string {
	return fmt.Sprintf("table_mutator(%s, %s)", t.Target, t.Value)
}
func (t TableMutator) Equals(other effect.Label) bool {
	if o, ok := other.(TableMutator); ok {
		return t.Target.Index == o.Target.Index && t.Value.Index == o.Value.Index
	}
	return false
}

func transformEquals(a, b TypeTransform) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return VisitTransform(a, TypeTransformVisitor[bool]{
		Unchanged: func(Unchanged) bool {
			_, ok := b.(Unchanged)
			return ok
		},
		ElementUnion: func(av ElementUnion) bool {
			bv, ok := b.(ElementUnion)
			return ok && av.Source.Index == bv.Source.Index
		},
		ContainerElementUnion: func(av ContainerElementUnion) bool {
			bv, ok := b.(ContainerElementUnion)
			return ok &&
				av.Container.Index == bv.Container.Index &&
				av.Value.Index == bv.Value.Index
		},
		ToArray: func(av ToArray) bool {
			bv, ok := b.(ToArray)
			return ok && av.Element.Index == bv.Element.Index
		},
		Default: func(TypeTransform) bool {
			return false
		},
	})
}
