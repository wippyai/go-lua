package target

import flowkind "github.com/wippyai/go-lua/program/flow/kind"

// GsubTableKey is the closed Lua key selection used only by the table
// replacement branch of string.gsub.  It is not a string fact or a key
// spelling: the match/capture value remains a runtime value.
type GsubTableKey uint8

const (
	GsubTableKeyInvalid GsubTableKey = iota
	GsubTableKeyFirstCaptureOrWholeMatch
)

type gsubTableReplacementDraft struct {
	replacement   ValueFormal
	accessSource  SubedgeRef
	accessRank    uint32
	resultOutcome uint32
	result        uint32
	effects       []uint32
}

type gsubTableReplacementRow struct {
	replacement   ValueFormal
	access        SubedgeID
	resultOutcome uint32
	result        uint32
	effects       indexRange
}

// GsubTableReplacement reports the sole closed table branch owned by op.
// key is always FirstCaptureOrWholeMatch and access is the exact IndexGet
// subedge; no callback or free-form evaluator is involved.
func (c *Contract) GsubTableReplacement(op Operation) (replacement ValueFormal, key GsubTableKey, access SubedgeID, resultOutcome, result uint32, ok bool) {
	row, found := c.operation(op)
	if !found || row.gsubTable == 0 || uint64(row.gsubTable) > uint64(len(c.gsubTables)) {
		return 0, GsubTableKeyInvalid, 0, 0, 0, false
	}
	branch := c.gsubTables[row.gsubTable-1]
	if branch.access == 0 || uint64(branch.access) > uint64(len(c.subedges)) || branch.resultOutcome >= uint32(row.outcomes.len()) {
		return 0, GsubTableKeyInvalid, 0, 0, 0, false
	}
	return branch.replacement, GsubTableKeyFirstCaptureOrWholeMatch, branch.access, branch.resultOutcome, branch.result, true
}

// GsubTableReplacementOutcome aliases a terminal of the exact table IndexGet
// to the outer gsub result outcome. Normal and Return complete the correlated
// substitution result; the remaining terminals retain their sealed subedge
// route outcome (including C-boundary rejected yield).
func (c *Contract) GsubTableReplacementOutcome(op Operation, kind flowkind.OutcomeKind) (uint32, bool) {
	_, _, access, resultOutcome, _, ok := c.GsubTableReplacement(op)
	if !ok {
		return 0, false
	}
	if kind == flowkind.OutcomeNormal || kind == flowkind.OutcomeReturn {
		return resultOutcome, true
	}
	route, _, _, _, _, outcome, _, _, found := c.SubedgeRouteAt(access, kind)
	if !found || (route != RouteOutcome && route != RouteRejectYield) {
		return 0, false
	}
	return outcome, true
}

func (c *Contract) GsubTableReplacementEffectAliasCount(op Operation) int {
	row, found := c.operation(op)
	if !found || row.gsubTable == 0 || uint64(row.gsubTable) > uint64(len(c.gsubTables)) {
		return 0
	}
	return c.gsubTables[row.gsubTable-1].effects.len()
}

func (c *Contract) GsubTableReplacementEffectAliasAt(op Operation, index int) (int, bool) {
	row, found := c.operation(op)
	if !found || row.gsubTable == 0 || uint64(row.gsubTable) > uint64(len(c.gsubTables)) || index < 0 {
		return 0, false
	}
	branch := c.gsubTables[row.gsubTable-1]
	if index >= branch.effects.len() {
		return 0, false
	}
	effect := c.gsubEffects[branch.effects.start+uint32(index)]
	if uint64(effect) >= uint64(row.effects.len()) {
		return 0, false
	}
	return int(effect), true
}
