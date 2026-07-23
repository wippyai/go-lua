package transformer

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

// formalReturnIdentityObservationFiber is one registration-owned B/C input.
// Each root is inspected independently; no product row is formed between
// unrelated heap skeletons or scalar edges.
type formalReturnIdentityObservationFiber struct {
	ordinal  formalFiberOrdinal
	family   state.CoordinateFamily
	skeleton bool
}

type formalReturnIdentityPublisherFiber struct {
	ordinal formalFiberOrdinal
	slot    state.CoordinateSlot
}

type formalReturnIdentityPublisherFamily struct {
	family  state.CoordinateFamily
	scalars []formalReturnIdentityPublisherFiber
}

// formalReturnIdentityStep is the exact registered B/C/D physical vocabulary.
// ProductDomain roles are the only selection authority: adding another axis
// changes this plan through registration, never through an evaluator switch.
type formalReturnIdentityStep struct {
	variable   relationVar
	observers  []formalReturnIdentityObservationFiber
	publishers []formalReturnIdentityPublisherFamily
	sealed     bool
}

func freezeFormalReturnIdentityStep(
	domain state.ProductDomain,
	span formalFiberDescriptorSpan,
) (formalReturnIdentityStep, error) {
	if !domain.Valid() || span.variable == 0 || span.keys == nil || !span.keys.Valid() {
		return formalReturnIdentityStep{}, fmt.Errorf("transformer: formal return-identity step is unowned")
	}
	step := formalReturnIdentityStep{variable: span.variable, sealed: true}
	for _, group := range span.groupDescriptors() {
		if group.kind != formalFiberGroupCoordinateLane {
			continue
		}
		for _, family := range group.coordinateFamilies {
			roles, err := domain.CoordinateReturnIdentityRoles(family.family)
			if err != nil {
				return formalReturnIdentityStep{}, err
			}
			if roles.Has(state.CoordinateReturnIdentitySeed) || roles.Has(state.CoordinateReturnIdentitySkeletonEdge) {
				step.observers = append(step.observers, formalReturnIdentityObservationFiber{
					ordinal: family.skeleton, family: family.family, skeleton: true,
				})
			}
			if roles.Has(state.CoordinateReturnIdentityScalarEdge) {
				for _, ordinal := range family.scalars {
					step.observers = append(step.observers, formalReturnIdentityObservationFiber{
						ordinal: ordinal, family: family.family,
					})
				}
			}
			if roles.Has(state.CoordinateReturnIdentityPublisher) {
				publisher := formalReturnIdentityPublisherFamily{family: family.family}
				for _, ordinal := range family.scalars {
					if int(ordinal) < 0 || int(ordinal) >= span.count {
						return formalReturnIdentityStep{}, errFormalComponentMalformed
					}
					descriptor := span.forest.descriptors[span.first+int(ordinal)]
					if descriptor.role != formalFiberCoordinate || descriptor.coordinateKind != formalFiberCoordinateFamilyScalar ||
						!coordinateFamilySame(descriptor.family, family.family) {
						return formalReturnIdentityStep{}, errFormalComponentMalformed
					}
					publisher.scalars = append(publisher.scalars, formalReturnIdentityPublisherFiber{
						ordinal: ordinal, slot: descriptor.coordinate,
					})
				}
				// Coordinate ordinals are a physical inventory order, not a
				// semantic key-order promise. Seal the publisher lookup order
				// explicitly through the registered family comparator.
				for index := 1; index < len(publisher.scalars); index++ {
					for current := index; current > 0; current-- {
						less, lessErr := domain.CoordinateSlotLess(publisher.scalars[current].slot, publisher.scalars[current-1].slot)
						if lessErr != nil {
							return formalReturnIdentityStep{}, lessErr
						}
						if !less {
							break
						}
						publisher.scalars[current], publisher.scalars[current-1] = publisher.scalars[current-1], publisher.scalars[current]
					}
				}
				step.publishers = append(step.publishers, publisher)
			}
		}
	}
	sort.Slice(step.observers, func(i, j int) bool { return step.observers[i].ordinal < step.observers[j].ordinal })
	for index := 1; index < len(step.observers); index++ {
		if step.observers[index-1].ordinal == step.observers[index].ordinal {
			return formalReturnIdentityStep{}, fmt.Errorf("transformer: return-identity observer fiber has duplicate roles")
		}
	}
	return step, nil
}

func (p formalReturnIdentityStep) validFor(span formalFiberDescriptorSpan) bool {
	return p.sealed && p.variable == span.variable
}

func (a *formalTupleAlgebra) formalDecisionLeafRegions(
	care, root decisionRef,
	visit func(decisionLeaf, decisionRef) error,
) error {
	if a == nil || visit == nil || care == decisionFalse || int(root) >= len(a.decisions.nodes) {
		return errDecisionMalformed
	}
	regions, err := a.decisions.partitionLeafTuplesUnderCare(a.ctx, care, []decisionRef{root})
	if err != nil {
		return err
	}
	for _, region := range regions {
		if len(region.leaves) != 1 || region.care == decisionFalse {
			return errDecisionMalformed
		}
		if err := visit(region.leaves[0], region.care); err != nil {
			return err
		}
	}
	return nil
}

func (a *formalTupleAlgebra) collectFormalReturnIdentityObservations(
	tuple formalRelationTuple,
	plan formalReturnIdentityStep,
) ([]factapply.ReturnIdentityCondition[decisionRef], []factapply.ReturnIdentityEdgeCondition[decisionRef], error) {
	span, directory, authority, ok := a.span(tuple.variable)
	if !ok || tuple.root.owner != directory || !plan.validFor(span) {
		return nil, nil, errFormalComponentForeignOwner
	}
	care, err := a.care(tuple)
	if err != nil || care == decisionFalse {
		return nil, nil, err
	}
	admissions := make([]factapply.ReturnIdentityCondition[decisionRef], 0)
	edges := make([]factapply.ReturnIdentityEdgeCondition[decisionRef], 0)
	for _, observer := range plan.observers {
		root, readErr := directory.valueAt(tuple.root, observer.ordinal)
		if readErr != nil {
			return nil, nil, readErr
		}
		err = a.formalDecisionLeafRegions(care, decisionRef(root), func(leaf decisionLeaf, guard decisionRef) error {
			if observer.skeleton {
				var skeleton state.CoordinateFamilySkeleton
				var terminalErr error
				if leaf == 0 {
					skeleton, terminalErr = authority.product.CoordinateSkeletonBottom(observer.family, span.keys)
				} else {
					terminal, lookupErr := authority.terminal(leaf)
					if lookupErr != nil || terminal.kind != formalComponentCoordinateSkeleton ||
						!coordinateFamilySame(terminal.skeleton.Family(), observer.family) {
						return errFormalComponentMalformed
					}
					skeleton = terminal.skeleton
				}
				if terminalErr != nil {
					return terminalErr
				}
				return authority.product.VisitCoordinateReturnIdentitySkeletonObservations(skeleton, func(observation state.CoordinateReturnIdentityObservation) bool {
					switch observation.Role() {
					case state.CoordinateReturnIdentitySeed:
						admissions = append(admissions, factapply.ReturnIdentityCondition[decisionRef]{
							Root: observation.Root(), Condition: guard,
						})
					case state.CoordinateReturnIdentitySkeletonEdge:
						edges = append(edges, factapply.ReturnIdentityEdgeCondition[decisionRef]{
							From: observation.Root(), To: observation.Target(), Condition: guard,
						})
					}
					return true
				})
			}
			// Zero is the canonical omitted scalar. Registered scalar-edge
			// families may only observe explicit scalars; their omitted value
			// therefore contributes no edge.
			if leaf == 0 {
				return nil
			}
			terminal, terminalErr := authority.terminal(leaf)
			if terminalErr != nil || terminal.kind != formalComponentCoordinateScalar ||
				!coordinateFamilySame(terminal.scalar.Slot().Family(), observer.family) {
				return errFormalComponentMalformed
			}
			return authority.product.VisitCoordinateReturnIdentityScalarObservations(terminal.scalar, func(observation state.CoordinateReturnIdentityObservation) bool {
				if observation.Role() == state.CoordinateReturnIdentityScalarEdge {
					edges = append(edges, factapply.ReturnIdentityEdgeCondition[decisionRef]{
						From: observation.Root(), To: observation.Target(), Condition: guard,
					})
				}
				return true
			})
		})
		if err != nil {
			return nil, nil, err
		}
	}
	return admissions, edges, nil
}

func (a *formalTupleAlgebra) applyFormalReturnIdentityClosure(
	tuple formalRelationTuple,
	plan formalReturnIdentityStep,
	sources []factapply.ReturnIdentityCondition[decisionRef],
) (formalRelationTuple, error) {
	admissions, edges, err := a.collectFormalReturnIdentityObservations(tuple, plan)
	if err != nil {
		return formalRelationTuple{}, err
	}
	closed, err := factapply.CloseReturnIdentities(a.ctx, factapply.ReturnBooleanAlgebra[decisionRef]{
		False: decisionFalse,
		And: func(left, right decisionRef) (decisionRef, error) {
			return a.decisions.apply(a.ctx, uint8(decisionAnd), true, left, right, decisionLeafAnd)
		},
		Or: func(left, right decisionRef) (decisionRef, error) {
			return a.decisions.apply(a.ctx, uint8(decisionOr), true, left, right, decisionLeafOr)
		},
		Not:   func(value decisionRef) (decisionRef, error) { return formalDecisionBooleanNot(a, value) },
		Equal: func(left, right decisionRef) bool { return left == right },
	}, sources, admissions, edges)
	if err != nil {
		return formalRelationTuple{}, err
	}
	return a.publishFormalReturnIdentities(tuple, plan, closed)
}

func formalReturnIdentityPublisher(
	domain state.ProductDomain,
	family formalReturnIdentityPublisherFamily,
	slot state.CoordinateSlot,
) (formalReturnIdentityPublisherFiber, bool, error) {
	low, high := 0, len(family.scalars)
	for low < high {
		middle := int(uint(low+high) >> 1)
		less, err := domain.CoordinateSlotLess(family.scalars[middle].slot, slot)
		if err != nil {
			return formalReturnIdentityPublisherFiber{}, false, err
		}
		if less {
			low = middle + 1
		} else {
			high = middle
		}
	}
	index := low
	if index >= len(family.scalars) {
		return formalReturnIdentityPublisherFiber{}, false, nil
	}
	equal, err := domain.CoordinateSlotEqual(family.scalars[index].slot, slot)
	if err != nil || !equal {
		return formalReturnIdentityPublisherFiber{}, false, err
	}
	return family.scalars[index], true, nil
}

func (a *formalTupleAlgebra) publishFormalReturnIdentities(
	tuple formalRelationTuple,
	plan formalReturnIdentityStep,
	reachable []factapply.ReturnIdentityCondition[decisionRef],
) (formalRelationTuple, error) {
	span, directory, authority, ok := a.span(tuple.variable)
	if !ok || tuple.root.owner != directory || !plan.validFor(span) {
		return formalRelationTuple{}, errFormalComponentForeignOwner
	}
	type guardedPublisher struct {
		fiber formalReturnIdentityPublisherFiber
		guard decisionRef
	}
	updates := make([]guardedPublisher, 0, len(reachable))
	for _, item := range reachable {
		if !item.Root.Valid() || item.Condition == decisionFalse {
			return formalRelationTuple{}, errFormalComponentMalformed
		}
		for _, family := range plan.publishers {
			slot, handled, err := authority.product.CoordinateReturnIdentityTermSlot(family.family, span.keys, item.Root)
			if err != nil {
				return formalRelationTuple{}, err
			}
			if !handled {
				continue
			}
			fiber, found, err := formalReturnIdentityPublisher(authority.product, family, slot)
			if err != nil {
				return formalRelationTuple{}, err
			}
			if !found {
				return formalRelationTuple{}, fmt.Errorf("transformer: return-identity publisher escaped frozen inventory")
			}
			updates = append(updates, guardedPublisher{fiber: fiber, guard: item.Condition})
		}
	}
	sort.Slice(updates, func(i, j int) bool { return updates[i].fiber.ordinal < updates[j].fiber.ordinal })
	// Several exact terms may lawfully project to one publisher coordinate.
	// The physical scalar is updated once under the union of their reach guards.
	write := 0
	for _, update := range updates {
		if write == 0 || updates[write-1].fiber.ordinal != update.fiber.ordinal {
			updates[write] = update
			write++
			continue
		}
		merged, err := a.decisions.apply(a.ctx, uint8(decisionOr), true, updates[write-1].guard, update.guard, decisionLeafOr)
		if err != nil {
			return formalRelationTuple{}, err
		}
		updates[write-1].guard = merged
	}
	updates = updates[:write]
	writes := make([]formalFiberWrite, 0, len(updates))
	for _, update := range updates {
		priorValue, err := directory.valueAt(tuple.root, update.fiber.ordinal)
		if err != nil {
			return formalRelationTuple{}, err
		}
		prior := decisionRef(priorValue)
		published := prior
		err = a.formalDecisionLeafRegions(update.guard, prior, func(leaf decisionLeaf, guard decisionRef) error {
			var scalar state.CoordinateScalarFactor
			var scalarErr error
			if leaf == 0 {
				var exact bool
				scalar, exact, scalarErr = authority.product.CoordinateReachableDefault(update.fiber.slot)
				if scalarErr == nil && !exact {
					return fmt.Errorf("transformer: return-identity publisher has no root-local default")
				}
			} else {
				terminal, terminalErr := authority.terminal(leaf)
				if terminalErr != nil || terminal.kind != formalComponentCoordinateScalar {
					return errFormalComponentMalformed
				}
				scalar = terminal.scalar
				equal, equalErr := authority.product.CoordinateSlotEqual(scalar.Slot(), update.fiber.slot)
				if equalErr != nil || !equal {
					return errFormalComponentMalformed
				}
			}
			if scalarErr != nil {
				return scalarErr
			}
			scalar, scalarErr = authority.product.PublishCoordinateReturnIdentity(scalar)
			if scalarErr != nil {
				return scalarErr
			}
			publishedLeaf, scalarErr := authority.internCoordinateScalar(scalar)
			if scalarErr != nil {
				return scalarErr
			}
			published, scalarErr = a.decisions.condition(a.ctx, guard, a.decisions.terminal(publishedLeaf), published)
			return scalarErr
		})
		if err != nil {
			return formalRelationTuple{}, err
		}
		if err := a.validateDescriptorRoot(authority, span.forest.descriptors[span.first+int(update.fiber.ordinal)], published); err != nil {
			return formalRelationTuple{}, err
		}
		if published != prior {
			writes = append(writes, formalFiberWrite{ordinal: update.fiber.ordinal, value: formalFiberValue(published)})
		}
	}
	if len(writes) == 0 {
		return tuple, nil
	}
	delta, err := directory.sealDelta(writes)
	if err != nil {
		return formalRelationTuple{}, err
	}
	root, _, err := directory.applyDelta(tuple.root, delta)
	if err != nil {
		return formalRelationTuple{}, err
	}
	return a.normalize(formalRelationTuple{variable: tuple.variable, root: root}), nil
}

func formalReturnIdentitySource(root identity.Term, guard decisionRef) factapply.ReturnIdentityCondition[decisionRef] {
	return factapply.ReturnIdentityCondition[decisionRef]{Root: root, Condition: guard}
}
