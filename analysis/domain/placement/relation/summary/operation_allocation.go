package summary

import (
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	"github.com/wippyai/go-lua/domain/placement/suspension"
)

// PlacementSummaryAllocationOperation is the terminal Placement summary
// judgment.  It is deliberately a query-site value: the exact sealed
// Placement schema is its only retained authority, while all five relation
// inputs are borrowed for one invocation.  The operation owns neither a
// store nor a cache and emits only the child rows its binding declared.
//
// Heap roots are the one heterogeneous input.  Their full span includes
// Boot roots because Boot may make an allocation non-deep-frozen; only the
// allocation projection is emitted.  Containment is derived once per
// invocation, before the allocation loop, so the graph is O(H) rather than
// rebuilt once per allocation.
type PlacementSummaryAllocationOperation struct {
	schema placementdomain.Schema
}

// NewPlacementSummaryAllocationOperation adopts one exact sealed Placement
// schema.  Placement's schema already retains Heap's owner and coordinate
// authority; the operation does not issue a second key directory.
func NewPlacementSummaryAllocationOperation(schema placementdomain.Schema) (PlacementSummaryAllocationOperation, bool) {
	if !schema.Valid() {
		return PlacementSummaryAllocationOperation{}, false
	}
	return PlacementSummaryAllocationOperation{schema: schema}, true
}

// Available reports whether the terminal operation carries its sealed
// Placement authority.
func (operation PlacementSummaryAllocationOperation) Available() bool {
	return operation.schema.Valid()
}

// Schema returns the exact Placement authority adopted by the operation.
func (operation PlacementSummaryAllocationOperation) Schema() placementdomain.Schema {
	if !operation.Available() {
		return placementdomain.Schema{}
	}
	return operation.schema
}

// Evaluate performs one complete allocation-summary query.  It validates all
// input widths and statuses before publishing anything, so Opaque and
// Refused answers can never leave a partial child relation in the emitter.
//
// The child relation is dense in the Placement allocation denominator.  A
// Heap span is dense in the complete Heap root denominator, which may include
// Boot roots and may therefore have a different width.  Placement's sealed
// AllocationKeyAt/Heap.KeyIndex mapping is the only bridge between those
// orders.
func (operation PlacementSummaryAllocationOperation) Evaluate(argument PlacementSummaryAllocationArgument, emitter *relbindgen.Emitter[AllocationRow]) outcome.Code {
	if !operation.Available() || emitter == nil || !validAllocationSummaryWidths(operation.schema, argument) {
		return outcome.Refused
	}

	// Metadata is required by the declaration. Validate its complete physical
	// pair before doing graph work, but do not reconstruct Source from Heap's
	// schema here. Heap's published metadata relation is the sole source for
	// that payload; this operation only checks the existing allocation ID
	// against Placement's exact sealed coordinate.
	if !validateAllocationSummaryMetadata(operation.schema, argument) {
		return outcome.Refused
	}

	// Read presence separately from values. Span.At intentionally presents a
	// common value/present view and therefore cannot distinguish an unfinished
	// miss from proven absence. The distinction is load-bearing here.
	heapStatus := classifyAllocationSummaryInputs(operation.schema, argument)
	if heapStatus != summaryInputReady {
		return heapStatus.code()
	}

	// The graph is one shared immutable projection for this query site. Its
	// adapter restores Heap's exact owner Bottom for a ProvenAbsent cell; this
	// is not a fabricated Placement result, but the Heap lattice's canonical
	// sparse value required by DeriveContainmentEvidence.
	containment, containmentOK := placementdomain.DeriveContainmentEvidence(operation.schema, heapRootRows{
		schema: operation.schema.Heap(),
		span:   argument.HeapRoots,
		status: heapStatus,
	})
	if !containmentOK {
		return outcome.Refused
	}

	for index := 0; index < operation.schema.KeyCount(); index++ {
		metadataRow, metadataPresent, metadataOK := argument.MetadataAt(index)
		if !metadataOK || !metadataPresent {
			return outcome.Refused
		}
		key, keyOK := operation.schema.KeyAt(index)
		if !keyOK {
			return outcome.Refused
		}

		// All input statuses were preflighted, so this branch cannot discover
		// Opaque or UnprovenMissing after an earlier row has been staged.
		placementPresence, placementPresenceOK := argument.PlacementFacts.PresenceAt(index)
		if !placementPresenceOK {
			return outcome.Refused
		}
		switch {
		case placementPresence.Is(model.ProvenAbsent):
			if !emitter.PutAbsent() {
				return outcome.Refused
			}
			continue
		case placementPresence.Is(model.Present):
			// Continue below.
		default:
			return outcome.Refused
		}

		fact, factPresent, factOK := argument.PlacementFacts.At(index)
		if !factOK || !factPresent {
			return outcome.Refused
		}
		fact, factOK = placementdomain.AuthenticateFactCell(fact, true, true)
		if !factOK {
			return outcome.Refused
		}
		base, baseOK := AllocationEvidenceForMetadata(metadataRow, fact)
		if !baseOK {
			return outcome.Refused
		}

		containmentEvidence, containmentOK := containment.Evidence(key)
		if !containmentOK {
			return outcome.Refused
		}
		composed, composedOK := placementdomain.ComposeAllocationEvidence(base, containmentEvidence)
		if !composedOK {
			return outcome.Refused
		}

		suspensionPresence, suspensionPresenceOK := argument.SuspensionEvidence.PresenceAt(index)
		if !suspensionPresenceOK {
			return outcome.Refused
		}
		if suspensionPresence.Is(model.Present) {
			state, statePresent, stateOK := argument.SuspensionEvidence.At(index)
			if !stateOK || !statePresent {
				return outcome.Refused
			}
			state, stateOK = suspension.AuthenticateEvidenceCell(state, true, true)
			if !stateOK {
				return outcome.Refused
			}
			public := state.Public()
			if !public.Valid() || public == placementdomain.EvidenceAbsent {
				return outcome.Refused
			}
			composed, composedOK = placementdomain.ComposeAllocationEvidence(composed, placementdomain.AllocationEvidence{
				DiesBeforeSuspension: public,
			})
			if !composedOK {
				return outcome.Refused
			}
		}

		row, rowOK := NewAllocationRow(metadataRow.ID(), fact, composed)
		if !rowOK || !emitter.Put(row) {
			return outcome.Refused
		}
	}
	return outcome.Produced
}

// summaryInputStatus is kept private to the operation. A complete input can
// be ready, refused, or opaque; an unfinished miss is refusal here because a
// terminal dense child result cannot be published until the input relation is
// complete. No status is converted into a semantic Unknown or a default row.
type summaryInputStatus uint8

const (
	summaryInputReady summaryInputStatus = iota + 1
	summaryInputRefused
	summaryInputOpaque
)

func (status summaryInputStatus) code() outcome.Code {
	if status == summaryInputOpaque {
		return outcome.Opaque
	}
	return outcome.Refused
}

func validAllocationSummaryWidths(schema placementdomain.Schema, argument PlacementSummaryAllocationArgument) bool {
	if !schema.Valid() {
		return false
	}
	allocationCount := schema.KeyCount()
	heapCount := schema.Heap().KeyCount()
	return argument.AllocationIDs.Len() == allocationCount &&
		argument.AllocationSources.Len() == allocationCount &&
		argument.PlacementFacts.Len() == allocationCount &&
		argument.SuspensionEvidence.Len() == allocationCount &&
		argument.HeapRoots.Len() == heapCount
}

// classifyAllocationSummaryInputs performs the presence and owner-value pass
// before any emitter writes. It intentionally retains no per-row state; the
// output pass re-reads the same borrowed spans. Placement and Suspension may
// be sparse, but an unproven miss is not a result and authenticated opacity is
// a terminal opaque answer.
func classifyAllocationSummaryInputs(schema placementdomain.Schema, argument PlacementSummaryAllocationArgument) summaryInputStatus {
	opaque := false
	allocationCount := schema.KeyCount()
	for index := 0; index < allocationCount; index++ {
		placementPresence, placementOK := argument.PlacementFacts.PresenceAt(index)
		suspensionPresence, suspensionOK := argument.SuspensionEvidence.PresenceAt(index)
		if !placementOK || !suspensionOK {
			return summaryInputRefused
		}
		placementKind, placementValid := summaryPresenceKind(placementPresence)
		suspensionKind, suspensionValid := summaryPresenceKind(suspensionPresence)
		if !placementValid || !suspensionValid {
			return summaryInputRefused
		}
		switch placementKind {
		case model.Present:
			fact, present, available := argument.PlacementFacts.At(index)
			if !present || !available {
				return summaryInputRefused
			}
			if _, ok := placementdomain.AuthenticateFactCell(fact, true, true); !ok {
				return summaryInputRefused
			}
		case model.AuthenticatedOpaque:
			opaque = true
		case model.ProvenAbsent:
		default:
			return summaryInputRefused
		}
		switch suspensionKind {
		case model.Present:
			state, present, available := argument.SuspensionEvidence.At(index)
			if !present || !available {
				return summaryInputRefused
			}
			state, ok := suspension.AuthenticateEvidenceCell(state, true, true)
			if !ok {
				return summaryInputRefused
			}
			public := state.Public()
			if !public.Valid() || public == placementdomain.EvidenceAbsent {
				return summaryInputRefused
			}
		case model.AuthenticatedOpaque:
			opaque = true
		case model.ProvenAbsent:
		default:
			return summaryInputRefused
		}
	}
	// Heap presence is checked separately because its dense denominator also
	// includes Boot. The exact sparse Bottom is restored by heapRootRows below.
	for index := 0; index < schema.Heap().KeyCount(); index++ {
		presence, presenceOK := argument.HeapRoots.PresenceAt(index)
		if !presenceOK || !presence.Available() || presence.Is(model.Refused) {
			return summaryInputRefused
		}
		if presence.Is(model.UnprovenMissing) {
			return summaryInputRefused
		}
		if presence.Is(model.AuthenticatedOpaque) {
			opaque = true
			continue
		}
		if !presence.Is(model.Present) && !presence.Is(model.ProvenAbsent) {
			return summaryInputRefused
		}
	}
	if opaque {
		return summaryInputOpaque
	}
	return summaryInputReady
}

func summaryPresenceKind(presence model.Presence) (model.PresenceKind, bool) {
	if !presence.Available() || presence.Is(model.Refused) {
		return model.InvalidPresence, false
	}
	switch {
	case presence.Is(model.Present):
		return model.Present, true
	case presence.Is(model.ProvenAbsent):
		return model.ProvenAbsent, true
	case presence.Is(model.UnprovenMissing):
		return model.UnprovenMissing, true
	case presence.Is(model.AuthenticatedOpaque):
		return model.AuthenticatedOpaque, true
	default:
		return model.InvalidPresence, false
	}
}

func validateAllocationSummaryMetadata(schema placementdomain.Schema, argument PlacementSummaryAllocationArgument) bool {
	for index := 0; index < schema.KeyCount(); index++ {
		idPresence, idOK := argument.AllocationIDs.PresenceAt(index)
		sourcePresence, sourceOK := argument.AllocationSources.PresenceAt(index)
		if !idOK || !sourceOK || !idPresence.Is(model.Present) || !sourcePresence.Is(model.Present) {
			return false
		}
		metadata, present, metadataOK := argument.MetadataAt(index)
		if !metadataOK || !present || !metadata.Valid() {
			return false
		}
		key, keyOK := schema.KeyAt(index)
		if !keyOK {
			return false
		}
		allocationID, allocationIDOK := key.ContentID()
		if !allocationIDOK || metadata.ID() != allocationID {
			return false
		}
	}
	return true
}

// heapRootRows adapts the borrowed binding span to Placement's neutral
// HeapRootRows seam. A proven-absent Heap cell is exactly the owner's Bottom;
// an unfinished miss is rejected before this method is called.
type heapRootRows struct {
	schema heapdomain.Schema
	span   relbindgen.Span[heapdomain.Value]
	status summaryInputStatus
}

func (rows heapRootRows) Len() int { return rows.span.Len() }

func (rows heapRootRows) At(index int) (heapdomain.Value, bool, bool) {
	if !rows.schema.Valid() || rows.status != summaryInputReady || index < 0 || index >= rows.span.Len() {
		return heapdomain.Value{}, false, false
	}
	presence, presenceOK := rows.span.PresenceAt(index)
	if !presenceOK || !presence.Available() || presence.Is(model.Refused) || presence.Is(model.UnprovenMissing) || presence.Is(model.AuthenticatedOpaque) {
		return heapdomain.Value{}, false, false
	}
	if presence.Is(model.ProvenAbsent) {
		return rows.schema.Bottom(), false, true
	}
	if !presence.Is(model.Present) {
		return heapdomain.Value{}, false, false
	}
	value, present, available := rows.span.At(index)
	if !available || !present {
		return heapdomain.Value{}, false, false
	}
	return value, true, true
}
