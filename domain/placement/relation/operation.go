package relation

import (
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	"github.com/wippyai/go-lua/domain/placement/birth"
	"github.com/wippyai/go-lua/domain/placement/formal"
	"github.com/wippyai/go-lua/domain/placement/publicationescape"
	"github.com/wippyai/go-lua/domain/placement/returnescape"
	"github.com/wippyai/go-lua/domain/placement/store"
	"github.com/wippyai/go-lua/domain/placement/suspension"
	"github.com/wippyai/go-lua/domain/placement/transfer"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// PlacementPublicationEscapeOperation is domain/placement/publicationescape's
// own fold: it raises the placement a published route requires.
type PlacementPublicationEscapeOperation struct{}

// Available reports whether the operation carries its owner mathematics. The
// fold is a package function over facts that carry their own owner, so there
// is no derived state to hold and nothing to be unavailable.
func (PlacementPublicationEscapeOperation) Available() bool { return true }

// Evaluate answers one publication escape against the placement its route
// requires.
func (PlacementPublicationEscapeOperation) Evaluate(argument PlacementPublicationEscapeArgument, emitter *relbindgen.Emitter[placementdomain.Fact]) outcome.Code {
	fact, reduction := publicationescape.PublicationEscapeFold(argument.Requirement, argument.Current)
	return relbindgen.Reduce(emitter, fact, reduction)
}

// PlacementReturnEscapeOperation is domain/placement/returnescape's own fold.
type PlacementReturnEscapeOperation struct{}

// Available reports whether the operation carries its owner mathematics.
func (PlacementReturnEscapeOperation) Available() bool { return true }

// Evaluate answers one return escape at the route its tag names.
func (PlacementReturnEscapeOperation) Evaluate(argument PlacementReturnEscapeArgument, emitter *relbindgen.Emitter[placementdomain.Fact]) outcome.Code {
	fact, reduction := returnescape.ReturnEscapeFold(argument.RouteTag, argument.Current)
	return relbindgen.Reduce(emitter, fact, reduction)
}

// PlacementTransferOperation is domain/placement/transfer's own fold, which
// displaces a placement along the route its tag names.
type PlacementTransferOperation struct{}

// Available reports whether the operation carries its owner mathematics.
func (PlacementTransferOperation) Available() bool { return true }

// Evaluate answers one placement transfer.
func (PlacementTransferOperation) Evaluate(argument PlacementTransferArgument, emitter *relbindgen.Emitter[placementdomain.Fact]) outcome.Code {
	fact, reduction := transfer.TransferFold(argument.RouteTag, argument.Selected)
	return relbindgen.Reduce(emitter, fact, reduction)
}

// PlacementAllocationBirthOperation is domain/placement/birth's own judgment
// for the placement an allocation is born at.
type PlacementAllocationBirthOperation struct{}

// Available reports whether the operation carries its owner mathematics.
func (PlacementAllocationBirthOperation) Available() bool { return true }

// Evaluate answers the birth placement of one allocation receipt.
func (PlacementAllocationBirthOperation) Evaluate(argument PlacementAllocationBirthArgument, emitter *relbindgen.Emitter[placementdomain.Fact]) outcome.Code {
	fact, reduction := birth.Allocation(argument.Candidate, argument.Allocated)
	return relbindgen.Reduce(emitter, fact, reduction)
}

// PlacementFreshBirthOperation is domain/placement/birth's own judgment for the
// placement a fresh call result is born at.
type PlacementFreshBirthOperation struct{}

// Available reports whether the operation carries its owner mathematics.
func (PlacementFreshBirthOperation) Available() bool { return true }

// Evaluate answers the birth placement of one fresh call result.
func (PlacementFreshBirthOperation) Evaluate(argument PlacementFreshBirthArgument, emitter *relbindgen.Emitter[placementdomain.Fact]) outcome.Code {
	fact, reduction := birth.Fresh(argument.Candidate, argument.Result)
	return relbindgen.Reduce(emitter, fact, reduction)
}

// PlacementFormalOperation is domain/placement/formal's own fold: the placement
// a formal route carries its predecessor to.
type PlacementFormalOperation struct{}

// Available reports whether the operation carries its owner mathematics.
func (PlacementFormalOperation) Available() bool { return true }

// Evaluate answers one formal placement at the route its tag names.
func (PlacementFormalOperation) Evaluate(argument PlacementFormalArgument, emitter *relbindgen.Emitter[placementdomain.Fact]) outcome.Code {
	fact, reduction := formal.FormalFold(argument.RouteTag, argument.Selected)
	return relbindgen.Reduce(emitter, fact, reduction)
}

// PlacementStorageOperation is domain/placement/store's own fold: the placement
// a stored value takes at the route the transfer selected.
type PlacementStorageOperation struct{}

// Available reports whether the operation carries its owner mathematics.
func (PlacementStorageOperation) Available() bool { return true }

// Evaluate answers one storage transfer's placement.
func (PlacementStorageOperation) Evaluate(argument PlacementStorageArgument, emitter *relbindgen.Emitter[placementdomain.Fact]) outcome.Code {
	fact, reduction := store.StorageFold(argument.Candidate, argument.Source, argument.RouteTag, argument.Selected)
	return relbindgen.Reduce(emitter, fact, reduction)
}

// suspensionSources is the reusable materialization both suspension folds read
// their delivered source vector through.
type suspensionSources struct {
	members *relbindgen.Members[valuedomain.Value]
}

func newSuspensionSources(reserve int) (suspensionSources, bool) {
	members, ok := relbindgen.NewMembers[valuedomain.Value](reserve)
	if !ok {
		return suspensionSources{}, false
	}
	return suspensionSources{members: members}, true
}

// PlacementSuspensionOperation is domain/placement/suspension's own fold: the
// placement a suspended subject's sources carry it to.
type PlacementSuspensionOperation struct {
	sources suspensionSources
	reserve int
}

// NewPlacementSuspensionOperation reserves the source materialization one
// caller's suspensions read through, at the width the sealed value schema
// states.
func NewPlacementSuspensionOperation(values *valuedomain.Schema) (PlacementSuspensionOperation, bool) {
	if values == nil || !values.Valid() {
		return PlacementSuspensionOperation{}, false
	}
	sources, ok := newSuspensionSources(values.CoordinateCount())
	if !ok {
		return PlacementSuspensionOperation{}, false
	}
	return PlacementSuspensionOperation{sources: sources, reserve: values.CoordinateCount()}, true
}

// NewOperation gives one solve-local worker its own materialization storage.
func (operation PlacementSuspensionOperation) NewOperation() relbindgen.Operation[PlacementSuspensionArgument, placementdomain.Fact] {
	sources, ok := newSuspensionSources(operation.reserve)
	if !ok {
		return nil
	}
	local := operation
	local.sources = sources
	return local
}

// Available reports whether the operation carries its materialization storage.
func (operation PlacementSuspensionOperation) Available() bool {
	return operation.sources.members != nil
}

// Evaluate answers one suspension's placement.
func (operation PlacementSuspensionOperation) Evaluate(argument PlacementSuspensionArgument, emitter *relbindgen.Emitter[placementdomain.Fact]) outcome.Code {
	sources, materialized := operation.sources.members.Fill(argument.Sources)
	if !materialized {
		return outcome.Refused
	}
	fact, reduction := suspension.SuspensionFold(argument.Candidate, sources, argument.Route, argument.RouteTag, argument.Selected)
	return relbindgen.Reduce(emitter, fact, reduction)
}

// PlacementSuspensionEvidenceOperation is the same fold over the evidence
// factor alone: it reads and writes evidence and never consumes placement
// class.
type PlacementSuspensionEvidenceOperation struct {
	sources suspensionSources
	reserve int
}

// NewPlacementSuspensionEvidenceOperation reserves the source materialization.
func NewPlacementSuspensionEvidenceOperation(values *valuedomain.Schema) (PlacementSuspensionEvidenceOperation, bool) {
	if values == nil || !values.Valid() {
		return PlacementSuspensionEvidenceOperation{}, false
	}
	sources, ok := newSuspensionSources(values.CoordinateCount())
	if !ok {
		return PlacementSuspensionEvidenceOperation{}, false
	}
	return PlacementSuspensionEvidenceOperation{sources: sources, reserve: values.CoordinateCount()}, true
}

// NewOperation gives one solve-local worker its own materialization storage.
func (operation PlacementSuspensionEvidenceOperation) NewOperation() relbindgen.Operation[PlacementSuspensionEvidenceArgument, suspension.Evidence] {
	sources, ok := newSuspensionSources(operation.reserve)
	if !ok {
		return nil
	}
	local := operation
	local.sources = sources
	return local
}

// Available reports whether the operation carries its materialization storage.
func (operation PlacementSuspensionEvidenceOperation) Available() bool {
	return operation.sources.members != nil
}

// Evaluate answers one suspension's evidence.
func (operation PlacementSuspensionEvidenceOperation) Evaluate(argument PlacementSuspensionEvidenceArgument, emitter *relbindgen.Emitter[suspension.Evidence]) outcome.Code {
	sources, materialized := operation.sources.members.Fill(argument.Sources)
	if !materialized {
		return outcome.Refused
	}
	fact, reduction := suspension.SuspensionEvidenceFold(argument.Candidate, sources, argument.Route, argument.RouteTag, argument.Selected)
	return relbindgen.Reduce(emitter, fact, reduction)
}
