package summary

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
)

// PlacementSummaryParentOperation is the Placement-owned parent judgment for
// one selected QuerySite. It retains only the exact sealed Placement schema
// identity; child rows are borrowed from the invocation and no result or
// relation is cached here.
type PlacementSummaryParentOperation struct {
	schemaID identity.ContentID
}

var _ relbindgen.Operation[PlacementSummaryParentArgument, ParentAnswer] = PlacementSummaryParentOperation{}

// NewPlacementSummaryParentOperation adopts one sealed Placement schema as
// the sole authority for the parent answer's schema identity.
func NewPlacementSummaryParentOperation(schema placementdomain.Schema) (PlacementSummaryParentOperation, bool) {
	if !schema.Valid() {
		return PlacementSummaryParentOperation{}, false
	}
	schemaID := schema.ContentID()
	if !schemaID.Available() {
		return PlacementSummaryParentOperation{}, false
	}
	return PlacementSummaryParentOperation{schemaID: schemaID}, true
}

// Available reports whether the operation still carries its sealed Placement
// authority.
func (operation PlacementSummaryParentOperation) Available() bool {
	return operation.schemaID.Available()
}

// SchemaID returns the exact sealed Placement identity adopted by the
// operation. The parent does not retain Heap keys or a second Placement
// directory: the complete child denominator is already authenticated by the
// binding contract.
func (operation PlacementSummaryParentOperation) SchemaID() (identity.ContentID, bool) {
	if !operation.Available() {
		return identity.ContentID{}, false
	}
	return operation.schemaID, true
}

// Evaluate reduces the complete child relation to one optional parent answer.
// A parent answer means that at least one complete, present child row exists;
// it carries no aggregate payload beyond the exact Placement schema identity.
//
// The operation first validates every row and status before emitting anything.
// This gives the parent a transactional semantic boundary: a malformed or
// unfinished later row cannot leave an earlier answer staged in the emitter.
func (operation PlacementSummaryParentOperation) Evaluate(argument PlacementSummaryParentArgument, emitter *relbindgen.Emitter[ParentAnswer]) outcome.Code {
	if !operation.Available() || emitter == nil || !sameParentSpanWidths(argument) {
		return outcome.Refused
	}

	present := false
	opaque := false
	for index := 0; index < argument.AllocationIDs.Len(); index++ {
		allocationPresence, allocationOK := argument.AllocationIDs.PresenceAt(index)
		factPresence, factOK := argument.Facts.PresenceAt(index)
		evidencePresence, evidenceOK := argument.Evidence.PresenceAt(index)
		if !allocationOK || !factOK || !evidenceOK || !allParentPresenceValid(allocationPresence, factPresence, evidencePresence) {
			return outcome.Refused
		}

		switch {
		case allocationPresence.Is(model.ProvenAbsent) && factPresence.Is(model.ProvenAbsent) && evidencePresence.Is(model.ProvenAbsent):
			// A child denominator member with no child value is a proved
			// absence. It contributes no parent answer and is not replaced by
			// a domain default.
			continue
		case allocationPresence.Is(model.AuthenticatedOpaque) && factPresence.Is(model.AuthenticatedOpaque) && evidencePresence.Is(model.AuthenticatedOpaque):
			// Opaque is a settled result, but it cannot prove that a parent
			// answer exists. A present row elsewhere may still prove one.
			opaque = true
			continue
		case allocationPresence.Is(model.Present) && factPresence.Is(model.Present) && evidencePresence.Is(model.Present):
			allocationID, allocationPresent, allocationAvailable := argument.AllocationIDs.At(index)
			fact, factPresent, factAvailable := argument.Facts.At(index)
			evidence, evidencePresent, evidenceAvailable := argument.Evidence.At(index)
			if !allocationAvailable || !allocationPresent || !factAvailable || !factPresent || !evidenceAvailable || !evidencePresent {
				return outcome.Refused
			}
			if _, rowOK := NewAllocationRow(allocationID, fact, evidence); !rowOK {
				return outcome.Refused
			}
			present = true
		default:
			// Mixed child-column statuses and all UnprovenMissing are not
			// settled parent evidence. They refuse/defer rather than being
			// collapsed into absence or opaque.
			return outcome.Refused
		}
	}

	if present {
		answer, answerOK := NewParentAnswer(operation.schemaID)
		// The canonical Q parent output is address-only/opaque. Preserve the
		// exact schema identity in the value token while publishing the
		// authenticated-opaque status required by its owner contract.
		if !answerOK || !emitter.PutOpaque(answer) {
			return outcome.Refused
		}
		return outcome.Produced
	}
	if opaque {
		return outcome.Opaque
	}
	return outcome.NoSelection
}

func sameParentSpanWidths(argument PlacementSummaryParentArgument) bool {
	return argument.AllocationIDs.Len() == argument.Facts.Len() && argument.AllocationIDs.Len() == argument.Evidence.Len()
}

func allParentPresenceValid(allocation, fact, evidence model.Presence) bool {
	return allocation.Available() && fact.Available() && evidence.Available() &&
		!allocation.Is(model.Refused) && !fact.Is(model.Refused) && !evidence.Is(model.Refused)
}
