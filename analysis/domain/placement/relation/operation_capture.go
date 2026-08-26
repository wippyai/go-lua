package relation

import (
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	placementcapture "github.com/wippyai/go-lua/domain/placement/capture"
)

// PlacementCaptureOperation is Placement Capture's owner judgment. The
// route key is carried as Heap-owned evidence from the declared route join;
// the capture fold consumes the correlated route tag and Placement facts.
// Keeping the key in this frame preserves the authored four-input protocol,
// while route validity remains a property of the owner route evidence.
type PlacementCaptureOperation struct{}

// Available reports whether the operation carries its owner mathematics.
func (PlacementCaptureOperation) Available() bool { return true }

// Evaluate applies CaptureFold to the exact typed scalars delivered by the
// sealed signature. A non-allocation route is malformed route evidence and is
// refused, matching the mounted hot protocol's nearest invalid-route path.
func (PlacementCaptureOperation) Evaluate(argument PlacementCaptureArgument, emitter *relbindgen.Emitter[placementdomain.Fact]) outcome.Code {
	if !argument.Route.Valid() || argument.Route.Kind() != heapdomain.RootAllocation {
		return outcome.Refused
	}
	fact, reduction := placementcapture.CaptureFold(argument.Parent, argument.RouteTag, argument.Current)
	return relbindgen.Reduce(emitter, fact, reduction)
}
