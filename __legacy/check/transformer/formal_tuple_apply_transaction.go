package transformer

import (
	"fmt"

	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

// formalApplyLeafPublication is one complete caller-owned tuple alternative.
// Leaves cover the full descriptor span, including Care and non-product
// syntax.  No partial group patch is representable at this publication seam.
type formalApplyLeafPublication struct {
	guard        decisionRef
	leaves       []decisionLeaf
	normalReturn *formalApplyNormalReturnSource
}

// formalApplyObservation is the exact observation carrier produced alongside
// one canonical Apply transfer. It retains the already-correlated region and
// the already-computed boundary publication; publication may encode a DTO from
// this witness, but may not execute Apply again.
type formalApplyObservation struct {
	step    *formalApplyStep
	regions []formalApplyObservedRegion
}

type formalApplyObservedRegion struct {
	region      formalApplyCorrelatedRegion
	publication formalApplyLeafPublication
}

// formalApplyObservationWitness binds an observation to the exact equation
// inputs that produced it. The post-WTO detach fence accepts it only when all
// of these tuples are the stabilized solution tuples for their named cells.
type formalApplyObservationWitness struct {
	predecessorCell  formalRelationCell
	predecessorValue formalRelationTuple
	outcomeCells     []formalRelationCell
	outcomeValues    []formalRelationTuple
	observation      formalApplyObservation
}

// formalApplyNormalReturnSource is the exact target-owned factor image from
// which the canonical normal-return encoder publishes one lexical CallOutcome
// alternative. It is captured after the one Apply identity substitution and
// before outbound transport while that transaction owns the selected image.
// Only the final input-certified observation witness survives WTO; no later
// publisher may rediscover roots, scan inventories, or reconstruct State.
type formalApplyNormalReturnSource struct {
	selection state.BoundaryFactorSelection
	factors   []state.LaneFactor
	roots     []formalApplyNormalReturnRoot
}

// formalApplyNormalReturnRoot pairs one immutable structural boundary role
// with its exact scalar. Slot is a formal alias witness; boundarySlot is its
// sealed caller-visible symbol/return identity. Selection owns the formal
// path/ordinal schema used by the encoder.
type formalApplyNormalReturnRoot struct {
	slot         FormalSlot
	boundarySlot statekey.Value
	value        product.Value
}

// factorFormalApplyProductLeaf replaces every registered product group in one
// caller leaf after the complete transport transaction has succeeded.  The
// non-product prefix (Care, Middle syntax and Outcome occurrences) is retained
// from base and may be updated by the result-lens layer before publication.
func (a *formalTupleAlgebra) factorFormalApplyProductLeaf(
	base formalTupleLeafEvaluator,
	values state.ValueFactor[FormalSlot],
	factors []state.LaneFactor,
) ([]decisionLeaf, error) {
	if a == nil || !base.valid() || base.algebra != a || !base.authority.product.Valid() {
		return nil, errFormalComponentForeignOwner
	}
	span := base.span
	layout := base.layout
	if len(factors) != len(layout.nonValues) {
		return nil, fmt.Errorf("transformer: formal Apply product tuple has %d factors, want %d", len(factors), len(layout.nonValues))
	}
	complete, err := base.leaves.complete()
	if err != nil {
		return nil, err
	}
	out := append([]decisionLeaf(nil), complete...)
	if err := a.factorFormalEffectGroup(base.authority, span, layout.values, values, state.LaneFactor{}, out); err != nil {
		return nil, err
	}
	for index, group := range layout.nonValues {
		if err := a.factorFormalEffectGroup(base.authority, span, group, state.ValueFactor[FormalSlot]{}, factors[index], out); err != nil {
			return nil, fmt.Errorf("transformer: formal Apply product lane %q: %w", group.lane.ID(), err)
		}
	}
	return out, nil
}

// publishFormalApplyLeafPublications performs the one caller directory write.
// Every factor and result alternative must already be validated; all decision
// roots are built in scratch first and one sealed full-span delta is applied at
// the end.  Cancellation or any semantic error publishes nothing.
func (a *formalTupleAlgebra) publishFormalApplyLeafPublications(
	predecessor formalRelationTuple,
	publications []formalApplyLeafPublication,
) (formalRelationTuple, error) {
	if err := a.validateTuple(predecessor); err != nil || predecessor.bottom() {
		if err != nil {
			return formalRelationTuple{}, err
		}
		return formalRelationTuple{}, fmt.Errorf("transformer: formal Apply publication predecessor is Bottom")
	}
	span, directory, authority, ok := a.span(predecessor.variable)
	if !ok || predecessor.root.owner != directory || len(publications) == 0 {
		return formalRelationTuple{}, errFormalComponentForeignOwner
	}
	mark := a.decisions.checkpoint()
	fail := func(err error) (formalRelationTuple, error) {
		a.decisions.rollback(mark)
		return formalRelationTuple{}, err
	}
	roots := make([]decisionRef, span.count)
	for _, publication := range publications {
		if publication.guard == decisionFalse {
			continue
		}
		if len(publication.leaves) != span.count {
			return fail(errFormalComponentMalformed)
		}
		if _, present := a.decisions.node(publication.guard); !present {
			return fail(errDecisionMalformed)
		}
		for ordinal, leaf := range publication.leaves {
			descriptor := span.forest.descriptors[span.first+ordinal]
			if err := a.validateDescriptorLeaf(authority, descriptor, leaf); err != nil {
				return fail(err)
			}
			var err error
			roots[ordinal], err = a.decisions.condition(a.ctx, publication.guard, a.decisions.terminal(leaf), roots[ordinal])
			if err != nil {
				return fail(err)
			}
		}
	}
	writes := make([]formalFiberWrite, span.count)
	for ordinal, root := range roots {
		descriptor := span.forest.descriptors[span.first+ordinal]
		if err := a.validateDescriptorRoot(authority, descriptor, root); err != nil {
			return fail(err)
		}
		writes[ordinal] = formalFiberWrite{ordinal: formalFiberOrdinal(ordinal), value: formalFiberValue(root)}
	}
	delta, err := directory.sealDelta(writes)
	if err != nil {
		return fail(err)
	}
	root, _, err := directory.applyDelta(predecessor.root, delta)
	if err != nil {
		return fail(err)
	}
	result := a.normalize(formalRelationTuple{variable: predecessor.variable, root: root})
	if err := a.validateTuple(result); err != nil {
		return fail(err)
	}
	return result, nil
}
