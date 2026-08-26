package relation

import (
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	placementstore "github.com/wippyai/go-lua/domain/placement/store"
)

// PlacementStorageOperation is Placement Store's owner judgment. Its sealed
// frame supplies the Value-issued transfer candidate and source, then the
// Placement route tag and selected fact that the declaration's two joins
// already authenticated. Route construction remains outside this binding.
type PlacementStorageOperation struct{}

// Available reports whether the operation carries its owner mathematics.
func (PlacementStorageOperation) Available() bool { return true }

// Evaluate applies Store's one irreducible lifetime fold to the exact typed
// scalars delivered by the sealed signature.
func (PlacementStorageOperation) Evaluate(argument PlacementStorageArgument, emitter *relbindgen.Emitter[placementdomain.Fact]) outcome.Code {
	fact, reduction := placementstore.StorageFold(argument.Candidate, argument.Source, argument.RouteTag, argument.Selected)
	return relbindgen.Reduce(emitter, fact, reduction)
}
