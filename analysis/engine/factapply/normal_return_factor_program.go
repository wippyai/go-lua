package factapply

import (
	"context"
	"fmt"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
	"github.com/wippyai/go-lua/analysis/engine/state/escapeevent"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// NormalReturnFactorFrame is the sparse factor product owned by one call
// transfer. Values contains only the access-sealed dependency/result fibers;
// unmentioned Values fibers remain structural carry in the outer equation.
type NormalReturnFactorFrame[K comparable] struct {
	Values    state.ValueFactor[K]
	Factors   []state.LaneFactor
	Reachable bool
}

// NormalReturnFactorCodec is the one bidirectional boundary between portable
// NormalReturnFacts syntax and the registered factor product. Preparation
// seals the complete address/factor vocabulary; Decode receives the actual
// provider outcome as scratch and never discovers State, inventory, or routes.
type NormalReturnFactorCodec[K comparable] struct {
	authority  *PathSemanticAuthority
	domain     state.ProductDomain
	keys       *keyspace.KeySpace
	point      cfg.Point
	paths      callboundary.PathBindings
	lanes      []state.ProductLane
	ordinals   map[state.ProductLane]int
	inventory  state.CoordinateFactorInventory
	heapRoots  []state.HeapObjectRootSlot
	valueRoots map[statekey.ValueDependency]K
	valueOrder []K
	rootSet    map[K]struct{}
	typestates state.TypestateQueryCapability
	seal       *normalReturnFactorCodecSeal
}

type normalReturnFactorCodecSeal struct{}

// NormalReturnFactorOperand is one positional root scalar supplied at Encode.
// The prepared encoder, not the caller, owns its structural role and ordinal.
// K carries alias equality only for value-only roots.
type NormalReturnFactorOperand[K comparable] struct {
	Slot K
	// BoundarySlot is the optional caller-visible scalar identity of this
	// root. Formal execution keeps its storage coordinate in K, so it supplies
	// the sealed concrete symbol/return identity here without contaminating
	// factor selection with a foreign coordinate. Zero inherits the prepared
	// root schema and then the existing value-only alias identity.
	BoundarySlot statekey.Value
	Value        product.Value
}

// NormalReturnFactorProjection is the complete concrete payload projected by
// one stabilized normal-return leaf. All three fields come from one exact
// BoundaryFactorView transaction; callers cannot pair facts with heap or
// placement evidence from another leaf.
type NormalReturnFactorProjection struct {
	NormalReturnFacts callboundary.NormalReturnFacts
	HeapTableObjects  map[identity.ID]heapidentity.TableObject
	Placements        map[identity.ID]placement.Value
	Roots             state.BoundaryRoots
}

// NormalReturnFactorEncoder is the sole factor-to-call-boundary projection.
// It is prepared directly from the ProductDomain and exact boundary selection;
// Decode's TransferAccess inventory is intentionally irrelevant to this
// opposite-direction operation.
type NormalReturnFactorEncoder[K comparable] struct {
	domain     state.ProductDomain
	view       state.BoundaryFactorViewPlan
	roots      []state.BoundaryFactorRoot
	paramCount int
	seal       *normalReturnFactorEncoderSeal
}

type normalReturnFactorEncoderSeal struct{}

func (p NormalReturnFactorCodec[K]) Lanes() []state.ProductLane {
	return append([]state.ProductLane(nil), p.lanes...)
}

// ValueRoots returns the exact finite Values fibers owned by Decode. The
// caller binds these roots positionally when opening a sparse formal leaf.
func (p NormalReturnFactorCodec[K]) ValueRoots() []K {
	return append([]K(nil), p.valueOrder...)
}

// PrepareNormalReturnFactorEncoder seals the descriptor-declared source union
// plus the two concrete CallOutcome companion lanes. Each lane is requested
// exactly once; disabled optional axes are omitted by BoundaryFactorViewPlan.
func PrepareNormalReturnFactorEncoder[K comparable](
	domain state.ProductDomain,
	selection state.BoundaryFactorSelection,
	paramCount int,
) (NormalReturnFactorEncoder[K], error) {
	if !domain.Valid() || paramCount < 0 {
		return NormalReturnFactorEncoder[K]{}, fmt.Errorf("factapply: invalid normal-return factor encoder")
	}
	requested := callboundary.NormalReturnFactSourceLanes()
	for _, companion := range []state.LaneID{state.LaneHeapTableIdentity, state.LanePlacement} {
		present := false
		for _, lane := range requested {
			if lane == companion {
				present = true
				break
			}
		}
		if !present {
			requested = append(requested, companion)
		}
	}
	view, err := domain.PrepareBoundaryFactorView(selection, requested)
	if err != nil {
		return NormalReturnFactorEncoder[K]{}, err
	}
	roots := view.RootSchemas()
	if paramCount > len(roots) {
		return NormalReturnFactorEncoder[K]{}, fmt.Errorf("factapply: normal-return parameter width exceeds root schema")
	}
	return NormalReturnFactorEncoder[K]{
		domain: domain, view: view, roots: roots, paramCount: paramCount,
		seal: new(normalReturnFactorEncoderSeal),
	}, nil
}

// Lanes returns the exact enabled positional factor tuple expected by Encode.
func (p NormalReturnFactorEncoder[K]) Lanes() []state.ProductLane {
	return p.view.Lanes()
}

// Encode projects one stabilized reachable tuple to a complete portable
// normal-return payload. It reconstructs neither State nor a second emitter
// table. Top or unresolved identity companions fail before any output escapes.
func (p NormalReturnFactorEncoder[K]) Encode(
	ctx context.Context,
	token *cancellation.Token,
	factors []state.LaneFactor,
	operands []NormalReturnFactorOperand[K],
) (NormalReturnFactorProjection, error) {
	if ctx == nil || p.seal == nil || !p.domain.Valid() || len(operands) != len(p.roots) {
		return NormalReturnFactorProjection{}, fmt.Errorf("factapply: invalid normal-return factor encode")
	}
	if err := normalReturnFactorCanceled(ctx, token); err != nil {
		return NormalReturnFactorProjection{}, err
	}
	view, err := p.view.Project(factors)
	if err != nil {
		return NormalReturnFactorProjection{}, err
	}
	heap, err := view.FiniteHeapTableObjectsSnapshot()
	if err != nil {
		return NormalReturnFactorProjection{}, fmt.Errorf("factapply: normal-return heap projection: %w", err)
	}
	placements, err := view.FinitePlacementsSnapshot()
	if err != nil {
		return NormalReturnFactorProjection{}, fmt.Errorf("factapply: normal-return placement projection: %w", err)
	}
	projectedRoots, err := p.projectRoots(operands)
	if err != nil {
		return NormalReturnFactorProjection{}, err
	}
	facts, err := callboundary.NormalReturnFactsFromProjectedSource(
		p.domain.Registry(), p.view.KeySpace(), view, projectedRoots, p.paramCount,
	)
	if err != nil {
		return NormalReturnFactorProjection{}, err
	}
	if err := normalReturnFactorCanceled(ctx, token); err != nil {
		return NormalReturnFactorProjection{}, err
	}
	return NormalReturnFactorProjection{
		NormalReturnFacts: facts,
		HeapTableObjects:  heap.Objects,
		Placements:        placements.Placements,
		Roots:             projectedRoots,
	}, nil
}

func (p NormalReturnFactorEncoder[K]) projectRoots(operands []NormalReturnFactorOperand[K]) (state.BoundaryRoots, error) {
	out := make(state.BoundaryRoots, len(operands))
	synthetic := make(map[K]statekey.Value, len(operands))
	for index, operand := range operands {
		if !product.BelongsToRegistry(p.domain.Registry(), operand.Value) {
			return nil, fmt.Errorf("factapply: normal-return projection root %d belongs to a foreign product", index)
		}
		schema := p.roots[index]
		slot := operand.BoundarySlot
		if slot == 0 {
			slot = schema.Slot
		}
		if slot == 0 {
			var present bool
			slot, present = synthetic[operand.Slot]
			if !present {
				// Small non-zero values are not valid tagged concrete slots. They
				// serve only as collision-free equality witnesses inside the
				// projector and cannot be mistaken for symbol/return identities.
				slot = statekey.Value(index + 1)
				synthetic[operand.Slot] = slot
			}
		}
		out[index] = state.BoundaryRoot{Slot: slot, Path: schema.Path, Value: operand.Value}
	}
	return out, nil
}

// normalReturnApplyPhase is the canonical factor-program publication order.
// It is storage-neutral vocabulary; the deleted State handler table no longer
// owns or interprets these phases.
type normalReturnApplyPhase uint8

const (
	normalReturnApplyBeforeParamFacts normalReturnApplyPhase = iota
	normalReturnApplyAfterParamFacts
	normalReturnApplyAfterParamRelations
	normalReturnApplyFinalWrites
)

type normalReturnFactorLanePlan struct{ phase normalReturnApplyPhase }

// callBranchProofAt translates portable boundary syntax into the canonical
// path-evidence factor vocabulary. It is pure address binding; State replay
// never owned this semantic operation.
func callBranchProofAt(
	ctx normalReturnApplyContext,
	proof callboundary.BranchProof,
) (pathevidence.BranchProof, bool) {
	path, ok := ctx.keyspaceKey(proof.Path)
	if !ok {
		return pathevidence.BranchProof{}, false
	}
	switch proof.Kind {
	case pathevidence.BranchProofPathPresence:
		return pathevidence.BranchProof{
			Kind:     pathevidence.BranchProofPathPresence,
			Path:     path,
			Presence: proof.Presence,
		}, true
	case pathevidence.BranchProofPathEqual, pathevidence.BranchProofPathNotEqual, pathevidence.BranchProofIndexInRange:
		other, ok := ctx.keyspaceKey(proof.Other)
		if !ok {
			return pathevidence.BranchProof{}, false
		}
		return pathevidence.BranchProof{Kind: proof.Kind, Path: path, Other: other}, true
	default:
		return pathevidence.BranchProof{}, false
	}
}

// normalReturnBranchPathRelation is the unique portable-proof to branch
// relation translation used by the factor program.
func normalReturnBranchPathRelation(
	kind pathevidence.BranchProofKind,
	leftPath pathdom.Path,
	rightPath pathdom.Path,
) (factflow.BranchPathRelation, bool) {
	switch kind {
	case pathevidence.BranchProofPathEqual:
		return factflow.NewBranchPathEquality(leftPath, rightPath, true, false), true
	case pathevidence.BranchProofPathNotEqual:
		return factflow.NewBranchPathInequality(leftPath, rightPath, true, false), true
	default:
		return factflow.BranchPathRelation{}, false
	}
}

// This is the sole normal-return execution schedule. It is keyed by the
// canonical storage descriptors and deliberately has no second step-kind
// vocabulary or outcome-specific instruction stream.
var normalReturnFactorLanePlans = callboundary.BindNormalReturnFactLanes(
	"normal-return factor codec",
	map[callboundary.NormalReturnFactLaneID]normalReturnFactorLanePlan{
		callboundary.LanePathRefinements:          {normalReturnApplyBeforeParamFacts},
		callboundary.LanePathInvalidations:        {normalReturnApplyAfterParamFacts},
		callboundary.LanePersistentPathWrites:     {normalReturnApplyFinalWrites},
		callboundary.LanePathStaticMembers:        {normalReturnApplyAfterParamRelations},
		callboundary.LanePathStaticMemberDeltas:   {normalReturnApplyAfterParamRelations},
		callboundary.LanePathPresenceImplications: {normalReturnApplyAfterParamRelations},
		callboundary.LaneDynamicIndexFacts:        {normalReturnApplyAfterParamRelations},
		callboundary.LaneKeyMemberships:           {normalReturnApplyAfterParamRelations},
		callboundary.LaneDynamicValueKeys:         {normalReturnApplyAfterParamRelations},
		callboundary.LaneDynamicAllValues:         {normalReturnApplyAfterParamRelations},
		callboundary.LaneBranchProofs:             {normalReturnApplyAfterParamRelations},
		callboundary.LaneNumFloors:                {normalReturnApplyAfterParamRelations},
		callboundary.LaneNumCeils:                 {normalReturnApplyAfterParamRelations},
		callboundary.LaneRelConstraints:           {normalReturnApplyAfterParamRelations},
		callboundary.LaneChannelSelects:           {normalReturnApplyAfterParamRelations},
		callboundary.LaneFrozenTables:             {normalReturnApplyAfterParamRelations},
		callboundary.LaneEffectDeltas:             {normalReturnApplyAfterParamRelations},
		callboundary.LaneStoreRelations:           {normalReturnApplyAfterParamRelations},
		callboundary.LaneLifecycleFacts:           {normalReturnApplyAfterParamRelations},
		callboundary.LaneEscapeEvents:             {normalReturnApplyAfterParamRelations},
	},
	func(plan normalReturnFactorLanePlan) bool { return plan.phase <= normalReturnApplyFinalWrites },
)

// PrepareNormalReturnFactorCodec seals every authority required by either
// direction. inventory is the operation's already-sealed exact coordinate
// footprint, not a body-wide inventory. The codec validates that declaration
// against TransferAccess but never widens or rediscovers it by lane. bind is
// consumed and never retained.
func PrepareNormalReturnFactorCodec[K comparable](
	authority *PathSemanticAuthority,
	domain state.ProductDomain,
	access state.TransferAccess,
	point cfg.Point,
	boundaryPaths callboundary.PathBindings,
	inventory state.CoordinateFactorInventory,
	heapRoots []state.HeapObjectRootSlot,
	extraValueRoots []statekey.ValueDependency,
	bind func(statekey.ValueDependency) (K, bool),
) (NormalReturnFactorCodec[K], error) {
	if authority == nil || !authority.Valid() || !domain.Valid() || !access.Valid() ||
		authority.KeySpace() == nil || bind == nil ||
		!inventory.ValidFor(domain, authority.KeySpace()) || access.LaneCarry() < 0 {
		return NormalReturnFactorCodec[K]{}, fmt.Errorf("factapply: invalid normal-return factor authority")
	}
	reads := access.LaneCarryReads()
	writes := access.LaneWrites()
	provider, ok := access.ProviderInput(access.LaneCarry())
	if !ok {
		return NormalReturnFactorCodec[K]{}, fmt.Errorf("factapply: normal-return factor codec has no lane-carry provider role")
	}
	for _, slot := range inventory.Slots() {
		if !writes.Has(slot.Family().Lane().ID()) {
			return NormalReturnFactorCodec[K]{}, fmt.Errorf(
				"factapply: normal-return coordinate footprint writes undeclared lane %q",
				slot.Family().Lane().ID(),
			)
		}
	}
	codec := NormalReturnFactorCodec[K]{
		authority: authority, domain: domain, keys: authority.KeySpace(), point: point, paths: boundaryPaths,
		ordinals: make(map[state.ProductLane]int), inventory: inventory,
		heapRoots:  append([]state.HeapObjectRootSlot(nil), heapRoots...),
		valueRoots: make(map[statekey.ValueDependency]K), rootSet: make(map[K]struct{}),
		seal: new(normalReturnFactorCodecSeal),
	}
	for _, lane := range domain.LaneInventory() {
		if !writes.Has(lane.ID()) {
			continue
		}
		if !reads.Has(lane.ID()) {
			return NormalReturnFactorCodec[K]{}, fmt.Errorf("factapply: normal-return write lane %q is not readable from lane carry", lane.ID())
		}
		codec.ordinals[lane] = len(codec.lanes)
		codec.lanes = append(codec.lanes, lane)
	}
	remember := func(dependency statekey.ValueDependency) error {
		if !dependency.Valid() {
			return fmt.Errorf("factapply: invalid normal-return Values dependency")
		}
		if _, exists := codec.valueRoots[dependency]; exists {
			return nil
		}
		root, bound := bind(dependency)
		if !bound {
			if concrete, exact := dependency.Concrete(); exact {
				return fmt.Errorf("factapply: unresolved normal-return Values dependency: concrete slot %d", concrete)
			}
			formalRoot, _ := dependency.Formal()
			return fmt.Errorf("factapply: unresolved normal-return Values dependency: formal root %v", formalRoot)
		}
		codec.valueRoots[dependency] = root
		if _, present := codec.rootSet[root]; !present {
			codec.valueOrder = append(codec.valueOrder, root)
		}
		codec.rootSet[root] = struct{}{}
		return nil
	}
	for _, slot := range append(provider.Values, access.ValueWrites()...) {
		if err := remember(statekey.ConcreteDependency(slot)); err != nil {
			return NormalReturnFactorCodec[K]{}, err
		}
	}
	// Normal-return payloads can carry a postcondition over any already-sealed
	// caller boundary root.  These roots are supplied by the owning boundary
	// adapter at freeze time; retaining their exact bindings here keeps Decode
	// closed while avoiding a solve-time Values-root lookup.
	for _, dependency := range extraValueRoots {
		if err := remember(dependency); err != nil {
			return NormalReturnFactorCodec[K]{}, err
		}
	}
	for _, slot := range inventory.Slots() {
		var bindErr error
		if err := domain.VisitCoordinateValueDependencies(slot, func(dependency statekey.ValueDependency) {
			if bindErr == nil {
				bindErr = remember(dependency)
			}
		}); err != nil {
			return NormalReturnFactorCodec[K]{}, err
		}
		if bindErr != nil {
			family := slot.Family()
			return NormalReturnFactorCodec[K]{}, fmt.Errorf("%w (lane=%s family=%s)", bindErr, family.Lane().ID(), family.ID())
		}
	}
	var err error
	codec.typestates, err = domain.SealTypestateQueryCapability(codec.keys)
	if err != nil {
		return NormalReturnFactorCodec[K]{}, err
	}
	return codec, nil
}

func (p NormalReturnFactorCodec[K]) valid() bool {
	return p.seal != nil && p.authority != nil && p.authority.Valid() && p.domain.Valid() &&
		p.keys != nil && p.inventory.ValidFor(p.domain, p.keys) && len(p.lanes) == len(p.ordinals) &&
		p.typestates.ValidFor(p.domain)
}

func (p NormalReturnFactorCodec[K]) validateFrame(input NormalReturnFactorFrame[K]) error {
	if len(input.Factors) != len(p.lanes) {
		return fmt.Errorf("factapply: incomplete normal-return factor frame")
	}
	for index, factor := range input.Factors {
		if factor.Lane() != p.lanes[index] {
			return fmt.Errorf("factapply: reordered normal-return factor %d", index)
		}
	}
	if !input.Values.Top {
		for root := range input.Values.Values {
			if _, owned := p.rootSet[root]; !owned {
				return fmt.Errorf("factapply: normal-return Values frame contains an unowned fiber")
			}
		}
	}
	return nil
}

// Decode applies one actual normal-return payload as one atomic transaction.
// Descriptor handlers mutate only the scratch frame. Cancellation or any
// rejected fact returns the original frame byte-for-byte.
func (p NormalReturnFactorCodec[K]) Decode(
	ctx context.Context,
	token *cancellation.Token,
	input NormalReturnFactorFrame[K],
	facts callboundary.NormalReturnFacts,
) (NormalReturnFactorFrame[K], error) {
	if ctx == nil || !p.valid() {
		return input, fmt.Errorf("factapply: invalid normal-return factor decode")
	}
	if err := p.validateFrame(input); err != nil {
		return input, err
	}
	if err := normalReturnFactorCanceled(ctx, token); err != nil {
		return input, err
	}
	out := NormalReturnFactorFrame[K]{
		Values: input.Values, Factors: append([]state.LaneFactor(nil), input.Factors...), Reachable: input.Reachable,
	}
	resolve := normalReturnApplyContext{
		node:     transfer.NodeContext{Registry: p.domain.Registry(), Point: p.point},
		resolver: p.authority.resolver, point: p.point, boundaryPaths: p.paths, normalFacts: facts,
	}
	for phase := normalReturnApplyBeforeParamFacts; phase <= normalReturnApplyFinalWrites; phase++ {
		for _, binding := range normalReturnFactorLanePlans {
			if binding.Value.phase != phase || binding.Storage.Len(facts) == 0 {
				continue
			}
			if err := normalReturnFactorCanceled(ctx, token); err != nil {
				return input, err
			}
			if err := p.decodeLane(ctx, token, resolve, binding.ID, &out); err != nil {
				return input, fmt.Errorf("factapply: decode normal-return %s: %w", binding.ID, err)
			}
		}
	}
	if err := normalReturnFactorCanceled(ctx, token); err != nil {
		return input, err
	}
	return out, nil
}

func (p NormalReturnFactorCodec[K]) factor(frame *NormalReturnFactorFrame[K], id state.LaneID) (*state.LaneFactor, bool, error) {
	lane, ok := p.domain.ProductLane(id)
	if !ok {
		return nil, false, nil
	}
	index, present := p.ordinals[lane]
	if !present {
		return nil, true, fmt.Errorf("required factor lane %q is outside frame ownership", id)
	}
	return &frame.Factors[index], true, nil
}

func (p NormalReturnFactorCodec[K]) decodeLane(
	ctx context.Context,
	token *cancellation.Token,
	resolve normalReturnApplyContext,
	id callboundary.NormalReturnFactLaneID,
	frame *NormalReturnFactorFrame[K],
) error {
	switch id {
	case callboundary.LanePathRefinements:
		return p.decodePathRefinements(ctx, token, resolve, frame)
	case callboundary.LanePathInvalidations:
		return p.decodePathInvalidations(resolve, frame)
	case callboundary.LanePersistentPathWrites:
		for _, fact := range resolve.normalFacts.PersistentPathWrites {
			target, ok := resolve.keyspaceKey(fact.Path)
			if !ok {
				continue
			}
			if err := p.decodePathReplacement(frame, state.PathReplacementConfig{Keys: p.keys, Target: target, Value: fact.Value}); err != nil {
				return err
			}
		}
		return nil
	case callboundary.LanePathStaticMembers:
		factor, enabled, err := p.factor(frame, state.LanePathEvidence)
		if err != nil {
			return err
		}
		for _, fact := range resolve.normalFacts.PathStaticMembers {
			target, ok := resolve.keyspaceKey(fact.Path)
			if !ok {
				continue
			}
			if enabled {
				plan, planErr := p.domain.PrepareStaticMemberFactorPlan(p.keys, target, fact.Value)
				if planErr != nil {
					return planErr
				}
				*factor, err = p.domain.ApplyStaticMemberFactor(plan, *factor)
				if err != nil {
					return err
				}
			}
			if err := p.decodeHeapStaticMember(resolve, fact.Path, fact.Value, frame); err != nil {
				return err
			}
		}
		return nil
	case callboundary.LanePathStaticMemberDeltas:
		factor, enabled, err := p.factor(frame, state.LanePathEvidence)
		if err != nil {
			return err
		}
		for _, fact := range resolve.normalFacts.PathStaticMemberDeltas {
			target, ok := resolve.keyspaceKey(fact.Path)
			if !ok {
				continue
			}
			value := fact.Value
			if !fact.Required {
				value = product.WithPresence(p.domain.Registry(), value, presence.Maybe())
			}
			if enabled {
				plan, planErr := p.domain.PrepareStaticMemberFactorPlan(p.keys, target, value)
				if planErr != nil {
					return planErr
				}
				*factor, err = p.domain.ApplyStaticMemberFactor(plan, *factor)
				if err != nil {
					return err
				}
			}
			if err := p.decodeHeapStaticMember(resolve, fact.Path, value, frame); err != nil {
				return err
			}
		}
		return nil
	case callboundary.LaneKeyMemberships, callboundary.LaneDynamicValueKeys, callboundary.LaneDynamicAllValues:
		return p.decodeKeyMemberships(resolve, id, frame)
	case callboundary.LaneBranchProofs:
		factor, enabled, err := p.factor(frame, state.LanePathEvidence)
		if err != nil {
			return err
		}
		if enabled {
			proofs := make([]pathevidence.BranchProof, 0, len(resolve.normalFacts.BranchProofs))
			for _, proof := range resolve.normalFacts.BranchProofs {
				if converted, ok := callBranchProofAt(resolve, proof); ok {
					proofs = append(proofs, converted)
				}
			}
			*factor, err = p.domain.ApplyCallOutcomeBranchProofFactors(*factor, proofs)
			if err != nil {
				return err
			}
		}
		return p.decodeBranchProofConsequences(ctx, resolve, frame)
	case callboundary.LaneNumFloors:
		for _, fact := range resolve.normalFacts.NumFloors {
			target, ok := resolve.keyspaceKey(fact.Path)
			if !ok {
				continue
			}
			mutation, err := p.domain.PrepareCoordinateBranchBound(state.CoordinateBoundValue, state.CoordinateBoundLower, p.keys, target, fact.Floor)
			if err != nil {
				return err
			}
			lane, err := p.domain.CoordinateBranchMutationLane(mutation)
			if err != nil {
				return err
			}
			factor, enabled, err := p.factor(frame, lane.ID())
			if err != nil {
				return err
			}
			if !enabled {
				continue
			}
			*factor, err = p.domain.ApplyCoordinateBranchMutationFactor(mutation, *factor)
			if err != nil {
				return err
			}
		}
		return nil
	case callboundary.LaneNumCeils:
		for _, fact := range resolve.normalFacts.NumCeils {
			target, ok := resolve.keyspaceKey(fact.Path)
			if !ok {
				continue
			}
			mutation, err := p.domain.PrepareCoordinateBranchBound(state.CoordinateBoundValue, state.CoordinateBoundUpper, p.keys, target, fact.Ceil)
			if err != nil {
				return err
			}
			lane, err := p.domain.CoordinateBranchMutationLane(mutation)
			if err != nil {
				return err
			}
			factor, enabled, err := p.factor(frame, lane.ID())
			if err != nil {
				return err
			}
			if !enabled {
				continue
			}
			*factor, err = p.domain.ApplyCoordinateBranchMutationFactor(mutation, *factor)
			if err != nil {
				return err
			}
		}
		return nil
	case callboundary.LaneRelConstraints:
		return p.decodeRelConstraints(resolve, frame)
	case callboundary.LaneChannelSelects:
		factor, enabled, err := p.factor(frame, state.LaneChannelSelect)
		if err != nil {
			return err
		}
		if !enabled {
			return nil
		}
		facts := make([]channelselectfact.Fact, 0, len(resolve.normalFacts.ChannelSelects))
		for _, event := range resolve.normalFacts.ChannelSelects {
			if fact, ok := callChannelSelectFactAt(resolve, event); ok {
				facts = append(facts, fact)
			}
		}
		*factor, err = p.domain.ApplyChannelSelectFactsFactor(*factor, facts)
		return err
	case callboundary.LaneEffectDeltas:
		factor, enabled, err := p.factor(frame, state.LaneEffectDeltas)
		if err != nil {
			return err
		}
		if !enabled {
			return nil
		}
		for _, delta := range resolve.normalFacts.EffectDeltas {
			target, ok := resolve.keyspaceKey(delta.Target)
			if !ok {
				continue
			}
			plan, planErr := p.domain.PrepareEffectDeltaFactorPlan(effectdelta.Key{Target: target, Site: delta.Site, Kind: delta.Kind}, delta.Value)
			if planErr != nil {
				return planErr
			}
			*factor, err = p.domain.ApplyEffectDeltaFactor(plan, *factor)
			if err != nil {
				return err
			}
		}
		return nil
	case callboundary.LaneStoreRelations:
		factor, enabled, err := p.factor(frame, state.LaneStoreRelations)
		if err != nil {
			return err
		}
		if !enabled {
			return nil
		}
		stores := make([]state.StoreRelation, 0, len(resolve.normalFacts.StoreRelations))
		for _, relation := range resolve.normalFacts.StoreRelations {
			source, sourceOK := resolve.stateKey(relation.Source)
			into, intoOK := resolve.stateKey(relation.Into)
			if sourceOK && intoOK {
				stores = append(stores, state.StoreRelation{Source: source, Into: into})
			}
		}
		*factor, err = p.domain.ApplyCallOutcomeStoreRelationFactors(*factor, stores)
		return err
	case callboundary.LaneLifecycleFacts:
		return p.decodeLifecycle(resolve, frame)
	case callboundary.LaneEscapeEvents:
		factor, enabled, err := p.factor(frame, state.LaneEscapeEvents)
		if err != nil {
			return err
		}
		if enabled {
			escapes := make([]escapeevent.Fact, 0, len(resolve.normalFacts.EscapeEvents))
			for _, event := range resolve.normalFacts.EscapeEvents {
				if target, ok := resolve.visibleStateKey(event.Target); ok {
					escapes = append(escapes, escapeevent.Fact{Target: target, Kind: event.Kind, Recursive: event.Recursive})
				}
			}
			*factor, err = p.domain.ApplyCallOutcomeEscapeEventFactors(*factor, escapes)
			if err != nil {
				return err
			}
		}
		return p.decodeEscapePlacements(ctx, resolve, frame)
	case callboundary.LaneFrozenTables:
		return p.decodeFrozenTables(resolve, frame)
	case callboundary.LanePathPresenceImplications:
		return p.decodePresenceImplications(ctx, token, resolve, frame)
	case callboundary.LaneDynamicIndexFacts:
		return p.decodeDynamicIndexFacts(ctx, resolve, frame)
	default:
		return fmt.Errorf("normal-return descriptor %q is not classified", id)
	}
}

func (p NormalReturnFactorCodec[K]) decodeBranchProofConsequences(
	ctx context.Context,
	resolve normalReturnApplyContext,
	frame *NormalReturnFactorFrame[K],
) error {
	for _, proof := range resolve.normalFacts.BranchProofs {
		if proof.Kind != pathevidence.BranchProofPathEqual && proof.Kind != pathevidence.BranchProofPathNotEqual {
			continue
		}
		left, leftOK := resolve.substitute(proof.Path)
		right, rightOK := resolve.substitute(proof.Other)
		if !leftOK || !rightOK {
			continue
		}
		relation, ok := normalReturnBranchPathRelation(proof.Kind, left, right)
		if !ok {
			continue
		}
		transaction := BranchRelationTransaction{
			point: p.point, cond: true,
			steps: []BranchRelationStep{{kind: BranchRelationStepPath, path: relation}},
		}
		factors, err := p.authority.PrepareBranchRelationFactors(p.domain, transaction, p.inventory)
		if err != nil {
			return err
		}
		original := *frame
		original.Factors = append([]state.LaneFactor(nil), frame.Factors...)
		edge := transfer.EdgeContext{
			Context: ctx, Registry: p.domain.Registry(),
			Edge: cfg.Edge{From: p.point, Cond: true}, HasCond: true,
		}
		for _, stage := range factors.Stages() {
			stageInput := *frame
			stageInput.Factors = append([]state.LaneFactor(nil), frame.Factors...)
			for _, index := range stage.Factors() {
				originalBound, bindErr := p.bindBranchFactorFrame(factors, index, BranchRelationFactorOriginal, original)
				if bindErr != nil {
					return bindErr
				}
				currentBound, bindErr := p.bindBranchFactorFrame(factors, index, BranchRelationFactorCurrent, stageInput)
				if bindErr != nil {
					return bindErr
				}
				patch, canceled, applyErr := factors.ApplyFactorFrames(index, edge, originalBound, currentBound)
				if applyErr != nil {
					return applyErr
				}
				if canceled {
					return context.Canceled
				}
				if err := p.applyBranchFactorPatch(frame, patch); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (p NormalReturnFactorCodec[K]) bindBranchFactorFrame(
	factors BranchRelationFactors,
	index int,
	role BranchRelationFactorRole,
	frame NormalReturnFactorFrame[K],
) (BranchRelationFactorFrame, error) {
	layout, ok := factors.FactorLayout(index)
	if !ok {
		return BranchRelationFactorFrame{}, fmt.Errorf("factapply: normal-return branch factor layout is absent")
	}
	roles, lanes, coordinates := layout.CurrentValueRoles(), layout.CurrentLanes(), layout.CurrentCoordinates()
	if role == BranchRelationFactorOriginal {
		roles, lanes, coordinates = layout.OriginalValueRoles(), layout.OriginalLanes(), layout.OriginalCoordinates()
	}
	values := make([]product.Value, len(roles))
	for ordinal, valueRole := range roles {
		symbolID, valid := valueRole.LexicalSymbol()
		if !valid {
			return BranchRelationFactorFrame{}, fmt.Errorf("factapply: malformed normal-return branch Values role")
		}
		root, bound := p.valueRoots[statekey.ConcreteDependency(statekey.SymbolValue(symbolID))]
		if !bound {
			return BranchRelationFactorFrame{}, fmt.Errorf("factapply: unbound normal-return branch Values role")
		}
		values[ordinal] = product.Bottom(p.domain.Registry())
		if !frame.Values.Top {
			if value, present := frame.Values.Values[root]; present {
				values[ordinal] = value
			}
		} else {
			values[ordinal] = product.Top()
		}
	}
	laneFactors := make([]state.LaneFactor, len(lanes))
	for ordinal, lane := range lanes {
		factor, enabled, err := p.factor(&frame, lane.ID())
		if err != nil || !enabled {
			if err == nil {
				err = fmt.Errorf("factapply: branch lane %q is outside normal-return frame", lane.ID())
			}
			return BranchRelationFactorFrame{}, err
		}
		laneFactors[ordinal] = *factor
	}
	coordinateFactors := make([]BranchRelationCoordinateOperands, len(coordinates))
	for groupIndex, group := range coordinates {
		factor, enabled, err := p.factor(&frame, group.Family().Lane().ID())
		if err != nil || !enabled {
			if err == nil {
				err = fmt.Errorf("factapply: branch coordinate lane is outside normal-return frame")
			}
			return BranchRelationFactorFrame{}, err
		}
		skeleton, explicit, err := p.domain.DecomposeCoordinateFamily(*factor, group.Family(), p.keys)
		if err != nil {
			return BranchRelationFactorFrame{}, err
		}
		coordinateFactors[groupIndex], err = bindBranchCoordinateLayout(p.domain, group, skeleton, explicit)
		if err != nil {
			return BranchRelationFactorFrame{}, err
		}
	}
	return factors.BindFactorFrame(index, role, BranchRelationFactorOperands{
		Values: values, ValuesTop: frame.Values.Top, Lanes: laneFactors,
		Coordinates: coordinateFactors, Reachable: frame.Reachable,
	})
}

func (p NormalReturnFactorCodec[K]) applyBranchFactorPatch(
	frame *NormalReturnFactorFrame[K],
	patch BranchRelationFactorPatch,
) error {
	if !patch.Reachable() {
		frame.Reachable = false
		return nil
	}
	for _, group := range patch.Coordinates() {
		lane := group.Skeleton.Family().Lane()
		factor, enabled, err := p.factor(frame, lane.ID())
		if err != nil || !enabled {
			if err == nil {
				err = fmt.Errorf("factapply: branch patch lane is outside normal-return frame")
			}
			return err
		}
		*factor, err = p.domain.PatchCoordinateFamily(*factor, group.Skeleton, group.Scalars)
		if err != nil {
			return err
		}
	}
	return nil
}

type normalReturnDynamicFactorMutation struct {
	table       keyspace.Key
	tablePath   pathdom.Path
	key         dynamicindex.Key
	fact        dynamicindex.Fact
	source      []pathaddr.StateKey
	priorAll    []pathaddr.StateKey
	freshAtCall bool
}

func (p NormalReturnFactorCodec[K]) decodeDynamicIndexFacts(
	ctx context.Context,
	resolve normalReturnApplyContext,
	frame *NormalReturnFactorFrame[K],
) error {
	memberships, membershipsEnabled, err := p.factor(frame, state.LaneKeyMemberships)
	if err != nil {
		return err
	}
	mutations := make([]normalReturnDynamicFactorMutation, 0, len(resolve.normalFacts.DynamicIndexFacts))
	counts := make(map[keyspace.Key]int)
	for _, fact := range resolve.normalFacts.DynamicIndexFacts {
		table, ok := resolve.keyspaceKey(fact.Table)
		if ok {
			counts[table]++
		}
	}
	prior := make(map[keyspace.Key][]pathaddr.StateKey)
	cleared := make(map[keyspace.Key]struct{})
	for _, fact := range resolve.normalFacts.DynamicIndexFacts {
		table, ok := resolve.keyspaceKey(fact.Table)
		if !ok {
			continue
		}
		tablePath, ok := resolve.substitute(fact.Table)
		if !ok {
			continue
		}
		mutation := normalReturnDynamicFactorMutation{
			table: table, tablePath: tablePath,
			key: dynamicindex.Key{Table: table, Site: fact.Site}, fact: fact.Value,
		}
		if membershipsEnabled {
			if _, seen := cleared[table]; !seen {
				snapshot, observeErr := p.domain.ObserveCallOutcomeDynamicIndexMembershipFactors(*memberships, table, "")
				if observeErr != nil {
					return observeErr
				}
				prior[table] = snapshot.AllValueTables
				cleared[table] = struct{}{}
			}
			if valuePath, hasValuePath := resolve.substitute(fact.ValuePath); hasValuePath {
				evidence, observeErr := p.domain.ObserveDynamicIndexMembershipEvidence(*memberships, state.DynamicIndexMembershipEvidenceQuery{
					Container: table, SourceStateKeys: pathMembershipSourceStateKeysAt(resolve.resolver, resolve.point, valuePath),
				})
				if observeErr != nil {
					return observeErr
				}
				mutation.source = evidence.SourceMemberships
			}
			mutation.priorAll = append([]pathaddr.StateKey(nil), prior[table]...)
		}
		if counts[table] == 1 {
			mutation.freshAtCall, err = p.factorRootFreshEmpty(tablePath, frame)
			if err != nil {
				return err
			}
		}
		mutations = append(mutations, mutation)
	}
	if membershipsEnabled {
		for table := range cleared {
			*memberships, err = p.domain.ClearCallOutcomeDynamicIndexValueKeyMembershipFactors(*memberships, table)
			if err != nil {
				return err
			}
		}
	}
	dynamicFactor, dynamicEnabled, err := p.factor(frame, state.LaneDynamicIndex)
	if err != nil {
		return err
	}
	if dynamicEnabled {
		batch := make([]state.CallOutcomeDynamicIndexMutation, len(mutations))
		for index, mutation := range mutations {
			batch[index] = state.CallOutcomeDynamicIndexMutation{Key: mutation.key, Fact: mutation.fact}
		}
		*dynamicFactor, err = p.domain.ApplyCallOutcomeDynamicIndexFactors(*dynamicFactor, batch)
		if err != nil {
			return err
		}
	}
	for _, mutation := range mutations {
		if membershipsEnabled {
			facts := make([]state.KeyMembership, 0, len(mutation.source)*2+len(mutation.priorAll))
			for _, table := range mutation.source {
				facts = append(facts, state.DynamicIndexValueKeyMembership(mutation.table, mutation.key.Site, table))
			}
			if dynamicIndexFactDefinitelyAbsent(p.domain.Registry(), mutation.fact) {
				for _, table := range mutation.priorAll {
					facts = append(facts, state.DynamicIndexAllValuesKeyMembership(mutation.table, table))
				}
			} else {
				for _, table := range mutation.priorAll {
					if stateKeyIn(mutation.source, table) {
						facts = append(facts, state.DynamicIndexAllValuesKeyMembership(mutation.table, table))
					}
				}
				if mutation.freshAtCall {
					for _, table := range mutation.source {
						facts = append(facts, state.DynamicIndexAllValuesKeyMembership(mutation.table, table))
					}
				}
			}
			*memberships, err = p.domain.ApplyCallOutcomeKeyMembershipFactors(*memberships, facts)
			if err != nil {
				return err
			}
		}
		if err := p.decodeHeapDynamicIndex(ctx, mutation, frame); err != nil {
			return err
		}
	}
	return p.decodeRootDynamicAllValueMemberships(mutations, frame)
}

func (p NormalReturnFactorCodec[K]) factorRootFreshEmpty(
	target pathdom.Path,
	frame *NormalReturnFactorFrame[K],
) (bool, error) {
	if target.Symbol == 0 || len(target.Segments) != 0 {
		return false, nil
	}
	key := p.keys.FromPath(target)
	if key.Kind == keyspace.KindInvalid {
		return false, nil
	}
	value, found, err := p.resolveFactorPathValue(key, frame)
	if err != nil || !found {
		return false, err
	}
	id, exact := product.Get(p.domain.Registry(), value, identity.Key).ID()
	if !exact {
		return false, nil
	}
	family, present := p.domain.RootAssignmentCoordinateFamily()
	if !present {
		return false, nil
	}
	factor, enabled, err := p.factor(frame, family.Lane().ID())
	if err != nil || !enabled {
		return false, err
	}
	skeleton, _, err := p.domain.DecomposeCoordinateFamily(*factor, family, p.keys)
	if err != nil {
		return false, err
	}
	return p.domain.CoordinateRootAssignmentFreshEmpty(skeleton, id)
}

func (p NormalReturnFactorCodec[K]) decodeHeapDynamicIndex(
	ctx context.Context,
	mutation normalReturnDynamicFactorMutation,
	frame *NormalReturnFactorFrame[K],
) error {
	tableValue, found, err := p.resolveFactorPathValue(mutation.table, frame)
	if err != nil || !found {
		return err
	}
	id, exact := product.Get(p.domain.Registry(), tableValue, identity.Key).ID()
	if !exact {
		return nil
	}
	heap, heapEnabled, err := p.factor(frame, state.LaneHeapTableIdentity)
	if err != nil {
		return err
	}
	if heapEnabled {
		object, readErr := p.domain.ReadHeapTableObjectTermFactor(*heap, identity.ConcreteTerm(id))
		if readErr != nil {
			return readErr
		}
		if !heapidentity.ObjectDomain(p.domain.Registry()).Equal(object, heapidentity.BottomObject(p.domain.Registry())) {
			dynamic := object.DynamicIndexFacts()
			if dynamic == nil {
				dynamic = make(map[dynamicindex.Key]dynamicindex.Fact, 1)
			}
			if prior, present := dynamic[mutation.key]; present {
				dynamic[mutation.key] = dynamicindex.Domain(p.domain.Registry()).Join(prior, mutation.fact)
			} else {
				dynamic[mutation.key] = mutation.fact
			}
			replacement := heapidentity.NewTableObject(heapidentity.TableObjectConfig{
				Root: object.Root(), StaticMembers: object.StaticMembers(), DynamicIndexFacts: dynamic,
			})
			plan, planErr := p.domain.PrepareObjectGraphReplacePlan(p.keys, []state.ObjectGraphMutation{{
				Identity: identity.ConcreteTerm(id), Object: replacement,
			}})
			if planErr != nil {
				return planErr
			}
			lanes, planErr := p.domain.ObjectGraphMutationLanes(plan)
			if planErr != nil {
				return planErr
			}
			for _, lane := range lanes {
				factor, enabled, factorErr := p.factor(frame, lane.ID())
				if factorErr != nil {
					return factorErr
				}
				if enabled {
					*factor, factorErr = p.domain.ApplyObjectGraphMutationFactor(plan, *factor)
					if factorErr != nil {
						return factorErr
					}
				}
			}
		}
	}
	placementFactor, placementEnabled, err := p.factor(frame, state.LanePlacement)
	if err != nil || !placementEnabled {
		return err
	}
	owner, err := p.domain.ReadPlacementTermFactor(*placementFactor, identity.ConcreteTerm(id))
	if err != nil {
		return err
	}
	switch owner {
	case placement.OwnedHeap, placement.SharedHeap, placement.Unknown:
		return p.decodePlacementReachability(ctx, mutation.fact.Value, owner, frame)
	default:
		return nil
	}
}

func (p NormalReturnFactorCodec[K]) decodeRootDynamicAllValueMemberships(
	mutations []normalReturnDynamicFactorMutation,
	frame *NormalReturnFactorFrame[K],
) error {
	memberships, enabled, err := p.factor(frame, state.LaneKeyMemberships)
	if err != nil || !enabled {
		return err
	}
	seen := make(map[keyspace.Key]struct{})
	for _, mutation := range mutations {
		if mutation.tablePath.Symbol == 0 || len(mutation.tablePath.Segments) != 0 {
			continue
		}
		if _, duplicate := seen[mutation.table]; duplicate {
			continue
		}
		seen[mutation.table] = struct{}{}
		tableValue, found, err := p.resolveFactorPathValue(mutation.table, frame)
		if err != nil {
			return err
		}
		if !found {
			continue
		}
		id, exact := product.Get(p.domain.Registry(), tableValue, identity.Key).ID()
		if !exact {
			continue
		}
		heap, heapEnabled, err := p.factor(frame, state.LaneHeapTableIdentity)
		if err != nil || !heapEnabled {
			if err != nil {
				return err
			}
			continue
		}
		object, err := p.domain.ReadHeapTableObjectTermFactor(*heap, identity.ConcreteTerm(id))
		if err != nil {
			return err
		}
		if !product.Equal(p.domain.Registry(), object.Root(), tableValue) || len(object.StaticMembers()) != 0 {
			continue
		}
		common := make(map[pathaddr.StateKey]struct{})
		foundSource := false
		for key, fact := range object.DynamicIndexFacts() {
			if key.Table != mutation.table || fact.Admission == dynamicindex.AdmissionRejected ||
				product.Equal(p.domain.Registry(), fact.Value, product.Bottom(p.domain.Registry())) ||
				presence.Equal(product.PresenceOf(fact.Value), presence.Absent()) {
				continue
			}
			snapshot, observeErr := p.domain.ObserveCallOutcomeDynamicIndexMembershipFactors(*memberships, mutation.table, key.Site)
			if observeErr != nil {
				return observeErr
			}
			if len(snapshot.ValueTables) == 0 {
				common = nil
				foundSource = false
				break
			}
			if !foundSource {
				for _, table := range snapshot.ValueTables {
					common[table] = struct{}{}
				}
				foundSource = true
				continue
			}
			for table := range common {
				if !stateKeyIn(snapshot.ValueTables, table) {
					delete(common, table)
				}
			}
		}
		if !foundSource || len(common) == 0 {
			continue
		}
		facts := make([]state.KeyMembership, 0, len(common))
		for table := range common {
			facts = append(facts, state.DynamicIndexAllValuesKeyMembership(mutation.table, table))
		}
		*memberships, err = p.domain.ApplyCallOutcomeKeyMembershipFactors(*memberships, facts)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p NormalReturnFactorCodec[K]) decodePresenceImplications(
	ctx context.Context,
	token *cancellation.Token,
	resolve normalReturnApplyContext,
	frame *NormalReturnFactorFrame[K],
) error {
	publications := make([]pathevidence.PathPresenceImplication, 0, len(resolve.normalFacts.PathPresenceImplications))
	for _, fact := range resolve.normalFacts.PathPresenceImplications {
		trigger, ok := resolve.keyspaceKey(fact.Trigger)
		if !ok {
			continue
		}
		target, ok := resolve.keyspaceKey(fact.Target)
		if !ok {
			continue
		}
		var implication pathevidence.PathPresenceImplication
		switch {
		case fact.HasTriggerValue && fact.HasTargetValue:
			implication = pathevidence.NewPathValueRefinementImplication(trigger, fact.TriggerValue, target, fact.TargetValue)
		case fact.HasTriggerValue:
			implication = pathevidence.NewPathValuePresenceImplication(trigger, fact.TriggerValue, target, fact.TargetPresence)
		case fact.HasTargetValue:
			implication = pathevidence.PathPresenceImplication{
				Trigger: trigger, TriggerPresence: fact.TriggerPresence,
				Target: target, TargetValue: fact.TargetValue, HasTargetValue: true,
			}
		default:
			implication = pathevidence.NewPathPresenceImplication(trigger, fact.TriggerPresence, target, fact.TargetPresence)
		}
		publications = append(publications, implication)
	}
	if len(publications) == 0 {
		return nil
	}
	source, err := p.authority.PreparePresenceImplicationPlan(
		p.domain.Registry(), p.point, publications, ConcretePresenceImplicationTrailingBarrier,
	)
	if err != nil {
		return err
	}
	plan, err := source.DependencyBlocks(p.domain, p.inventory)
	if err != nil {
		return err
	}
	binding, err := SealPresenceImplicationRootBinding(plan,
		func(dependency statekey.ValueDependency) (K, bool) {
			root, ok := p.valueRoots[dependency]
			return root, ok
		},
		func(root K) bool { _, ok := p.rootSet[root]; return ok },
	)
	if err != nil {
		return err
	}
	program := CallResultPostconditionFactorProgram[K]{
		domain: p.domain, keys: p.keys,
		lanes: p.lanes, roots: p.valueRoots, rootSet: p.rootSet,
		presence: plan, presenceRoots: binding,
	}
	next, err := program.applyPresence(ctx, token, CallResultPostconditionFactorFrame[K]{
		Values: frame.Values, Factors: frame.Factors, Reachable: frame.Reachable,
	})
	if err != nil {
		return err
	}
	frame.Values, frame.Factors, frame.Reachable = next.Values, next.Factors, next.Reachable
	return nil
}

func (p NormalReturnFactorCodec[K]) resolveFactorPathValue(
	target keyspace.Key,
	frame *NormalReturnFactorFrame[K],
) (product.Value, bool, error) {
	lanes := p.domain.PathReplacementReadLanes()
	factors := make([]state.LaneFactor, len(lanes))
	for index, lane := range lanes {
		factor, enabled, err := p.factor(frame, lane.ID())
		if err != nil {
			return product.Value{}, false, err
		}
		if !enabled {
			return product.Value{}, false, fmt.Errorf("factapply: path resolver lane %q is outside the factor frame", lane.ID())
		}
		factors[index] = *factor
	}
	value, found := p.domain.ResolveFactorPathValue(
		p.keys, target,
		normalReturnFactorValueReader[K]{values: frame.Values, roots: p.valueRoots},
		factors,
	)
	return value, found, nil
}

func (p NormalReturnFactorCodec[K]) decodeFrozenTables(
	resolve normalReturnApplyContext,
	frame *NormalReturnFactorFrame[K],
) error {
	for _, fact := range resolve.normalFacts.FrozenTables {
		target, ok := resolve.keyspaceKey(fact.Target)
		if !ok {
			continue
		}
		if err := p.decodeEffectDelta(target, callboundary.FrozenTableEffectSite(), effectdelta.Freeze, effectdelta.Top(), frame); err != nil {
			return err
		}
		value, found, err := p.resolveFactorPathValue(target, frame)
		if err != nil {
			return err
		}
		if !found {
			continue
		}
		id, exact := product.Get(p.domain.Registry(), value, identity.Key).ID()
		if !exact {
			continue
		}
		factor, enabled, err := p.factor(frame, state.LaneFrozenTables)
		if err != nil {
			return err
		}
		if enabled {
			*factor, err = p.domain.ApplyCallOutcomeFrozenTableFactor(*factor, id)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (p NormalReturnFactorCodec[K]) decodeEffectDelta(
	target keyspace.Key,
	site effectdelta.Site,
	kind effectdelta.Kind,
	value effectdelta.Value,
	frame *NormalReturnFactorFrame[K],
) error {
	factor, enabled, err := p.factor(frame, state.LaneEffectDeltas)
	if err != nil || !enabled {
		return err
	}
	plan, err := p.domain.PrepareEffectDeltaFactorPlan(effectdelta.Key{Target: target, Site: site, Kind: kind}, value)
	if err != nil {
		return err
	}
	*factor, err = p.domain.ApplyEffectDeltaFactor(plan, *factor)
	return err
}

func (p NormalReturnFactorCodec[K]) decodeEscapePlacements(
	ctx context.Context,
	resolve normalReturnApplyContext,
	frame *NormalReturnFactorFrame[K],
) error {
	for _, event := range resolve.normalFacts.EscapeEvents {
		targetPlacement, ok := escapeEventPlacement(event.Kind)
		if !ok {
			continue
		}
		target, ok := resolve.keyspaceKey(event.Target)
		if !ok {
			continue
		}
		seed, found, err := p.resolveFactorPathValue(target, frame)
		if err != nil {
			return err
		}
		if !found {
			if err := p.decodeEscapeCandidatePlacements(ctx, resolve, event.Target, targetPlacement, event.Recursive, frame); err != nil {
				return err
			}
			continue
		}
		id, exact := product.Get(p.domain.Registry(), seed, identity.Key).ID()
		if !exact {
			if err := p.decodeEscapeCandidatePlacements(ctx, resolve, event.Target, targetPlacement, event.Recursive, frame); err != nil {
				return err
			}
			continue
		}
		if event.Recursive {
			if err := p.decodePlacementReachability(ctx, seed, targetPlacement, frame); err != nil {
				return err
			}
			continue
		}
		factor, enabled, err := p.factor(frame, state.LanePlacement)
		if err != nil {
			return err
		}
		if enabled {
			*factor, err = p.domain.ApplyCallOutcomePlacementFactor(*factor, id, targetPlacement)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (p NormalReturnFactorCodec[K]) decodeEscapeCandidatePlacements(
	ctx context.Context,
	resolve normalReturnApplyContext,
	boundaryPath pathdom.Path,
	target placement.Value,
	recursive bool,
	frame *NormalReturnFactorFrame[K],
) error {
	path, ok := resolve.substitute(boundaryPath)
	if !ok || len(path.Segments) == 0 {
		return nil
	}
	parent := path.ParentView()
	if parent.IsEmpty() {
		return nil
	}
	last := path.Segments[len(path.Segments)-1]
	parentKey, ok := visibility.AddressAt(resolve.resolver, resolve.point, parent).RootOrVisibleKeyspaceKey()
	if !ok {
		return nil
	}
	seen := make(map[identity.ID]struct{})
	apply := func(value product.Value) error {
		id, exact := product.Get(p.domain.Registry(), value, identity.Key).ID()
		if !exact {
			return nil
		}
		if _, duplicate := seen[id]; duplicate {
			return nil
		}
		seen[id] = struct{}{}
		if recursive {
			return p.decodePlacementReachability(ctx, value, target, frame)
		}
		factor, enabled, err := p.factor(frame, state.LanePlacement)
		if err != nil || !enabled {
			return err
		}
		*factor, err = p.domain.ApplyCallOutcomePlacementFactor(*factor, id, target)
		return err
	}
	if dynamic, enabled, err := p.factor(frame, state.LaneDynamicIndex); err != nil {
		return err
	} else if enabled {
		facts, err := p.domain.ObserveCallOutcomeDynamicIndexFactors(*dynamic, parentKey)
		if err != nil {
			return err
		}
		for _, candidate := range facts {
			if candidate.Fact.Admission != dynamicindex.AdmissionRejected &&
				dynamicIndexFactCanEscapeThroughStaticSegment(p.domain.Registry(), candidate.Fact, last) {
				if err := apply(candidate.Fact.Value); err != nil {
					return err
				}
			}
		}
	}
	parentValue, found, err := p.resolveFactorPathValue(parentKey, frame)
	if err != nil || !found {
		return err
	}
	parentID, exact := product.Get(p.domain.Registry(), parentValue, identity.Key).ID()
	if !exact {
		return nil
	}
	heap, enabled, err := p.factor(frame, state.LaneHeapTableIdentity)
	if err != nil || !enabled {
		return err
	}
	object, err := p.domain.ReadHeapTableObjectTermFactor(*heap, identity.ConcreteTerm(parentID))
	if err != nil {
		return err
	}
	for _, candidate := range object.DynamicIndexFacts() {
		if candidate.Admission != dynamicindex.AdmissionRejected &&
			dynamicIndexFactCanEscapeThroughStaticSegment(p.domain.Registry(), candidate, last) {
			if err := apply(candidate.Value); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p NormalReturnFactorCodec[K]) decodePlacementReachability(
	ctx context.Context,
	seed product.Value,
	target placement.Value,
	frame *NormalReturnFactorFrame[K],
) error {
	plan, err := p.domain.PreparePlacementReachabilityPlan(p.keys, []product.Value{seed}, target)
	if err != nil {
		return err
	}
	lanes, err := p.domain.PlacementReachabilityLanes(plan)
	if err != nil {
		return err
	}
	factors := make([]state.LaneFactor, len(lanes))
	indices := make([]int, len(lanes))
	for index, lane := range lanes {
		factor, enabled, factorErr := p.factor(frame, lane.ID())
		if factorErr != nil {
			return factorErr
		}
		if !enabled {
			return fmt.Errorf("factapply: placement reachability lane %q is outside the factor frame", lane.ID())
		}
		factors[index], indices[index] = *factor, p.ordinals[lane]
	}
	next, err := p.domain.ApplyPlacementReachabilityFactors(ctx, plan, factors)
	if err != nil {
		return err
	}
	for index, global := range indices {
		frame.Factors[global] = next[index]
	}
	return nil
}

func (p NormalReturnFactorCodec[K]) decodePathInvalidations(
	resolve normalReturnApplyContext,
	frame *NormalReturnFactorFrame[K],
) error {
	dynamicTables := normalReturnDynamicIndexMutationTables(resolve.normalFacts.DynamicIndexFacts)
	for _, fact := range resolve.normalFacts.PathInvalidations {
		targetPath, substituted := resolve.substitute(fact.Path)
		if !substituted {
			continue
		}
		target, exact := resolve.keyspaceKey(fact.Path)
		if exact {
			memberships, enabled, err := p.factor(frame, state.LaneKeyMemberships)
			if err != nil {
				return err
			}
			if enabled {
				*memberships, err = p.domain.ClearCallOutcomeDynamicIndexValueKeyMembershipFactors(*memberships, target)
				if err != nil {
					return err
				}
			}
		}

		mutatesDynamicIndex := normalReturnPathMatchesAny(fact.Path, dynamicTables)
		if callOutcomeConcreteRootInvalidation(fact.Path) && !mutatesDynamicIndex {
			if exact {
				if err := p.decodePathReplacement(frame, state.PathReplacementConfig{
					Keys: p.keys, Target: target, Value: product.Top(),
				}); err != nil {
					return err
				}
			}
			continue
		}

		preserveStructural := fact.PreserveStructuralWitness || boundaryRootBoundToDescendant(fact.Path, targetPath)
		clearStructural := !preserveStructural && !mutatesDynamicIndex
		clearTarget := fact.ClearTarget || (!fact.PreserveStructuralWitness && preserveStructural)
		if exact {
			if err := p.decodePathInvalidationEffect(target, preserveStructural, frame); err != nil {
				return err
			}
		}
		targetPathKey, addressable := resolve.pathKey(fact.Path)
		if !addressable {
			continue
		}
		if err := p.decodePathDescendantMutation(targetPathKey, frame); err != nil {
			return err
		}
		if len(targetPath.Segments) != 0 && (clearTarget || clearStructural) {
			if err := p.decodePathSubtreeMutation(targetPathKey, frame); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p NormalReturnFactorCodec[K]) decodePathInvalidationEffect(
	target keyspace.Key,
	preserveStructural bool,
	frame *NormalReturnFactorFrame[K],
) error {
	factor, enabled, err := p.factor(frame, state.LaneEffectDeltas)
	if err != nil || !enabled {
		return err
	}
	site := callboundary.PathInvalidationEffectSite()
	if preserveStructural {
		site = callboundary.PathStructuralPreservingInvalidationEffectSite()
	}
	plan, err := p.domain.PrepareEffectDeltaFactorPlan(effectdelta.Key{
		Target: target, Site: site, Kind: effectdelta.Mutation,
	}, effectdelta.Top())
	if err != nil {
		return err
	}
	*factor, err = p.domain.ApplyEffectDeltaFactor(plan, *factor)
	return err
}

func (p NormalReturnFactorCodec[K]) pathValueCoordinates(
	frame *NormalReturnFactorFrame[K],
) (state.CoordinateFamily, state.CoordinateFamilySkeleton, []state.CoordinateScalarFactor, bool, error) {
	family, present := p.domain.PathValueFamily()
	if !present {
		return state.CoordinateFamily{}, state.CoordinateFamilySkeleton{}, nil, false, nil
	}
	factor, enabled, err := p.factor(frame, family.Lane().ID())
	if err != nil || !enabled {
		return state.CoordinateFamily{}, state.CoordinateFamilySkeleton{}, nil, false, err
	}
	skeleton, scalars, err := p.domain.DecomposeCoordinateFamily(*factor, family, p.keys)
	if err != nil {
		return state.CoordinateFamily{}, state.CoordinateFamilySkeleton{}, nil, false, err
	}
	return family, skeleton, scalars, true, nil
}

func (p NormalReturnFactorCodec[K]) decodePathDescendantMutation(
	target pathdom.PathKey,
	frame *NormalReturnFactorFrame[K],
) error {
	_, skeleton, scalars, enabled, err := p.pathValueCoordinates(frame)
	if err != nil || !enabled {
		return err
	}
	mutation, present, err := p.domain.PrepareCoordinatePathDescendantMutationIfPresent(skeleton, scalars, target)
	if err != nil {
		return err
	}
	if !present {
		return nil
	}
	for _, lane := range p.domain.PathDescendantMutationParticipantLanes() {
		factor, laneEnabled, laneErr := p.factor(frame, lane.ID())
		if laneErr != nil {
			return laneErr
		}
		if !laneEnabled {
			continue
		}
		*factor, laneErr = p.domain.ApplyPathDescendantMutationLane(mutation, *factor)
		if laneErr != nil {
			return laneErr
		}
	}
	return nil
}

func (p NormalReturnFactorCodec[K]) decodePathSubtreeMutation(
	target pathdom.PathKey,
	frame *NormalReturnFactorFrame[K],
) error {
	_, skeleton, scalars, enabled, err := p.pathValueCoordinates(frame)
	if err != nil || !enabled {
		return err
	}
	mutation, present, err := p.domain.PrepareCoordinatePathSubtreeMutationIfPresent(skeleton, scalars, target)
	if err != nil {
		return err
	}
	if !present {
		return nil
	}
	topology, err := p.domain.SealPathSubtreeMutationFactorTopology()
	if err != nil {
		return err
	}
	lanes := topology.Lanes()
	for _, family := range topology.Families() {
		found := false
		for _, lane := range lanes {
			found = found || lane == family.Lane()
		}
		if !found {
			lanes = append(lanes, family.Lane())
		}
	}
	factors := make([]state.LaneFactor, len(lanes))
	factorEnabled := make([]bool, len(lanes))
	for index, lane := range lanes {
		factor, laneEnabled, laneErr := p.factor(frame, lane.ID())
		if laneErr != nil {
			return laneErr
		}
		factorEnabled[index] = laneEnabled
		if laneEnabled {
			factors[index] = *factor
			continue
		}
		factors[index], laneErr = p.domain.LaneBottom(lane)
		if laneErr != nil {
			return laneErr
		}
	}
	factors, err = applyPathSubtreeMutationFactorLanes(p.domain, skeleton.KeySpace(), mutation, factors)
	if err != nil {
		return err
	}
	for index, lane := range lanes {
		if !factorEnabled[index] {
			continue
		}
		factor, _, laneErr := p.factor(frame, lane.ID())
		if laneErr != nil {
			return laneErr
		}
		*factor = factors[index]
	}
	return nil
}

type normalReturnFactorValueReader[K comparable] struct {
	values state.ValueFactor[K]
	roots  map[statekey.ValueDependency]K
}

func (r normalReturnFactorValueReader[K]) ReadPathReplacementValue(dependency statekey.ValueDependency) (product.Value, bool) {
	if r.values.Top {
		return product.Value{}, false
	}
	root, bound := r.roots[dependency]
	if !bound {
		return product.Value{}, false
	}
	value, present := r.values.Values[root]
	return value, present
}

func (p NormalReturnFactorCodec[K]) decodePathReplacement(frame *NormalReturnFactorFrame[K], config state.PathReplacementConfig) error {
	readLanes := p.domain.PathReplacementReadLanes()
	reads := make([]state.LaneFactor, len(readLanes))
	for index, lane := range readLanes {
		factor, enabled, err := p.factor(frame, lane.ID())
		if err != nil {
			return err
		}
		if !enabled {
			return fmt.Errorf("factapply: enabled path-replacement read lane %q disappeared", lane.ID())
		}
		reads[index] = *factor
	}
	transaction, err := p.domain.PreparePathReplacement(
		config,
		normalReturnFactorValueReader[K]{values: frame.Values, roots: p.valueRoots},
		reads,
	)
	if err != nil {
		return err
	}
	nextValues, err := state.ApplyPathReplacementValues(
		p.domain, transaction, frame.Values,
		func(dependency statekey.ValueDependency) (K, bool) {
			root, ok := p.valueRoots[dependency]
			return root, ok
		},
	)
	if err != nil {
		return err
	}
	type stagedFactor struct {
		index int
		value state.LaneFactor
	}
	writes := p.domain.PathReplacementWriteLanes()
	staged := make([]stagedFactor, 0, len(writes))
	for _, lane := range writes {
		current, enabled, factorErr := p.factor(frame, lane.ID())
		if factorErr != nil {
			return factorErr
		}
		if !enabled {
			continue
		}
		next, factorErr := p.domain.ApplyPathReplacementFactor(transaction, *current, *current)
		if factorErr != nil {
			return factorErr
		}
		staged = append(staged, stagedFactor{index: p.ordinals[lane], value: next})
	}
	frame.Values = nextValues
	for _, factor := range staged {
		frame.Factors[factor.index] = factor.value
	}
	return nil
}

func (p NormalReturnFactorCodec[K]) decodePathRefinements(ctx context.Context, token *cancellation.Token, resolve normalReturnApplyContext, frame *NormalReturnFactorFrame[K]) error {
	transaction := CallResultTransaction{point: p.point, steps: make([]CallResultStep, 0, len(resolve.normalFacts.PathRefinements))}
	for _, fact := range resolve.normalFacts.PathRefinements {
		target, ok := p.paths.Substitute(fact.Path)
		if !ok {
			continue
		}
		transaction.steps = append(transaction.steps, CallResultStep{kind: CallResultStepPostconditionRefinement, refinement: factflow.NewPostconditionRefinement(target, factflow.NewValueConstraint(fact.Value))})
	}
	if len(transaction.steps) == 0 {
		return nil
	}
	program, err := PrepareCallResultPostconditionFactorProgram(
		p.authority, p.domain, transaction, p.inventory,
		func(dependency statekey.ValueDependency) (K, bool) {
			root, ok := p.valueRoots[dependency]
			return root, ok
		},
		p.authority.typeValues, p.authority.projectPath,
	)
	if err != nil {
		return err
	}
	factors := make([]state.LaneFactor, len(program.lanes))
	indices := make([]int, len(program.lanes))
	for index, lane := range program.lanes {
		global, present := p.ordinals[lane]
		if !present {
			return fmt.Errorf("path-refinement lane %q is outside frame ownership", lane.ID())
		}
		factors[index], indices[index] = frame.Factors[global], global
	}
	next, err := program.Apply(ctx, token, CallResultPostconditionFactorFrame[K]{Values: frame.Values, Factors: factors, Reachable: frame.Reachable})
	if err != nil {
		return err
	}
	frame.Values, frame.Reachable = next.Values, next.Reachable
	for index, global := range indices {
		frame.Factors[global] = next.Factors[index]
	}
	return nil
}

func (p NormalReturnFactorCodec[K]) decodeKeyMemberships(resolve normalReturnApplyContext, id callboundary.NormalReturnFactLaneID, frame *NormalReturnFactorFrame[K]) error {
	factor, enabled, err := p.factor(frame, state.LaneKeyMemberships)
	if err != nil {
		return err
	}
	if !enabled {
		return nil
	}
	var facts []state.KeyMembership
	switch id {
	case callboundary.LaneKeyMemberships:
		facts = make([]state.KeyMembership, 0, len(resolve.normalFacts.KeyMemberships))
		for _, fact := range resolve.normalFacts.KeyMemberships {
			key, keyOK := resolve.visibleStateKey(fact.Key)
			table, tableOK := resolve.visibleStateKey(fact.Table)
			if keyOK && tableOK {
				facts = append(facts, state.PathKeyMembership(key, table))
			}
		}
	case callboundary.LaneDynamicValueKeys:
		facts = make([]state.KeyMembership, 0, len(resolve.normalFacts.DynamicValueKeys))
		for _, fact := range resolve.normalFacts.DynamicValueKeys {
			container, containerOK := resolve.keyspaceKey(fact.Container)
			table, tableOK := resolve.visibleStateKey(fact.Table)
			if containerOK && tableOK {
				facts = append(facts, state.DynamicIndexValueKeyMembership(container, fact.Site, table))
			}
		}
	case callboundary.LaneDynamicAllValues:
		facts = make([]state.KeyMembership, 0, len(resolve.normalFacts.DynamicAllValues))
		for _, fact := range resolve.normalFacts.DynamicAllValues {
			container, containerOK := resolve.keyspaceKey(fact.Container)
			table, tableOK := resolve.visibleStateKey(fact.Table)
			if containerOK && tableOK {
				facts = append(facts, state.DynamicIndexAllValuesKeyMembership(container, table))
			}
		}
	}
	*factor, err = p.domain.ApplyCallOutcomeKeyMembershipFactors(*factor, facts)
	return err
}

func (p NormalReturnFactorCodec[K]) decodeHeapStaticMember(
	resolve normalReturnApplyContext,
	boundaryPath pathdom.Path,
	value product.Value,
	frame *NormalReturnFactorFrame[K],
) error {
	targetPath, ok := resolve.substitute(boundaryPath)
	if !ok || len(targetPath.Segments) == 0 {
		return nil
	}
	heapFactor, heapEnabled, err := p.factor(frame, state.LaneHeapTableIdentity)
	if err != nil || !heapEnabled {
		return err
	}
	ownerPath := targetPath.Clone()
	suffix := ownerPath.Segments[len(ownerPath.Segments)-1:]
	ownerPath.Segments = ownerPath.Segments[:len(ownerPath.Segments)-1]
	owner, ok := visibility.AddressAt(p.authority.resolver, p.point, ownerPath).RootOrVisibleKeyspaceKey()
	if !ok {
		return nil
	}
	ownerValue, present, err := p.resolveFactorPathValue(owner, frame)
	if err != nil || !present {
		return err
	}
	id, exact := product.Get(p.domain.Registry(), ownerValue, identity.Key).ID()
	if !exact {
		return nil
	}
	object, err := p.domain.ReadHeapTableObjectTermFactor(*heapFactor, identity.ConcreteTerm(id))
	if err != nil {
		return err
	}
	if heapidentity.ObjectDomain(p.domain.Registry()).Equal(object, heapidentity.BottomObject(p.domain.Registry())) {
		return nil
	}
	object, ok = object.WithStaticMember(p.domain.Registry(), p.keys, suffix, value)
	if !ok {
		return nil
	}
	plan, err := p.domain.PrepareObjectGraphReplacePlan(p.keys, []state.ObjectGraphMutation{{Identity: identity.ConcreteTerm(id), Object: object}})
	if err != nil {
		return err
	}
	lanes, err := p.domain.ObjectGraphMutationLanes(plan)
	if err != nil {
		return err
	}
	for _, lane := range lanes {
		factor, enabled, factorErr := p.factor(frame, lane.ID())
		if factorErr != nil {
			return factorErr
		}
		if !enabled {
			continue
		}
		*factor, factorErr = p.domain.ApplyObjectGraphMutationFactor(plan, *factor)
		if factorErr != nil {
			return factorErr
		}
	}
	return nil
}

func (p NormalReturnFactorCodec[K]) decodeRelConstraints(resolve normalReturnApplyContext, frame *NormalReturnFactorFrame[K]) error {
	for _, fact := range resolve.normalFacts.RelConstraints {
		a, ok := resolve.relationGraphKey(fact.A)
		if !ok {
			continue
		}
		c, ok := resolve.relationGraphKey(fact.C)
		if !ok {
			continue
		}
		var b state.RelOperand
		coB := fact.CoB
		if coB != 0 && !fact.B.Path.IsEmpty() {
			b, ok = resolve.relationGraphKey(fact.B)
			if !ok {
				continue
			}
		} else {
			coB = 0
		}
		mutation, err := p.domain.PrepareCoordinateBranchConstraint(p.keys, state.RelConstraint{CoA: fact.CoA, A: a, CoB: coB, B: b, C: c, K: fact.K})
		if err != nil {
			return err
		}
		lane, err := p.domain.CoordinateBranchMutationLane(mutation)
		if err != nil {
			return err
		}
		factor, enabled, err := p.factor(frame, lane.ID())
		if err != nil {
			return err
		}
		if !enabled {
			continue
		}
		*factor, err = p.domain.ApplyCoordinateBranchMutationFactor(mutation, *factor)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p NormalReturnFactorCodec[K]) decodeLifecycle(resolve normalReturnApplyContext, frame *NormalReturnFactorFrame[K]) error {
	typestateFactor, typestateEnabled, err := p.factor(frame, state.LaneTypestates)
	if err != nil {
		return err
	}
	if !typestateEnabled {
		return nil
	}
	pathFactor, pathEnabled, err := p.factor(frame, state.LanePathEvidence)
	if err != nil {
		return err
	}
	if !pathEnabled {
		return nil
	}
	mutations := make([]state.CallOutcomeLifecycleMutation, 0, len(resolve.normalFacts.LifecycleFacts))
	for _, fact := range resolve.normalFacts.LifecycleFacts {
		target, ok := resolve.visibleStateKey(fact.Target)
		if !ok || fact.Protocol == "" {
			continue
		}
		resource, _, _, err := p.domain.CanonicalTypestateResourceFactor(p.typestates, *typestateFactor, *pathFactor, target, fact.Protocol)
		if err != nil {
			return err
		}
		kind := state.CallOutcomeLifecycleInvalid
		switch fact.Kind {
		case callboundary.LifecycleAcquire:
			kind = state.CallOutcomeLifecycleAcquire
		case callboundary.LifecycleTransition:
			kind = state.CallOutcomeLifecycleTransition
		case callboundary.LifecycleEscape:
			kind = state.CallOutcomeLifecycleEscape
		default:
			return fmt.Errorf("invalid lifecycle kind")
		}
		mutations = append(mutations, state.CallOutcomeLifecycleMutation{Resource: resource, Kind: kind, From: fact.From, To: fact.To, Obligation: fact.Obligation, Site: uint32(p.point)})
	}
	*typestateFactor, err = p.domain.ApplyCallOutcomeLifecycleFactors(*typestateFactor, mutations)
	return err
}

func normalReturnFactorCanceled(ctx context.Context, token *cancellation.Token) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if token != nil {
		return token.Err()
	}
	return nil
}
