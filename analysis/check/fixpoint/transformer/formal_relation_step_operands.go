package transformer

import "fmt"

// formalRelationStepOperands is the closed equation-wide input lens shared by
// every formal Step transaction.  It classifies the already-frozen influence
// row once; individual Step implementations must not rescan Inputs or dispatch
// one influence at a time, because a transfer is defined over the simultaneous
// operand tuple.
type formalRelationStepOperands struct {
	Flow               formalRelationTemplateInput
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
	var out formalRelationStepOperands
	if !equation.Cell.valid() || equation.Cell.cell.Kind != formalRelationCellStep ||
		equation.Operator.kind != formalRelationCellStep {
		return out, fmt.Errorf("transformer: formal Step operand row is unowned")
	}
	flow := false
	for _, input := range equation.Inputs {
		if !input.valid(equation.Cell) {
			return formalRelationStepOperands{}, fmt.Errorf("transformer: formal Step operand is malformed")
		}
		switch input.Influence {
		case formalRelationInfluenceFlow:
			if flow {
				return formalRelationStepOperands{}, fmt.Errorf("transformer: formal Step has multiple Flow predecessors")
			}
			out.Flow, flow = input, true
		case formalRelationInfluenceStepNodeEntry:
			if out.HasNodeEntry {
				return formalRelationStepOperands{}, fmt.Errorf("transformer: formal Step has multiple NodeEntry operands")
			}
			out.NodeEntry, out.HasNodeEntry = input, true
		case formalRelationInfluenceStepPublishedRead:
			out.PublishedReads = append(out.PublishedReads, input)
		case formalRelationInfluenceCalleeOutcome:
			out.CalleeOutcomes = append(out.CalleeOutcomes, input)
		case formalRelationInfluenceClosureDefinition:
			out.ClosureDefinitions = append(out.ClosureDefinitions, input)
		default:
			return formalRelationStepOperands{}, fmt.Errorf("transformer: influence %d is not a Step operand", input.Influence)
		}
	}
	if !flow {
		return formalRelationStepOperands{}, fmt.Errorf("transformer: formal Step has no Flow predecessor")
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
