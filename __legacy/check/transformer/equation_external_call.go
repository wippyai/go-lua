package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/domain/formal"
)

// ExternalCallEquationKernelID identifies the already-sealed external-call
// transaction.  The equation lowering binds this kernel; it does not recreate
// provider evaluation, factor application, normal-return decoding, or
// publication.
const ExternalCallEquationKernelID = "transformer/formal-external-call/v1"

// ExternalCallEquationBinding is the closed, occurrence-local projection of
// one sealed external-call transaction.  The relation-program owner supplies
// the complete access footprint from its frozen provider/factor plan.  Terms
// are indexed by the frozen catalog roles, so a map cannot affect canonical
// artifact order.
//
// Selectors must include every reduction, guard, diagnostic, allocation, and
// boundary observation or publication performed by the existing kernel.
type ExternalCallEquationBinding struct {
	Occurrence formal.OccurrenceID
	Target     equation.Coordinate
	Entry      equation.EntryParameter
	Guards     []equation.Guard
	Terms      map[AccessRole]equation.Term

	Reads             []ContractSelector
	Writes            []ContractSelector
	GuardAtoms        []string
	Advances          []formal.LexicalClassID
	AliasSupport      []formal.LexicalClassID
	WriteAlphabet     []formal.Root
	Outcomes          []OutcomeKind
	DiagnosticOutputs []DiagnosticDescriptor
	Dependencies      []ContractDependency
}

// NewExternalCallEquationDraft constructs the one frozen OperatorContract for
// an external-call occurrence and binds all of its mandatory roles to closed
// terms.  No concrete State, predecessor result, or transfer callback can
// enter this boundary.
func NewExternalCallEquationDraft(binding ExternalCallEquationBinding) (equation.Draft, OperatorContract, error) {
	if !binding.Occurrence.Valid() || binding.Target.Body != equation.BodyID(binding.Occurrence.Owner()) ||
		binding.Entry.Body != equation.BodyID(binding.Occurrence.Owner()) {
		return equation.Draft{}, OperatorContract{}, fmt.Errorf("transformer: external-call equation binding has foreign occurrence ownership")
	}
	contract, err := NewOperatorContract(OperatorExternalCall, binding.Occurrence)
	if err != nil {
		return equation.Draft{}, OperatorContract{}, err
	}
	contract.Reads = append([]ContractSelector(nil), binding.Reads...)
	contract.Writes = append([]ContractSelector(nil), binding.Writes...)
	contract.GuardAtoms = append([]string(nil), binding.GuardAtoms...)
	contract.Advances = append([]formal.LexicalClassID(nil), binding.Advances...)
	contract.AliasSupport = append([]formal.LexicalClassID(nil), binding.AliasSupport...)
	contract.WriteAlphabet = append([]formal.Root(nil), binding.WriteAlphabet...)
	contract.Outcomes = append([]OutcomeKind(nil), binding.Outcomes...)
	contract.DiagnosticOutputs = append([]DiagnosticDescriptor(nil), binding.DiagnosticOutputs...)
	contract.Dependencies = append([]ContractDependency(nil), binding.Dependencies...)
	if !contract.ContentID().Valid() {
		return equation.Draft{}, OperatorContract{}, fmt.Errorf("transformer: external-call equation contract is incomplete")
	}

	if len(binding.Terms) != len(contract.Operands) {
		return equation.Draft{}, OperatorContract{}, fmt.Errorf("transformer: external-call equation has incomplete operand bindings")
	}
	operands := make([]equation.Operand, 0, len(contract.Operands))
	for _, role := range contract.Operands {
		term, present := binding.Terms[role]
		// ExternalCall has no formal-entry operand.  Each role is a sealed,
		// closed provider/factor term rather than a State fallback.
		if !present || term.Entry || len(term.Encoding) == 0 {
			return equation.Draft{}, OperatorContract{}, fmt.Errorf("transformer: external-call equation operand %q is not closed", role)
		}
		operands = append(operands, equation.Operand{Role: string(role), Term: term})
	}
	return equation.Draft{
		Target:     binding.Target,
		Entry:      binding.Entry,
		Guards:     append([]equation.Guard(nil), binding.Guards...),
		Occurrence: equation.Occurrence{Kind: string(contract.Kind), ContractID: equation.ContentID(contract.ContentID())},
		Operands:   operands,
	}, contract, nil
}

// ExternalCallEquationLowerer binds the existing factapply-backed formal
// external-call transaction.  It deliberately contains no transfer logic.
func ExternalCallEquationLowerer() equation.Lowerer {
	return equation.BindExistingKernel(ExternalCallEquationKernelID)
}

// VerifyExternalCallLoweredAccess verifies a dynamic audit record emitted
// around the existing external-call kernel.  The audit is post-execution and
// therefore cannot change which result is computed or published.
func VerifyExternalCallLoweredAccess(contract OperatorContract, execution equation.Execution) error {
	if contract.Kind != OperatorExternalCall {
		return fmt.Errorf("transformer: external-call audit received %q contract", contract.Kind)
	}
	return VerifyLoweredOperatorAccess(contract, execution)
}
