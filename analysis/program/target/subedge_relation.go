package target

import (
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
)

import flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"

// OperationSubedgeRelation reports the one neutral relation attached to op.
// The relation only joins existing Target coordinates; any interpretation of
// selector belongs to the domain adapter that authored it.
func (c *Contract) OperationSubedgeRelation(op vocabulary.Operation) (operand vocabulary.ValueFormal, selector uint32, subedge vocabulary.SubedgeID, resultOutcome, result uint32, ok bool) {
	row, found := c.operation(op)
	if !found || row.subedgeRelation == 0 || uint64(row.subedgeRelation) > uint64(len(c.subedgeRelations)) {
		return 0, 0, 0, 0, 0, false
	}
	relation := c.subedgeRelations[row.subedgeRelation-1]
	if relation.subedge == 0 || uint64(relation.subedge) > uint64(len(c.subedges)) || relation.resultOutcome >= uint32(row.outcomes.len()) {
		return 0, 0, 0, 0, 0, false
	}
	return relation.operand, relation.selector, relation.subedge, relation.resultOutcome, relation.result, true
}

// OperationSubedgeRelationOutcome projects a terminal of the related
// Subedge to the owning operation outcome when the route explicitly carries
// an outcome. The relation itself does not assign meaning to selector.
func (c *Contract) OperationSubedgeRelationOutcome(op vocabulary.Operation, kind flowkind.OutcomeKind) (uint32, bool) {
	_, _, subedge, resultOutcome, _, ok := c.OperationSubedgeRelation(op)
	if !ok {
		return 0, false
	}
	if kind == flowkind.OutcomeNormal || kind == flowkind.OutcomeReturn {
		return resultOutcome, true
	}
	route, _, _, _, _, outcome, _, _, found := c.subedgeRouteAt(subedge, kind)
	if !found || (route != vocabulary.RouteOutcome && route != vocabulary.RouteRejectYield) {
		return 0, false
	}
	return outcome, true
}

func (c *Contract) OperationSubedgeRelationEffectAliasCount(op vocabulary.Operation) int {
	row, found := c.operation(op)
	if !found || row.subedgeRelation == 0 || uint64(row.subedgeRelation) > uint64(len(c.subedgeRelations)) {
		return 0
	}
	return c.subedgeRelations[row.subedgeRelation-1].effects.len()
}

func (c *Contract) OperationSubedgeRelationEffectAliasAt(op vocabulary.Operation, index int) (int, bool) {
	row, found := c.operation(op)
	if !found || row.subedgeRelation == 0 || uint64(row.subedgeRelation) > uint64(len(c.subedgeRelations)) || index < 0 {
		return 0, false
	}
	relation := c.subedgeRelations[row.subedgeRelation-1]
	if index >= relation.effects.len() {
		return 0, false
	}
	effect := c.subedgeRelationEffects[relation.effects.start+uint32(index)]
	if uint64(effect) >= uint64(row.effects.len()) {
		return 0, false
	}
	return int(effect), true
}
