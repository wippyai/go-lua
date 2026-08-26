package relation

import (
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen"
	suspensiondomain "github.com/wippyai/go-lua/domain/placement/suspension"
)

// PlacementSuspensionEvidenceOperation is the Placement-owned evidence
// judgment. The complete Value vector has already been reduced to the
// owner-issued SourceSummary by the route relation; this operation receives
// only that scalar, the route evidence, and the selected Evidence cell.
type PlacementSuspensionEvidenceOperation struct{}

// Available reports whether the owner judgment is present.
func (PlacementSuspensionEvidenceOperation) Available() bool { return true }

// Evaluate delegates the concrete evidence consequence to Suspension. The
// binding only carries typed owner values and the closed disposition.
func (PlacementSuspensionEvidenceOperation) Evaluate(argument PlacementSuspensionEvidenceArgument, emitter *relbindgen.Emitter[suspensiondomain.Evidence]) outcome.Code {
	fact, reduction := suspensiondomain.SuspensionEvidenceFold(argument.Candidate, argument.SourceSummary, argument.Route, argument.RouteTag, argument.Selected)
	return relbindgen.Reduce(emitter, fact, reduction)
}
