package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/domain/formal"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

// EffectEquationBinding is the sealed, body-owned information needed to bind
// one existing effect transaction into parameterized equation IR.  It carries
// declarations, not transfer semantics: the relation template and the bound
// factapply kernel remain the sole evaluators.
//
// Reads and Writes must include every semantic observation made by the
// canonical transaction (including reduction, guard, allocation, diagnostic,
// and boundary observations where applicable).  They are deliberately
// supplied by the owner rather than inferred from a relation cell index or a
// mutable execution value.
type EffectEquationBinding struct {
	Kind       OperatorKind
	Occurrence formal.OccurrenceID
	Target     equation.Coordinate
	Entry      equation.EntryParameter

	Flow, State, Allocation, Guard equation.Term

	Reads, Writes          []ContractSelector
	GuardAtoms             []string
	Advances, AliasSupport []formal.LexicalClassID
	WriteAlphabet          []formal.Root
	Outcomes               []OutcomeKind
	DiagnosticOutputs      []DiagnosticDescriptor
	Dependencies           []ContractDependency
}

// EffectEquationDraft creates the one occurrence contract and binds its exact
// catalog roles to sealed terms.  A term is never synthesized from State or a
// predecessor; an incomplete binding yields no equation draft.
func EffectEquationDraft(binding EffectEquationBinding) (equation.Draft, OperatorContract, error) {
	if !isEffectEquationKind(binding.Kind) || !binding.Occurrence.Valid() ||
		binding.Occurrence.Owner() != lexicalidentity.StableLexicalBodyID(binding.Target.Body) ||
		binding.Target.Body != binding.Entry.Body || !effectTermsClosed(binding) {
		return equation.Draft{}, OperatorContract{}, fmt.Errorf("transformer: effect equation binding has foreign or incomplete identity")
	}
	contract, err := NewOperatorContract(binding.Kind, binding.Occurrence)
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
	if contract.CanonicalBytes() == nil {
		return equation.Draft{}, OperatorContract{}, fmt.Errorf("transformer: effect equation binding has incomplete contract footprint")
	}

	operands := []equation.Operand{{Role: string(AccessFlow), Term: binding.Flow}, {Role: string(AccessGuard), Term: binding.Guard}}
	switch binding.Kind {
	case OperatorPathReplacement, OperatorPathInvalidation, OperatorIndexMutation:
		operands = append(operands, equation.Operand{Role: string(AccessState), Term: binding.State})
	case OperatorAllocationTemplate, OperatorObjectMaterialization:
		operands = append(operands, equation.Operand{Role: string(AccessAllocation), Term: binding.Allocation})
	default:
		return equation.Draft{}, OperatorContract{}, fmt.Errorf("transformer: non-effect equation binding %q", binding.Kind)
	}
	return equation.Draft{
		Target: binding.Target, Entry: binding.Entry,
		Occurrence: equation.Occurrence{Kind: string(contract.Kind), ContractID: equation.ContentID(contract.ContentID())},
		Operands:   operands,
	}, contract, nil
}

// InstallEffectEquationLowerings installs mechanical hooks for all effect
// families.  BindExistingKernel identifies the pre-existing canonical kernel;
// this function intentionally contains no transfer implementation.
func InstallEffectEquationLowerings(compiler *equation.Compiler) (*equation.Compiler, error) {
	if compiler == nil {
		return nil, fmt.Errorf("transformer: nil equation compiler")
	}
	for _, binding := range []struct {
		kind   OperatorKind
		kernel string
	}{
		{OperatorPathReplacement, "transformer/formal-path-replacement/v1"},
		{OperatorPathInvalidation, "transformer/formal-path-invalidation/v1"},
		{OperatorIndexMutation, "transformer/formal-index-mutation/v1"},
		{OperatorAllocationTemplate, "transformer/formal-allocation-template/v1"},
		{OperatorObjectMaterialization, "transformer/formal-object-materialization/v1"},
	} {
		var err error
		compiler, err = compiler.With(string(binding.kind), equation.BindExistingKernel(binding.kernel))
		if err != nil {
			return nil, err
		}
	}
	return compiler, nil
}

func isEffectEquationKind(kind OperatorKind) bool {
	switch kind {
	case OperatorPathReplacement, OperatorPathInvalidation, OperatorIndexMutation,
		OperatorAllocationTemplate, OperatorObjectMaterialization:
		return true
	default:
		return false
	}
}

func effectTermsClosed(binding EffectEquationBinding) bool {
	return !binding.Flow.Entry && !binding.State.Entry && !binding.Allocation.Entry && !binding.Guard.Entry
}
