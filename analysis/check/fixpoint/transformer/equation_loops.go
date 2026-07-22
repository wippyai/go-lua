package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/domain/formal"
)

const (
	formalGenericForEquationKernel  = "transformer/formal-generic-for/v1"
	formalLoopControlEquationKernel = "transformer/formal-loop-control/v1"
)

// InstallLoopEquationLowerings installs only the two frozen loop-family
// bindings.  Both bindings name existing transformer kernels; equation owns
// neither a State value nor a transfer implementation.
func InstallLoopEquationLowerings(compiler *equation.Compiler) (*equation.Compiler, error) {
	if compiler == nil {
		return nil, fmt.Errorf("transformer: nil loop equation compiler")
	}
	installed, err := compiler.With(string(OperatorGenericFor), equation.BindExistingKernel(formalGenericForEquationKernel))
	if err != nil {
		return nil, err
	}
	return installed.With(string(OperatorLoopControl), equation.BindExistingKernel(formalLoopControlEquationKernel))
}

// BindLoopEquationOccurrence is the Stage-2 binder for the loop family.  It
// copies only sealed, body-owned facts into the parameterized equation.  The
// actual GenericFor and loop-control transfers remain the pre-existing formal
// kernels named above.
func BindLoopEquationOccurrence(occurrence RelationEquationOccurrence) (equation.Draft, error) {
	if occurrence.Kind != OperatorGenericFor && occurrence.Kind != OperatorLoopControl {
		return equation.Draft{}, fmt.Errorf("transformer: %q is outside the loop equation family", occurrence.Kind)
	}
	if occurrence.cell.Kind != formalRelationCellStep || occurrence.cell.Variable == 0 ||
		occurrence.cell.Root == 0 || occurrence.cell.Step == 0 || occurrence.operator.kind != formalRelationCellStep {
		return equation.Draft{}, fmt.Errorf("transformer: loop equation occurrence has no sealed Step capability")
	}
	if kind, ok := operatorKindForStepCapability(occurrence.operator.stepCapability); !ok || kind != occurrence.Kind {
		return equation.Draft{}, fmt.Errorf("transformer: loop equation occurrence capability drifted")
	}

	ordinal := uint64(occurrence.cell.Root)<<32 | uint64(occurrence.cell.Step)
	contract, err := NewOperatorContract(occurrence.Kind, formal.NewOccurrenceID(occurrence.Body, ordinal))
	if err != nil {
		return equation.Draft{}, err
	}
	switch occurrence.Kind {
	case OperatorGenericFor:
		contract, _, err = bindGenericForEquationContract(contract, occurrence.operator)
	case OperatorLoopControl:
		contract, _, err = bindLoopControlEquationContract(contract, occurrence.operator)
	}
	if err != nil {
		return equation.Draft{}, err
	}

	body := equation.BodyID(occurrence.Body)
	entry := equation.EntryParameter{Body: body, Name: "entry"}
	operands := make([]equation.Operand, 0, len(contract.Operands))
	for _, role := range contract.Operands {
		operands = append(operands, equation.Operand{
			Role: string(role),
			Term: equation.ClosedTerm(loopOperandEncoding(occurrence.Kind, occurrence.cell, role)),
		})
	}
	return equation.Draft{
		Target: equation.Coordinate{Body: body, Name: loopCoordinateName(occurrence.Kind, occurrence.cell)},
		Entry:  entry, Occurrence: equation.Occurrence{Kind: string(contract.Kind), ContractID: equation.ContentID(contract.ContentID())},
		Operands: operands,
	}, nil
}

func bindGenericForEquationContract(contract OperatorContract, operator formalRelationOperatorRef) (OperatorContract, string, error) {
	step, ok := formalRelationStepOperator(operator)
	if !ok || step.kind != boundaryStepGenericFor || operator.genericFor == nil || !operator.genericFor.valid(operator) {
		return OperatorContract{}, "", fmt.Errorf("transformer: GenericFor equation has no sealed factor transaction")
	}
	// The existing transaction reads its immediate flow, the lexical entry,
	// published inputs, Values/projection factors, and any execution guard. It
	// writes exactly the selected Values/factor surfaces; allocation is a
	// projection read, never a second transfer path.
	contract.Reads = []ContractSelector{
		{Role: AccessFlow, Name: "predecessor"},
		{Role: AccessNodeEntry, Name: "node-entry"},
		{Role: AccessPublished, Name: "published-inputs"},
		{Role: AccessState, Name: "values-and-factors"},
		{Role: AccessGuard, Name: "execute"},
		{Role: AccessAllocation, Name: "projection"},
	}
	contract.Writes = []ContractSelector{
		{Role: AccessState, Name: "target-values"},
		{Role: AccessState, Name: "write-factors"},
	}
	if step.guard != 0 {
		contract.GuardAtoms = []string{"generic-for/execute"}
	}
	contract.Dependencies = []ContractDependency{{Kind: formalGenericForEquationKernel, ID: contentID([]byte(formalGenericForEquationKernel))}}
	return contract, formalGenericForEquationKernel, nil
}

func bindLoopControlEquationContract(contract OperatorContract, operator formalRelationOperatorRef) (OperatorContract, string, error) {
	step, ok := formalRelationStepOperator(operator)
	if !ok || (step.kind != boundaryStepLoopFeedback && step.kind != boundaryStepLoopExit) || step.binder == 0 {
		return OperatorContract{}, "", fmt.Errorf("transformer: loop-control equation has no sealed control transaction")
	}
	// Feedback closes the exact loop-lifetime guard cone; exit deliberately
	// preserves final-iteration evidence.  Both are the existing control
	// kernel's declared flow/guard surface, with no hidden State fallback.
	contract.Reads = []ContractSelector{{Role: AccessFlow, Name: "predecessor"}, {Role: AccessGuard, Name: "loop-lifetime"}}
	contract.Writes = []ContractSelector{{Role: AccessGuard, Name: "loop-lifetime"}}
	contract.GuardAtoms = []string{"loop-control/lifetime"}
	contract.Dependencies = []ContractDependency{{Kind: formalLoopControlEquationKernel, ID: contentID([]byte(formalLoopControlEquationKernel))}}
	return contract, formalLoopControlEquationKernel, nil
}

func loopCoordinateName(kind OperatorKind, cell formalRelationCell) string {
	return fmt.Sprintf("%s/root-%d/step-%d", kind, cell.Root, cell.Step)
}

func loopOperandEncoding(kind OperatorKind, cell formalRelationCell, role AccessRole) []byte {
	// Root and Step are the sealed relation-code syntax coordinate, not the
	// region's dense equation index.  CellLabel is intentionally absent.
	return []byte(fmt.Sprintf("%s/root-%d/step-%d/%s", kind, cell.Root, cell.Step, role))
}
