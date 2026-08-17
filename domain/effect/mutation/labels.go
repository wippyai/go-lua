package mutation

import (
	"fmt"

	"github.com/wippyai/go-lua/domain/constraint/expr"
	"github.com/wippyai/go-lua/domain/effect"
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
	if o, ok := effect.NormalizeLabel(other).(Mutate); ok {
		return m.Target.Index == o.Target.Index &&
			transformEquals(m.Transform, o.Transform) &&
			expr.ExprEquals(m.LengthDelta, o.LengthDelta)
	}
	return false
}

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
	if o, ok := effect.NormalizeLabel(other).(LengthChange); ok {
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
	if o, ok := effect.NormalizeLabel(other).(TableMutator); ok {
		return t.Target.Index == o.Target.Index && t.Value.Index == o.Value.Index
	}
	return false
}
