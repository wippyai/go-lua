package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/engine/solve"
)

// CyclicDependencyCertificate is the detached Stage-4 view of the already
// frozen relation region.  It carries the production WTO as a frozen copy;
// constructing it never invokes solve.NewWTOPlan.
type CyclicDependencyCertificate struct {
	Cells        []equation.CellID
	Plan         *solve.WTOPlan[equation.CellID]
	Dependencies []equation.SemanticDependency
}

// CyclicDependencyCertificate returns the complete semantic graph for the
// sealed relation program.  The region inventory is the only graph authority:
// typed influences and the exact production schedule are transcribed, never
// rediscovered from lowered readiness edges.
func (p *RelationProgram) CyclicDependencyCertificate() (CyclicDependencyCertificate, error) {
	if p == nil || p.formalRegion == nil || p.formalRegion.plan == nil || !p.formalRegion.plan.Matches(p.formalRegion.cells) {
		return CyclicDependencyCertificate{}, fmt.Errorf("transformer: cyclic dependency certificate has no frozen region")
	}
	region := p.formalRegion
	cellIDs := make([]equation.CellID, len(region.cells))
	byCell := make(map[formalRelationCell]equation.CellID, len(region.cells))
	for index, cell := range region.cells {
		id := cyclicCellID(cell)
		if _, duplicate := byCell[cell]; duplicate {
			return CyclicDependencyCertificate{}, fmt.Errorf("transformer: cyclic dependency certificate has duplicate cell")
		}
		cellIDs[index] = id
		byCell[cell] = id
	}
	elements, err := cyclicPlanElements(region.plan.Elements(), byCell)
	if err != nil {
		return CyclicDependencyCertificate{}, err
	}
	// FreezeWTOPlan's canonical cell order is its structural pre-order. The
	// source plan may retain a different construction-time cell slice, so take
	// the order from the preserved structure rather than asking it to re-plan.
	cellIDs = cyclicPlanCells(elements)
	influences := make([]solve.WTOInfluence[equation.CellID], 0)
	dependencies := make([]equation.SemanticDependency, 0)
	for _, target := range region.cells {
		for _, influence := range region.incoming[target] {
			from, fromOK := byCell[influence.Source]
			to, toOK := byCell[influence.Target]
			if !fromOK || !toOK || influence.Target != target {
				return CyclicDependencyCertificate{}, fmt.Errorf("transformer: cyclic dependency certificate has foreign influence")
			}
			influences = append(influences, solve.WTOInfluence[equation.CellID]{From: from, To: to})
			dependencies = append(dependencies, equation.SemanticDependency{From: from, To: to, Reason: cyclicInfluenceReason(influence.Kind), Evidence: cyclicInfluenceEvidence(influence)})
		}
	}
	plan, err := solve.FreezeWTOPlan(cellIDs, elements, influences)
	if err != nil {
		return CyclicDependencyCertificate{}, fmt.Errorf("transformer: cyclic dependency certificate freezes WTO: %w", err)
	}
	return CyclicDependencyCertificate{Cells: cellIDs, Plan: plan, Dependencies: dependencies}, nil
}

func cyclicPlanCells(elements []solve.WTOElement[equation.CellID]) []equation.CellID {
	var cells []equation.CellID
	var visit func([]solve.WTOElement[equation.CellID])
	visit = func(items []solve.WTOElement[equation.CellID]) {
		for _, item := range items {
			cells = append(cells, item.Vertex)
			visit(item.Body)
		}
	}
	visit(elements)
	return cells
}

func cyclicPlanElements(in []solve.WTOElement[formalRelationCell], ids map[formalRelationCell]equation.CellID) ([]solve.WTOElement[equation.CellID], error) {
	out := make([]solve.WTOElement[equation.CellID], len(in))
	for index, element := range in {
		id, ok := ids[element.Vertex]
		if !ok {
			return nil, fmt.Errorf("transformer: cyclic dependency certificate has foreign WTO cell")
		}
		body, err := cyclicPlanElements(element.Body, ids)
		if err != nil {
			return nil, err
		}
		out[index] = solve.WTOElement[equation.CellID]{Vertex: id, Body: body}
	}
	return out, nil
}

func cyclicCellID(cell formalRelationCell) equation.CellID {
	return equation.CellID(fmt.Sprintf("relation-cell/v1/%d/%d/%d/%d/%d/%d/%d", cell.Variable, cell.Root, cell.Step, cell.Outcome, cell.Definition, cell.Resource, cell.Kind))
}

func cyclicInfluenceReason(kind formalRelationInfluenceKind) equation.EdgeReason {
	switch kind {
	case formalRelationInfluenceStepNodeEntry, formalRelationInfluenceFlow, formalRelationInfluenceChoiceTrue, formalRelationInfluenceChoiceFalse, formalRelationInfluenceLoopFeedback, formalRelationInfluenceLoopExit:
		return equation.EdgeContractRead
	case formalRelationInfluenceStepPublishedRead:
		return equation.EdgePublishedRead
	case formalRelationInfluenceCalleeOutcome, formalRelationInfluenceApplyNonreturningPredecessor, formalRelationInfluenceCalleeNonreturning:
		return equation.EdgePairedApply
	case formalRelationInfluenceDefinitionSeed, formalRelationInfluenceDefinitionOutcome, formalRelationInfluenceClosureDefinition:
		return equation.EdgePathEquality
	case formalRelationInfluenceResourceSeed, formalRelationInfluenceResourceFeedback:
		return equation.EdgeAllocationPlacement
	case formalRelationInfluenceLocalNonreturning:
		return equation.EdgeContractOutcome
	default:
		// formalRelationInfluenceKind is sealed; retaining an unknown class as
		// a conservative read is safer than erasing the edge, while the
		// evidence string below makes the audit failure actionable.
		return equation.EdgeContractRead
	}
}

func cyclicInfluenceEvidence(influence formalRelationInfluence) string {
	return fmt.Sprintf("formal-influence/%d/read-point/%d/site/%s", influence.Kind, influence.ReadPoint, cyclicCellID(influence.Site))
}
