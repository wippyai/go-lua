package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// formalRelationStepDependencyContract is the complete, finite input row of
// one lexical Step equation. It is sealed beside the cell inventory so the
// executor cannot acquire an undeclared State through a closure or callback.
// Inputs retain multiplicity: duplicate semantic operands are malformed, not
// silently collapsed into an apparently equivalent graph.
type formalRelationStepDependencyContract struct {
	inputs []formalRelationInfluence
}

// formalRelationStepDependencyShape is the closed law relating reduced step
// syntax to non-predecessor state inputs. Every valid step has its immediate
// Flow predecessor. Four transactions additionally compare against the exact
// lexical node entry; ExternalCall and GenericFor additionally consume the
// published outputs selected by body.nodeReads. Apply outcomes and escaped
// closure definitions are added by their typed composition owners.
//
// BranchRelations intentionally has neither extra input: its "original" and
// "current" operands are internal stages of one operator applied to the
// immediate predecessor, exactly as relationCodeRuntime.emitBranchFactorStep.
func formalRelationStepDependencyShape(kind boundaryStepKind) (nodeEntry, publishedReads bool, valid bool) {
	switch kind {
	case boundaryStepEffect,
		boundaryStepApply,
		boundaryStepEnvironmentWrite,
		boundaryStepContribution,
		boundaryStepLoopFeedback,
		boundaryStepLoopExit,
		boundaryStepBranchRelations,
		boundaryStepCallResults,
		boundaryStepChannelSelect:
		return false, false, true
	case boundaryStepExternalCall:
		return false, true, true
	case boundaryStepRootAssignment,
		boundaryStepPresenceImplications,
		boundaryStepCovariantExposure:
		return true, false, true
	case boundaryStepGenericFor:
		return true, true, true
	default:
		return false, false, false
	}
}

func (i *formalRelationRegionInventory) linkStepExecutionDependencies(
	program *RelationProgram,
	variable relationVar,
	nodeEntry, target formalRelationCell,
	step boundaryStep,
	declared map[formalRelationCell]struct{},
) error {
	needsEntry, needsReads, valid := formalRelationStepDependencyShape(step.kind)
	if !valid {
		return fmt.Errorf("transformer: formal relation %d Step %+v has invalid boundary kind %d", variable, target, step.kind)
	}
	if needsEntry {
		if err := i.addStepDependency(target, formalRelationInfluence{
			Source: nodeEntry, Target: target, Kind: formalRelationInfluenceStepNodeEntry,
		}, declared); err != nil {
			return err
		}
	}
	if !needsReads {
		return nil
	}
	if program == nil || variable == 0 || int(variable) > len(program.bodies) {
		return fmt.Errorf("transformer: formal Step published reads have no lexical body")
	}
	body := &program.bodies[variable-1]
	if int(step.point) >= len(body.nodeReads) {
		return fmt.Errorf("transformer: formal relation %d call point %d has no read dependency row", variable, step.point)
	}
	for _, readPoint := range body.nodeReads[step.point] {
		for _, publication := range body.relation.code.publication.points {
			if publication.point != readPoint {
				continue
			}
			source, dependency, valid := formalRelationPublishedOutputCell(variable, body.relation.code, publication.ref)
			if !valid {
				return fmt.Errorf("transformer: formal relation %d read point %d has no equation output", variable, readPoint)
			}
			if !dependency {
				continue
			}
			if err := i.addStepDependency(target, formalRelationInfluence{
				Source: source, Target: target, Kind: formalRelationInfluenceStepPublishedRead, ReadPoint: readPoint,
			}, declared); err != nil {
				return err
			}
		}
	}
	return nil
}

// formalRelationPublishedOutputCell is the static twin of
// relationCodeRuntime.output. It names the already-declared equation whose
// value is published for a CFG point; it never allocates a runtime read cell.
func formalRelationPublishedOutputCell(variable relationVar, code *relationCode, ref relationRootRef) (cell formalRelationCell, dependency, valid bool) {
	if variable == 0 || code == nil || ref == 0 || int(ref) >= len(code.nodes) {
		return formalRelationCell{}, false, false
	}
	node := code.nodes[ref]
	switch node.kind {
	case relationNodeBottom:
		// The physical executor reads the shared Bottom coordinate even if the
		// lexical bottom node acquired an InitialStatePlan seed. Bottom is the
		// empty join, so it contributes no equation edge.
		return formalRelationCell{}, false, true
	case relationNodeNonreturning:
		return formalRelationCell{Variable: variable, Kind: formalRelationCellNonreturning}, true, true
	case relationNodeOutcome:
		if node.outcome == 0 || int(node.outcome) >= len(code.outcomes) {
			return formalRelationCell{}, false, false
		}
		return formalRelationCell{Variable: variable, Outcome: node.outcome, Kind: formalRelationCellOutcome}, true, true
	case relationNodeSequence:
		for index := len(node.steps) - 1; index >= 0; index-- {
			if relationCodeStepHasCoordinate(code, node.steps[index]) {
				return formalRelationCell{Variable: variable, Root: ref, Step: uint32(index + 1), Kind: formalRelationCellStep}, true, true
			}
		}
	}
	return formalRelationCell{Variable: variable, Root: ref, Kind: formalRelationCellNode}, true, true
}

func (i *formalRelationRegionInventory) addStepDependency(
	target formalRelationCell,
	influence formalRelationInfluence,
	declared map[formalRelationCell]struct{},
) error {
	if target.Kind != formalRelationCellStep || influence.Target != target || influence.Site.valid() {
		return fmt.Errorf("transformer: malformed formal Step dependency")
	}
	contract, exists := i.stepInputs[target]
	if !exists {
		contract = formalRelationStepDependencyContract{}
	}
	for _, prior := range contract.inputs {
		if prior.Source == influence.Source && prior.Kind == influence.Kind && prior.ReadPoint == influence.ReadPoint {
			return fmt.Errorf("transformer: duplicate formal Step dependency %+v -> %+v (%d)", influence.Source, target, influence.Kind)
		}
	}
	contract.inputs = append(contract.inputs, influence)
	i.stepInputs[target] = contract
	return i.addInfluence(influence, declared)
}

func (i *formalRelationRegionInventory) validateStepDependencyContracts() error {
	if i == nil {
		return fmt.Errorf("transformer: formal Step dependency inventory is unowned")
	}
	for _, cell := range i.cells {
		if cell.Kind != formalRelationCellStep {
			continue
		}
		contract, ok := i.stepInputs[cell]
		if !ok || len(contract.inputs) == 0 {
			return fmt.Errorf("transformer: formal Step %+v has no semantic dependency contract", cell)
		}
		actual := i.incoming[cell]
		if len(actual) != len(contract.inputs) {
			return fmt.Errorf("transformer: formal Step %+v dependency cardinality changed: got %d want %d", cell, len(actual), len(contract.inputs))
		}
		used := make([]bool, len(actual))
		for _, expected := range contract.inputs {
			matched := -1
			for index, candidate := range actual {
				if !used[index] && candidate == expected {
					if matched >= 0 {
						return fmt.Errorf("transformer: formal Step %+v has duplicate dependency %+v", cell, expected)
					}
					matched = index
				}
			}
			if matched < 0 {
				return fmt.Errorf("transformer: formal Step %+v is missing dependency %+v", cell, expected)
			}
			used[matched] = true
		}
	}
	return nil
}

// stepDependencyDeclared is the region-owned legality oracle used while the
// immutable template is frozen. It rejects a same-kind input from any other
// point even when cardinality happens to match.
func (i *formalRelationRegionInventory) stepDependencyDeclared(target, source formalRelationCell, kind formalRelationInfluenceKind, readPoint cfg.Point) bool {
	if i == nil || target.Kind != formalRelationCellStep {
		return false
	}
	for _, input := range i.stepInputs[target].inputs {
		if input.Target == target && input.Source == source && input.Kind == kind && input.ReadPoint == readPoint {
			return true
		}
	}
	return false
}

func (i *formalRelationRegionInventory) stepDependencyCount(target formalRelationCell, kind formalRelationInfluenceKind) int {
	if i == nil || target.Kind != formalRelationCellStep {
		return 0
	}
	count := 0
	for _, input := range i.stepInputs[target].inputs {
		if input.Target == target && input.Kind == kind {
			count++
		}
	}
	return count
}
