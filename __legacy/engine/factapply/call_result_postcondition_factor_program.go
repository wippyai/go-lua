package factapply

import (
	"context"
	"fmt"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
)

type callResultPostconditionAtomKind uint8

const (
	callResultPostconditionRefinement callResultPostconditionAtomKind = iota + 1
	callResultPostconditionEquality
)

type callResultPostconditionAtom[K comparable] struct {
	kind       callResultPostconditionAtomKind
	refinement ValueRefinementFactorProgram[K]
	equality   PathEqualityFactorProgram[K]
}

// CallResultPostconditionFactorProgram is the complete ordered N3 law. It
// contains only factor programs and one presealed implication SCC cone; no
// State, provider, runtime inventory or resolver survives preparation.
type CallResultPostconditionFactorProgram[K comparable] struct {
	domain        state.ProductDomain
	keys          *keyspace.KeySpace
	atoms         []callResultPostconditionAtom[K]
	presence      PresenceImplicationDependencyPlan
	presenceRoots PresenceImplicationRootBinding[K]
	lanes         []state.ProductLane
	roots         map[statekey.ValueDependency]K
	rootSet       map[K]struct{}
	valueRoots    []K
	reads         []state.CoordinateSlot
	writes        []state.CoordinateSlot
	writeSkeleton bool
}

// CallResultPostconditionFactorFrame is the exact N3 tuple surface. Factors
// are in Program.Lanes order; every other product lane is residual identity.
type CallResultPostconditionFactorFrame[K comparable] struct {
	Values    state.ValueFactor[K]
	Factors   []state.LaneFactor
	Reachable bool
}

func PrepareCallResultPostconditionFactorProgram[K comparable](
	authority *PathSemanticAuthority,
	domain state.ProductDomain,
	transaction CallResultTransaction,
	inventory state.CoordinateFactorInventory,
	bind func(statekey.ValueDependency) (K, bool),
	typeValues *typevalue.Cache,
	projectPath PathTypeProjector,
) (CallResultPostconditionFactorProgram[K], error) {
	if authority == nil || !authority.Valid() || !domain.Valid() || bind == nil ||
		!transaction.Valid(domain.Registry()) || !inventory.ValidFor(domain, authority.resolver.KeySpace()) {
		return CallResultPostconditionFactorProgram[K]{}, fmt.Errorf("factapply: invalid call-result N3 factor preparation")
	}
	family, ok := domain.PathEvidenceCoordinateFamily()
	if !ok {
		return CallResultPostconditionFactorProgram[K]{}, fmt.Errorf("factapply: call-result N3 has no path factor authority")
	}
	union, err := inventory.FamilySlots(family)
	if err != nil {
		return CallResultPostconditionFactorProgram[K]{}, err
	}
	out := CallResultPostconditionFactorProgram[K]{
		domain: domain, keys: authority.resolver.KeySpace(), roots: make(map[statekey.ValueDependency]K), rootSet: make(map[K]struct{}),
	}
	needsPathMutation := false
	rememberRoot := func(dependency statekey.ValueDependency) (K, bool) {
		root, bound := bind(dependency)
		if !bound {
			return root, false
		}
		prior, exists := out.roots[dependency]
		if exists && prior != root {
			return root, false
		}
		if !exists {
			out.valueRoots = append(out.valueRoots, root)
		}
		out.roots[dependency] = root
		out.rootSet[root] = struct{}{}
		return root, true
	}
	seed := branchAtomAccess{}
	for index := 0; index < transaction.Len(); index++ {
		step, _ := transaction.Step(index)
		switch step.Kind() {
		case CallResultStepPostconditionRefinement:
			needsPathMutation = true
			refinement, _ := step.PostconditionRefinement()
			target, resolved := visibility.AddressAt(authority.resolver, transaction.point, refinement.TargetPath()).RootOrVisibleKeyspaceKey()
			if !resolved {
				return CallResultPostconditionFactorProgram[K]{}, fmt.Errorf("factapply: unresolved N3 refinement target")
			}
			plan, planErr := domain.SealValueRefinementPlan(out.keys, target, inventory)
			if planErr != nil {
				return CallResultPostconditionFactorProgram[K]{}, planErr
			}
			program, programErr := PrepareValueRefinementFactorProgram(
				domain, plan, refinement.Value(), rememberRoot, typeValues, projectPath,
			)
			if programErr != nil {
				return CallResultPostconditionFactorProgram[K]{}, programErr
			}
			out.atoms = append(out.atoms, callResultPostconditionAtom[K]{kind: callResultPostconditionRefinement, refinement: program})
			out.reads = appendBranchCoordinateSlots(domain, out.reads, program.CoordinateReads()...)
			out.reads = appendBranchCoordinateSlots(domain, out.reads, program.plan.FactorCoordinateReads()...)
			out.writes = appendBranchCoordinateSlots(domain, out.writes, program.CoordinateWrites()...)
			out.writeSkeleton = out.writeSkeleton || program.WritesCoordinateSkeleton()
			seed.coordinateWrites = appendBranchCoordinateSlots(domain, seed.coordinateWrites, program.CoordinateWrites()...)
			seed.laneWrites = appendProductLanes(seed.laneWrites, domain.PathDescendantMutationParticipantLanes()...)
			if concrete, exact := plan.RootValue().Concrete(); exact {
				seed.valueWrites = appendBranchValues(seed.valueWrites, concrete)
			}
			seed.dependencyWrites = appendUniqueValueDependency(seed.dependencyWrites, plan.RootValue())
			out.lanes = appendProductLanes(out.lanes, program.Lanes()...)
		case CallResultStepPostconditionPathRelation:
			needsPathMutation = true
			relation, _ := step.PostconditionPathRelation()
			left, leftOK := visibility.AddressAt(authority.resolver, transaction.point, relation.LeftPath()).RootOrVisibleKeyspaceKey()
			right, rightOK := visibility.AddressAt(authority.resolver, transaction.point, relation.RightPath()).RootOrVisibleKeyspaceKey()
			if !leftOK || !rightOK || left == right {
				return CallResultPostconditionFactorProgram[K]{}, fmt.Errorf("factapply: unresolved N3 equality endpoints")
			}
			plan, planErr := domain.SealPathEqualityFactorPlan(out.keys, left, right, union)
			if planErr != nil {
				return CallResultPostconditionFactorProgram[K]{}, planErr
			}
			program, programErr := PreparePathEqualityFactorProgram(domain, plan, rememberRoot, typeValues)
			if programErr != nil {
				return CallResultPostconditionFactorProgram[K]{}, programErr
			}
			out.atoms = append(out.atoms, callResultPostconditionAtom[K]{kind: callResultPostconditionEquality, equality: program})
			out.reads = appendBranchCoordinateSlots(domain, out.reads, program.CoordinateReads()...)
			out.writes = appendBranchCoordinateSlots(domain, out.writes, program.CoordinateWrites()...)
			out.writeSkeleton = true
			seed.coordinateWrites = appendBranchCoordinateSlots(domain, seed.coordinateWrites, program.CoordinateWrites()...)
			seed.laneWrites = appendProductLanes(seed.laneWrites, domain.PathDescendantMutationParticipantLanes()...)
			for _, dependency := range []statekey.ValueDependency{plan.LeftValue(), plan.RightValue()} {
				seed.dependencyWrites = appendUniqueValueDependency(seed.dependencyWrites, dependency)
				if concrete, exact := dependency.Concrete(); exact {
					seed.valueWrites = appendBranchValues(seed.valueWrites, concrete)
				}
			}
			out.lanes = appendProductLanes(out.lanes, program.Lanes()...)
		}
	}
	if len(out.atoms) == 0 {
		return out, nil
	}
	presenceSource := PresenceImplicationPlan{
		reg: domain.Registry(), keys: out.keys, resolver: authority.resolver,
		point: transaction.point, barriers: ConcretePresenceImplicationTrailingBarrier,
	}
	presence, err := presenceSource.DependencyBlocks(domain, inventory)
	if err != nil {
		return CallResultPostconditionFactorProgram[K]{}, err
	}
	out.presence, err = selectPresenceImplicationAffectedCone(domain, presence, seed)
	if err != nil {
		return CallResultPostconditionFactorProgram[K]{}, err
	}
	for _, stage := range out.presence.stages {
		for _, block := range stage.blocks {
			out.reads = appendBranchCoordinateSlots(domain, out.reads, block.CoordinateReads()...)
			out.writes = appendBranchCoordinateSlots(domain, out.writes, block.CoordinateWrites()...)
			for _, dependency := range append(block.ValueReadDependencies(), block.ValueWriteDependencies()...) {
				if _, exists := out.roots[dependency]; exists {
					continue
				}
				root, bound := rememberRoot(dependency)
				if !bound {
					return CallResultPostconditionFactorProgram[K]{}, fmt.Errorf("factapply: unresolved N3 presence Values root")
				}
				out.roots[dependency] = root
			}
		}
	}
	out.presenceRoots, err = SealPresenceImplicationRootBinding(out.presence, rememberRoot, func(root K) bool {
		_, bound := out.rootSet[root]
		return bound
	})
	if err != nil {
		return CallResultPostconditionFactorProgram[K]{}, err
	}
	out.lanes = appendProductLanes(out.lanes, family.Lane())
	for _, stage := range out.presence.stages {
		for _, block := range stage.blocks {
			if block.PathMutation() {
				out.lanes = appendProductLanes(out.lanes, domain.PathDescendantMutationParticipantLanes()...)
			}
		}
	}
	if needsPathMutation {
		out.lanes = appendProductLanes(out.lanes, domain.PathDescendantMutationParticipantLanes()...)
	}
	return out, nil
}

func (p CallResultPostconditionFactorProgram[K]) Lanes() []state.ProductLane {
	return append([]state.ProductLane(nil), p.lanes...)
}

func (p CallResultPostconditionFactorProgram[K]) ValueRoots() []K {
	return append([]K(nil), p.valueRoots...)
}
func (p CallResultPostconditionFactorProgram[K]) CoordinateReads() []state.CoordinateSlot {
	return append([]state.CoordinateSlot(nil), p.reads...)
}
func (p CallResultPostconditionFactorProgram[K]) CoordinateWrites() []state.CoordinateSlot {
	return append([]state.CoordinateSlot(nil), p.writes...)
}
func (p CallResultPostconditionFactorProgram[K]) WritesCoordinateSkeleton() bool {
	return p.writeSkeleton
}

// PrepareFormalCallResultPostconditionFactorProgram projects visibility once
// through the sealed root rekey and prepares the same canonical N3 program
// used by concrete State execution.
func PrepareFormalCallResultPostconditionFactorProgram[K comparable](
	authority *PathSemanticAuthority,
	domain state.ProductDomain,
	transaction CallResultTransaction,
	inventory state.CoordinateFactorInventory,
	rekey state.CoordinateFormalRootRekey,
	target *keyspace.KeySpace,
	bind func(statekey.ValueDependency) (K, bool),
) (CallResultPostconditionFactorProgram[K], error) {
	formal, err := authority.ProjectFormal(domain, rekey, target)
	if err != nil {
		return CallResultPostconditionFactorProgram[K]{}, err
	}
	return PrepareCallResultPostconditionFactorProgram(
		formal, domain, transaction, inventory, bind, authority.typeValues, authority.projectPath,
	)
}

func (p CallResultPostconditionFactorProgram[K]) Apply(
	ctx context.Context,
	token *cancellation.Token,
	input CallResultPostconditionFactorFrame[K],
) (CallResultPostconditionFactorFrame[K], error) {
	if ctx == nil || !p.domain.Valid() || len(input.Factors) != len(p.lanes) {
		return input, fmt.Errorf("factapply: invalid call-result N3 factor frame")
	}
	for index, factor := range input.Factors {
		if factor.Lane() != p.lanes[index] {
			return input, fmt.Errorf("factapply: reordered call-result N3 factor %d", index)
		}
	}
	if err := valueRefinementCanceled(ctx, token); err != nil {
		return input, err
	}
	out := CallResultPostconditionFactorFrame[K]{
		Values: cloneValueRefinementValues(input.Values), Factors: append([]state.LaneFactor(nil), input.Factors...), Reachable: input.Reachable,
	}
	for _, atom := range p.atoms {
		if !out.Reachable {
			break
		}
		var err error
		switch atom.kind {
		case callResultPostconditionRefinement:
			out, err = p.applyRefinement(ctx, token, out, atom.refinement)
		case callResultPostconditionEquality:
			out, err = p.applyEquality(ctx, token, out, atom.equality)
		default:
			err = fmt.Errorf("factapply: invalid call-result N3 atom")
		}
		if err != nil {
			return input, err
		}
	}
	if out.Reachable {
		var err error
		out, err = p.applyPresence(ctx, token, out)
		if err != nil {
			return input, err
		}
	}
	if err := valueRefinementCanceled(ctx, token); err != nil {
		return input, err
	}
	return out, nil
}

func (p CallResultPostconditionFactorProgram[K]) applyRefinement(
	ctx context.Context, token *cancellation.Token, input CallResultPostconditionFactorFrame[K], program ValueRefinementFactorProgram[K],
) (CallResultPostconditionFactorFrame[K], error) {
	frame, indices, err := p.openAtomFrame(input, program.Lanes(), program.PathEvidenceAuthority(), true)
	if err != nil {
		return input, err
	}
	next, err := program.Apply(ctx, token, frame)
	if err != nil {
		return input, err
	}
	out := p.mergeAtomFrame(input, next, indices)
	out.Factors, err = freezePathEvidenceCarrier(p.domain, next.Carrier, out.Factors, p.lanes)
	if err != nil {
		return input, err
	}
	return out, nil
}

func (p CallResultPostconditionFactorProgram[K]) applyEquality(
	ctx context.Context, token *cancellation.Token, input CallResultPostconditionFactorFrame[K], program PathEqualityFactorProgram[K],
) (CallResultPostconditionFactorFrame[K], error) {
	frame, indices, err := p.openAtomFrame(input, program.Lanes(), program.PathEvidenceAuthority(), true)
	if err != nil {
		return input, err
	}
	next, err := program.Apply(ctx, token, frame)
	if err != nil {
		return input, err
	}
	out := p.mergeAtomFrame(input, next, indices)
	out.Factors, err = freezePathEvidenceCarrier(p.domain, next.Carrier, out.Factors, p.lanes)
	if err != nil {
		return input, err
	}
	return out, nil
}

func (p CallResultPostconditionFactorProgram[K]) openAtomFrame(
	input CallResultPostconditionFactorFrame[K], lanes []state.ProductLane,
	authority state.CoordinatePathEvidenceAuthority[K], pathMutation bool,
) (ValueRefinementFactorFrame[K], []int, error) {
	factors, indices, err := p.selectFactors(input.Factors, lanes)
	if err != nil {
		return ValueRefinementFactorFrame[K]{}, nil, err
	}
	carrier, err := p.openCarrier(input, authority, nil, nil, pathMutation)
	if err != nil {
		return ValueRefinementFactorFrame[K]{}, nil, err
	}
	return ValueRefinementFactorFrame[K]{Values: input.Values, Factors: factors, Carrier: carrier, Reachable: input.Reachable}, indices, nil
}

func (p CallResultPostconditionFactorProgram[K]) mergeAtomFrame(
	input CallResultPostconditionFactorFrame[K], atom ValueRefinementFactorFrame[K], indices []int,
) CallResultPostconditionFactorFrame[K] {
	out := input
	out.Values, out.Reachable = atom.Values, atom.Reachable
	for index, global := range indices {
		out.Factors[global] = atom.Factors[index]
	}
	return out
}

func (p CallResultPostconditionFactorProgram[K]) selectFactors(
	factors []state.LaneFactor, lanes []state.ProductLane,
) ([]state.LaneFactor, []int, error) {
	out := make([]state.LaneFactor, len(lanes))
	indices := make([]int, len(lanes))
	for index, lane := range lanes {
		found := -1
		for candidate, owned := range p.lanes {
			if owned.ID() == lane.ID() {
				found = candidate
				break
			}
		}
		if found < 0 {
			return nil, nil, fmt.Errorf("factapply: N3 factor lane is absent")
		}
		out[index], indices[index] = factors[found], found
	}
	return out, indices, nil
}

func (p CallResultPostconditionFactorProgram[K]) openCarrier(
	input CallResultPostconditionFactorFrame[K],
	authority state.CoordinatePathEvidenceAuthority[K],
	valueReads, valueWrites []statekey.ValueDependency,
	pathMutation bool,
) (*state.CoordinatePathEvidenceCarrier[K], error) {
	family, ok := p.domain.PathEvidenceCoordinateFamily()
	if !ok {
		return nil, fmt.Errorf("factapply: N3 path family is absent")
	}
	pathIndex := p.laneIndex(family.Lane().ID())
	if pathIndex < 0 {
		return nil, fmt.Errorf("factapply: N3 path factor is absent")
	}
	skeleton, scalars, err := p.domain.DecomposeCoordinateFamily(input.Factors[pathIndex], family, p.keys)
	if err != nil {
		return nil, err
	}
	values := state.ValueFactor[K]{Top: input.Values.Top}
	if !values.Top {
		declared := make(map[statekey.ValueDependency]struct{}, len(valueReads)+len(valueWrites))
		for _, dependency := range append(append([]statekey.ValueDependency(nil), valueReads...), valueWrites...) {
			declared[dependency] = struct{}{}
		}
		values.Values = make(map[K]product.Value, len(declared))
		for dependency := range declared {
			root, bound := p.roots[dependency]
			if !bound {
				return nil, fmt.Errorf("factapply: N3 carrier Values root is unbound")
			}
			if value, present := input.Values.Values[root]; present {
				values.Values[root] = value
			}
		}
	}
	var mutation state.PathDescendantMutationFactors
	if pathMutation {
		mutation, err = bindPathDescendantMutationFactors(p.domain, p.keys, p.lanes, input.Factors)
		if err != nil {
			return nil, err
		}
	}
	return state.OpenCoordinatePathEvidenceCarrier(
		p.domain, skeleton, scalars, values, input.Reachable, authority, mutation,
	)
}

func (p CallResultPostconditionFactorProgram[K]) applyPresence(
	ctx context.Context, token *cancellation.Token, input CallResultPostconditionFactorFrame[K],
) (CallResultPostconditionFactorFrame[K], error) {
	out := input
	for _, stage := range p.presence.stages {
		if len(stage.publications) != 0 {
			if err := valueRefinementCanceled(ctx, token); err != nil {
				return input, err
			}
			authority, authorityOK := p.presenceRoots.StageAuthority(stage)
			if !authorityOK {
				return input, fmt.Errorf("factapply: N3 presence reducer authority is absent")
			}
			carrier, err := p.openCarrier(out, authority, nil, nil, false)
			if err != nil {
				return input, err
			}
			if err := ApplyPresenceImplicationCoordinateReducer(p.presence, carrier, stage, authority); err != nil {
				return input, err
			}
			out.Factors, err = freezePathEvidenceCarrier(p.domain, carrier, out.Factors, p.lanes)
			if err != nil {
				return input, err
			}
		}
		for _, block := range stage.blocks {
			if err := valueRefinementCanceled(ctx, token); err != nil {
				return input, err
			}
			authority, authorityOK := p.presenceRoots.BlockAuthority(block)
			if !authorityOK {
				return input, fmt.Errorf("factapply: N3 presence block authority is absent")
			}
			carrier, err := p.openCarrier(out, authority, block.ValueReadDependencies(), block.ValueWriteDependencies(), block.PathMutation())
			if err != nil {
				return input, err
			}
			feasible, err := ApplyPresenceImplicationCoordinateBlock(p.presence, ctx, carrier, block, p.presenceRoots)
			if err != nil {
				return input, err
			}
			skeleton, scalars, values, _, _, reachable, err := carrier.Freeze()
			if err != nil {
				return input, err
			}
			out.Factors, err = freezePathEvidenceCarrier(p.domain, carrier, out.Factors, p.lanes)
			_ = skeleton
			_ = scalars
			if err != nil {
				return input, err
			}
			out.Reachable = feasible && reachable
			if !out.Reachable {
				return out, nil
			}
			if !out.Values.Top {
				for _, dependency := range block.ValueWriteDependencies() {
					root, bound := p.roots[dependency]
					if !bound {
						return input, fmt.Errorf("factapply: N3 presence write root is unbound")
					}
					value := product.Bottom(p.domain.Registry())
					if found, present := values.Values[root]; present {
						value = found
					}
					if product.Equal(p.domain.Registry(), value, product.Bottom(p.domain.Registry())) {
						delete(out.Values.Values, root)
					} else {
						out.Values.Values[root] = value
					}
				}
			}
		}
	}
	return out, nil
}

func (p CallResultPostconditionFactorProgram[K]) laneIndex(id state.LaneID) int {
	for index, lane := range p.lanes {
		if lane.ID() == id {
			return index
		}
	}
	return -1
}
