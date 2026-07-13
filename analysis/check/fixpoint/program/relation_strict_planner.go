package program

import (
	"slices"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/compiler/ast"
)

// prepareStagedStrictRelationCatalog builds the smallest transactional
// phase-collapse slice without paying transformer preparation for the broad
// lexical corpus. Stage one deliberately admits only call-free, zero-boundary,
// stateless owners. Every potentially relevant owner and its call surface is
// scanned once, before the shared PlanCompiler prepares any survivor.
//
// This is an internal differential gate, not a production default. Expanding
// it to composed calls must retain the same cheap-funnel property and the
// whole-owner parity/lifecycle transaction enforced by its consumer.
func prepareStagedStrictRelationCatalog(reg *axis.Registry, bindings *bind.Result, keys programKeys, prepared preparedBodies, rootFn *ast.FunctionExpr, stats *Stats) relationRunCatalog {
	if reg == nil || bindings == nil {
		return relationRunCatalog{}
	}
	owners := relationCatalogOwners(keys, prepared, rootFn)
	slices.SortFunc(owners, func(a, b relationCatalogOwner) int {
		if a.key.Less(b.key) {
			return -1
		}
		if b.key.Less(a.key) {
			return 1
		}
		return 0
	})

	type survivor struct {
		owner      relationCatalogOwner
		planPoints int
	}
	survivors := make([]survivor, 0, len(owners))
	seen := make(map[summary.SummaryKey]struct{}, len(owners))
	for _, owner := range owners {
		if stats != nil {
			stats.RelationPlannerOwnersScanned++
		}
		if owner.fn == nil || owner.key.Ref.IsZero() || owner.hasEntryState {
			continue
		}
		if _, duplicate := seen[owner.key]; duplicate {
			continue
		}
		seen[owner.key] = struct{}{}
		origin, ok := bindings.FunctionOrigin(owner.fn)
		if !ok || origin.Kind == bind.FunctionOriginMethod || owner.prepared == nil || !owner.prepared.CompositionEligibility().Eligible() {
			continue
		}
		plan := owner.prepared.OperationPlan()
		if plan == nil || !plan.BoundaryCapturesValid() ||
			len(plan.BoundaryParams()) != 0 || len(plan.BoundaryCaptures()) != 0 ||
			!relationBoundaryMatchesBindings(bindings, owner.fn, plan.BoundaryParams(), plan.BoundaryCaptures()) {
			continue
		}
		if !stagedStrictCallFree(owner.prepared.HasCallSites(), plan) {
			continue
		}
		if stats != nil {
			stats.RelationPlannerOwnersPrefiltered++
		}
		survivors = append(survivors, survivor{owner: owner, planPoints: plan.PointCount()})
	}

	compiler := transformer.NewPlanCompiler()
	out := relationRunCatalog{
		entries: make([]relationCatalogEntry, 0, len(survivors)),
		byKey:   make(map[summary.SummaryKey]int, len(survivors)),
		consumers: relationConsumerPolicy{
			entries: make([]relationConsumerEntry, 0, len(survivors)),
			byKey:   make(map[summary.SummaryKey]int, len(survivors)),
		},
	}
	out.generation = &relationCatalogGeneration{}
	out.consumers.generation = out.generation
	for _, candidate := range survivors {
		owner, plan := candidate.owner, candidate.owner.prepared.OperationPlan()
		if stats != nil {
			stats.RelationPlannerOwnersCompiled++
		}
		compiled, err := compiler.Prepare(reg, owner.prepared.Graph(), plan, transformer.Shape{})
		if err != nil || !compiled.EffectFree() {
			continue
		}
		direct, err := transformer.NewDirectCallCatalog(candidate.planPoints, nil)
		if err != nil {
			continue
		}
		identity := relationCellIdentity{
			Cell:       transformer.CellRef{Function: uint64(len(out.entries) + 1)},
			Summary:    owner.key,
			BodyDigest: owner.prepared.IdentityDigest(),
			Prepared:   owner.prepared,
			Generation: out.generation,
		}
		out.byKey[owner.key] = len(out.entries)
		out.entries = append(out.entries, relationCatalogEntry{
			identity: identity, function: owner.fn, compiler: compiled, direct: direct,
		})
		consumerIdentity := relationConsumerIdentity{
			Summary: owner.key, BodyDigest: identity.BodyDigest,
			Prepared: owner.prepared, Generation: out.generation,
		}
		out.consumers.byKey[owner.key] = len(out.consumers.entries)
		out.consumers.entries = append(out.consumers.entries, relationConsumerEntry{
			identity: consumerIdentity, direct: direct, active: true,
		})
		if stats != nil {
			stats.RelationPlannerOwnersActivated++
		}
	}
	return out
}

// stagedStrictCallFree uses the cached static census as an O(1) rejection,
// then independently scans the immutable plan when the census reports empty.
// A disagreement therefore fails closed instead of admitting an unscanned
// call. This is the candidate's only authoritative call-surface scan.
func stagedStrictCallFree(metadataHasCalls bool, plan *operationplan.Plan) bool {
	if metadataHasCalls || plan == nil {
		return false
	}
	facts := plan.Facts()
	// Facts retains the complete immutable input map, including malformed
	// out-of-range points that the plan's dense index cannot represent.
	if facts.HasCallSites() {
		return false
	}
	for raw := 0; raw < plan.PointCount(); raw++ {
		if _, call := facts.CallSiteView(cfg.Point(raw)); call {
			return false
		}
	}
	return true
}
