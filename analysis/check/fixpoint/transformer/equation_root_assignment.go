package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/domain/formal"
)

// FormalRootAssignmentEquationKernel is the identity of the already-frozen
// formal N4 adapter.  It names the existing RootAssignmentFactorProgram path;
// it is not a second transfer implementation.
const FormalRootAssignmentEquationKernel = "transformer/formal-root-assignment/v1"

// RootAssignmentEquationLowerer installs the root-assignment family hook in
// the equation skeleton.  BindExistingKernel retains the sole canonical
// factapply/transformer evaluator and rejects an incomplete lowering.
func RootAssignmentEquationLowerer() equation.Lowerer {
	return equation.BindExistingKernel(FormalRootAssignmentEquationKernel)
}

// RootAssignmentEquationBinding is the complete occurrence-local declaration
// supplied by the owner of one sealed root-assignment transaction.  Terms are
// already closed relation terms (except Flow, which may be the exact Entry
// parameter).  The footprint is deliberately explicit: a lowerer cannot
// infer reads through lexical-class adjacency or manufacture a State value.
type RootAssignmentEquationBinding struct {
	Target equation.Coordinate
	Entry  equation.EntryParameter

	Occurrence formal.OccurrenceID
	Guards     []equation.Guard
	Flow       equation.Term
	NodeEntry  equation.Term
	State      equation.Term
	Guard      equation.Term

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

// NewRootAssignmentEquationDraft constructs exactly one frozen contract and
// its closed role bindings for one root-assignment occurrence.  No part of the
// transaction is evaluated here; equation compilation later binds the one
// existing kernel through RootAssignmentEquationLowerer.
func NewRootAssignmentEquationDraft(binding RootAssignmentEquationBinding) (equation.Draft, OperatorContract, error) {
	body := equation.BodyID(binding.Occurrence.Owner())
	if !binding.Occurrence.Valid() || binding.Target.Body != body || binding.Entry.Body != body {
		return equation.Draft{}, OperatorContract{}, fmt.Errorf("transformer: root-assignment equation binding has foreign body ownership")
	}
	contract, err := NewOperatorContract(OperatorRootAssignment, binding.Occurrence)
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
		return equation.Draft{}, OperatorContract{}, fmt.Errorf("transformer: root-assignment equation binding has incomplete contract")
	}
	draft := equation.Draft{
		Target: binding.Target, Entry: binding.Entry, Guards: append([]equation.Guard(nil), binding.Guards...),
		Occurrence: equation.Occurrence{Kind: string(OperatorRootAssignment), ContractID: equation.ContentID(contract.ContentID())},
		Operands: []equation.Operand{
			{Role: string(AccessFlow), Term: binding.Flow},
			{Role: string(AccessNodeEntry), Term: binding.NodeEntry},
			{Role: string(AccessState), Term: binding.State},
			{Role: string(AccessGuard), Term: binding.Guard},
		},
	}
	if err := draftValidForRootAssignment(draft); err != nil {
		return equation.Draft{}, OperatorContract{}, err
	}
	return draft, contract, nil
}

func draftValidForRootAssignment(draft equation.Draft) error {
	if draft.Occurrence.Kind != string(OperatorRootAssignment) || len(draft.Operands) != 4 {
		return fmt.Errorf("transformer: root-assignment equation binding has incomplete operands")
	}
	seen := make(map[string]bool, len(draft.Operands))
	for _, operand := range draft.Operands {
		if operand.Role == "" || seen[operand.Role] {
			return fmt.Errorf("transformer: root-assignment equation binding has duplicate operands")
		}
		seen[operand.Role] = true
	}
	for _, role := range []AccessRole{AccessFlow, AccessNodeEntry, AccessState, AccessGuard} {
		if !seen[string(role)] {
			return fmt.Errorf("transformer: root-assignment equation binding omits %s", role)
		}
	}
	return nil
}
