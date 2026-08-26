package relation

import (
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	"github.com/wippyai/go-lua/domain/placement/birth"
	"github.com/wippyai/go-lua/domain/placement/formal"
	placementpublicationescape "github.com/wippyai/go-lua/domain/placement/publicationescape"
	placementreturnescape "github.com/wippyai/go-lua/domain/placement/returnescape"
	placementtransfer "github.com/wippyai/go-lua/domain/placement/transfer"
)

// PlacementAllocationBirthOperation is domain/placement/birth's own judgment
// for the initial placement of an owner-issued allocation receipt.
type PlacementAllocationBirthOperation struct{}

// Available reports whether the operation carries its owner mathematics.
func (PlacementAllocationBirthOperation) Available() bool { return true }

// Evaluate answers the initial Placement fact after Value has authenticated
// both the allocation receipt and the exact fresh value it published.
func (PlacementAllocationBirthOperation) Evaluate(argument PlacementAllocationBirthArgument, emitter *relbindgen.Emitter[placementdomain.Fact]) outcome.Code {
	fact, reduction := birth.Allocation(argument.Candidate, argument.Allocated)
	return relbindgen.Reduce(emitter, fact, reduction)
}

// PlacementFreshBirthOperation is domain/placement/birth's own judgment for
// the placement a fresh call result is born at.
type PlacementFreshBirthOperation struct{}

// Available reports whether the operation carries its owner mathematics.
func (PlacementFreshBirthOperation) Available() bool { return true }

// Evaluate answers the birth placement of one fresh call result.
func (PlacementFreshBirthOperation) Evaluate(argument PlacementFreshBirthArgument, emitter *relbindgen.Emitter[placementdomain.Fact]) outcome.Code {
	fact, reduction := birth.Fresh(argument.Candidate, argument.Result)
	return relbindgen.Reduce(emitter, fact, reduction)
}

// PlacementFormalOperation is domain/placement/formal's own fold: the
// placement a formal route carries its predecessor to.
type PlacementFormalOperation struct{}

// Available reports whether the operation carries its owner mathematics.
func (PlacementFormalOperation) Available() bool { return true }

// Evaluate answers one formal placement at the route its tag names.
func (PlacementFormalOperation) Evaluate(argument PlacementFormalArgument, emitter *relbindgen.Emitter[placementdomain.Fact]) outcome.Code {
	fact, reduction := formal.FormalFold(argument.RouteTag, argument.Selected)
	return relbindgen.Reduce(emitter, fact, reduction)
}

// PlacementPublicationEscapeOperation is placement/publicationescape's own
// judgment for applying an authenticated publication requirement to a
// selected Placement fact.
type PlacementPublicationEscapeOperation struct{}

// Available reports whether the operation carries its owner mathematics.
func (PlacementPublicationEscapeOperation) Available() bool { return true }

// Evaluate answers one publication-escape displacement through the owner fold.
func (PlacementPublicationEscapeOperation) Evaluate(argument PlacementPublicationEscapeArgument, emitter *relbindgen.Emitter[placementdomain.Fact]) outcome.Code {
	fact, reduction := placementpublicationescape.PublicationEscapeFold(argument.Requirement, argument.Current)
	return relbindgen.Reduce(emitter, fact, reduction)
}

// PlacementReturnEscapeOperation is placement/returnescape's own judgment for
// applying an authenticated return route to a selected Placement fact.
type PlacementReturnEscapeOperation struct{}

// Available reports whether the operation carries its owner mathematics.
func (PlacementReturnEscapeOperation) Available() bool { return true }

// Evaluate answers one return-escape displacement through the owner fold.
func (PlacementReturnEscapeOperation) Evaluate(argument PlacementReturnEscapeArgument, emitter *relbindgen.Emitter[placementdomain.Fact]) outcome.Code {
	fact, reduction := placementreturnescape.ReturnEscapeFold(argument.RouteTag, argument.Current)
	return relbindgen.Reduce(emitter, fact, reduction)
}

// PlacementTransferOperation is placement/transfer's own judgment for
// applying an authenticated Send route to a selected Placement fact.
type PlacementTransferOperation struct{}

// Available reports whether the operation carries its owner mathematics.
func (PlacementTransferOperation) Available() bool { return true }

// Evaluate answers one transfer displacement through the owner fold.
func (PlacementTransferOperation) Evaluate(argument PlacementTransferArgument, emitter *relbindgen.Emitter[placementdomain.Fact]) outcome.Code {
	fact, reduction := placementtransfer.TransferFold(argument.RouteTag, argument.Selected)
	return relbindgen.Reduce(emitter, fact, reduction)
}
