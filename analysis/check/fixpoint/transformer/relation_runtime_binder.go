package transformer

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/domain/formal"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

// RelationRuntimeOperand is one sealed runtime slot selected for a lowered
// occurrence. Value is an opaque capability token: only the production bridge
// that created the binding can interpret it as a formal operand slot.
type RelationRuntimeOperand struct {
	Role  AccessRole
	Value []byte
}

// BoundRelationOccurrence is the total real-occurrence binding record used by
// the Stage-3 bridge. It retains no solver callback or mutable tuple value.
type BoundRelationOccurrence struct {
	Ordinal  uint64
	Kind     OperatorKind
	Target   equation.Coordinate
	Operands []RelationRuntimeOperand
}

// RealRelationBodyBinding binds every frozen occurrence owned by one real
// production body. The entry State is accepted at this boundary solely to
// prove that the binding belongs to a concrete production invocation; State
// is never encoded into equation IR.
type RealRelationBodyBinding struct {
	program     *RelationProgram
	body        lexicalidentity.StableLexicalBodyID
	entry       state.State
	occurrences map[uint64]BoundRelationOccurrence
	contracts   map[equation.ContentID]OperatorContract
}

// BindRealRelationBody constructs a total, read-only operand binder for body.
// It pre-enumerates the sealed template, so a subsequent lowering can neither
// invent an occurrence nor silently omit a runtime operand. Every slot token
// is body- and occurrence-qualified; the VM sees only its closed bytes.
func (p *RelationProgram) BindRealRelationBody(body lexicalidentity.StableLexicalBodyID, entry state.State) (*RealRelationBodyBinding, error) {
	if p == nil || p.formalTemplate == nil || !p.formalTemplate.validFor(p) {
		return nil, fmt.Errorf("transformer: real relation binder has no sealed relation template")
	}
	if _, ok := p.byBody[body]; !ok {
		return nil, fmt.Errorf("transformer: real relation binder has no body %s", body)
	}
	binding := &RealRelationBodyBinding{
		program: p, body: body, entry: entry,
		occurrences: make(map[uint64]BoundRelationOccurrence),
		contracts:   make(map[equation.ContentID]OperatorContract),
	}
	// CompileEquationIR is the sole occurrence walk.  This dry binder records
	// all real slots first; its drafts are intentionally discarded.
	compiler, err := Stage2EquationCompiler()
	if err != nil {
		return nil, err
	}
	_, err = p.CompileEquationIR(compiler, func(occurrence RelationEquationOccurrence) (equation.Draft, error) {
		if occurrence.Body != body {
			return relationPlaceholderDraft(occurrence)
		}
		bound, contract, bindErr := bindRealRelationOccurrence(occurrence)
		if bindErr != nil {
			return equation.Draft{}, bindErr
		}
		binding.occurrences[occurrence.Ordinal] = bound
		binding.contracts[equation.ContentID(contract.ContentID())] = contract
		return relationDraftFromBoundOccurrence(occurrence, bound, contract)
	})
	if err != nil {
		return nil, err
	}
	if len(binding.occurrences) == 0 {
		return nil, fmt.Errorf("transformer: real relation binder found no occurrences for %s", body)
	}
	return binding, nil
}

// Occurrences returns detached copies in canonical occurrence order.
func (b *RealRelationBodyBinding) Occurrences() []BoundRelationOccurrence {
	if b == nil {
		return nil
	}
	ordinals := make([]uint64, 0, len(b.occurrences))
	for ordinal := range b.occurrences {
		ordinals = append(ordinals, ordinal)
	}
	sort.Slice(ordinals, func(i, j int) bool { return ordinals[i] < ordinals[j] })
	out := make([]BoundRelationOccurrence, 0, len(ordinals))
	for _, ordinal := range ordinals {
		occurrence := b.occurrences[ordinal]
		occurrence.Operands = cloneRelationRuntimeOperands(occurrence.Operands)
		out = append(out, occurrence)
	}
	return out
}

// Compile lowers this body only through the complete Stage-2 catalog using
// this binding's exact occurrence contracts and slots.
func (b *RealRelationBodyBinding) Compile() (equation.Artifact, error) {
	if b == nil || b.program == nil {
		return equation.Artifact{}, fmt.Errorf("transformer: real relation binder is unowned")
	}
	compiler, err := Stage2EquationCompiler()
	if err != nil {
		return equation.Artifact{}, err
	}
	return b.program.CompileBodyEquationIR(b.body, compiler, b.Binder())
}

// Binder returns the exact total binder for this concrete body. Any occurrence
// absent from the pre-enumerated production binding names itself in the error.
func (b *RealRelationBodyBinding) Binder() RelationEquationBinder {
	return func(occurrence RelationEquationOccurrence) (equation.Draft, error) {
		if b == nil || b.program == nil || occurrence.Body != b.body {
			return equation.Draft{}, fmt.Errorf("transformer: unbound occurrence %s/%d", occurrence.Body, occurrence.Ordinal)
		}
		bound, ok := b.occurrences[occurrence.Ordinal]
		if !ok || bound.Kind != occurrence.Kind {
			return equation.Draft{}, fmt.Errorf("transformer: unbound occurrence %s/%d", occurrence.Body, occurrence.Ordinal)
		}
		contract, ok := b.contractsForOccurrence(occurrence.Ordinal)
		if !ok {
			return equation.Draft{}, fmt.Errorf("transformer: unbound occurrence %s/%d", occurrence.Body, occurrence.Ordinal)
		}
		return relationDraftFromBoundOccurrence(occurrence, bound, contract)
	}
}

func (b *RealRelationBodyBinding) contractsForOccurrence(ordinal uint64) (OperatorContract, bool) {
	occurrence, ok := b.occurrences[ordinal]
	if !ok {
		return OperatorContract{}, false
	}
	for id, contract := range b.contracts {
		if id == equation.ContentID(contract.ContentID()) && contract.Kind == occurrence.Kind && contract.Occurrence.Ordinal() == ordinal {
			return contract, true
		}
	}
	return OperatorContract{}, false
}

func bindRealRelationOccurrence(occurrence RelationEquationOccurrence) (BoundRelationOccurrence, OperatorContract, error) {
	if occurrence.Ordinal == 0 || occurrence.Body == (lexicalidentity.StableLexicalBodyID{}) || occurrence.Kind == "" {
		return BoundRelationOccurrence{}, OperatorContract{}, fmt.Errorf("transformer: unbound occurrence %s/%d", occurrence.Body, occurrence.Ordinal)
	}
	contract, err := NewOperatorContract(occurrence.Kind, formal.NewOccurrenceID(occurrence.Body, occurrence.Ordinal))
	if err != nil {
		return BoundRelationOccurrence{}, OperatorContract{}, err
	}
	body := equation.BodyID(occurrence.Body)
	bound := BoundRelationOccurrence{
		Ordinal:  occurrence.Ordinal,
		Kind:     occurrence.Kind,
		Target:   equation.Coordinate{Body: body, Name: fmt.Sprintf("occurrence-%d", occurrence.Ordinal)},
		Operands: make([]RelationRuntimeOperand, 0, len(contract.Operands)),
	}
	for _, role := range contract.Operands {
		bound.Operands = append(bound.Operands, RelationRuntimeOperand{
			Role: role, Value: []byte(fmt.Sprintf("relation-runtime-slot/v1/%x/%d/%s", occurrence.Body, occurrence.Ordinal, role)),
		})
	}
	return bound, contract, nil
}

func relationDraftFromBoundOccurrence(occurrence RelationEquationOccurrence, bound BoundRelationOccurrence, contract OperatorContract) (equation.Draft, error) {
	body := equation.BodyID(occurrence.Body)
	entry := equation.EntryParameter{Body: body, Name: "entry"}
	operands := make([]equation.Operand, 0, len(bound.Operands))
	for _, operand := range bound.Operands {
		term := equation.ClosedTerm(operand.Value)
		if operand.Role == AccessEntry {
			term = equation.EntryTerm(entry)
		}
		operands = append(operands, equation.Operand{Role: string(operand.Role), Term: term})
	}
	return equation.Draft{
		Target: bound.Target, Entry: entry,
		Occurrence: equation.Occurrence{Kind: string(contract.Kind), ContractID: equation.ContentID(contract.ContentID())},
		Operands:   operands,
	}, nil
}

// relationPlaceholderDraft keeps the dry whole-template walk fail-closed for
// foreign bodies without granting their binding to this body.
func relationPlaceholderDraft(occurrence RelationEquationOccurrence) (equation.Draft, error) {
	bound, contract, err := bindRealRelationOccurrence(occurrence)
	if err != nil {
		return equation.Draft{}, err
	}
	return relationDraftFromBoundOccurrence(occurrence, bound, contract)
}

func cloneRelationRuntimeOperands(in []RelationRuntimeOperand) []RelationRuntimeOperand {
	out := make([]RelationRuntimeOperand, len(in))
	for index, operand := range in {
		out[index] = RelationRuntimeOperand{Role: operand.Role, Value: append([]byte(nil), operand.Value...)}
	}
	return out
}
