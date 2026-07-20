package identity

import (
	"bytes"
	"strconv"

	"github.com/wippyai/go-lua/analysis/domain/formal"
	internal "github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

// FormalSchemaID names one identity coordinate in a sealed lexical relation.
// Ordinal is dense and one-based in that relation's frozen schema.  The body
// owner prevents two independently sealed schemas from aliasing while keeping
// the name entirely program-structural.
type FormalSchemaID struct {
	owner   lexicalidentity.StableLexicalBodyID
	ordinal uint64
}

func NewFormalSchemaID(owner lexicalidentity.StableLexicalBodyID, ordinal uint64) FormalSchemaID {
	if owner == (lexicalidentity.StableLexicalBodyID{}) || ordinal == 0 {
		return FormalSchemaID{}
	}
	return FormalSchemaID{owner: owner, ordinal: ordinal}
}

func (id FormalSchemaID) Valid() bool {
	return id.owner != (lexicalidentity.StableLexicalBodyID{}) && id.ordinal != 0
}

func (id FormalSchemaID) Owner() lexicalidentity.StableLexicalBodyID { return id.owner }
func (id FormalSchemaID) Ordinal() uint64                            { return id.ordinal }

// FormalVar is one vocabulary-qualified occurrence of a sealed identity
// coordinate.  Renaming Input to Middle or Output changes only Vocabulary;
// the underlying finite schema coordinate remains identical.
type FormalVar struct {
	root formal.Root
}

func NewFormalVar(schema FormalSchemaID, vocabulary formal.Vocabulary) FormalVar {
	return NewFormalVarRoot(formal.NewRoot(schema.owner, schema.ordinal, vocabulary))
}

// NewFormalVarRoot imports the one neutral formal root without reconstructing
// or adapting its structural identity.
func NewFormalVarRoot(root formal.Root) FormalVar {
	if !root.Valid() {
		return FormalVar{}
	}
	return FormalVar{root: root}
}

func (v FormalVar) Valid() bool { return v.root.Valid() }
func (v FormalVar) Schema() FormalSchemaID {
	return NewFormalSchemaID(v.root.Owner(), v.root.Ordinal())
}
func (v FormalVar) Root() formal.Root             { return v.root }
func (v FormalVar) Vocabulary() formal.Vocabulary { return v.root.Vocabulary() }
func (v FormalVar) In(vocabulary formal.Vocabulary) FormalVar {
	return NewFormalVarRoot(formal.NewRoot(v.root.Owner(), v.root.Ordinal(), vocabulary))
}

type TermKind uint8

const (
	TermInvalid TermKind = iota
	TermConcrete
	TermFormal
	TermAllocation
)

// Term is the closed relational identity atom.  The alternatives are
// deliberately stored as typed fields rather than encoded into ID.Kind/Site:
// a concrete runtime identity, a formal schema variable, and an allocation
// template have different substitution and quantification laws.
type Term struct {
	kind       TermKind
	concrete   ID
	formal     FormalVar
	allocation AllocationTemplate
}

func ConcreteTerm(id ID) Term {
	if id == (ID{}) {
		return Term{}
	}
	return Term{kind: TermConcrete, concrete: id}
}

func FormalTerm(variable FormalVar) Term {
	if !variable.Valid() {
		return Term{}
	}
	return Term{kind: TermFormal, formal: variable}
}

func AllocationTerm(template AllocationTemplate) Term {
	if !template.Valid() {
		return Term{}
	}
	return Term{kind: TermAllocation, allocation: template}
}

func (t Term) Kind() TermKind { return t.kind }

func (t Term) Valid() bool {
	switch t.kind {
	case TermConcrete:
		return t.concrete != (ID{}) &&
			t.formal == (FormalVar{}) && t.allocation == (AllocationTemplate{})
	case TermFormal:
		return t.formal.Valid() && t.concrete == (ID{}) && t.allocation == (AllocationTemplate{})
	case TermAllocation:
		return t.allocation.Valid() && t.concrete == (ID{}) && t.formal == (FormalVar{})
	default:
		return false
	}
}

func (t Term) Concrete() (ID, bool) {
	return t.concrete, t.kind == TermConcrete && t.Valid()
}

func (t Term) Formal() (FormalVar, bool) {
	return t.formal, t.kind == TermFormal && t.Valid()
}

func (t Term) Allocation() (AllocationTemplate, bool) {
	return t.allocation, t.kind == TermAllocation && t.Valid()
}

func (t Term) hash() uint64 {
	h := internal.MixHash(internal.FnvString("identity.term"), uint64(t.kind))
	switch t.kind {
	case TermConcrete:
		return internal.MixHash(h, t.concrete.hash())
	case TermFormal:
		owner := t.formal.root.Owner()
		ownerHash := internal.NewWriter()
		_, _ = ownerHash.Write(owner[:])
		h = internal.MixHash(h, ownerHash.Sum64())
		h = internal.MixHash(h, t.formal.root.Ordinal())
		return internal.MixHash(h, uint64(t.formal.root.Vocabulary()))
	case TermAllocation:
		ownerHash := internal.NewWriter()
		_, _ = ownerHash.Write(t.allocation.owner[:])
		h = internal.MixHash(h, ownerHash.Sum64())
		h = internal.MixHash(h, uint64(t.allocation.allocation))
		return internal.MixHash(h, uint64(t.allocation.object))
	default:
		return h
	}
}

// Hash returns the stable structural hash of the typed term. Equality remains
// the semantic decision; this value is only an indexing/fingerprint aid.
func (t Term) Hash() uint64 { return t.hash() }

func (t Term) String() string {
	switch t.kind {
	case TermConcrete:
		return t.concrete.String()
	case TermFormal:
		return "formal(" + strconv.Itoa(int(t.formal.root.Vocabulary())) + "," + strconv.FormatUint(t.formal.root.Ordinal(), 10) + ")"
	case TermAllocation:
		return "allocation(" + strconv.FormatUint(uint64(t.allocation.allocation), 10) + "," + strconv.FormatUint(uint64(t.allocation.object), 10) + ")"
	default:
		return "identity-term(invalid)"
	}
}

// Less is the canonical structural order of identity terms. It includes the
// lexical owner of formal and allocation alternatives; diagnostic String
// spellings intentionally omit that owner and must never be used for durable
// ordering.
func Less(left, right Term) bool {
	if left.Kind() != right.Kind() {
		return left.Kind() < right.Kind()
	}
	if a, ok := left.Concrete(); ok {
		b, _ := right.Concrete()
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Site != b.Site {
			return a.Site < b.Site
		}
		return a.Index < b.Index
	}
	if a, ok := left.Formal(); ok {
		b, _ := right.Formal()
		aOwner, bOwner := a.Schema().Owner(), b.Schema().Owner()
		if aOwner != bOwner {
			return bytes.Compare(aOwner[:], bOwner[:]) < 0
		}
		if a.Schema().Ordinal() != b.Schema().Ordinal() {
			return a.Schema().Ordinal() < b.Schema().Ordinal()
		}
		return a.Vocabulary() < b.Vocabulary()
	}
	a, _ := left.Allocation()
	b, _ := right.Allocation()
	aOwner, bOwner := a.Owner(), b.Owner()
	if aOwner != bOwner {
		return bytes.Compare(aOwner[:], bOwner[:]) < 0
	}
	if a.AllocationOrdinal() != b.AllocationOrdinal() {
		return a.AllocationOrdinal() < b.AllocationOrdinal()
	}
	return a.ObjectOrdinal() < b.ObjectOrdinal()
}

// Substitution is an immutable, total-on-use binding from formal identity
// variables to the existing flat identity lattice.  Bottom means the relation
// branch is unreachable; Singleton is an exact rename; Top is an exact unknown
// image whose treatment is owned by each registered factor law.
//
// Allocation templates cannot be keys and therefore cannot be accidentally
// eliminated or rebound by existential substitution.
type Substitution struct {
	bindings map[FormalVar]Value
}

type Binding struct {
	Variable FormalVar
	Image    Value
}

func NewSubstitution(bindings []Binding) (Substitution, bool) {
	if len(bindings) == 0 {
		return Substitution{}, true
	}
	out := Substitution{bindings: make(map[FormalVar]Value, len(bindings))}
	for _, binding := range bindings {
		if !binding.Variable.Valid() {
			return Substitution{}, false
		}
		if id, singleton := binding.Image.ID(); singleton && id == (ID{}) {
			return Substitution{}, false
		}
		if _, duplicate := out.bindings[binding.Variable]; duplicate {
			return Substitution{}, false
		}
		out.bindings[binding.Variable] = binding.Image
	}
	return out, true
}

func (s Substitution) Image(variable FormalVar) (Value, bool) {
	if !variable.Valid() {
		return Value{}, false
	}
	value, ok := s.bindings[variable]
	return value, ok
}

// Substitute resolves concrete and formal terms.  Allocation terms return
// false even if their ID spelling resembles a concrete token: only the state
// layer's BoundaryAllocationAuthority may instantiate them.
func (s Substitution) Substitute(term Term) (Value, bool) {
	if concrete, ok := term.Concrete(); ok {
		return Singleton(concrete), true
	}
	if formal, ok := term.Formal(); ok {
		return s.Image(formal)
	}
	return Value{}, false
}
