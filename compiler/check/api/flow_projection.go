package api

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

// PreStateFlow projects a flow solution to the entry side of each point.
//
// The returned FlowOps keeps numeric, reachability, and key-existence queries
// unchanged, but makes NarrowedTypeAt read PreStateTypeAt. Consumers can then
// use normal synthesis against a pre-transfer environment without rebuilding
// predecessor joins locally.
func PreStateFlow(inner FlowOps) FlowOps {
	if inner == nil {
		return nil
	}
	return preStateFlowOps{inner: inner}
}

// ConditionFlow returns a flow projection scoped by an expression-local
// condition. It is the canonical short-circuit/refinement view: the wrapped
// flow solution owns condition application and product-domain projection.
func ConditionFlow(inner FlowOps, condition constraint.Condition) FlowOps {
	if inner == nil || (!condition.HasConstraints() && !condition.IsFalse()) {
		return inner
	}
	return conditionFlowOps{inner: inner, condition: condition}
}

type preStateFlowOps struct {
	inner FlowOps
}

func (p preStateFlowOps) NarrowedTypeAt(point cfg.Point, path constraint.Path) typ.Type {
	return p.inner.PreStateTypeAt(point, path)
}

func (p preStateFlowOps) NarrowedTypeAtWithCondition(point cfg.Point, path constraint.Path, condition constraint.Condition) typ.Type {
	if condition.IsFalse() {
		return typ.Never
	}
	return p.inner.PreStateTypeAt(point, path)
}

func (p preStateFlowOps) PreStateTypeAt(point cfg.Point, path constraint.Path) typ.Type {
	return p.inner.PreStateTypeAt(point, path)
}

func (p preStateFlowOps) ExcludesTypeAt(point cfg.Point, path constraint.Path, declared typ.Type) bool {
	return p.inner.ExcludesTypeAt(point, path, declared)
}

func (p preStateFlowOps) BoundsAt(point cfg.Point, name string) (lower, upper int64, ok bool) {
	return p.inner.BoundsAt(point, name)
}

func (p preStateFlowOps) ArrayLenBoundAt(point cfg.Point, varName string) (arrKey string, ok bool) {
	return p.inner.ArrayLenBoundAt(point, varName)
}

func (p preStateFlowOps) ArrayLenBoundWithOffsetAt(point cfg.Point, varName string) (arrKey string, offset int64, ok bool) {
	return p.inner.ArrayLenBoundWithOffsetAt(point, varName)
}

func (p preStateFlowOps) LengthBoundsAt(point cfg.Point, path constraint.Path) (lower, upper int64, ok bool) {
	return p.inner.LengthBoundsAt(point, path)
}

func (p preStateFlowOps) IsPointDead(point cfg.Point) bool {
	return p.inner.IsPointDead(point)
}

func (p preStateFlowOps) HasKeyOf(point cfg.Point, tablePath, keyPath constraint.Path) bool {
	return p.inner.HasKeyOf(point, tablePath, keyPath)
}

type conditionFlowOps struct {
	inner     FlowOps
	condition constraint.Condition
}

func (o conditionFlowOps) NarrowedTypeAt(point cfg.Point, path constraint.Path) typ.Type {
	if o.inner == nil {
		return nil
	}
	return o.inner.NarrowedTypeAtWithCondition(point, path, o.condition)
}

func (o conditionFlowOps) NarrowedTypeAtWithCondition(point cfg.Point, path constraint.Path, condition constraint.Condition) typ.Type {
	if o.inner == nil {
		return nil
	}
	combined := o.condition
	if condition.HasConstraints() || condition.IsFalse() {
		combined = constraint.And(combined, condition)
	}
	return o.inner.NarrowedTypeAtWithCondition(point, path, combined)
}

func (o conditionFlowOps) PreStateTypeAt(point cfg.Point, path constraint.Path) typ.Type {
	if o.inner != nil {
		return o.inner.PreStateTypeAt(point, path)
	}
	return nil
}

func (o conditionFlowOps) ExcludesTypeAt(point cfg.Point, path constraint.Path, declared typ.Type) bool {
	if o.inner != nil {
		return o.inner.ExcludesTypeAt(point, path, declared)
	}
	return false
}

func (o conditionFlowOps) BoundsAt(point cfg.Point, name string) (lower, upper int64, ok bool) {
	if o.inner != nil {
		return o.inner.BoundsAt(point, name)
	}
	return 0, 0, false
}

func (o conditionFlowOps) ArrayLenBoundAt(point cfg.Point, varName string) (arrKey string, ok bool) {
	if o.inner != nil {
		return o.inner.ArrayLenBoundAt(point, varName)
	}
	return "", false
}

func (o conditionFlowOps) ArrayLenBoundWithOffsetAt(point cfg.Point, varName string) (arrKey string, offset int64, ok bool) {
	if o.inner != nil {
		return o.inner.ArrayLenBoundWithOffsetAt(point, varName)
	}
	return "", 0, false
}

func (o conditionFlowOps) LengthBoundsAt(point cfg.Point, path constraint.Path) (lower, upper int64, ok bool) {
	if o.inner != nil {
		return o.inner.LengthBoundsAt(point, path)
	}
	return 0, 0, false
}

func (o conditionFlowOps) IsPointDead(point cfg.Point) bool {
	if o.inner != nil {
		return o.inner.IsPointDead(point)
	}
	return false
}

func (o conditionFlowOps) HasKeyOf(point cfg.Point, tablePath, keyPath constraint.Path) bool {
	if o.inner != nil {
		return o.inner.HasKeyOf(point, tablePath, keyPath)
	}
	return false
}
