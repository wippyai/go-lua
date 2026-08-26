package relation

import (
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	"github.com/wippyai/go-lua/domain/placement/suspension"
)

// PlacementSuspensionOperation is the Placement-owned scalar judgment for
// suspension. SuspensionRoutes has already consumed the complete Value
// delivery and retained SourceSummary on the selected route row, so this
// operation has no per-solve state, vector adapter, or Value schema handle.
type PlacementSuspensionOperation struct{}

// Available reports whether the stateless owner judgment is present.
func (PlacementSuspensionOperation) Available() bool { return true }

// Evaluate delegates the irreducible scalar fold. Route geometry and source
// summarization are owner-derived before this binding is invoked.
func (PlacementSuspensionOperation) Evaluate(argument PlacementSuspensionArgument, emitter *relbindgen.Emitter[placementdomain.Fact]) outcome.Code {
	fact, reduction := suspension.SuspensionFold(argument.Candidate, argument.SourceSummary, argument.Route, argument.RouteTag, argument.Selected)
	return relbindgen.Reduce(emitter, fact, reduction)
}
