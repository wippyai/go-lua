package target

import (
	"errors"
	"fmt"
	"sort"
)

func (d *operationDraft) freezeSubedgeRelation(input SubedgeRelationSpec) (subedgeRelationDraft, error) {
	if uint64(input.Operand) >= uint64(d.valueFormalCount()) {
		return subedgeRelationDraft{}, errors.New("target: subedge relation operand outside operation scope")
	}
	if input.Subedge == 0 || uint64(input.Subedge) > uint64(len(d.subedges)) {
		return subedgeRelationDraft{}, errors.New("target: subedge relation subedge outside operation scope")
	}
	if uint64(input.ResultOutcome) >= uint64(len(d.outcomes)) {
		return subedgeRelationDraft{}, errors.New("target: subedge relation outcome outside operation scope")
	}
	resultOutcome := d.outcomes[int(input.ResultOutcome)]
	if uint64(input.Result) >= uint64(len(resultOutcome.values.types)) {
		return subedgeRelationDraft{}, errors.New("target: subedge relation result outside outcome prefix")
	}
	effects := append([]uint32(nil), input.EffectAliases...)
	sort.Slice(effects, func(left, right int) bool { return effects[left] < effects[right] })
	for index, effect := range effects {
		if uint64(effect) >= uint64(len(d.effects)) || (index != 0 && effects[index-1] == effect) {
			return subedgeRelationDraft{}, fmt.Errorf("target: subedge relation effect alias %d invalid", index)
		}
	}
	rank := indexOfSubedgeSource(d.subedges, int(input.Subedge-1))
	if rank < 0 {
		return subedgeRelationDraft{}, errors.New("target: subedge relation subedge outside operation scope")
	}
	outcome := indexOfOutcomeSource(d.outcomes, int(input.ResultOutcome))
	if outcome < 0 {
		return subedgeRelationDraft{}, errors.New("target: subedge relation outcome outside operation scope")
	}
	return subedgeRelationDraft{
		operand: input.Operand, selector: input.Selector, subedgeSource: input.Subedge,
		subedgeRank: uint32(rank), resultOutcome: uint32(outcome), result: input.Result,
		effects: effects,
	}, nil
}

func indexOfSubedgeSource(rows []subedgeDraft, source int) int {
	for index := range rows {
		if rows[index].source == source {
			return index
		}
	}
	return -1
}

func indexOfOutcomeSource(rows []outcomeDraft, source int) int {
	for index := range rows {
		if rows[index].source == source {
			return index
		}
	}
	return -1
}

func (c *Contract) appendSubedgeRelation(owner Operation, draft subedgeRelationDraft, row operationRow) (uint32, error) {
	if draft.subedgeRank >= uint32(row.subedges.len()) || draft.resultOutcome >= uint32(row.outcomes.len()) {
		return 0, errors.New("target: malformed subedge relation")
	}
	subedge := SubedgeID(row.subedges.start + draft.subedgeRank + 1)
	if _, ok := c.SubedgeFamily(subedge); !ok {
		return 0, errors.New("target: malformed subedge relation subedge")
	}
	effects, err := checkedStoredRange("subedge relation effect aliases", len(c.subedgeRelationEffects), len(draft.effects))
	if err != nil {
		return 0, err
	}
	for _, effect := range draft.effects {
		if uint64(effect) >= uint64(row.effects.len()) {
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
