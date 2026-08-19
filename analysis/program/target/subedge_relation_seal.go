package target

import (
	"errors"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
)

func (c *Contract) appendSubedgeRelation(_ vocabulary.Operation, draft subedgeRelationDraft, subedges, outcomes indexRange, effectCount int) (uint32, error) {
	if draft.subedgeRank >= uint32(subedges.len()) || draft.resultOutcome >= uint32(outcomes.len()) {
		return 0, errors.New("target: malformed subedge relation")
	}
	subedge := vocabulary.SubedgeID(subedges.start + draft.subedgeRank + 1)
	if _, ok := c.SubedgeFamily(subedge); !ok {
		return 0, errors.New("target: malformed subedge relation subedge")
	}
	effects, err := checkedStoredRange("subedge relation effect aliases", len(c.subedgeRelationEffects), len(draft.effects))
	if err != nil {
		return 0, err
	}
	for _, effect := range draft.effects {
		if uint64(effect) >= uint64(effectCount) {
			return 0, errors.New("target: malformed subedge relation effect alias")
		}
		c.subedgeRelationEffects = append(c.subedgeRelationEffects, effect)
	}
	if _, err := checkedStoredHandle("subedge relation table", len(c.subedgeRelations)); err != nil {
		return 0, err
	}
	c.subedgeRelations = append(c.subedgeRelations, subedgeRelationRow{
		operand: draft.operand, selector: draft.selector, subedge: subedge,
		resultOutcome: draft.resultOutcome, result: draft.result, effects: effects,
	})
	return uint32(len(c.subedgeRelations)), nil
}
