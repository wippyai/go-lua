package operation

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
)

// CountRows publishes the complete operation-owner contribution to Target's
// denominator. The counts come from Core's sealed dense planes; the Contract
// owner only joins this vector with the protocol and boot vectors.
//
// This is deliberately not a Counts DTO. A DTO would copy the same cardinality
// vocabulary into another owner and make the Contract the second authority.
func (core Core) CountRows() denominator.CountRows {
	if core.OperationCount() == 0 || len(core.query.operations) != core.OperationCount() {
		return denominator.CountRows{}
	}
	ids := denominator.GeneratedTargetIDs()
	counts := make([]denominator.CountRow, 0, 26)
	add := func(id schema.EntryID, value int) bool {
		if value < 0 {
			return false
		}
		row, ok := denominator.NewCountRow(id, uint64(value))
		if !ok {
			return false
		}
		counts = append(counts, row)
		return true
	}
	bindingCount := 0
	outcomeCount := 0
	operationEffects := 0
	formalEffects := 0
	callbackEffects := 0
	publicationEffects := 0
	callbackReleases := 0
	for index := 0; index < core.OperationCount(); index++ {
		geometry, geometryOK := core.geometry.operations.At(index)
		if !geometryOK {
			return denominator.CountRows{}
		}
		query := core.query.operations[index]
		bindingCount += geometry.bindings.Len()
		outcomeCount += geometry.outcomes.Len()
		operationEffects += len(query.effects)
		formalEffects += len(query.formalEffects)
		callbackReleases += query.callbackReleases.len()
		for _, handle := range query.effects {
			if handle < 0 || handle >= len(core.query.effects) {
				return denominator.CountRows{}
			}
			if core.query.effects[handle].hasPublication {
				publicationEffects++
			}
		}
	}
	for index := range core.query.callbacks {
		callback := core.query.callbacks[index]
		if !callback.published {
			continue
		}
		callbackEffects += callback.effects.len()
		for effectIndex := 0; effectIndex < callback.effects.len(); effectIndex++ {
			handle := callback.effects.start + effectIndex
			if handle < 0 || handle >= len(core.query.effects) {
				return denominator.CountRows{}
			}
			if core.query.effects[handle].hasPublication {
				publicationEffects++
			}
		}
	}

	suspensionCount := len(core.query.suspensions)
	spawnSiblingCount := 0
	for _, spawn := range core.query.spawns {
		var countOK bool
		spawnSiblingCount, countOK = denominator.SumInts(spawnSiblingCount, len(spawn.alternatives))
		if !countOK {
			return denominator.CountRows{}
		}
	}
	resumeOutcomeCount := 0
	for _, resume := range core.query.resumes {
		var countOK bool
		resumeOutcomeCount, countOK = denominator.SumInts(resumeOutcomeCount, len(resume.outcomes))
		if !countOK {
			return denominator.CountRows{}
		}
	}

	if !add(ids.TargetOperation, core.OperationCount()) ||
		!add(ids.TargetABI, core.OperationCount()) ||
		!add(ids.TargetOpaque, 1) ||
		!add(ids.TargetBinding, bindingCount) ||
		!add(ids.TargetOutcome, outcomeCount) ||
		!add(ids.TargetOperationEffect, operationEffects) ||
		!add(ids.TargetFormalEffect, formalEffects) ||
		!add(ids.TargetCallbackEffect, callbackEffects) ||
		!add(ids.TargetPublicationEffect, publicationEffects) ||
		!add(ids.TargetCallback, core.geometry.callbacks.Count()) ||
		!add(ids.TargetCallbackRelease, callbackReleases) ||
		!add(ids.TargetCallbackResult, len(core.query.callbackResults)) ||
		!add(ids.TargetResultAlias, len(core.query.resultAliases)) ||
		!add(ids.TargetProduced, len(core.query.produced)) ||
		!add(ids.TargetProducedCapture, len(core.query.captures)) ||
		!add(ids.TargetFreshResult, len(core.query.fresh)) ||
		!add(ids.TargetSubedge, len(core.query.subedges)) ||
		!add(ids.TargetSubedgeArgumentOrigin, len(core.query.subedgeOrigins)) ||
		!add(ids.TargetSubedgeRelation, len(core.query.subedgeRelations)) ||
		!add(ids.TargetSuspension, suspensionCount) ||
		!add(ids.TargetSpawn, len(core.query.spawns)) ||
		!add(ids.TargetSpawnSibling, spawnSiblingCount) ||
		!add(ids.TargetResume, len(core.query.resumes)) ||
		!add(ids.TargetResumeOutcome, resumeOutcomeCount) ||
		!add(ids.TargetTransfer, len(core.query.transfers)) ||
		!add(ids.TargetTransferOutcome, len(core.query.transferEnds)) {
		return denominator.CountRows{}
	}
	rows, ok := denominator.NewCountRows(counts)
	if !ok {
		return denominator.CountRows{}
	}
	return rows
}
