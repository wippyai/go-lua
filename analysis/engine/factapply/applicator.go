package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	valuerefine "github.com/wippyai/go-lua/analysis/domain/value/refinement"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// PathTypeProjector projects a path through a structural root type. Engine
// packages receive this from the language/check layer so they stay syntax-free.
type PathTypeProjector func(root typ.Type, path pathdom.Path) (typ.Type, bool)

// CovariantWiden rebuilds a covariantly-exposed object's witness type. Given the
// object's current witness type, the exposure contract type, and the source path
// segments locating the exposed sub-object under its ancestor symbol, it returns
// the ancestor witness type with every strictly-wider field widened to the
// contract and the deduplicated top segment of every widened leaf (so the engine
// can drop the precise per-field facts beneath it). It returns ok=false when no
// field widens. Engine packages receive this from the check layer to keep the
// subtype/unwrap reasoning out of the engine.
type CovariantWiden func(sourceWitness, contract typ.Type, segments []segment.Segment) (widened typ.Type, topWidenedSegments [][]segment.Segment, ok bool)

// ClosedDynamicAllValueInvariant states that every present value reachable via
// Container is a key of Table. The program layer infers these for closed
// reverse-map writer sets; fact application seeds them when the container is
// created as a fresh empty table.
type ClosedDynamicAllValueInvariant struct {
	Container pathdom.Path
	Table     pathdom.Path
	// Site is the exact admitted writer whose values satisfy the invariant.
	// Empty retains the site-independent all-values theorem only.
	Site dynamicindex.Site
}

func activeBranchRefinementHasStrictPrefix(refinements []ActiveBranchRefinement, target pathdom.Path) bool {
	if target.Symbol == 0 || len(target.Segments) == 0 {
		return false
	}
	for _, fact := range refinements {
		if fact.targetPath.Symbol != target.Symbol || len(fact.targetPath.Segments) >= len(target.Segments) {
			continue
		}
		if branchRefinementSegmentsHavePrefix(target.Segments, fact.targetPath.Segments) {
			return true
		}
	}
	return false
}

func branchRefinementSegmentsHavePrefix(target []segment.Segment, prefix []segment.Segment) bool {
	if len(prefix) > len(target) {
		return false
	}
	for i := range prefix {
		if target[i] != prefix[i] {
			return false
		}
	}
	return true
}

func tokenOf(session *cancellation.Session) *cancellation.Token {
	if session == nil {
		return nil
	}
	return session.Token()
}

func stateIsBottom(reg *axis.Registry, st state.State) bool {
	return state.IsBottom(reg, st)
}

func unreachableState(reg *axis.Registry) state.State {
	return state.Domain(reg).Bottom()
}

func branchTruthyEvidenceContradictsCurrentValue(
	typeValues *typevalue.Cache,
	reg *axis.Registry,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
) bool {
	current, ok := branchFeasibilityValue(typeValues, reg, resolver, projectPath, point, out, targetPath)
	if !ok {
		return false
	}
	return !valuerefine.CanBeTruthy(reg, current)
}

func branchFalsyEvidenceContradictsCurrentValue(
	typeValues *typevalue.Cache,
	reg *axis.Registry,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
) bool {
	current, ok := branchFeasibilityValue(typeValues, reg, resolver, projectPath, point, out, targetPath)
	if !ok {
		return false
	}
	return !valuerefine.CanBeFalsy(reg, current)
}
