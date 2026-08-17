package target

import (
	"errors"

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

// buildCountRows derives the Target-owned cardinalities directly from the
// sealed Contract queries. The schema denominator owns the relation keys;
// Target owns only the native measurements.
func (c *Contract) buildCountRows() (denominator.CountRows, error) {
	if c == nil || len(c.operations) == 0 || c.opaque != Operation(len(c.operations)) {
		return denominator.CountRows{}, errCountRows
	}

	ids := denominator.GeneratedTargetIDs()
	counts := make(map[schema.EntryID]uint64, 37)
	put := func(key schema.EntryID, value uint64) {
		counts[key] = value
	}
	add := func(key schema.EntryID, value int) bool {
		if value < 0 {
			return false
		}
		current := counts[key]
		addition := uint64(value)
		if ^uint64(0)-current < addition {
			return false
		}
		counts[key] = current + addition
		return true
	}

	put(ids.TargetContract, 1)
	put(ids.TargetOperation, 0)
	put(ids.TargetABI, 0)
	put(ids.TargetSubedge, 0)
	put(ids.TargetCallback, 0)
	put(ids.TargetBinding, 0)
	put(ids.TargetResume, 0)
	put(ids.TargetSpawn, 0)
	put(ids.TargetOpaque, 0)
	put(ids.TargetOperationEffect, 0)
	put(ids.TargetCallbackEffect, 0)
	put(ids.TargetCallbackRelease, 0)
	put(ids.TargetOutcome, 0)
	put(ids.TargetTransfer, 0)
	put(ids.TargetTransferOutcome, 0)
	put(ids.TargetSuspension, 0)
	put(ids.TargetResumeOutcome, 0)
	put(ids.TargetSpawnSibling, 0)
	put(ids.TargetSubedgeArgumentOrigin, 0)
	put(ids.TargetCallbackResult, 0)
	put(ids.TargetResultAlias, 0)
	put(ids.TargetProduced, 0)
	put(ids.TargetProducedCapture, 0)
	put(ids.TargetFreshResult, 0)
	put(ids.TargetPublicationEffect, 0)
	put(ids.TargetProtocol, 0)
	put(ids.TargetProtocolState, 0)
	put(ids.TargetProtocolAcquisition, 0)
	put(ids.TargetProtocolTransition, 0)
	put(ids.TargetProtocolTransitionOutcome, 0)
	put(ids.TargetProtocolEscape, 0)
	put(ids.TargetProtocolCallbackHolder, 0)
	put(ids.TargetBoot, uint64(c.InitialRootCount()))
	put(ids.TargetBootEntry, uint64(c.InitialEntryCount()))
	put(ids.TargetBootMetatableAttachment, uint64(c.InitialMetatableAttachmentCount()))
	put(ids.TargetBootBinding, uint64(c.InitialBindingCount()))
	put(ids.TargetSubedgeRelation, 0)

	for index := 0; index < c.OperationCount(); index++ {
		op, ok := c.OperationAt(index)
		if !ok {
			return denominator.CountRows{}, errCountRows
		}
		if !add(ids.TargetOperation, 1) || !add(ids.TargetABI, 1) {
			return denominator.CountRows{}, errCountRows
		}
		if !add(ids.TargetOperationEffect, c.EffectCount(op)) ||
			!add(ids.TargetSubedge, c.SubedgeCount(op)) ||
			!add(ids.TargetBinding, c.BindingCount(op)) ||
			!add(ids.TargetResume, c.ResumeCount(op)) ||
			!add(ids.TargetSpawn, c.SpawnCount(op)) ||
			!add(ids.TargetSuspension, c.SuspensionCount(op)) ||
			!add(ids.TargetTransfer, c.TransferCount(op)) {
			return denominator.CountRows{}, errCountRows
		}
		for effect := 0; effect < c.EffectCount(op); effect++ {
			if _, found := c.PublicationEffectDescriptor(op, effect); found && !add(ids.TargetPublicationEffect, 1) {
				return denominator.CountRows{}, errCountRows
			}
		}
		for subedge := 0; subedge < c.SubedgeCount(op); subedge++ {
			edge, found := c.SubedgeAt(op, subedge)
			if !found || !add(ids.TargetSubedgeArgumentOrigin, c.ArgumentOriginCount(edge)) {
				return denominator.CountRows{}, errCountRows
			}
		}
		for transfer := 0; transfer < c.TransferCount(op); transfer++ {
			if !add(ids.TargetTransferOutcome, c.TransferOutcomeCount(op, transfer)) {
				return denominator.CountRows{}, errCountRows
			}
		}
		for callbackIndex := 0; callbackIndex < c.CallbackCount(op); callbackIndex++ {
			callback, found := c.CallbackAt(op, callbackIndex)
			if !found {
				return denominator.CountRows{}, errCountRows
			}
			if !add(ids.TargetCallback, 1) ||
				!add(ids.TargetCallbackEffect, c.CallbackEffectCount(callback)) {
				return denominator.CountRows{}, errCountRows
			}
			for effect := 0; effect < c.CallbackEffectCount(callback); effect++ {
				if _, found := c.CallbackPublicationEffectDescriptor(callback, effect); found && !add(ids.TargetPublicationEffect, 1) {
					return denominator.CountRows{}, errCountRows
				}
			}
			if _, _, _, _, found := c.CallbackRelease(callback); found && !add(ids.TargetCallbackRelease, 1) {
				return denominator.CountRows{}, errCountRows
			}
		}
		for resumeIndex := 0; resumeIndex < c.ResumeCount(op); resumeIndex++ {
			resume, found := c.ResumeIDAt(op, resumeIndex)
			if !found || !add(ids.TargetResumeOutcome, c.ResumeOutcomeCount(resume)) {
				return denominator.CountRows{}, errCountRows
			}
		}
		for spawnIndex := 0; spawnIndex < c.SpawnCount(op); spawnIndex++ {
			spawn, found := c.SpawnIDAt(op, spawnIndex)
			if !found || !add(ids.TargetSpawnSibling, c.SpawnSiblingCount(spawn)) {
				return denominator.CountRows{}, errCountRows
			}
		}
		for outcome := 0; outcome < c.OutcomeCount(op); outcome++ {
			if !add(ids.TargetOutcome, 1) ||
				!add(ids.TargetCallbackResult, c.CallbackResultCount(op, outcome)) ||
				!add(ids.TargetResultAlias, c.ResultAliasCount(op, outcome)) ||
				!add(ids.TargetFreshResult, c.FreshResultCount(op, outcome)) ||
				!add(ids.TargetProduced, c.ProducedCount(op, outcome)) {
				return denominator.CountRows{}, errCountRows
			}
			for produced := 0; produced < c.ProducedCount(op, outcome); produced++ {
				if !add(ids.TargetProducedCapture, c.ProducedCaptureCount(op, outcome, produced)) {
					return denominator.CountRows{}, errCountRows
				}
			}
		}
		if _, _, _, _, _, found := c.OperationSubedgeRelation(op); found && !add(ids.TargetSubedgeRelation, 1) {
			return denominator.CountRows{}, errCountRows
		}
	}
	put(ids.TargetOpaque, 1)

	for index := 0; index < c.ProtocolCount(); index++ {
		protocol, ok := c.ProtocolAt(index)
		if !ok {
			return denominator.CountRows{}, errCountRows
		}
		if !add(ids.TargetProtocol, 1) ||
			!add(ids.TargetProtocolState, c.StateCount(protocol)) ||
			!add(ids.TargetProtocolAcquisition, c.ProtocolAcquisitionCount(protocol)) ||
			!add(ids.TargetProtocolEscape, c.EscapeCount(protocol)) ||
			!add(ids.TargetProtocolCallbackHolder, c.ProtocolCallbackHolderCount(protocol)) {
			return denominator.CountRows{}, errCountRows
		}
		for transition := 0; transition < c.TransitionCount(protocol); transition++ {
			if !add(ids.TargetProtocolTransition, 1) ||
				!add(ids.TargetProtocolTransitionOutcome, c.TransitionOutcomeCount(protocol, transition)) {
				return denominator.CountRows{}, errCountRows
			}
		}
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
	if !ok {
		return denominator.CountRows{}, errCountRows
	}
	expected := make(map[schema.EntryID]struct{})
	for _, entry := range denominator.GeneratedRelationEntries() {
		if entry == nil {
			return denominator.CountRows{}, errCountRows
		}
		if entry.Owner() == denominator.RelationOwnerTarget {
			expected[entry.ID()] = struct{}{}
		}
	}
	if len(expected) == 0 || sealed.Count() != len(expected) {
		return denominator.CountRows{}, errCountRows
	}
	for index := 0; index < sealed.Count(); index++ {
		row, rowOK := sealed.At(index)
		if !rowOK {
			return denominator.CountRows{}, errCountRows
		}
		if _, known := expected[row.ID()]; !known {
			return denominator.CountRows{}, errCountRows
		}
	}
	return sealed, nil
}
