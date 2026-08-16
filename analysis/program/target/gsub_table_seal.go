package target

import (
	"errors"
	"fmt"
	"sort"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
)

func (d *operationDraft) freezeGsubTableReplacement(input GsubTableReplacementSpec) (gsubTableReplacementDraft, error) {
	if !hasStringGsubBinding(d.bindings) {
		return gsubTableReplacementDraft{}, errors.New("target: gsub table replacement belongs only to string.gsub")
	}
	if input.Replacement != 2 || uint64(input.Replacement) >= uint64(d.valueFormalCount()) {
		return gsubTableReplacementDraft{}, errors.New("target: gsub table replacement must use third fixed input")
	}
	if input.Access == 0 || uint64(input.Access) > uint64(len(d.subedges)) {
		return gsubTableReplacementDraft{}, errors.New("target: gsub table replacement access outside operation scope")
	}
	var access *subedgeDraft
	for index := range d.subedges {
		if d.subedges[index].source == int(input.Access-1) {
			access = &d.subedges[index]
			break
		}
	}
	if access == nil || access.family != SubedgeFamilyIndexGet || access.callee != SubedgeCalleeInvalid || len(access.argumentOrigins) != 2 ||
		access.argumentOrigins[0].segment != ArgumentFixed || access.argumentOrigins[0].index != 0 || access.argumentOrigins[0].kind != ArgumentSourceInput || access.argumentOrigins[0].source != (InputSource{Kind: InputSourceValueFormal, Ordinal: 2}) ||
		access.argumentOrigins[1].segment != ArgumentFixed || access.argumentOrigins[1].index != 1 || access.argumentOrigins[1].kind != ArgumentSourceRule {
		return gsubTableReplacementDraft{}, errors.New("target: gsub table replacement lacks exact table IndexGet route")
	}
	if uint64(input.ResultOutcome) >= uint64(len(d.outcomes)) || input.Result != 0 {
		return gsubTableReplacementDraft{}, errors.New("target: gsub table replacement result outside first correlated result")
	}
	resultOutcome := d.outcomes[int(input.ResultOutcome)]
	if resultOutcome.kind != flowkind.OutcomeNormal || len(resultOutcome.values.types) == 0 {
		return gsubTableReplacementDraft{}, errors.New("target: gsub table replacement requires normal fixed result")
	}
	effects := append([]uint32(nil), input.EffectAliases...)
	if len(effects) == 0 {
		return gsubTableReplacementDraft{}, errors.New("target: gsub table replacement requires effect alias")
	}
	sort.Slice(effects, func(left, right int) bool { return effects[left] < effects[right] })
	for index, effect := range effects {
		if uint64(effect) >= uint64(len(d.effects)) || (index != 0 && effects[index-1] == effect) {
			return gsubTableReplacementDraft{}, fmt.Errorf("target: gsub table replacement effect alias %d invalid", index)
		}
	}
	return gsubTableReplacementDraft{replacement: input.Replacement, accessSource: input.Access, accessRank: uint32(indexOfSubedgeSource(d.subedges, int(input.Access-1))), resultOutcome: uint32(indexOfOutcomeSource(d.outcomes, int(input.ResultOutcome))), result: input.Result, effects: effects}, nil
}

func hasStringGsubBinding(bindings []BindingSpec) bool {
	for _, binding := range bindings {
		if binding.Namespace == BindingModule && len(binding.Owner) == 1 && binding.Owner[0] == "string" && len(binding.Member) == 1 && binding.Member[0] == "gsub" {
			return true
		}
	}
	return false
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

func (c *Contract) appendGsubTableReplacement(owner Operation, draft gsubTableReplacementDraft, row operationRow) (uint32, error) {
	if draft.accessRank >= uint32(row.subedges.len()) || draft.resultOutcome >= uint32(row.outcomes.len()) || draft.result != 0 {
		return 0, errors.New("target: malformed gsub table replacement")
	}
	access := SubedgeID(row.subedges.start + draft.accessRank + 1)
	if got, ok := c.SubedgeFamily(access); !ok || got != SubedgeFamilyIndexGet {
		return 0, errors.New("target: malformed gsub table IndexGet route")
	}
	effects, err := checkedStoredRange("gsub table replacement effect aliases", len(c.gsubEffects), len(draft.effects))
	if err != nil {
		return 0, err
	}
	for _, effect := range draft.effects {
		if uint64(effect) >= uint64(row.effects.len()) {
			return 0, errors.New("target: malformed gsub table replacement effect alias")
		}
		c.gsubEffects = append(c.gsubEffects, effect)
	}
	if _, err := checkedStoredHandle("gsub table replacement table", len(c.gsubTables)); err != nil {
		return 0, err
	}
	c.gsubTables = append(c.gsubTables, gsubTableReplacementRow{replacement: draft.replacement, access: access, resultOutcome: draft.resultOutcome, result: draft.result, effects: effects})
	return uint32(len(c.gsubTables)), nil
}
