package factapply

import (
	"fmt"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// NormalReturnPathRefinementProductionFootprint prepares the same N3 program
// that a normal-return path-refinement producer executes and returns its exact
// coordinate dependency certificate. The supplied inventory is only the
// closed lookup universe; callers add this certificate to the owning point's
// selector and must not union that universe into the selector.
func (a *PathSemanticAuthority) NormalReturnPathRefinementProductionFootprint(
	domain state.ProductDomain,
	point cfg.Point,
	carrier state.CoordinateFactorInventory,
	target pathdom.Path,
	value presence.Value,
) ([]state.CoordinateSlot, error) {
	if a == nil || !a.Valid() || !domain.Valid() || !carrier.ValidFor(domain, a.KeySpace()) || point == 0 || target.IsEmpty() ||
		(!presence.Equal(value, presence.Present()) && !presence.Equal(value, presence.Absent())) {
		return nil, fmt.Errorf("factapply: normal-return production footprint is unowned")
	}
	facts := factflow.NewFacts(factflow.FactsInput{PostconditionRefinements: map[cfg.Point]factflow.PostconditionRefinementSet{
		point: factflow.NewPostconditionRefinementSet(factflow.NewPostconditionRefinement(
			target, factflow.NewValueConstraint(product.NewWithPresence(domain.Registry(), product.ShapeTop, value)),
		)),
	}})
	program, err := PrepareCallResultPostconditionFactorProgram(
		a, domain, PlanCallResultTransaction(facts, point), carrier,
		func(dependency statekey.ValueDependency) (statekey.ValueDependency, bool) { return dependency, true },
		a.typeValues, a.projectPath,
	)
	if err != nil {
		return nil, err
	}
	return append(program.CoordinateReads(), program.CoordinateWrites()...), nil
}

// NormalReturnPresenceProofSeed is one occurrence-bound seed for a sibling
// return presence consequence. The transformer binds its paths through the
// occurrence's boundary bindings before handing it to this registered closure.
type NormalReturnPresenceProofSeed struct {
	Trigger         pathdom.Path
	TriggerPresence presence.Value
	Target          pathdom.Path
	TargetPresence  presence.Value
}

// NormalReturnPresenceProofSeedFootprint closes only the coordinates required
// by already-bound provider proof seeds. It is intentionally separate from
// the general normal-return footprint: callers must not manufacture sibling
// proof support by enumerating every boundary root.
func (a *PathSemanticAuthority) NormalReturnPresenceProofSeedFootprint(
	domain state.ProductDomain,
	point cfg.Point,
	lanes state.LaneSet,
	carrier state.CoordinateFactorInventory,
	seeds []NormalReturnPresenceProofSeed,
) ([]state.CoordinateSlot, error) {
	if len(seeds) == 0 {
		return nil, nil
	}
	if a == nil || !a.Valid() || !domain.Valid() || !carrier.ValidFor(domain, a.KeySpace()) || point == 0 {
		return nil, fmt.Errorf("factapply: normal-return proof seed footprint is unowned")
	}
	family, present := domain.PathEvidenceCoordinateFamily()
	if !present || !lanes.Has(family.Lane().ID()) {
		return nil, fmt.Errorf("factapply: normal-return proof seed path evidence lane is absent")
	}
	pathSlots, err := carrier.FamilySlots(family)
	if err != nil {
		return nil, err
	}
	var out []state.CoordinateSlot
	seedAccess := branchAtomAccess{}
	for index, seed := range seeds {
		if seed.Trigger.IsEmpty() || seed.Target.IsEmpty() ||
			(!presence.Equal(seed.TriggerPresence, presence.Present()) && !presence.Equal(seed.TriggerPresence, presence.Absent())) ||
			(!presence.Equal(seed.TargetPresence, presence.Present()) && !presence.Equal(seed.TargetPresence, presence.Absent())) {
			return nil, fmt.Errorf("factapply: normal-return proof seed %d is malformed", index)
		}
		trigger, triggerOK := visibility.AddressAt(a.resolver, point, seed.Trigger).RootOrVisibleKeyspaceKey()
		target, targetOK := visibility.AddressAt(a.resolver, point, seed.Target).RootOrVisibleKeyspaceKey()
		if !triggerOK || !targetOK {
			return nil, fmt.Errorf("factapply: normal-return proof seed %d has no keyspace binding", index)
		}
		seedAccess.predicateActivations = append(seedAccess.predicateActivations, pathPredicateActivation{
			path: trigger, kind: pathPredicateActivationTruthiness,
		})
		selected, selectErr := domain.PathCoordinateMutationClosure(pathSlots, []keyspace.Key{trigger, target}, nil)
		if selectErr != nil {
			return nil, selectErr
		}
		for _, slotIndex := range selected {
			if slotIndex < 0 || slotIndex >= len(pathSlots) {
				return nil, fmt.Errorf("factapply: normal-return proof seed path selection is malformed")
			}
			out = append(out, pathSlots[slotIndex])
		}
		for _, root := range []keyspace.Key{trigger, target} {
			rootSlots, rootErr := domain.BoundaryRootCoordinateSlots(a.KeySpace(), []keyspace.Key{root})
			if rootErr != nil {
				return nil, rootErr
			}
			for _, slot := range rootSlots {
				if lanes.Has(slot.Family().Lane().ID()) {
					out = append(out, slot)
				}
			}
		}
		proofs := []pathevidence.BranchProof{{Kind: pathevidence.BranchProofPathPresence, Path: trigger, Presence: seed.TriggerPresence}}
		for _, proof := range proofs {
			proofSlot, proofErr := domain.PathBranchProofCoordinateSlot(a.KeySpace(), proof)
			if proofErr != nil {
				return nil, proofErr
			}
			out = append(out, proofSlot)
			seedAccess.coordinateWrites = append(seedAccess.coordinateWrites, proofSlot)
		}
		for _, slotIndex := range selected {
			seedAccess.coordinateWrites = append(seedAccess.coordinateWrites, pathSlots[slotIndex])
		}
	}
	presencePlan, planErr := (PresenceImplicationPlan{
		reg: domain.Registry(), keys: a.KeySpace(), resolver: a.resolver, point: point,
		barriers: ConcretePresenceImplicationTrailingBarrier,
	}).DependencyBlocks(domain, carrier)
	if planErr != nil {
		return nil, planErr
	}
	cone, coneErr := selectPresenceImplicationAffectedCone(domain, presencePlan, seedAccess)
	if coneErr != nil {
		return nil, coneErr
	}
	for _, stage := range cone.stages {
		for _, block := range stage.blocks {
			out = append(out, block.CoordinateReads()...)
			out = append(out, block.CoordinateWrites()...)
		}
	}
	return out, nil
}

// NormalReturnCoordinateFootprint selects the exact frozen coordinate image
// through which a normal-return payload can refine its caller boundary roots.
// The selection is derived from the canonical placeholder/return binding and
// the already-sealed carrier; it neither scans a body inventory nor permits a
// solve-time coordinate discovery.
func (a *PathSemanticAuthority) NormalReturnCoordinateFootprint(
	domain state.ProductDomain,
	point cfg.Point,
	bindings callboundary.PathBindings,
	lanes state.LaneSet,
	identities []identity.Term,
	carrier state.CoordinateFactorInventory,
) ([]state.CoordinateSlot, error) {
	if a == nil || !a.Valid() || !domain.Valid() ||
		!carrier.ValidFor(domain, a.KeySpace()) {
		return nil, fmt.Errorf("factapply: normal-return coordinate footprint is unowned")
	}
	if point == 0 {
		return nil, fmt.Errorf("factapply: normal-return coordinate footprint has no point")
	}
	var out []state.CoordinateSlot
	if family, present := domain.PathEvidenceCoordinateFamily(); present && lanes.Has(family.Lane().ID()) {
		pathSlots, slotErr := carrier.FamilySlots(family)
		if slotErr != nil {
			return nil, slotErr
		}
		roots := append(bindings.ParameterRoots(), bindings.ReturnRoots()...)
		seeds := make([]keyspace.Key, 0, len(roots))
		for _, root := range roots {
			if root.IsEmpty() {
				continue
			}
			seed, present := visibility.AddressAt(a.resolver, point, root).RootOrVisibleKeyspaceKey()
			if !present {
				return nil, fmt.Errorf("factapply: normal-return boundary root has no keyspace seed")
			}
			seeds = append(seeds, seed)
		}
		selected, selectErr := domain.PathCoordinateMutationClosure(pathSlots, seeds, nil)
		if selectErr != nil {
			return nil, selectErr
		}
		for _, index := range selected {
			if index < 0 || index >= len(pathSlots) {
				return nil, fmt.Errorf("factapply: normal-return path selection is malformed")
			}
			out = append(out, pathSlots[index])
		}
		rootSlots, rootErr := domain.BoundaryRootCoordinateSlots(a.KeySpace(), seeds)
		if rootErr != nil {
			return nil, rootErr
		}
		for _, slot := range rootSlots {
			if lanes.Has(slot.Family().Lane().ID()) {
				out = append(out, slot)
			}
		}
		for _, root := range seeds {
			refinement, refinementErr := domain.PathRefinementCoordinateSlot(a.KeySpace(), root)
			if refinementErr != nil {
				return nil, refinementErr
			}
			out = append(out, refinement)
			staticMember, staticErr := domain.PathStaticMemberCoordinateSlot(a.KeySpace(), root)
			if staticErr != nil {
				return nil, staticErr
			}
			out = append(out, staticMember)
			for _, value := range []presence.Value{presence.Present(), presence.Absent()} {
				proof, proofErr := domain.PathBranchProofCoordinateSlot(a.KeySpace(), pathevidence.BranchProof{
					Kind: pathevidence.BranchProofPathPresence, Path: root, Presence: value,
				})
				if proofErr != nil {
					return nil, proofErr
				}
				const dependencyID state.CoordinateDependencyID = 1
				dependencies, dependencyErr := domain.PlanPathCoordinateDependencies(a.KeySpace(), pathSlots, []state.CoordinateDependencySeed{{
					ID: dependencyID, AddCoordinates: []state.CoordinateSlot{proof},
				}})
				if dependencyErr != nil {
					return nil, dependencyErr
				}
				dependency, present := dependencies.Dependency(dependencyID)
				if !present {
					return nil, fmt.Errorf("factapply: normal-return branch-proof dependency certificate is absent")
				}
				out = append(out, dependency.CoordinateWrites()...)
			}
		}
	}
	for _, lane := range domain.LaneInventory() {
		if !lanes.Has(lane.ID()) {
			continue
		}
		families, familyErr := domain.CoordinateFamilies(lane)
		if familyErr != nil {
			return nil, familyErr
		}
		for _, family := range families {
			roles, roleErr := domain.CoordinateReturnIdentityRoles(family)
			if roleErr != nil || !roles.Has(state.CoordinateReturnIdentityPublisher) {
				if roleErr != nil {
					return nil, roleErr
				}
				continue
			}
			for _, term := range identities {
				slot, handled, slotErr := domain.CoordinateReturnIdentityTermSlot(family, a.KeySpace(), term)
				if slotErr != nil {
					return nil, slotErr
				}
				if handled {
					out = append(out, slot)
				}
			}
		}
	}
	return out, nil
}
