package propagate

import (
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

type pointConditionComputer struct {
	inputs          *Inputs
	pointConditions map[cfg.Point]constraint.Condition
	preheaderConds  map[cfg.Point]constraint.Condition
	point           cfg.Point
	node            *cfg.Node
	preds           []cfg.Point
}

func newPointConditionComputer(
	inputs *Inputs,
	pointConditions map[cfg.Point]constraint.Condition,
	preheaderConds map[cfg.Point]constraint.Condition,
	p cfg.Point,
) pointConditionComputer {
	return pointConditionComputer{
		inputs:          inputs,
		pointConditions: pointConditions,
		preheaderConds:  preheaderConds,
		point:           p,
	}
}

func (c pointConditionComputer) compute() constraint.Condition {
	if !c.prepare() {
		return constraint.TrueCondition()
	}
	if c.isDead(c.point) || len(c.preds) == 0 {
		return constraint.FalseCondition()
	}

	var result conditionAccumulator
	for _, pred := range c.preds {
		incoming, ok := c.incomingCondition(pred)
		if !ok {
			continue
		}
		result.add(incoming)
	}
	if !result.hasValue {
		return constraint.FalseCondition()
	}
	return KillRedefinedConditions(result.condition, c.point, c.inputs.Assignments)
}

func (c *pointConditionComputer) prepare() bool {
	g := c.inputs.Graph
	c.node = g.Node(c.point)
	if c.node == nil || c.node.Kind == cfg.NodeEntry {
		return false
	}
	c.preds = graphPredecessors(g, c.point)
	return true
}

func (c pointConditionComputer) incomingCondition(pred cfg.Point) (constraint.Condition, bool) {
	if c.isDead(pred) {
		return constraint.Condition{}, false
	}
	predCond, exists := c.pointConditions[pred]
	if !exists || predCond.IsFalse() {
		return constraint.Condition{}, false
	}
	if reinforced, ok := c.reinforceLoopPreheader(predCond, pred); ok {
		predCond = reinforced
	} else if c.shouldReinforcePreheader(pred) {
		return constraint.Condition{}, false
	}
	return c.applyEdgeCondition(predCond, pred)
}

func (c pointConditionComputer) isDead(p cfg.Point) bool {
	return c.inputs.DeadPoints != nil && c.inputs.DeadPoints[p]
}

func (c pointConditionComputer) reinforceLoopPreheader(
	predCond constraint.Condition,
	pred cfg.Point,
) (constraint.Condition, bool) {
	if !c.shouldReinforcePreheader(pred) {
		return predCond, true
	}
	preCond, ok := c.loopPreheaderCondition()
	if !ok {
		return constraint.Condition{}, false
	}
	if preCond.IsFalse() {
		return constraint.Condition{}, true
	}
	if !preCond.HasConstraints() {
		return predCond, true
	}
	if len(c.node.LoopVars) > 0 {
		preCond = FilterConditionSymbols(preCond, c.node.LoopVars)
	}
	if predCond.HasConstraints() {
		return constraint.And(predCond, preCond), true
	}
	return preCond, true
}

func (c pointConditionComputer) shouldReinforcePreheader(pred cfg.Point) bool {
	return c.node != nil && c.node.LoopPreheaderSet && pred != c.node.LoopPreheader
}

func (c pointConditionComputer) loopPreheaderCondition() (constraint.Condition, bool) {
	if preCond, cached := c.preheaderConds[c.point]; cached {
		return preCond, true
	}
	preCond, ok := c.pointConditions[c.node.LoopPreheader]
	if !ok {
		return constraint.Condition{}, false
	}
	if preEdge, ok := c.inputs.EdgeConditions[EdgeKey{From: c.node.LoopPreheader, To: c.point}]; ok && preEdge.HasConstraints() {
		preCond = constraint.And(preCond, preEdge)
	}
	c.preheaderConds[c.point] = preCond
	return preCond, true
}

func (c pointConditionComputer) applyEdgeCondition(
	predCond constraint.Condition,
	pred cfg.Point,
) (constraint.Condition, bool) {
	edgeCond, ok := c.inputs.EdgeConditions[EdgeKey{From: pred, To: c.point}]
	if !ok || (!edgeCond.HasConstraints() && !edgeCond.IsFalse()) {
		return predCond, true
	}
	switch {
	case edgeCond.IsFalse():
		return constraint.Condition{}, false
	case !edgeCond.HasConstraints():
		return predCond, true
	case !predCond.HasConstraints():
		return edgeCond, true
	default:
		combined := constraint.And(predCond, edgeCond)
		return combined, !combined.IsFalse()
	}
}

type conditionAccumulator struct {
	condition constraint.Condition
	hasValue  bool
}

func (a *conditionAccumulator) add(cond constraint.Condition) {
	if cond.IsFalse() {
		return
	}
	if !a.hasValue {
		a.condition = cond
		a.hasValue = true
		return
	}
	a.condition = constraint.Or(a.condition, cond)
}
