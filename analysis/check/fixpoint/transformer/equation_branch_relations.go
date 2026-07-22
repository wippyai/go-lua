package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/domain/formal"
)

// FormalBranchRelationsKernelID is the already-sealed formal adapter for the
// canonical factapply BranchRelationFactors transaction.  It is an identity
// for that existing kernel, not a second implementation of the transfer.
const FormalBranchRelationsKernelID = "transformer/formal-branch-relations/v1"

// BranchRelationsEquationLowerer installs the one existing branch-relations
// kernel in an equation compiler.  The returned hook only transports sealed
// terms; evaluation remains owned by formalTupleAlgebra and factapply.
func BranchRelationsEquationLowerer() equation.Lowerer {
	return equation.BindExistingKernel(FormalBranchRelationsKernelID)
}

// BranchRelationsEquationBinding is the complete occurrence-local lowering
// declaration supplied by the owner of sealed relation syntax.  Target is a
// body-owned semantic coordinate name; CellLabel is intentionally not an
// input because it is diagnostic routing rather than semantic identity.
//
// Flow, State, and Guard must be sealed closed terms, except that Flow may be
// the enclosing equation.EntryTerm.  In particular State is opaque term
// syntax, never a concrete State fallback.
type BranchRelationsEquationBinding struct {
	Occurrence formal.OccurrenceID
	Target     string
	Entry      string
	Flow       equation.Term
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

// BindBranchRelationsEquationOccurrence constructs the frozen contract for
// exactly one occurrence, binds all mandatory catalog operands, and returns
// the opaque equation draft plus its contract for post-execution auditing.
// It does not inspect or execute a transfer.
func BindBranchRelationsEquationOccurrence(
	occurrence RelationEquationOccurrence,
	binding BranchRelationsEquationBinding,
) (equation.Draft, OperatorContract, error) {
	if occurrence.Kind != OperatorBranchRelations {
		return equation.Draft{}, OperatorContract{}, fmt.Errorf("transformer: branch-relations binder received %q", occurrence.Kind)
	}
	if binding.Occurrence.Owner() != occurrence.Body || binding.Target == "" || binding.Entry == "" {
		return equation.Draft{}, OperatorContract{}, fmt.Errorf("transformer: branch-relations binding has foreign or incomplete semantic identity")
	}
	contract, err := NewOperatorContract(OperatorBranchRelations, binding.Occurrence)
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
	contractID := contract.ContentID()
	if !contractID.Valid() {
		return equation.Draft{}, OperatorContract{}, fmt.Errorf("transformer: branch-relations contract is incomplete")
	}
	body := equation.BodyID(occurrence.Body)
	entry := equation.EntryParameter{Body: body, Name: binding.Entry}
	return equation.Draft{
		Target: equation.Coordinate{Body: body, Name: binding.Target},
		Entry:  entry,
		Occurrence: equation.Occurrence{
			Kind:       string(contract.Kind),
			ContractID: equation.ContentID(contractID),
		},
		Operands: []equation.Operand{
			{Role: string(AccessFlow), Term: binding.Flow},
			{Role: string(AccessState), Term: binding.State},
			{Role: string(AccessGuard), Term: binding.Guard},
		},
	}, contract, nil
}

// NewBranchRelationsEquationBinder adapts a sealed-syntax binding provider to
// RelationProgram.CompileEquationIR.  The provider, rather than the template
// walker, owns occurrence identity and every declared semantic access.
func NewBranchRelationsEquationBinder(
	bind func(RelationEquationOccurrence) (BranchRelationsEquationBinding, error),
) RelationEquationBinder {
	return func(occurrence RelationEquationOccurrence) (equation.Draft, error) {
		if bind == nil {
			return equation.Draft{}, fmt.Errorf("transformer: nil branch-relations equation binding provider")
		}
		binding, err := bind(occurrence)
		if err != nil {
			return equation.Draft{}, err
		}
		draft, _, err := BindBranchRelationsEquationOccurrence(occurrence, binding)
		return draft, err
	}
}
