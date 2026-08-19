package target

import (
	"errors"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
)

var errCountRows = errors.New("target: invalid denominator counts")

// CountRows returns the immutable Target denominator rows frozen at seal.
// Target counts its own already-sealed columns; no row identity, digest, or
// secondary publication is reconstructed here.
func (c *Contract) CountRows() denominator.CountRows {
	if c == nil || !c.sealed {
		return denominator.CountRows{}
	}
	return c.counts
}

// buildCountRows measures the sealed columns. Every relation of the target is a
// dense table, so its cardinality is that table's length; the two effect
// populations share one table and are told apart by the effect owner. Nothing
// here re-derives a relation by walking the indexes into those tables.
func (c *Contract) buildCountRows() (denominator.CountRows, error) {
	opaque, opaqueOK := c.Operations.Opaque()
	if c == nil || c.Operations.OperationCount() == 0 || !opaqueOK || opaque != vocabulary.Operation(c.Operations.OperationCount()) {
		return denominator.CountRows{}, errCountRows
	}

	ids := denominator.GeneratedTargetIDs()
	counts := make(map[schema.EntryID]uint64, 37)
	put := func(key schema.EntryID, value int) bool {
		if value < 0 {
			return false
		}
		counts[key] = uint64(value)
		return true
	}

	var operationEffects, callbackEffects, publicationEffects int
	for operationIndex := 0; operationIndex < c.Operations.OperationCount(); operationIndex++ {
		op := vocabulary.Operation(operationIndex + 1)
		operationEffects += c.Operations.EffectCount(op)
		for effect := 0; effect < c.Operations.EffectCount(op); effect++ {
			if _, ok := c.Operations.EffectPublication(op, effect); ok {
				publicationEffects++
			}
		}
		for callbackIndex := 0; callbackIndex < c.Operations.CallbackCount(op); callbackIndex++ {
			callback, callbackOK := c.Operations.CallbackAt(op, callbackIndex)
			if !callbackOK {
				return denominator.CountRows{}, errCountRows
			}
			callbackEffects += c.Operations.CallbackEffectCount(callback)
			for effect := 0; effect < c.Operations.CallbackEffectCount(callback); effect++ {
				if _, ok := c.Operations.CallbackEffectPublication(callback, effect); ok {
					publicationEffects++
				}
			}
		}
	}

	var callbackReleases int
	for index := range c.callbacks {
		if _, _, _, _, ok := c.callbackRelease(vocabulary.CallbackID(index + 1)); ok {
			callbackReleases++
		}
	}

	var subedgeCount, subedgeOrigins, subedgeRelations int
	for operationIndex := 0; operationIndex < c.Operations.OperationCount(); operationIndex++ {
		op := vocabulary.Operation(operationIndex + 1)
		subedgeCount += c.Operations.SubedgeCount(op)
		if _, _, _, _, _, found := c.Operations.OperationSubedgeRelation(op); found {
			subedgeRelations++
		}
		for subedgeIndex := 0; subedgeIndex < c.Operations.SubedgeCount(op); subedgeIndex++ {
			edge, edgeOK := c.Operations.SubedgeAt(op, subedgeIndex)
			if !edgeOK {
				return denominator.CountRows{}, errCountRows
			}
			subedgeOrigins += c.Operations.SubedgeArgumentOriginCount(edge)
		}
	}
	protocolCounts := c.protocols.Counts()
	bootCounts := c.Table.Counts()
	bindingCount := 0
	for operationIndex := 0; operationIndex < c.Operations.OperationCount(); operationIndex++ {
		bindingCount += c.Operations.BindingCount(vocabulary.Operation(operationIndex + 1))
	}
	transferCount, transferOutcomeCount := 0, 0
	outcomeCount, callbackCount := 0, 0
	callbackResultCount, resultAliasCount, producedCount, producedCaptureCount, freshResultCount := 0, 0, 0, 0, 0
	suspensionCount, spawnCount, resumeCount, resumeOutcomeCount := 0, 0, 0, 0
	for operationIndex := 0; operationIndex < c.Operations.OperationCount(); operationIndex++ {
		op := vocabulary.Operation(operationIndex + 1)
		outcomeCount += c.Operations.OutcomeCount(op)
		callbackCount += c.Operations.CallbackCount(op)
		suspensionCount += c.Operations.SuspensionCount(op)
		spawnCount += c.Operations.SpawnCount(op)
		resumeCount += c.Operations.ResumeCount(op)
		resumeOutcomeCount += c.Operations.ResumeCount(op) * 5
		for outcome := 0; outcome < c.Operations.OutcomeCount(op); outcome++ {
			callbackResultCount += c.Operations.CallbackResultCount(op, outcome)
			resultAliasCount += c.Operations.ResultAliasCount(op, outcome)
			producedCount += c.Operations.ProducedCount(op, outcome)
			freshResultCount += c.Operations.FreshResultCount(op, outcome)
			for produced := 0; produced < c.Operations.ProducedCount(op, outcome); produced++ {
				producedCaptureCount += c.Operations.ProducedCaptureCount(op, outcome, produced)
			}
		}
		transferCount += c.Operations.TransferCount(op)
		for index := 0; index < c.Operations.TransferCount(op); index++ {
			transferOutcomeCount += c.Operations.TransferOutcomeCount(op, index)
		}
	}

	ok := put(ids.TargetContract, 1) &&
		put(ids.TargetOpaque, 1) &&
		put(ids.TargetOperation, c.Operations.OperationCount()) &&
		put(ids.TargetABI, c.Operations.OperationCount()) &&
		put(ids.TargetBinding, bindingCount) &&
		put(ids.TargetOutcome, outcomeCount) &&
		put(ids.TargetOperationEffect, operationEffects) &&
		put(ids.TargetCallbackEffect, callbackEffects) &&
		put(ids.TargetPublicationEffect, publicationEffects) &&
		put(ids.TargetCallback, callbackCount) &&
		put(ids.TargetCallbackRelease, callbackReleases) &&
		put(ids.TargetCallbackResult, callbackResultCount) &&
		put(ids.TargetResultAlias, resultAliasCount) &&
		put(ids.TargetProduced, producedCount) &&
		put(ids.TargetProducedCapture, producedCaptureCount) &&
		put(ids.TargetFreshResult, freshResultCount) &&
		put(ids.TargetSubedge, subedgeCount) &&
		put(ids.TargetSubedgeArgumentOrigin, subedgeOrigins) &&
		put(ids.TargetSubedgeRelation, subedgeRelations) &&
		put(ids.TargetSuspension, suspensionCount) &&
		put(ids.TargetSpawn, spawnCount) &&
		put(ids.TargetSpawnSibling, spawnCount*2) &&
		put(ids.TargetResume, resumeCount) &&
		put(ids.TargetResumeOutcome, resumeOutcomeCount) &&
		put(ids.TargetTransfer, transferCount) &&
		put(ids.TargetTransferOutcome, transferOutcomeCount) &&
		put(ids.TargetProtocol, protocolCounts.Protocols) &&
		put(ids.TargetProtocolState, protocolCounts.States) &&
		put(ids.TargetProtocolAcquisition, protocolCounts.Acquisitions) &&
		put(ids.TargetProtocolTransition, protocolCounts.Transitions) &&
		put(ids.TargetProtocolTransitionOutcome, protocolCounts.TransitionOutcomes) &&
		put(ids.TargetProtocolEscape, protocolCounts.Escapes) &&
		put(ids.TargetProtocolCallbackHolder, protocolCounts.CallbackHolders) &&
		put(ids.TargetBoot, bootCounts.Roots) &&
		put(ids.TargetBootEntry, bootCounts.Entries) &&
		put(ids.TargetBootMetatableAttachment, bootCounts.MetatableAttachments) &&
		put(ids.TargetBootBinding, bootCounts.Bindings)
	if !ok {
		return denominator.CountRows{}, errCountRows
	}

	rows := make([]denominator.CountRow, 0, len(counts))
	for id, value := range counts {
		row, ok := denominator.NewCountRow(id, value)
		if !ok {
			return denominator.CountRows{}, errCountRows
		}
		rows = append(rows, row)
	}
	sealed, ok := denominator.NewCountRows(rows)
	if !ok || !denominator.GeneratedCountRowsCompleteForOwners(sealed, denominator.RelationOwnerTarget) {
		return denominator.CountRows{}, errCountRows
	}
	return sealed, nil
}
