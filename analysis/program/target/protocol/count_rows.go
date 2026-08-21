package protocol

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
)

// CountRows publishes the complete protocol-owner contribution to Target's
// denominator directly from the sealed protocol planes. Contract only joins
// this vector with the operation and boot owner vectors.
func (table Table) CountRows() denominator.CountRows {
	ids := denominator.GeneratedTargetIDs()
	counts := make([]denominator.CountRow, 0, 7)
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
	if !add(ids.TargetProtocol, table.protocols.Count()) ||
		!add(ids.TargetProtocolState, table.states.Len()) ||
		!add(ids.TargetProtocolAcquisition, table.acquisitions.Len()) ||
		!add(ids.TargetProtocolTransition, table.transitions.Len()) ||
		!add(ids.TargetProtocolTransitionOutcome, table.transitionOutcomes.Len()) ||
		!add(ids.TargetProtocolEscape, table.escapes.Len()+table.protocols.Count()) ||
		!add(ids.TargetProtocolCallbackHolder, table.callbackHolders.Len()) {
		return denominator.CountRows{}
	}
	rows, ok := denominator.NewCountRows(counts)
	if !ok {
		return denominator.CountRows{}
	}
	return rows
}
