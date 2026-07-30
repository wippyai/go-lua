package transformer

import (
	"fmt"
	"sort"
)

// formalClosedFactorRole is one immutable provider projection in a correlated
// factor transaction. The base role additionally carries every declared write
// coordinate, including write-only coordinates that are excluded from reads.
type formalClosedFactorRole struct {
	reads     []formalFiberOrdinal
	positions formalOrdinalPositions
}

// formalClosedFactorLift is the carrier-only contract shared by closed
// factor programs. Semantic reads and writes remain owned by the program that
// produced these physical ordinals; this plan only correlates their DD roots
// and patches declared writes into one base tuple.
type formalClosedFactorLift struct {
	roles          []formalClosedFactorRole
	baseProjection []formalFiberOrdinal
	basePositions  formalOrdinalPositions
	writes         []formalFiberOrdinal
	writePositions formalOrdinalPositions
	variable       relationVar
	sealed         bool
}

type formalClosedFactorLeafWrite struct {
	ordinal formalFiberOrdinal
	leaf    decisionLeaf
}

func sealFormalClosedFactorLift(
	span formalFiberDescriptorSpan,
	reads [][]formalFiberOrdinal,
	writes []formalFiberOrdinal,
) (formalClosedFactorLift, error) {
	if span.variable == 0 || len(reads) == 0 || len(writes) == 0 {
		return formalClosedFactorLift{}, fmt.Errorf("transformer: closed factor lift is unowned")
	}
	seal := func(input []formalFiberOrdinal) ([]formalFiberOrdinal, formalOrdinalPositions, error) {
		out := append([]formalFiberOrdinal(nil), input...)
		sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
		for index, ordinal := range out {
			if ordinal < 0 || int(ordinal) >= span.count || index != 0 && out[index-1] == ordinal {
				return nil, formalOrdinalPositions{}, errFormalComponentMalformed
			}
		}
		positions, err := sealFormalOrdinalPositions(span.count, out)
		return out, positions, err
	}
	roles := make([]formalClosedFactorRole, len(reads))
	for index := range reads {
		var err error
		roles[index].reads, roles[index].positions, err = seal(reads[index])
		if err != nil {
			return formalClosedFactorLift{}, err
		}
	}
	sealedWrites, writePositions, err := seal(writes)
	if err != nil {
		return formalClosedFactorLift{}, err
	}
	projection := append(append([]formalFiberOrdinal(nil), roles[0].reads...), sealedWrites...)
	sort.Slice(projection, func(i, j int) bool { return projection[i] < projection[j] })
	compact := 0
	for _, ordinal := range projection {
		if compact == 0 || projection[compact-1] != ordinal {
			projection[compact] = ordinal
			compact++
		}
	}
	projection = projection[:compact]
	basePositions, err := sealFormalOrdinalPositions(span.count, projection)
	if err != nil {
		return formalClosedFactorLift{}, err
	}
	return formalClosedFactorLift{
		roles: roles, baseProjection: projection, basePositions: basePositions,
		writes: sealedWrites, writePositions: writePositions, variable: span.variable, sealed: true,
	}, nil
}

func (p formalClosedFactorLift) validFor(span formalFiberDescriptorSpan, providers []formalRelationTuple) bool {
	if !p.sealed || p.variable != span.variable || len(p.roles) == 0 || len(providers) != len(p.roles) ||
		len(p.baseProjection) == 0 || len(p.writes) == 0 || !p.basePositions.validFor(span.count, p.baseProjection) ||
		!p.writePositions.validFor(span.count, p.writes) {
		return false
	}
	for index, role := range p.roles {
		if providers[index].variable != p.variable || !role.positions.validFor(span.count, role.reads) {
			return false
		}
	}
	return true
}

// applyFormalClosedFactorLift evaluates one persistent DD vector for every
// provider role and commits only declared writes to the base tuple. Callers own
// the surrounding decision checkpoint so follow-on publication remains atomic.
func (a *formalTupleAlgebra) applyFormalClosedFactorLift(
	plan formalClosedFactorLift,
	providers []formalRelationTuple,
	demands []formalQualifiedGuardDemand,
	execute decisionRef,
	apply func(decisionRef, []formalSparseLeafView) ([]formalClosedFactorLeafWrite, error),
) (formalRelationTuple, error) {
	return a.applyFormalClosedFactorLiftWithDerived(plan, providers, demands, nil, execute, apply)
}

// applyFormalClosedFactorLiftWithDerived retains the generic closed-factor
// transaction while admitting already-lifted semantic operands. Derived roots
// are correlated as read-only leaves and can never enter the publication set.
func (a *formalTupleAlgebra) applyFormalClosedFactorLiftWithDerived(
	plan formalClosedFactorLift,
	providers []formalRelationTuple,
	demands []formalQualifiedGuardDemand,
	derived []decisionRef,
	execute decisionRef,
	apply func(decisionRef, []formalSparseLeafView) ([]formalClosedFactorLeafWrite, error),
) (formalRelationTuple, error) {
	if a == nil || apply == nil || int(execute) >= len(a.decisions.nodes) {
		return formalRelationTuple{}, errFormalComponentForeignOwner
	}
	span, directory, authority, ok := a.span(plan.variable)
	if !ok || !plan.validFor(span, providers) {
		return formalRelationTuple{}, errFormalComponentForeignOwner
	}
	jointCare := execute
	for _, provider := range providers {
		if err := a.validateTuple(provider); err != nil || provider.bottom() || provider.root.owner != directory {
			if err != nil {
				return formalRelationTuple{}, err
			}
			return formalRelationTuple{}, errFormalComponentForeignOwner
		}
		care, err := a.care(provider)
		if err != nil {
			return formalRelationTuple{}, err
		}
		jointCare, err = a.decisions.apply(a.ctx, uint8(decisionAnd), true, jointCare, care, decisionLeafAnd)
		if err != nil {
			return formalRelationTuple{}, err
		}
	}
	base := providers[0]
	if jointCare == decisionFalse {
		return base, nil
	}

	roleOffsets := make([]int, len(plan.roles)+1)
	projectionRoots := make([]decisionRef, 0)
	for roleIndex, role := range plan.roles {
		ordinals := role.reads
		if roleIndex == 0 {
			ordinals = plan.baseProjection
		}
		roleOffsets[roleIndex] = len(projectionRoots)
		for _, ordinal := range ordinals {
			if roleIndex == 0 {
				if _, read := role.positions.position(ordinal); !read {
					projectionRoots = append(projectionRoots, decisionFalse)
					continue
				}
			}
			value, err := directory.valueAt(providers[roleIndex].root, ordinal)
			if err != nil {
				return formalRelationTuple{}, err
			}
			projectionRoots = append(projectionRoots, decisionRef(value))
		}
	}
	roleOffsets[len(plan.roles)] = len(projectionRoots)
	demandRoots := make([]decisionRef, len(demands))
	for index, demand := range demands {
		var err error
		demandRoots[index], err = a.decisionForGuard(demand.owner, demand.scope, demand.arena, demand.guard)
		if err != nil {
			return formalRelationTuple{}, err
		}
	}
	for _, root := range derived {
		if int(root) >= len(a.decisions.nodes) {
			return formalRelationTuple{}, errDecisionMalformed
		}
	}
	derivedOffset := len(projectionRoots)
	transactionRoots := append(append([]decisionRef(nil), projectionRoots...), derived...)
	demandOffset := len(transactionRoots)
	transactionRoots = append(transactionRoots, demandRoots...)
	assignmentOffset := len(transactionRoots)
	transactionRoots = append(transactionRoots, make([]decisionRef, len(plan.writes))...)
	transformed, err := a.decisions.applyVectorUnderCare(
		a.ctx, jointCare, jointCare, decisionFalse, transactionRoots, transactionRoots,
		func(input, unreachable []decisionLeaf) ([]decisionLeaf, error) {
			if len(input) != len(transactionRoots) || len(unreachable) != 0 {
				return nil, errDecisionMalformed
			}
			regionGuard := decisionTrue
			var leafErr error
			for index, root := range demandRoots {
				leaf := input[demandOffset+index]
				if leaf > 1 {
					return nil, errDecisionMalformed
				}
				literal := root
				if leaf == 0 {
					literal, leafErr = formalDecisionBooleanNot(a, root)
					if leafErr != nil {
						return nil, leafErr
					}
				}
				regionGuard, leafErr = a.decisions.apply(a.ctx, uint8(decisionAnd), true, regionGuard, literal, decisionLeafAnd)
				if leafErr != nil {
					return nil, leafErr
				}
			}
			views := make([]formalSparseLeafView, len(plan.roles))
			for roleIndex, role := range plan.roles {
				start, end := roleOffsets[roleIndex], roleOffsets[roleIndex+1]
				leaves := input[start:end]
				if roleIndex == 0 && len(role.reads) != len(plan.baseProjection) {
					selected := make([]decisionLeaf, len(role.reads))
					for index, ordinal := range role.reads {
						position, present := plan.basePositions.position(ordinal)
						if !present || start+position >= end {
							return nil, errFormalComponentMalformed
						}
						selected[index] = input[start+position]
					}
					leaves = selected
				}
				views[roleIndex] = formalSparseLeafView{
					algebra: a, variable: plan.variable, span: span, authority: authority,
					body: &a.program.bodies[plan.variable-1], guard: regionGuard,
					ordinals: role.reads, positions: role.positions, leaves: leaves,
					derived: input[derivedOffset:demandOffset],
				}
			}
			// Correlating the role projections has produced exact sparse rows.
			// Publish every complete lane row here, at that producer boundary,
			// before the component's strict factor lookup consumes it.  A role may
			// omit unrelated lanes; those remain absent rather than being invented.
			for _, view := range views {
				if cacheErr := a.cacheFormalSparseFactorSpellings(view); cacheErr != nil {
					return nil, cacheErr
				}
			}
			writes, err := apply(regionGuard, views)
			if err != nil {
				return nil, err
			}
			output := append([]decisionLeaf(nil), input...)
			var previous formalFiberOrdinal
			for index, write := range writes {
				projectionPosition, present := plan.basePositions.position(write.ordinal)
				writePosition, writable := plan.writePositions.position(write.ordinal)
				if index != 0 && previous >= write.ordinal || !present || !writable || projectionPosition >= roleOffsets[1] {
					return nil, errFormalComponentMalformed
				}
				previous = write.ordinal
				output[projectionPosition] = write.leaf
				output[assignmentOffset+writePosition] = decisionLeaf(decisionTrue)
			}
			return output, nil
		},
	)
	if err != nil || len(transformed) != len(transactionRoots) {
		if err == nil {
			err = errDecisionMalformed
		}
		return formalRelationTuple{}, err
	}
	publication := make([]formalFiberWrite, 0, len(plan.writes))
	for writeIndex, ordinal := range plan.writes {
		position, present := plan.basePositions.position(ordinal)
		if !present || position >= len(transformed) {
			return formalRelationTuple{}, errFormalComponentMalformed
		}
		prior, err := directory.valueAt(base.root, ordinal)
		if err != nil {
			return formalRelationTuple{}, err
		}
		root, err := a.decisions.condition(a.ctx, transformed[assignmentOffset+writeIndex], transformed[position], decisionRef(prior))
		if err != nil {
			return formalRelationTuple{}, err
		}
		descriptor := span.forest.descriptors[span.first+int(ordinal)]
		if err := a.validateDescriptorRoot(authority, descriptor, root); err != nil {
			return formalRelationTuple{}, err
		}
		if formalFiberValue(root) != prior {
			publication = append(publication, formalFiberWrite{ordinal: ordinal, value: formalFiberValue(root)})
		}
	}
	if len(publication) == 0 {
		return base, nil
	}
	delta, err := directory.sealDelta(publication)
	if err != nil {
		return formalRelationTuple{}, err
	}
	root, _, err := directory.applyDelta(base.root, delta)
	if err != nil {
		return formalRelationTuple{}, err
	}
	result := a.normalize(formalRelationTuple{variable: base.variable, root: root})
	// A closed-factor lift is a producer boundary: its sparse writes combine
	// with the base tuple's structural carry into complete registered lane
	// spellings. Register those exact spellings here, while they remain owned
	// by the producer. Consumers retain the fail-closed lookup and never
	// reconstruct a carrier from individual leaves.
	if err := a.cacheFormalTupleFactorSpellings(result); err != nil {
		return formalRelationTuple{}, err
	}
	return result, nil
}
