package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/domain/formal"
)

// outcomeFamilyBindings is the Stage-2 lane owned by the outcome boundary and
// its directly related structural and factor transactions.  The list is kept
// separate from FrozenOperatorKinds so this lane cannot accidentally claim a
// transfer family that belongs to another lowering.
var outcomeFamilyBindings = []struct {
	kind   OperatorKind
	kernel string
}{
	{OperatorOutcome, "transformer/formal-outcome/v1"},
	{OperatorNonreturning, "transformer/formal-nonreturning/v1"},
	{OperatorDefinition, "transformer/formal-definition/v1"},
	{OperatorResource, "transformer/formal-resource/v1"},
	{OperatorEntry, "transformer/formal-entry/v1"},
	{OperatorPublication, "transformer/formal-publication/v1"},
	{OperatorPresenceImplications, "transformer/formal-presence-implications/v1"},
	{OperatorCovariantExposure, "transformer/formal-covariant-exposure/v1"},
	{OperatorContribution, "transformer/formal-contribution/v1"},
}

// OutcomeFamilyKinds returns the lane's immutable catalog partition.
func OutcomeFamilyKinds() []OperatorKind {
	out := make([]OperatorKind, len(outcomeFamilyBindings))
	for index, binding := range outcomeFamilyBindings {
		out[index] = binding.kind
	}
	return out
}

// OutcomeFamilyCompiler installs this lane's mechanical hooks into the
// fail-closed skeleton.  Each hook is bound to an already-frozen transformer
// transaction identifier; it has no State fallback and implements no transfer
// logic of its own.
func OutcomeFamilyCompiler() (*equation.Compiler, error) {
	compiler := equation.Skeleton()
	for _, binding := range outcomeFamilyBindings {
		var err error
		compiler, err = compiler.With(string(binding.kind), equation.BindExistingKernel(binding.kernel))
		if err != nil {
			return nil, err
		}
	}
	return compiler, nil
}

// OutcomeFamilyOccurrence is the complete lowering input for one already
// sealed occurrence.  The caller supplies the exact semantic footprint from
// the frozen template and one closed term for each catalog role.  In
// particular, CellLabel is deliberately absent: it is diagnostic routing and
// may not contribute to occurrence or artifact identity.
type OutcomeFamilyOccurrence struct {
	Kind       OperatorKind
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

func outcomeFamilyContains(kind OperatorKind) bool {
	for _, binding := range outcomeFamilyBindings {
		if binding.kind == kind {
			return true
		}
	}
	return false
}

// Draft constructs a fresh frozen contract and binds its exact mandatory
// roles to sealed terms.  It never fills in an omitted access from a lexical
// class, a coordinate adjacency, or a concrete State; omission is left for
// the post-kernel VerifyAccess audit to reject.
func (o OutcomeFamilyOccurrence) Draft() (equation.Draft, OperatorContract, error) {
	if !outcomeFamilyContains(o.Kind) {
		return equation.Draft{}, OperatorContract{}, fmt.Errorf("transformer: %q is outside the outcome family", o.Kind)
	}
	contract, err := NewOperatorContract(o.Kind, o.Occurrence)
	if err != nil {
		return equation.Draft{}, OperatorContract{}, err
	}
	contract.Reads = append([]ContractSelector(nil), o.Reads...)
	contract.Writes = append([]ContractSelector(nil), o.Writes...)
	contract.GuardAtoms = append([]string(nil), o.GuardAtoms...)
	contract.Advances = append([]formal.LexicalClassID(nil), o.Advances...)
	contract.AliasSupport = append([]formal.LexicalClassID(nil), o.AliasSupport...)
	contract.WriteAlphabet = append([]formal.Root(nil), o.WriteAlphabet...)
	contract.Outcomes = append([]OutcomeKind(nil), o.Outcomes...)
	contract.DiagnosticOutputs = append([]DiagnosticDescriptor(nil), o.DiagnosticOutputs...)
	contract.Dependencies = append([]ContractDependency(nil), o.Dependencies...)
	if !contract.ContentID().Valid() {
		return equation.Draft{}, OperatorContract{}, fmt.Errorf("transformer: outcome family occurrence has an incomplete contract")
	}

	operands := make([]equation.Operand, 0, len(contract.Operands))
	for _, role := range contract.Operands {
		term, present := o.Terms[role]
		if !present {
			return equation.Draft{}, OperatorContract{}, fmt.Errorf("transformer: outcome family occurrence omitted closed %q operand", role)
		}
		if term.Entry != (role == AccessEntry) {
			return equation.Draft{}, OperatorContract{}, fmt.Errorf("transformer: outcome family occurrence has an invalid entry binding for %q", role)
		}
		operands = append(operands, equation.Operand{Role: string(role), Term: term})
	}
	if len(o.Terms) != len(contract.Operands) {
		return equation.Draft{}, OperatorContract{}, fmt.Errorf("transformer: outcome family occurrence has an undeclared operand role")
	}
	return equation.Draft{
		Target:     o.Target,
		Entry:      o.Entry,
		Guards:     append([]equation.Guard(nil), o.Guards...),
		Occurrence: equation.Occurrence{Kind: string(contract.Kind), ContractID: equation.ContentID(contract.ContentID())},
		Operands:   operands,
	}, contract, nil
}

// AuditOutcomeFamilyExecution verifies the access observed around the
// canonical transformer/factapply transaction.  Evaluation has already
// happened when this is called; verification is audit-only.
func AuditOutcomeFamilyExecution(contract OperatorContract, execution Execution) error {
	return VerifyLoweredOperatorAccess(contract, execution)
}
