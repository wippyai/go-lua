package factapply

import (
	"context"
	"fmt"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	valuerefine "github.com/wippyai/go-lua/__legacy/analysis/domain/value/refinement"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// ValueRefinementFactorProgram is the sole carrier-neutral value-refinement
// algebra. K is only the collision-free Values-root vocabulary selected by an
// adapter. Path reads, descendant invalidation, equality sidecars and heap
// evidence remain ProductDomain-registered factors; no State or inventory
// callback is retained by the program.
type ValueRefinementFactorProgram[K comparable] struct {
	domain      state.ProductDomain
	plan        state.ValueRefinementPlan
	refinement  factflow.ValueRefinement
	root        K
	typeValues  *typevalue.Cache
	projectPath PathTypeProjector
	lanes       []state.ProductLane
	projection  state.DynamicReadFactorProjectionPlan
	inventory   state.CoordinateFactorInventory
	authority   state.CoordinatePathEvidenceAuthority[K]
	guard       bool
}

func bindPathDescendantMutationFactors(
	domain state.ProductDomain,
	keys *keyspace.KeySpace,
	lanes []state.ProductLane,
	factors []state.LaneFactor,
) (state.PathDescendantMutationFactors, error) {
	if len(lanes) != len(factors) {
		return state.PathDescendantMutationFactors{}, fmt.Errorf("factapply: descendant mutation factor inventory differs from lanes")
	}
	return domain.BindPathDescendantMutationFactors(keys, func(wanted state.ProductLane) (state.LaneFactor, bool) {
		for index, lane := range lanes {
			if lane == wanted {
				return factors[index], true
			}
		}
		return state.LaneFactor{}, false
	})
}

// PrepareValueRefinementFactorProgram binds the plan's abstract Values root
// once. bind is discarded after preparation; Apply is a closed factor law.
func PrepareValueRefinementFactorProgram[K comparable](
	domain state.ProductDomain,
	plan state.ValueRefinementPlan,
	refinement factflow.ValueRefinement,
	bind func(statekey.ValueDependency) (K, bool),
	typeValues *typevalue.Cache,
	projectPath PathTypeProjector,
) (ValueRefinementFactorProgram[K], error) {
	return prepareValueRefinementFactorProgram(domain, plan, refinement, bind, typeValues, projectPath, false)
}

// PrepareGuardValueRefinementFactorProgram prepares the same canonical
// narrowing law with edge-feasibility closure. A contradictory guard makes
// the returned frame unreachable; no adapter reimplements that decision.
func PrepareGuardValueRefinementFactorProgram[K comparable](
	domain state.ProductDomain,
	plan state.ValueRefinementPlan,
	refinement factflow.ValueRefinement,
	bind func(statekey.ValueDependency) (K, bool),
	typeValues *typevalue.Cache,
	projectPath PathTypeProjector,
) (ValueRefinementFactorProgram[K], error) {
	return prepareValueRefinementFactorProgram(domain, plan, refinement, bind, typeValues, projectPath, true)
}

func prepareValueRefinementFactorProgram[K comparable](
	domain state.ProductDomain,
	plan state.ValueRefinementPlan,
	refinement factflow.ValueRefinement,
	bind func(statekey.ValueDependency) (K, bool),
	typeValues *typevalue.Cache,
	projectPath PathTypeProjector,
	guard bool,
) (ValueRefinementFactorProgram[K], error) {
	reg := domain.Registry()
	constraint, constrained := refinement.Constraint()
	if !domain.Valid() || !plan.ValidFor(domain) || bind == nil || constrained && !product.BelongsToRegistry(reg, constraint) {
		return ValueRefinementFactorProgram[K]{}, fmt.Errorf("factapply: invalid value-refinement factor program")
	}
	root, ok := bind(plan.RootValue())
	if !ok {
		return ValueRefinementFactorProgram[K]{}, fmt.Errorf("factapply: unresolved value-refinement Values root")
	}
	potential, err := domain.DynamicReadPotentialLanes()
	if err != nil {
		return ValueRefinementFactorProgram[K]{}, err
	}
	mutationTopology, err := domain.SealPathDescendantMutationFactorTopology()
	if err != nil {
		return ValueRefinementFactorProgram[K]{}, err
	}
	for _, lane := range mutationTopology.Lanes() {
		potential = potential.With(lane.ID())
	}
	factoredLanes := make(map[state.LaneID]bool)
	for _, family := range mutationTopology.Families() {
		factoredLanes[family.Lane().ID()] = true
	}
	if heap, present := domain.ProductLane(state.LaneHeapTableIdentity); present {
		potential = potential.With(heap.ID())
	}
	projection, err := domain.SealDynamicReadFactorProjectionPlan(plan.KeySpace())
	if err != nil {
		return ValueRefinementFactorProgram[K]{}, err
	}
	inventory := plan.CoordinateInventory()
	lanes := make([]state.ProductLane, 0, potential.Len())
	for _, lane := range domain.LaneInventory() {
		if potential.Has(lane.ID()) && !factoredLanes[lane.ID()] {
			lanes = append(lanes, lane)
		}
	}
	out := ValueRefinementFactorProgram[K]{
		domain: domain, plan: plan, refinement: refinement, root: root,
		typeValues: typeValues, projectPath: projectPath, lanes: lanes, projection: projection, inventory: inventory, guard: guard,
	}
	out.authority, err = state.SealCoordinatePathEvidenceAuthority(
		domain, plan.KeySpace(), nil, nil, plan.CoordinateReadInventory(), plan.CoordinateWriteInventory(),
		true, out.WritesCoordinateSkeleton(), func(slot K) bool { return slot == root },
	)
	if err != nil {
		return ValueRefinementFactorProgram[K]{}, err
	}
	return out, nil
}

func (p ValueRefinementFactorProgram[K]) Valid() bool {
	return p.domain.Valid() && p.plan.ValidFor(p.domain) && p.projection.ValidFor(p.domain, p.plan.KeySpace()) &&
		p.inventory.ValidFor(p.domain, p.plan.KeySpace())
}

// Lanes returns the complete registration-owned factor envelope. DynamicRead
// still demands only its runtime cone inside this sealed envelope.
func (p ValueRefinementFactorProgram[K]) Lanes() []state.ProductLane {
	if !p.Valid() {
		return nil
	}
	return append([]state.ProductLane(nil), p.lanes...)
}

func (p ValueRefinementFactorProgram[K]) CoordinateReads() []state.CoordinateSlot {
	return p.plan.CoordinateReads()
}
func (p ValueRefinementFactorProgram[K]) CoordinateWrites() []state.CoordinateSlot {
	return p.plan.CoordinateWrites()
}
func (p ValueRefinementFactorProgram[K]) PathEvidenceAuthority() state.CoordinatePathEvidenceAuthority[K] {
	return p.authority
}
func (p ValueRefinementFactorProgram[K]) NeedsDescendantInvalidation() bool { return p.Valid() }
func (p ValueRefinementFactorProgram[K]) WritesCoordinateSkeleton() bool {
	if !p.Valid() {
		return false
	}
	// A descendant write can grow the path-evidence skeleton, and a feasible
	// root write can invalidate its descendant cone. Guard rejection is the
	// sparse product-bottom coordinate and writes no component skeleton.
	if p.plan.Target() != p.plan.Root() {
		return true
	}
	constraint, constrained := p.refinement.Constraint()
	return constrained && rootRefinementInvalidatesDescendants(p.domain.Registry(), p.refinement) && product.BelongsToRegistry(p.domain.Registry(), constraint)
}

// ValueRefinementFactorFrame is the exact mutable fiber of one refinement.
// Factors are in Lanes order. Carrier is opened with the program's exact
// coordinate authorities and descendant-mutation factors.
type ValueRefinementFactorFrame[K comparable] struct {
	Values    state.ValueFactor[K]
	Factors   []state.LaneFactor
	Carrier   *state.CoordinatePathEvidenceCarrier[K]
	Reachable bool
}

type valueRefinementFactorReader[K comparable] struct {
	dependency statekey.ValueDependency
	root       K
	values     state.ValueFactor[K]
	reg        *axis.Registry
}

func (r valueRefinementFactorReader[K]) ReadPathReplacementValue(dependency statekey.ValueDependency) (product.Value, bool) {
	if dependency != r.dependency || r.values.Top {
		if dependency == r.dependency && r.values.Top {
			return product.Top(), true
		}
		return product.Value{}, false
	}
	if value, present := r.values.Values[r.root]; present {
		return value, true
	}
	return product.Bottom(r.reg), true
}

// Apply evaluates one refinement atomically. Cancellation or any rejected
// registered law returns the exact input frame; publication occurs only in the
// returned detached frame.
func (p ValueRefinementFactorProgram[K]) Apply(
	ctx context.Context,
	token *cancellation.Token,
	input ValueRefinementFactorFrame[K],
) (ValueRefinementFactorFrame[K], error) {
	if ctx == nil || !p.Valid() || input.Carrier == nil || !input.Carrier.Valid() || len(input.Factors) != len(p.lanes) {
		return input, fmt.Errorf("factapply: invalid value-refinement factor frame")
	}
	for index, factor := range input.Factors {
		if factor.Lane() != p.lanes[index] {
			return input, fmt.Errorf("factapply: reordered value-refinement factor %d", index)
		}
	}
	if !input.Carrier.MatchesAuthority(p.authority) {
		return input, fmt.Errorf("factapply: value-refinement carrier authority mismatch")
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
	if out.Carrier == nil {
		return input, fmt.Errorf("factapply: value-refinement carrier cannot fork")
	}
	projection, err := p.bindDynamicReadProjection(out)
	if err != nil {
		return input, err
	}
	if p.guard {
		current, found := p.resolve(out, projection)
		if found && valueRefinementContradictsCurrentValue(p.domain.Registry(), p.typeValues, current, p.refinement) {
			// Reachability is the product-bottom coordinate. Component payloads
			// are unobservable below it, so retain their sparse input spelling
			// instead of manufacturing writes to every registered family.
			out.Reachable = false
			return out, nil
		}
	}
	if p.refinement.FalsyAbsent() {
		current, found := p.resolve(out, projection)
		if !found || !falsyAbsentValueProven(p.domain.Registry(), current) {
			return input, nil
		}
	}
	if p.plan.Target() == p.plan.Root() {
		err = p.applyRoot(&out, nil)
	} else {
		targetSegments, ok := p.plan.KeySpace().SegmentsView(p.plan.Target())
		if !ok || len(targetSegments) == 0 {
			return input, fmt.Errorf("factapply: value-refinement descendant has no structural suffix")
		}
		err = p.applyDescendant(&out, targetSegments, projection)
	}
	if err != nil {
		return input, err
	}
	if err := valueRefinementCanceled(ctx, token); err != nil {
		return input, err
	}
	if !out.Reachable {
		return out, nil
	}
	out.Factors, err = p.freezeCarrier(out.Carrier, out.Factors)
	if err != nil {
		return input, err
	}
	return out, nil
}

func valueRefinementCanceled(ctx context.Context, token *cancellation.Token) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if token != nil && token.Canceled() {
		return context.Canceled
	}
	return nil
}

func cloneValueRefinementValues[K comparable](input state.ValueFactor[K]) state.ValueFactor[K] {
	out := state.ValueFactor[K]{Top: input.Top, Values: make(map[K]product.Value, len(input.Values))}
	for slot, value := range input.Values {
		out.Values[slot] = value
	}
	return out
}

func (p ValueRefinementFactorProgram[K]) rootValue(frame ValueRefinementFactorFrame[K]) product.Value {
	if frame.Values.Top {
		return product.Top()
	}
	if value, present := frame.Values.Values[p.root]; present {
		return value
	}
	return product.Bottom(p.domain.Registry())
}

func (p ValueRefinementFactorProgram[K]) writeRoot(frame *ValueRefinementFactorFrame[K], value product.Value) error {
	if frame.Values.Top {
		return nil
	}
	current := p.rootValue(*frame)
	if product.Equal(p.domain.Registry(), current, value) {
		return nil
	}
	if product.Equal(p.domain.Registry(), value, product.Bottom(p.domain.Registry())) {
		delete(frame.Values.Values, p.root)
	} else {
		frame.Values.Values[p.root] = value
	}
	return p.rewriteHeapRoot(frame, current, value)
}

func (p ValueRefinementFactorProgram[K]) rewriteHeapRoot(frame *ValueRefinementFactorFrame[K], before, after product.Value) error {
	beforeID, beforeExact := identityvalue.ExactTerm(p.domain.Registry(), before)
	afterID, afterExact := identityvalue.ExactTerm(p.domain.Registry(), after)
	if !beforeExact || !afterExact || beforeID != afterID {
		return nil
	}
	_, owned := p.plan.HeapRootSlot(beforeID)
	if !owned {
		return nil
	}
	heapIndex := p.factorIndex(state.LaneHeapTableIdentity)
	if heapIndex < 0 {
		return fmt.Errorf("factapply: value-refinement heap factor is absent")
	}
	skeleton, roots, members, err := p.domain.DecomposeHeapTableIdentity(frame.Factors[heapIndex], p.plan.KeySpace())
	if err != nil {
		return fmt.Errorf("factapply: value-refinement heap-root binding drift")
	}
	ordinal := -1
	for index := range roots {
		if roots[index].IdentityTerm() == beforeID {
			ordinal = index
			break
		}
	}
	if ordinal < 0 {
		// The plan inventory is a static ownership envelope. A particular
		// sparse frame may legitimately carry no object for an owned identity;
		// in that case there is no heap-root value to keep synchronized and no
		// object may be invented from the Values refinement.
		return nil
	}
	roots[ordinal], err = p.domain.WithHeapObjectRootValue(roots[ordinal], after)
	if err != nil {
		return err
	}
	frame.Factors[heapIndex], err = p.domain.ComposeHeapTableIdentity(skeleton, roots, members, p.plan.KeySpace())
	return err
}

func (p ValueRefinementFactorProgram[K]) factorIndex(id state.LaneID) int {
	for index, lane := range p.lanes {
		if lane.ID() == id {
			return index
		}
	}
	return -1
}

func (p ValueRefinementFactorProgram[K]) applyRoot(frame *ValueRefinementFactorFrame[K], narrowedRoot typ.Type) error {
	if p.refinement.NegatedLiteral() {
		return nil
	}
	current := p.rootValue(*frame)
	constraint, constrained := p.refinement.Constraint()
	if constrained && rootRefinementInvalidatesDescendants(p.domain.Registry(), p.refinement) {
		if err := p.invalidateRoot(frame, constraint, narrowedRoot); err != nil {
			return err
		}
	}
	next := current
	if product.Equal(p.domain.Registry(), current, product.Bottom(p.domain.Registry())) && constrained {
		next = constraint
	} else {
		next = refineProductValue(p.domain.Registry(), current, p.refinement)
	}
	return p.writeRoot(frame, next)
}

func (p ValueRefinementFactorProgram[K]) invalidateRoot(frame *ValueRefinementFactorFrame[K], constraint product.Value, narrowedRoot typ.Type) error {
	facts, err := state.SnapshotValueRefinementDescendants(p.domain, p.plan, frame.Carrier)
	if err != nil {
		return err
	}
	keep := make([]state.ValueRefinementDescendantFact, 0, len(facts))
	if rootRefinementCanKeepDescendants(p.domain.Registry(), p.typeValues, constraint) {
		for _, fact := range facts {
			value, compatible := compatibleNarrowedRootDescendant(
				p.domain.Registry(), p.typeValues, p.plan.KeySpace(), p.projectPath,
				p.plan.Root(), fact.Coordinate().Path(), fact.Value(), narrowedRoot,
			)
			if !compatible {
				continue
			}
			updated, updateErr := p.domain.WithValueRefinementDescendantValue(p.plan, fact, value)
			if updateErr != nil {
				return updateErr
			}
			keep = append(keep, updated)
		}
	}
	rootPath := pathdom.PathKey(p.plan.KeySpace().FormatReadOnly(p.plan.Root()))
	if _, ok := frame.Carrier.InvalidateDescendants(rootPath); !ok {
		return fmt.Errorf("factapply: value-refinement descendant invalidation rejected")
	}
	return state.RestoreValueRefinementDescendants(p.domain, p.plan, frame.Carrier, keep)
}

func (p ValueRefinementFactorProgram[K]) applyDescendant(
	frame *ValueRefinementFactorFrame[K],
	target []segment.Segment,
	projection *state.DynamicReadFactorProjection,
) error {
	constraint, constrained := p.refinement.Constraint()
	if constrained {
		if lit, literal := literalConstraintType(p.domain.Registry(), constraint); literal {
			applied, err := p.narrowRootByLiteral(frame, target, lit, p.refinement.NegatedLiteral())
			if err != nil || applied {
				return err
			}
			applied, err = p.narrowNestedUnion(frame, target, lit, p.refinement.NegatedLiteral(), projection)
			if err != nil || applied || p.refinement.NegatedLiteral() {
				return err
			}
		}
	}
	if refinementHasPresentConstraint(p.refinement) {
		applied, err := p.narrowRootByLiteral(frame, target, typ.LiteralBool(true), false)
		if err != nil {
			return err
		}
		if !frame.Reachable {
			return nil
		}
		if applied {
			projection, err = p.bindDynamicReadProjection(*frame)
			if err != nil {
				return err
			}
		}
	}
	current, found := p.resolve(*frame, projection)
	refinement := p.inheritRootEvidence(*frame, current, found)
	if !found {
		constraint, ok := refinement.Constraint()
		if !ok {
			return nil
		}
		_, valid := frame.Carrier.WritePath(p.plan.Target(), constraint)
		if !valid {
			return fmt.Errorf("factapply: value-refinement path publication rejected")
		}
		return nil
	}
	_, valid := frame.Carrier.WritePath(p.plan.Target(), refineProductValue(p.domain.Registry(), current, refinement))
	if !valid {
		return fmt.Errorf("factapply: value-refinement path write rejected")
	}
	return nil
}

func (p ValueRefinementFactorProgram[K]) narrowRootByLiteral(frame *ValueRefinementFactorFrame[K], target []segment.Segment, lit typ.Type, negate bool) (bool, error) {
	root := p.rootValue(*frame)
	if product.Equal(p.domain.Registry(), root, product.Bottom(p.domain.Registry())) || valueHasUntrustedTopEvidence(p.domain.Registry(), root) {
		return false, nil
	}
	rootType, ok := typevalue.StructuralTypeOf(p.domain.Registry(), p.typeValues, root, typevalue.StructuralTypeOptions{ApplyPresence: true})
	if !ok {
		return false, nil
	}
	var family uint64
	var cases []int
	if negate {
		family, cases, ok = p.typeValues.OriginByPathLiteralNot(rootType, target, lit)
	} else {
		family, cases, ok = p.typeValues.OriginByPathLiteral(rootType, target, lit)
	}
	if !ok {
		return false, nil
	}
	narrowed, ok := p.typeValues.NarrowVariantByOrigin(rootType, family, cases)
	if !ok {
		return false, nil
	}
	constraint := p.typeValues.FromTypeWithWitness(p.domain.Registry(), narrowed)
	constraint = product.Set(p.domain.Registry(), constraint, variantorigin.Key, variantorigin.Of(family, cases))
	refined := refineProductValue(p.domain.Registry(), root, factflow.NewValueConstraint(constraint))
	if product.Equal(p.domain.Registry(), refined, product.Bottom(p.domain.Registry())) {
		frame.Reachable = false
		return true, nil
	}
	if err := p.invalidateRoot(frame, constraint, narrowed); err != nil {
		return false, err
	}
	return true, p.writeRoot(frame, refined)
}

func (p ValueRefinementFactorProgram[K]) narrowNestedUnion(
	frame *ValueRefinementFactorFrame[K],
	target []segment.Segment,
	lit typ.Type,
	negate bool,
	projection *state.DynamicReadFactorProjection,
) (bool, error) {
	root := p.rootValue(*frame)
	rootType, ok := typevalue.StructuralTypeOf(p.domain.Registry(), p.typeValues, root, typevalue.StructuralTypeOptions{ApplyPresence: true})
	if !ok {
		return false, nil
	}
	anchors := p.plan.Anchors()
	for index := 0; index+1 < len(anchors); index++ {
		prefixLength := index + 1
		unionType, found := variant.FieldAtPath(rootType, target[:prefixLength])
		if !found {
			continue
		}
		rest := target[prefixLength:]
		var family uint64
		var cases []int
		if negate {
			family, cases, found = p.typeValues.OriginByPathLiteralNot(unionType, rest, lit)
		} else {
			family, cases, found = p.typeValues.OriginByPathLiteral(unionType, rest, lit)
		}
		if !found {
			continue
		}
		narrowed, found := p.typeValues.NarrowVariantByOrigin(unionType, family, cases)
		if !found {
			continue
		}
		constraint := p.typeValues.FromTypeWithWitness(p.domain.Registry(), narrowed)
		constraint = product.Set(p.domain.Registry(), constraint, variantorigin.Key, variantorigin.Of(family, cases))
		anchor := anchors[index]
		current, readable := p.resolveAt(*frame, anchor, projection)
		if readable {
			constraint = product.Meet(p.domain.Registry(), current, constraint)
		}
		_, valid := frame.Carrier.WritePath(anchor, constraint)
		if !valid {
			return false, fmt.Errorf("factapply: nested value-refinement write rejected")
		}
		return true, nil
	}
	return false, nil
}

func (p ValueRefinementFactorProgram[K]) inheritRootEvidence(frame ValueRefinementFactorFrame[K], current product.Value, found bool) factflow.ValueRefinement {
	constraint, ok := p.refinement.Constraint()
	if !ok || constraintProvesRuntimeCheckedValue(p.domain.Registry(), constraint) || found && (valueProvesScalarValue(p.domain.Registry(), current) || valueHasUntrustedTopEvidence(p.domain.Registry(), current)) {
		return p.refinement
	}
	root := p.rootValue(frame)
	if product.Equal(p.domain.Registry(), root, product.Bottom(p.domain.Registry())) {
		return p.refinement
	}
	got := product.Get(p.domain.Registry(), root, evidence.Key)
	if !got.IsGradualTop() && !got.IsExplicitTop() {
		return p.refinement
	}
	return p.refinement.WithConstraint(p.domain.Registry(), product.Set(p.domain.Registry(), product.Top(), evidence.Key, got))
}

func falsyAbsentValueProven(reg *axis.Registry, current product.Value) bool {
	return !product.Equal(reg, current, product.Bottom(reg)) && !valuerefine.CanBeFalse(reg, current)
}

func (p ValueRefinementFactorProgram[K]) resolve(
	frame ValueRefinementFactorFrame[K],
	projection *state.DynamicReadFactorProjection,
) (product.Value, bool) {
	return p.resolveAt(frame, p.plan.Target(), projection)
}

func (p ValueRefinementFactorProgram[K]) resolveAt(
	frame ValueRefinementFactorFrame[K],
	target keyspace.Key,
	projection *state.DynamicReadFactorProjection,
) (product.Value, bool) {
	reader := valueRefinementFactorReader[K]{
		dependency: p.plan.RootValue(), root: p.root, values: frame.Values, reg: p.domain.Registry(),
	}
	if value, found := p.domain.ResolveValueRefinementAnchorFromFactorProjection(p.plan, target, reader, projection); found {
		return value, true
	}
	if target == p.plan.Root() || p.projectPath == nil {
		return product.Value{}, false
	}
	targetSegments, ok := p.plan.KeySpace().SegmentsView(target)
	if !ok || len(targetSegments) == 0 {
		return product.Value{}, false
	}
	root := p.rootValue(frame)
	rootType, ok := typevalue.StructuralTypeOf(p.domain.Registry(), p.typeValues, root, typevalue.StructuralTypeOptions{ApplyPresence: true})
	if ok {
		if projected, projectedOK := projectStructuralSegments(p.projectPath, rootType, targetSegments); projectedOK {
			return projectedPathValue(p.domain.Registry(), p.typeValues, projected), true
		}
	}
	return projectPathOriginFromRootSegments(p.typeValues, p.domain.Registry(), root, targetSegments, p.projectPath)
}

func (p ValueRefinementFactorProgram[K]) bindDynamicReadProjection(
	frame ValueRefinementFactorFrame[K],
) (*state.DynamicReadFactorProjection, error) {
	skeleton, scalars, _, invalidation, coordinateInvalidation, _, err := frame.Carrier.Freeze()
	if err != nil {
		return nil, err
	}
	factors := append([]state.LaneFactor(nil), frame.Factors...)
	for _, factor := range invalidation {
		index := p.factorIndex(factor.Lane().ID())
		if index < 0 {
			return nil, fmt.Errorf("factapply: value-refinement invalidation factor is absent")
		}
		factors[index] = factor
	}
	for _, coordinate := range coordinateInvalidation {
		index := p.factorIndex(coordinate.Family().Lane().ID())
		if index < 0 {
			continue
		}
		factors[index], err = p.domain.ReplaceCoordinateFamily(
			factors[index], coordinate.Skeleton(), coordinate.Scalars(),
		)
		if err != nil {
			return nil, err
		}
	}
	pathFactor, err := p.domain.SealCoordinateFamilyFactor(skeleton, scalars)
	if err != nil {
		return nil, err
	}
	pathSource, err := p.domain.SealDynamicReadCoordinateFactorSource(p.inventory, pathFactor)
	if err != nil {
		return nil, err
	}
	direct := make([]state.DynamicReadCoordinateFactorSource, 0, len(coordinateInvalidation)+1)
	direct = append(direct, pathSource)
	for _, coordinate := range coordinateInvalidation {
		if p.factorIndex(coordinate.Family().Lane().ID()) >= 0 {
			continue
		}
		source, sourceErr := p.domain.SealDynamicReadCoordinateFactorSource(p.inventory, coordinate)
		if sourceErr != nil {
			return nil, sourceErr
		}
		direct = append(direct, source)
	}
	projection, err := p.domain.BindDynamicReadFactorProjection(p.projection, factors, direct...)
	if err != nil {
		return nil, err
	}
	return &projection, nil
}

func (p ValueRefinementFactorProgram[K]) freezeCarrier(carrier *state.CoordinatePathEvidenceCarrier[K], factors []state.LaneFactor) ([]state.LaneFactor, error) {
	return freezePathEvidenceCarrier(p.domain, carrier, factors, p.lanes)
}

func freezePathEvidenceCarrier[K comparable](
	domain state.ProductDomain,
	carrier *state.CoordinatePathEvidenceCarrier[K],
	factors []state.LaneFactor,
	lanes []state.ProductLane,
) ([]state.LaneFactor, error) {
	skeleton, scalars, _, invalidation, coordinateInvalidation, _, err := carrier.Freeze()
	if err != nil {
		return nil, err
	}
	out := append([]state.LaneFactor(nil), factors...)
	factorIndex := func(id state.LaneID) int {
		for index, lane := range lanes {
			if lane.ID() == id {
				return index
			}
		}
		return -1
	}
	pathIndex := factorIndex(skeleton.Family().Lane().ID())
	if pathIndex >= 0 {
		// Legacy whole-lane consumers outside the formal tuple are updated at the
		// adapter edge. Factor-native branch execution carries this family only as
		// coordinates, so absence here is intentional.
		out[pathIndex], err = domain.ReplaceCoordinateFamily(out[pathIndex], skeleton, scalars)
		if err != nil {
			return nil, err
		}
	}
	for _, factor := range invalidation {
		index := factorIndex(factor.Lane().ID())
		if index < 0 {
			return nil, fmt.Errorf("factapply: value-refinement invalidation factor is absent")
		}
		out[index] = factor
	}
	for _, coordinate := range coordinateInvalidation {
		index := factorIndex(coordinate.Family().Lane().ID())
		if index < 0 {
			continue
		}
		out[index], err = domain.ReplaceCoordinateFamily(out[index], coordinate.Skeleton(), coordinate.Scalars())
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}
