package relation

import (
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	placementcontainment "github.com/wippyai/go-lua/domain/placement/containment"
)

// PlacementContainmentOperation is placement/containment's own scalar
// judgment. ContainmentRoutes has already authenticated the child and parent
// facts from its complete inputs; this operation only folds that selected row.
type PlacementContainmentOperation struct{}

// Available reports whether the operation carries its owner mathematics.
func (PlacementContainmentOperation) Available() bool { return true }

// Evaluate answers the containment fact from the selected child and retained
// parent facts in ContainmentFold's canonical order.
func (PlacementContainmentOperation) Evaluate(argument PlacementContainmentArgument, emitter *relbindgen.Emitter[placementdomain.Fact]) outcome.Code {
	fact, reduction := placementcontainment.ContainmentFold(argument.Current, argument.Parent)
	return relbindgen.Reduce(emitter, fact, reduction)
}
