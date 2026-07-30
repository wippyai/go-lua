package factapply

import (
	"context"
	"fmt"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

// PathEqualityFactorProgram is the one factor-native equality transaction.
// It publishes the registered equality quotient before resolving and meeting
// the two values, matching the canonical N3 order.
type PathEqualityFactorProgram[K comparable] struct {
	domain           state.ProductDomain
	plan             state.PathEqualityFactorPlan
	leftRoot         K
	rightRoot        K
	lanes            []state.ProductLane
	equalityOrdinals []int
	persistent       bool
	typeValues       *typevalue.Cache
	authority        state.CoordinatePathEvidenceAuthority[K]
}

func PreparePathEqualityFactorProgram[K comparable](
	domain state.ProductDomain,
	plan state.PathEqualityFactorPlan,
	bind func(statekey.ValueDependency) (K, bool),
	typeValues *typevalue.Cache,
) (PathEqualityFactorProgram[K], error) {
	return preparePathEqualityFactorProgram(domain, plan, bind, typeValues, false)
}

func preparePathEqualityFactorProgram[K comparable](
	domain state.ProductDomain,
	plan state.PathEqualityFactorPlan,
	bind func(statekey.ValueDependency) (K, bool),
	typeValues *typevalue.Cache,
	persistent bool,
) (PathEqualityFactorProgram[K], error) {
	if !domain.Valid() || !plan.ValidFor(domain) || bind == nil {
		return PathEqualityFactorProgram[K]{}, fmt.Errorf("factapply: invalid path-equality factor program")
	}
	left, leftOK := bind(plan.LeftValue())
	right, rightOK := bind(plan.RightValue())
	if !leftOK || !rightOK {
		return PathEqualityFactorProgram[K]{}, fmt.Errorf("factapply: unresolved path-equality Values roots")
	}
	potential, err := domain.DynamicReadPotentialLanes()
	if err != nil {
		return PathEqualityFactorProgram[K]{}, err
	}
	mutationTopology, err := domain.SealPathDescendantMutationFactorTopology()
	if err != nil {
		return PathEqualityFactorProgram[K]{}, err
	}
	for _, lane := range mutationTopology.Lanes() {
		potential = potential.With(lane.ID())
	}
	for _, lane := range domain.PathEqualityQuotientLanes() {
		potential = potential.With(lane.ID())
	}
	pathFamily, hasPathFamily := domain.PathEvidenceCoordinateFamily()
	lanes := make([]state.ProductLane, 0, potential.Len())
	equalityIDs := make(map[state.LaneID]struct{})
	for _, lane := range domain.PathEqualityQuotientLanes() {
		equalityIDs[lane.ID()] = struct{}{}
	}
	var equalityOrdinals []int
	for _, lane := range domain.LaneInventory() {
		if !potential.Has(lane.ID()) {
			continue
		}
		// The path family is carried exclusively by Carrier. Keeping the same
		// family as both a whole-lane factor and a coordinate carrier creates two
		// independently publishable representations of one semantic component.
		if hasPathFamily && lane == pathFamily.Lane() {
			continue
		}
		if _, participates := equalityIDs[lane.ID()]; participates {
			equalityOrdinals = append(equalityOrdinals, len(lanes))
		}
		lanes = append(lanes, lane)
	}
	out := PathEqualityFactorProgram[K]{
		domain: domain, plan: plan, leftRoot: left, rightRoot: right,
		lanes: lanes, equalityOrdinals: equalityOrdinals, persistent: persistent, typeValues: typeValues,
	}
	out.authority, err = state.SealCoordinatePathEvidenceAuthority(
		domain, plan.KeySpace(), nil, nil, plan.CoordinateReadInventory(), plan.CoordinateWriteInventory(), true, true,
		func(slot K) bool { return slot == left || slot == right },
	)
	if err != nil {
		return PathEqualityFactorProgram[K]{}, err
	}
	return out, nil
}

func (p PathEqualityFactorProgram[K]) Valid() bool {
	return p.domain.Valid() && p.plan.ValidFor(p.domain)
}
func (p PathEqualityFactorProgram[K]) Lanes() []state.ProductLane {
	return append([]state.ProductLane(nil), p.lanes...)
}
func (p PathEqualityFactorProgram[K]) CoordinateReads() []state.CoordinateSlot {
	return p.plan.CoordinateReads()
}
func (p PathEqualityFactorProgram[K]) CoordinateWrites() []state.CoordinateSlot {
	return p.plan.CoordinateWrites()
}
func (p PathEqualityFactorProgram[K]) KeySpace() *keyspace.KeySpace { return p.plan.KeySpace() }
func (p PathEqualityFactorProgram[K]) PathEvidenceAuthority() state.CoordinatePathEvidenceAuthority[K] {
	return p.authority
}

type pathEqualityFactorReader[K comparable] struct {
	leftDependency, rightDependency statekey.ValueDependency
	leftRoot, rightRoot             K
	values                          state.ValueFactor[K]
	reg                             state.ProductDomain
}

func (r pathEqualityFactorReader[K]) ReadPathReplacementValue(dependency statekey.ValueDependency) (product.Value, bool) {
	root := r.leftRoot
	if dependency == r.rightDependency {
		root = r.rightRoot
	} else if dependency != r.leftDependency {
		return product.Value{}, false
	}
	if r.values.Top {
		return product.Top(), true
	}
	if value, present := r.values.Values[root]; present {
		return value, true
	}
	return product.Bottom(r.reg.Registry()), true
}

func (p PathEqualityFactorProgram[K]) Apply(
	ctx context.Context,
	token *cancellation.Token,
	input ValueRefinementFactorFrame[K],
) (ValueRefinementFactorFrame[K], error) {
	if ctx == nil || !p.Valid() || input.Carrier == nil || !input.Carrier.Valid() || len(input.Factors) != len(p.lanes) {
		return input, fmt.Errorf("factapply: invalid path-equality factor frame")
	}
	for index, factor := range input.Factors {
		if factor.Lane() != p.lanes[index] {
			return input, fmt.Errorf("factapply: reordered path-equality factor %d", index)
		}
	}
	if !input.Carrier.MatchesAuthority(p.authority) {
		return input, fmt.Errorf("factapply: path-equality carrier authority mismatch")
	}
	if err := valueRefinementCanceled(ctx, token); err != nil {
		return input, err
	}
	if !input.Reachable {
		return input, nil
	}
	out := ValueRefinementFactorFrame[K]{
		Values: cloneValueRefinementValues(input.Values), Factors: append([]state.LaneFactor(nil), input.Factors...),
		Carrier: input.Carrier.Clone(), Reachable: true,
	}
	leftBefore, leftBeforeOK := p.resolve(out.Carrier, pathEqualityFactorReader[K]{
		leftDependency: p.plan.LeftValue(), rightDependency: p.plan.RightValue(),
		leftRoot: p.leftRoot, rightRoot: p.rightRoot, values: out.Values, reg: p.domain,
	}, out.Factors, p.plan.Left(), p.plan.LeftRoot())
	rightBefore, rightBeforeOK := p.resolve(out.Carrier, pathEqualityFactorReader[K]{
		leftDependency: p.plan.LeftValue(), rightDependency: p.plan.RightValue(),
		leftRoot: p.leftRoot, rightRoot: p.rightRoot, values: out.Values, reg: p.domain,
	}, out.Factors, p.plan.Right(), p.plan.RightRoot())
	if rightBeforeOK {
		if err := p.narrowParentOrigin(&out, p.plan.Left(), p.plan.LeftRoot(), p.leftRoot, rightBefore); err != nil {
			return input, err
		}
		if !out.Reachable {
			return out, nil
		}
	}
	readerAfterLeft := pathEqualityFactorReader[K]{
		leftDependency: p.plan.LeftValue(), rightDependency: p.plan.RightValue(),
		leftRoot: p.leftRoot, rightRoot: p.rightRoot, values: out.Values, reg: p.domain,
	}
	leftAfter, leftAfterOK := p.resolve(out.Carrier, readerAfterLeft, out.Factors, p.plan.Left(), p.plan.LeftRoot())
	if leftAfterOK {
		if err := p.narrowParentOrigin(&out, p.plan.Right(), p.plan.RightRoot(), p.rightRoot, leftAfter); err != nil {
			return input, err
		}
		if !out.Reachable {
			return out, nil
		}
	}
	memberSafe := sourcevalue.RuntimeMayBeTable(p.domain.Registry(), leftBefore, leftBeforeOK) &&
		sourcevalue.RuntimeMayBeTable(p.domain.Registry(), rightBefore, rightBeforeOK)
	transaction, err := state.PrepareCoordinateTransientPathEqualityTransaction(p.domain, out.Carrier, p.plan.Proof())
	if p.persistent {
		transaction, err = state.PrepareCoordinatePathEqualityTransaction(p.domain, out.Carrier, p.plan.Proof())
	}
	if err != nil {
		return input, err
	}
	if _, err = state.ApplyCoordinatePathEqualityTransaction(p.domain, transaction, out.Carrier); err != nil {
		return input, err
	}
	if p.persistent {
		if _, ok := out.Carrier.CloseProofsAcrossKnownEqualities(); !ok {
			return input, fmt.Errorf("factapply: path equality proof closure rejected")
		}
	}
	leftCongruence, rightCongruence := p.plan.Left(), p.plan.Right()
	if segments, segmentsOK := p.plan.KeySpace().SegmentsView(leftCongruence); segmentsOK && len(segments) == 0 {
		leftCongruence = p.plan.LeftRoot()
	}
	if segments, segmentsOK := p.plan.KeySpace().SegmentsView(rightCongruence); segmentsOK && len(segments) == 0 {
		rightCongruence = p.plan.RightRoot()
	}
	_, ok := out.Carrier.CloseRefinementsAcrossTransientEquality(leftCongruence, rightCongruence, memberSafe)
	if !ok {
		return input, fmt.Errorf("factapply: path equality refinement closure rejected")
	}
	for _, ordinal := range p.equalityOrdinals {
		out.Factors[ordinal], err = p.domain.ApplyPathEqualityTransactionFactor(transaction, out.Factors[ordinal])
		if err != nil {
			return input, err
		}
	}
	reader := pathEqualityFactorReader[K]{
		leftDependency: p.plan.LeftValue(), rightDependency: p.plan.RightValue(),
		leftRoot: p.leftRoot, rightRoot: p.rightRoot, values: out.Values, reg: p.domain,
	}
	left, leftOK := p.resolve(out.Carrier, reader, out.Factors, p.plan.Left(), p.plan.LeftRoot())
	right, rightOK := p.resolve(out.Carrier, reader, out.Factors, p.plan.Right(), p.plan.RightRoot())
	if !leftOK || !rightOK {
		return out, nil
	}
	meet := product.Meet(p.domain.Registry(), left, right)
	if product.Equal(p.domain.Registry(), meet, product.Bottom(p.domain.Registry())) {
		out.Reachable = false
		return out, nil
	}
	if err := p.write(&out, p.plan.Left(), p.plan.LeftRoot(), p.leftRoot, meet); err != nil {
		return input, err
	}
	if err := p.write(&out, p.plan.Right(), p.plan.RightRoot(), p.rightRoot, meet); err != nil {
		return input, err
	}
	if err := valueRefinementCanceled(ctx, token); err != nil {
		return input, err
	}
	return out, nil
}

// narrowParentOrigin is the equality relation's registered-fiber backward
// projection. It refines only a descendant's lexical root and invalidates the
// root's stale dependent coordinates through the carrier's registered laws.
// No State or axis-specific storage is reconstructed here.
func (p PathEqualityFactorProgram[K]) narrowParentOrigin(
	frame *ValueRefinementFactorFrame[K],
	target, root keyspace.Key,
	rootSlot K,
	constraint product.Value,
) error {
	if target == root || frame.Values.Top {
		return nil
	}
	segments, ok := p.plan.KeySpace().SegmentsView(target)
	if !ok || len(segments) == 0 {
		return fmt.Errorf("factapply: path-equality descendant has no structural suffix")
	}
	rootValue, present := frame.Values.Values[rootSlot]
	if !present || product.Equal(p.domain.Registry(), rootValue, product.Bottom(p.domain.Registry())) {
		return nil
	}
	origin, ok := typevalue.VariantOriginOfValue(p.domain.Registry(), p.typeValues, rootValue)
	if !ok {
		return nil
	}
	cases, ok := narrowOriginCasesByPathConstraint(
		p.typeValues, p.domain.Registry(), origin, segments, constraint, true,
	)
	if !ok {
		return nil
	}
	if len(cases) == 0 {
		frame.Reachable = false
		frame.Carrier.MakeUnreachable()
		return nil
	}
	narrowed := product.Set(
		p.domain.Registry(), rootValue, variantorigin.Key,
		variantorigin.Of(origin.Family(), cases),
	)
	if product.Equal(p.domain.Registry(), narrowed, rootValue) {
		return nil
	}
	rootPath := p.plan.KeySpace().FormatReadOnly(root)
	if rootPath == "" {
		return fmt.Errorf("factapply: path-equality root path is unresolved")
	}
	if _, valid := frame.Carrier.InvalidateDescendants(rootPath); !valid {
		return fmt.Errorf("factapply: path-equality descendant invalidation rejected")
	}
	frame.Values.Values[rootSlot] = narrowed
	return nil
}

// resolve observes an explicitly tracked descendant through the registered
// path carrier before asking the structural projection laws. A descendant
// coordinate is authoritative even when its lexical root has no abstract
// value (the common local-table/member case); requiring a root value first
// would silently discard that coordinate.
func (p PathEqualityFactorProgram[K]) resolve(
	carrier *state.CoordinatePathEvidenceCarrier[K],
	reader pathEqualityFactorReader[K],
	factors []state.LaneFactor,
	target, root keyspace.Key,
) (product.Value, bool) {
	if target != root {
		if value, ok := carrier.ReadPath(target); ok && !product.Equal(p.domain.Registry(), value, product.Bottom(p.domain.Registry())) {
			return value, true
		}
	}
	return p.domain.ResolveFactorPathValue(p.plan.KeySpace(), target, reader, factors)
}

func (p PathEqualityFactorProgram[K]) write(frame *ValueRefinementFactorFrame[K], target, root keyspace.Key, slot K, value product.Value) error {
	if target != root {
		if _, ok := frame.Carrier.WritePath(target, value); !ok {
			return fmt.Errorf("factapply: path-equality path write rejected")
		}
		return nil
	}
	if frame.Values.Top {
		return nil
	}
	if product.Equal(p.domain.Registry(), value, product.Bottom(p.domain.Registry())) {
		delete(frame.Values.Values, slot)
	} else {
		frame.Values.Values[slot] = value
	}
	return nil
}
