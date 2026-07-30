package identity

import (
	"bytes"
	"strconv"

	"github.com/wippyai/go-lua/analysis/domain/formal"
	internal "github.com/wippyai/go-lua/analysis/internal/hash"
)

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
	formal     formal.Root
	allocation AllocationTemplate
}

func ConcreteTerm(id ID) Term {
	if id == (ID{}) {
		return Term{}
	}
	return Term{kind: TermConcrete, concrete: id}
}

func FormalTerm(root formal.Root) Term {
	if !root.Valid() {
		return Term{}
	}
	return Term{kind: TermFormal, formal: root}
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
			t.formal == (formal.Root{}) && t.allocation == (AllocationTemplate{})
	case TermFormal:
		return t.formal.Valid() && t.concrete == (ID{}) && t.allocation == (AllocationTemplate{})
	case TermAllocation:
		return t.allocation.Valid() && t.concrete == (ID{}) && t.formal == (formal.Root{})
	default:
		return false
	}
}

func (t Term) Concrete() (ID, bool) {
	return t.concrete, t.kind == TermConcrete && t.Valid()
}

func (t Term) Formal() (formal.Root, bool) {
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
		owner := t.formal.Owner()
		ownerHash := internal.NewWriter()
		_, _ = ownerHash.Write(owner[:])
		h = internal.MixHash(h, ownerHash.Sum64())
		h = internal.MixHash(h, t.formal.Ordinal())
		return internal.MixHash(h, uint64(t.formal.Vocabulary()))
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
		return "formal(" + strconv.Itoa(int(t.formal.Vocabulary())) + "," + strconv.FormatUint(t.formal.Ordinal(), 10) + ")"
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
		aOwner, bOwner := a.Owner(), b.Owner()
		if aOwner != bOwner {
			return bytes.Compare(aOwner[:], bOwner[:]) < 0
		}
		if a.Ordinal() != b.Ordinal() {
			return a.Ordinal() < b.Ordinal()
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
	bindings map[formal.Root]Value
}

type Binding struct {
	Variable formal.Root
	Image    Value
}

func NewSubstitution(bindings []Binding) (Substitution, bool) {
	if len(bindings) == 0 {
		return Substitution{}, true
	}
	out := Substitution{bindings: make(map[formal.Root]Value, len(bindings))}
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

func (s Substitution) Image(variable formal.Root) (Value, bool) {
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
