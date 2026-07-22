package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

// RelationEquationOccurrence is the compiler's read-only walk record for one
// already-frozen operator occurrence.  CellLabel is diagnostic routing only:
// it must never be used as contract, coordinate, cache, or artifact identity.
// The binder supplies the Stage-1 contract instance and closed operands.
type RelationEquationOccurrence struct {
	Body      lexicalidentity.StableLexicalBodyID
	Kind      OperatorKind
	CellLabel string
}

// RelationEquationBinder binds a Stage-1 contract occurrence and its closed
// operands.  In particular it is the only authority that can choose a
// contract ContentID; the walker does not manufacture identities from dense
// cell references, source positions, or execution generations.
type RelationEquationBinder func(RelationEquationOccurrence) (equation.Draft, error)

// CompileEquationIR walks the sealed relation template and dispatches each
// operator occurrence through the Stage-2 compiler.  It is additive: current
// transformer execution is not invoked, replaced, or given a symbolic State
// path.  Node cells have no transfer occurrence and are intentionally absent.
func (p *RelationProgram) CompileEquationIR(compiler *equation.Compiler, bind RelationEquationBinder) (equation.Artifact, error) {
	if p == nil || p.formalTemplate == nil || !p.formalTemplate.validFor(p) {
		return equation.Artifact{}, fmt.Errorf("transformer: equation compiler has no sealed relation template")
	}
	if compiler == nil || bind == nil {
		return equation.Artifact{}, fmt.Errorf("transformer: equation compiler has no lowerer or occurrence binder")
	}
	drafts := make([]equation.Draft, 0, len(p.formalTemplate.equations))
	appendOccurrence := func(body lexicalidentity.StableLexicalBodyID, kind OperatorKind, label string) error {
		draft, err := bind(RelationEquationOccurrence{Body: body, Kind: kind, CellLabel: label})
		if err != nil {
			return err
		}
		if draft.Occurrence.Kind != string(kind) || draft.Target.Body != equation.BodyID(body) || draft.Entry.Body != equation.BodyID(body) {
			return fmt.Errorf("transformer: equation binder changed frozen occurrence ownership")
		}
		drafts = append(drafts, draft)
		return nil
	}
	for index, relationEquation := range p.formalTemplate.equations {
		cell := relationEquation.Cell.cell
		if cell.Variable == 0 || int(cell.Variable) > len(p.bodies) {
			return equation.Artifact{}, fmt.Errorf("transformer: equation cell %d has foreign body", index)
		}
		body := p.bodies[cell.Variable-1].body
		label := fmt.Sprintf("equation-%d", index) // diagnostic only; see RelationEquationOccurrence.
		if cell.Kind == formalRelationCellStep {
			stages := relationEquation.StepStages
			if len(stages) == 0 {
				stages = []formalRelationStepStage{{Operator: relationEquation.Operator}}
			}
			for stageIndex, stage := range stages {
				kind, present := operatorKindForStepCapability(stage.Operator.stepCapability)
				if !present {
					return equation.Artifact{}, fmt.Errorf("transformer: equation %s stage %d has no frozen operator kind", label, stageIndex)
				}
				if err := appendOccurrence(body, kind, fmt.Sprintf("%s-stage-%d", label, stageIndex)); err != nil {
					return equation.Artifact{}, err
				}
			}
			continue
		}
		var kind OperatorKind
		switch cell.Kind {
		case formalRelationCellOutcome:
			kind = OperatorOutcome
		case formalRelationCellNonreturning:
			kind = OperatorNonreturning
		case formalRelationCellDefinition:
			kind = OperatorDefinition
		case formalRelationCellResource:
			kind = OperatorResource
		case formalRelationCellNode:
			continue
		default:
			return equation.Artifact{}, fmt.Errorf("transformer: equation %s has unknown cell kind", label)
		}
		if err := appendOccurrence(body, kind, label); err != nil {
			return equation.Artifact{}, err
		}
	}
	return compiler.Compile(equation.Source{Drafts: drafts})
}
