package transformer

import "fmt"

// formalRelationStepOperands is the closed equation-wide input lens shared by
// every formal Step transaction.  It classifies the already-frozen influence
// row once; individual Step implementations must not rescan Inputs or dispatch
// one influence at a time, because a transfer is defined over the simultaneous
// operand tuple.
type formalRelationStepOperands struct {
	Flow               formalRelationTemplateInput
	HasFlow            bool
	NodeEntry          formalRelationTemplateInput
	HasNodeEntry       bool
	PublishedReads     []formalRelationTemplateInput
	CalleeOutcomes     []formalRelationTemplateInput
	ClosureDefinitions []formalRelationTemplateInput
}

// partitionFormalRelationStepOperands validates the complete Step operand
// vocabulary and returns each repeated family in semantic order.  There is
// exactly one control-flow predecessor and at most one node-entry dependency;
// all other cardinalities are owned by the frozen equation-completeness proof.
func partitionFormalRelationStepOperands(equation formalRelationEquation) (formalRelationStepOperands, error) {
	if !equation.Cell.valid() || equation.Cell.cell.Kind != formalRelationCellStep ||
		equation.Operator.kind != formalRelationCellStep {
		return formalRelationStepOperands{}, fmt.Errorf("transformer: formal Step operand row is unowned")
	}
	for _, input := range equation.Inputs {
		if !input.valid(equation.Cell) {
			return formalRelationStepOperands{}, fmt.Errorf("transformer: formal Step operand is malformed")
		}
	}
	return freezeFormalRelationStageOperands(equation.Inputs, true)
}

// freezeFormalRelationStageOperands performs the same closed partition once at
// template-freeze time. Internal Flow between stages is deliberately absent;
// only the first stage owns an external Flow operand.
func freezeFormalRelationStageOperands(inputs []formalRelationTemplateInput, requireFlow bool) (formalRelationStepOperands, error) {
	var out formalRelationStepOperands
	for _, input := range inputs {
		if !input.Source.valid() {
			return formalRelationStepOperands{}, fmt.Errorf("transformer: formal Step stage operand is malformed")
		}
		switch input.Influence {
		case formalRelationInfluenceFlow:
			if out.HasFlow {
				return formalRelationStepOperands{}, fmt.Errorf("transformer: formal Step stage has multiple Flow predecessors")
			}
			out.Flow, out.HasFlow = input, true
		case formalRelationInfluenceStepNodeEntry:
			if out.HasNodeEntry {
				return formalRelationStepOperands{}, fmt.Errorf("transformer: formal Step stage has multiple NodeEntry operands")
			}
			out.NodeEntry, out.HasNodeEntry = input, true
		case formalRelationInfluenceStepPublishedRead:
			out.PublishedReads = append(out.PublishedReads, input)
		case formalRelationInfluenceCalleeOutcome:
			out.CalleeOutcomes = append(out.CalleeOutcomes, input)
		case formalRelationInfluenceClosureDefinition:
			out.ClosureDefinitions = append(out.ClosureDefinitions, input)
		default:
			return formalRelationStepOperands{}, fmt.Errorf("transformer: influence %d is not a Step stage operand", input.Influence)
		}
	}
	if out.HasFlow != requireFlow {
		return formalRelationStepOperands{}, fmt.Errorf("transformer: formal Step stage Flow ownership is malformed")
	}
	sortFormalStepPublishedReads(out.PublishedReads)
	sortFormalStepSources(out.CalleeOutcomes)
	sortFormalStepSources(out.ClosureDefinitions)
	return out, nil
}

func sortFormalStepPublishedReads(inputs []formalRelationTemplateInput) {
	for index := 1; index < len(inputs); index++ {
		value := inputs[index]
		position := index
		for position > 0 && formalStepPublishedReadLess(value, inputs[position-1]) {
			inputs[position] = inputs[position-1]
			position--
		}
		inputs[position] = value
	}
}

func formalStepPublishedReadLess(left, right formalRelationTemplateInput) bool {
	if left.ReadPoint != right.ReadPoint {
		return left.ReadPoint < right.ReadPoint
	}
	return formalRelationCellLess(left.Source.cell, right.Source.cell)
}

func sortFormalStepSources(inputs []formalRelationTemplateInput) {
	for index := 1; index < len(inputs); index++ {
		value := inputs[index]
		position := index
		for position > 0 && formalRelationCellLess(value.Source.cell, inputs[position-1].Source.cell) {
			inputs[position] = inputs[position-1]
			position--
		}
		inputs[position] = value
	}
}
