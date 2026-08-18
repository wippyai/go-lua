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
	if c == nil || len(c.operations) == 0 || c.opaque != vocabulary.Operation(len(c.operations)) {
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
	for _, effect := range c.effects {
		switch effect.owner {
		case effectOwnerOperation:
			operationEffects++
		case effectOwnerCallback:
			callbackEffects++
		default:
			return denominator.CountRows{}, errCountRows
		}
		if c.validPublicationEffectRow(effect) {
			publicationEffects++
		}
	}

	var callbackReleases int
	for index := range c.callbacks {
		if _, _, _, _, ok := c.callbackRelease(vocabulary.CallbackID(index + 1)); ok {
			callbackReleases++
		}
	}

	var subedgeRelations int
	for index := range c.operations {
		if c.operations[index].subedgeRelation != 0 {
			subedgeRelations++
		}
	}

	ok := put(ids.TargetContract, 1) &&
		put(ids.TargetOpaque, 1) &&
		put(ids.TargetOperation, len(c.operations)) &&
		put(ids.TargetABI, len(c.operations)) &&
		put(ids.TargetBinding, len(c.bindings)) &&
		put(ids.TargetOutcome, len(c.outcomes)) &&
		put(ids.TargetOperationEffect, operationEffects) &&
		put(ids.TargetCallbackEffect, callbackEffects) &&
		put(ids.TargetPublicationEffect, publicationEffects) &&
		put(ids.TargetCallback, len(c.callbacks)) &&
		put(ids.TargetCallbackRelease, callbackReleases) &&
		put(ids.TargetCallbackResult, len(c.callbackResults)) &&
		put(ids.TargetResultAlias, len(c.resultAliases)) &&
		put(ids.TargetProduced, len(c.produced)) &&
		put(ids.TargetProducedCapture, len(c.captures)) &&
		put(ids.TargetFreshResult, len(c.fresh)) &&
		put(ids.TargetSubedge, len(c.subedges)) &&
		put(ids.TargetSubedgeArgumentOrigin, len(c.subedgeOrigins)) &&
		put(ids.TargetSubedgeRelation, subedgeRelations) &&
		put(ids.TargetSuspension, len(c.suspensions)+opaqueSuspensionCount) &&
		put(ids.TargetSpawn, len(c.spawns)) &&
		put(ids.TargetSpawnSibling, len(c.spawns)*spawnSiblingAlternatives) &&
		put(ids.TargetResume, len(c.resumes)) &&
		put(ids.TargetResumeOutcome, len(c.resumes)*crossActivationOutcomes) &&
		put(ids.TargetTransfer, len(c.transfers)) &&
		put(ids.TargetTransferOutcome, len(c.transferOutcomes)) &&
		put(ids.TargetProtocol, len(c.protocols)) &&
		put(ids.TargetProtocolState, len(c.states)) &&
		put(ids.TargetProtocolAcquisition, len(c.acquisitions)) &&
		put(ids.TargetProtocolTransition, len(c.transitions)) &&
		put(ids.TargetProtocolTransitionOutcome, len(c.transitionOutcomes)) &&
		put(ids.TargetProtocolEscape, len(c.escapes)+len(c.protocols)*derivedProtocolEscapes) &&
		put(ids.TargetProtocolCallbackHolder, len(c.callbackHolders)) &&
		put(ids.TargetBoot, len(c.initialRoots)) &&
		put(ids.TargetBootEntry, len(c.initialEntries)) &&
		put(ids.TargetBootMetatableAttachment, len(c.initialMetatables)) &&
		put(ids.TargetBootBinding, len(c.initialBindings))
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
