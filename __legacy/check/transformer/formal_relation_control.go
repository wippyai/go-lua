package transformer

import "fmt"

// evaluateFormalControlInput interprets the canonical control operators whose
// payload is owned by the source relationCode syntax. The source operator is
// reachable only through the equation's explicit frozen Input capability.
func evaluateFormalControlInput(
	a *formalTupleAlgebra,
	equation formalRelationEquation,
	input formalRelationTemplateInput,
	value formalRelationTuple,
) (formalRelationTuple, bool, error) {
	switch input.Influence {
	case formalRelationInfluenceChoiceTrue, formalRelationInfluenceChoiceFalse,
		formalRelationInfluenceLoopFeedback, formalRelationInfluenceLoopExit:
		if a == nil || a.program == nil || a.program.formalTemplate == nil {
			return formalRelationTuple{}, true, fmt.Errorf("transformer: formal control input is unowned")
		}
	default:
		// Non-control influences are invisible here. In particular rootless NR
		// cells must reach the Apply/NR dispatcher without requiring control
		// syntax ownership.
		return formalRelationTuple{}, false, nil
	}
	switch input.Influence {
	case formalRelationInfluenceChoiceTrue, formalRelationInfluenceChoiceFalse:
		source, scope, ok := a.program.formalTemplate.sourceOperator(input)
		if !ok {
			return formalRelationTuple{}, true, fmt.Errorf("transformer: formal Choice has no frozen source operator")
		}
		if equation.Cell.cell.Kind != formalRelationCellNode || input.Source.cell.Kind != formalRelationCellNode ||
			value.variable != input.Source.cell.Variable {
			return formalRelationTuple{}, true, fmt.Errorf("transformer: formal Choice has incompatible tuple ownership")
		}
		node, nodeOK := formalRelationNodeOperator(source)
		if !nodeOK || node.kind != relationNodeChoice || node.guard == 0 || source.code == nil || source.code.terms == nil {
			return formalRelationTuple{}, true, fmt.Errorf("transformer: formal Choice has no exact frozen guard")
		}
		condition, err := a.decisionForGuard(input.Source.cell.Variable, scope, source.code.terms, node.guard)
		if err != nil {
			return formalRelationTuple{}, true, err
		}
		if input.Influence == formalRelationInfluenceChoiceFalse {
			condition, err = formalDecisionBooleanNot(a, condition)
			if err != nil {
				return formalRelationTuple{}, true, err
			}
		}
		result, err := a.restrictTupleCare(value, condition)
		return result, true, err

	case formalRelationInfluenceLoopFeedback:
		source, _, ok := a.program.formalTemplate.sourceOperator(input)
		if !ok {
			return formalRelationTuple{}, true, fmt.Errorf("transformer: formal loop feedback has no frozen source operator")
		}
		if equation.Cell.cell.Kind != formalRelationCellNode || input.Source.cell.Kind != formalRelationCellStep ||
			value.variable != input.Source.cell.Variable {
			return formalRelationTuple{}, true, fmt.Errorf("transformer: formal loop feedback has incompatible tuple ownership")
		}
		step, stepOK := formalRelationStepOperator(source)
		if !stepOK || step.kind != boundaryStepLoopFeedback || step.binder == 0 {
			return formalRelationTuple{}, true, fmt.Errorf("transformer: formal loop feedback has no exact binder")
		}
		result, err := a.closeLoopTuple(value, input.Source.cell.Variable, step.binder)
		return result, true, err

	case formalRelationInfluenceLoopExit:
		source, _, ok := a.program.formalTemplate.sourceOperator(input)
		if !ok {
			return formalRelationTuple{}, true, fmt.Errorf("transformer: formal loop exit has no frozen source operator")
		}
		// The final iteration's guard facts remain observable after its selected
		// exit. Only reuse of the loop head ends an iteration lifetime; closing
		// here would erase exact exit-path evidence and outer/sibling guards.
		if equation.Cell.cell.Kind != formalRelationCellNode || input.Source.cell.Kind != formalRelationCellStep ||
			value.variable != input.Source.cell.Variable {
			return formalRelationTuple{}, true, fmt.Errorf("transformer: formal loop exit has incompatible tuple ownership")
		}
		step, stepOK := formalRelationStepOperator(source)
		if !stepOK || step.kind != boundaryStepLoopExit || step.binder == 0 {
			return formalRelationTuple{}, true, fmt.Errorf("transformer: formal loop exit has no exact binder")
		}
		return value, true, nil
	default:
		panic("unreachable formal control influence")
	}
}

func formalDecisionBooleanNot(a *formalTupleAlgebra, root decisionRef) (decisionRef, error) {
	if a == nil || a.ctx == nil {
		return 0, fmt.Errorf("transformer: formal Boolean complement is unowned")
	}
	return a.decisions.apply(a.ctx, uint8(decisionNot), false, root, decisionFalse, func(left, right decisionLeaf) (decisionLeaf, error) {
		if left > 1 || right > 1 {
			return 0, errDecisionMalformed
		}
		return 1 - left, nil
	})
}

// restrictTupleCare quotients one route-owned tuple by an exact Boolean
// condition. Product roots are intentionally not rewritten: all registered
// lanes retain the same structural carrier and are interpreted under the new
// Care, so adding another ProductLane adds no control-operator work.
func (a *formalTupleAlgebra) restrictTupleCare(tuple formalRelationTuple, condition decisionRef) (formalRelationTuple, error) {
	if err := a.validateTuple(tuple); err != nil {
		return formalRelationTuple{}, err
	}
	if tuple.bottom() {
		return tuple, nil
	}
	// condition is a run-local decision capability returned by
	// decisionForGuard (or its exact Boolean complement). Do not rescan its DAG
	// on every WTO visit; Boolean leaf legality is enforced by the And operator.
	if _, ok := a.decisions.node(condition); !ok {
		return formalRelationTuple{}, errDecisionMalformed
	}
	care, err := a.care(tuple)
	if err != nil {
		return formalRelationTuple{}, err
	}
	restricted, err := a.decisions.apply(a.ctx, uint8(decisionAnd), true, care, condition, decisionLeafAnd)
	if err != nil {
		return formalRelationTuple{}, err
	}
	if restricted == decisionFalse {
		return formalRelationTuple{}, nil
	}
	return a.writeCare(tuple, restricted)
}

// closeLoopTuple is the sole loop-feedback lifetime transaction. The frozen
// rank set already contains the binder's complete registered descendant scope
// cone; composeGuardBoundary performs one synchronized Care+product traversal.
func (a *formalTupleAlgebra) closeLoopTuple(tuple formalRelationTuple, variable relationVar, binder loopMuTerm) (formalRelationTuple, error) {
	if a == nil || a.program == nil || a.program.formalGuards == nil || tuple.variable != variable {
		return formalRelationTuple{}, fmt.Errorf("transformer: formal loop tuple closure is unowned")
	}
	lifetime, ok := a.program.formalGuards.loopLifetime(variable, binder)
	if !ok {
		return formalRelationTuple{}, fmt.Errorf("transformer: formal loop tuple lifetime is unranked")
	}
	owner := a.program.formalGuards
	boundary := formalGuardBoundary{
		owner:  owner,
		rename: formalGuardRankMap{owner: owner},
		domain: formalGuardRankSet{owner: owner},
		close:  lifetime,
	}
	return a.composeGuardBoundary(tuple, boundary)
}
