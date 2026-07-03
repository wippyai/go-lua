package typewitness

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekindof"
	internal "github.com/wippyai/go-lua/analysis/internal/hash"
	typelit "github.com/wippyai/go-lua/analysis/type/literal"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/refinement"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

var Key = axis.NewKey[Value]("typewitness")

func Spec() axis.Spec[Value] {
	return axis.Spec[Value]{
		Key:          Key,
		Bottom:       Bottom,
		Top:          Top,
		Equal:        Equal,
		LessOrEq:     LessOrEq,
		Join:         Join,
		Meet:         Meet,
		Widen:        Widen,
		Hash:         Value.Hash,
		Reducer:      reduceByRuntimeKind,
		ReducerReads: []string{Key.ID(), runtimekind.Key.ID()},
	}
}

// reduceByRuntimeKind is the typewitness x runtimekind reduced-product rule: a
// concrete union witness drops the alternatives whose runtime kind the
// runtimekind axis excludes, materializing the complement of a type() guard
// (e.g. the else edge of type(v) == "number" narrows v to string). The guard
// keeps the common case branch-only and allocation-free: it returns immediately
// unless the witness is a concrete union and the runtime-kind axis is a strict,
// non-top constraint, and RestrictTypeToRuntimeKind only allocates when it
// actually drops a member.
func reduceByRuntimeKind(w axis.Writer) bool {
	twAny, ok := w.GetAny(Key.ID())
	if !ok {
		return false
	}
	tw, ok := twAny.(Value)
	if !ok || tw.state != concrete {
		return false
	}
	rkAny, ok := w.GetAny(runtimekind.Key.ID())
	if !ok {
		return false
	}
	rk, ok := rkAny.(runtimekind.Value)
	if !ok || rk.IsTop() || rk.IsBottom() {
		return false
	}
	narrowed, changed := runtimekindof.RestrictTypeToRuntimeKind(tw.t, rk)
	if !changed {
		return false
	}
	if narrowed == typ.Never {
		w.SetAny(Key.ID(), Bottom())
		return true
	}
	w.SetAny(Key.ID(), Of(narrowed))
	return true
}

type state uint8

const (
	bottom state = iota
	concrete
	top
)

// Value carries exact type evidence proven by runtime type witnesses.
type Value struct {
	state state
	t     typ.Type
}

func Bottom() Value { return Value{state: bottom} }
func Top() Value    { return Value{state: top} }

func Of(t typ.Type) Value {
	t = unwrap.Alias(t)
	if t == nil || typ.IsAny(t) || typ.IsUnknown(t) {
		return Top()
	}
	switch t.(type) {
	case *typ.Ref, *typ.Generic:
		return Top()
	case *typ.Instantiated:
		if refinement.ContainsFreeTypeParam(t) && !openInstantiatedWitnessAllowed(t) {
			return Top()
		}
	}
	if t == nil {
		return Top()
	}
	// Canonicalize union witnesses so member order does not affect Equal/Hash.
	// Containment (LessOrEq) treats number|string and string|number as equal, so
	// they must also be structurally equal for the partial order to be
	// antisymmetric and for Hash to stay consistent.
	if u, ok := t.(*typ.Union); ok {
		canonical := typ.MaterializeUnion(u.Members)
		if _, stillUnion := canonical.(*typ.Union); !stillUnion {
			return Of(canonical)
		}
		t = canonical
	}
	return Value{state: concrete, t: t}
}

func openInstantiatedWitnessAllowed(t typ.Type) bool {
	inst, ok := t.(*typ.Instantiated)
	if !ok || inst.Generic == nil || inst.Generic.Body == nil {
		return false
	}
	_, ok = unwrap.Alias(inst.Generic.Body).(*typ.Interface)
	return ok
}

func (v Value) IsBottom() bool { return v.state == bottom }
func (v Value) IsTop() bool    { return v.state == top }

func (v Value) Type() (typ.Type, bool) {
	if v.state != concrete || v.t == nil {
		return nil, false
	}
	return v.t, true
}

func Join(a, b Value) Value {
	if a.state == bottom {
		return b
	}
	if b.state == bottom {
		return a
	}
	if a.state == top || b.state == top {
		return Top()
	}
	if witnessTypeLeq(a.t, b.t) {
		return b
	}
	if witnessTypeLeq(b.t, a.t) {
		return a
	}
	return Of(normalizeWitnessUnion(a.t, b.t))
}

// LessOrEq is the witness-containment order: a's alternatives are each contained
// in some alternative of b. It is intentionally a cheap structural/literal-family
// containment, not full subtyping, so it stays allocation-free on the hot path.
func LessOrEq(a, b Value) bool {
	if a.state == bottom || b.state == top {
		return true
	}
	if a.state == top || b.state == bottom {
		return false
	}
	return witnessTypeLeq(a.t, b.t)
}

// Widen is a true widening: once a concrete witness strictly grows it jumps to
// a stable primitive family when possible, otherwise Top, so ascending chains
// under repeated Widen are eventually stationary. Join synthesizes unions, so
// Widen cannot remain Join without risking unbounded witness growth at loop
// heads; but literal scalar growth such as 0, 1, 2 should widen to integer, not
// erase the proof entirely.
func Widen(prev, next Value) Value {
	if prev.state == bottom {
		return next
	}
	if next.state == bottom {
		return prev
	}
	if prev.state == top || next.state == top {
		return Top()
	}
	if LessOrEq(next, prev) {
		return prev
	}
	if family, ok := widenedPrimitiveFamily(prev.t, next.t); ok {
		return Of(family)
	}
	if widened, ok := widenedStableRecord(prev.t, next.t); ok {
		return Of(widened)
	}
	return Top()
}

func widenedPrimitiveFamily(a, b typ.Type) (typ.Type, bool) {
	aBase, aOK := typelit.FamilyBase(a)
	bBase, bOK := typelit.FamilyBase(b)
	if !aOK || !bOK {
		return nil, false
	}
	return typelit.MergeFamilyBases(aBase, bBase)
}

func widenedStableRecord(a, b typ.Type) (typ.Type, bool) {
	ar, aOK := unwrap.Annotated(a).(*typ.Record)
	br, bOK := unwrap.Annotated(b).(*typ.Record)
	if !aOK || !bOK {
		return nil, false
	}
	if ar.Open != br.Open ||
		!sameWitnessType(ar.Metatable, br.Metatable) ||
		!sameWitnessType(ar.MapKey, br.MapKey) ||
		len(ar.Fields) != len(br.Fields) ||
		len(ar.StaticMembers) != len(br.StaticMembers) {
		return nil, false
	}
	var mapValue typ.Type
	switch {
	case ar.MapValue == nil && br.MapValue == nil:
	case ar.MapValue == nil || br.MapValue == nil:
		return nil, false
	default:
		widened, ok := widenRecordMemberType(ar.MapValue, br.MapValue)
		if !ok {
			return nil, false
		}
		mapValue = widened
	}
	fields := make([]typ.Field, len(ar.Fields))
	for i := range ar.Fields {
		af, bf := ar.Fields[i], br.Fields[i]
		if af.Name != bf.Name || af.Optional != bf.Optional || af.Readonly != bf.Readonly {
			return nil, false
		}
		widened, ok := widenRecordMemberType(af.Type, bf.Type)
		if !ok {
			return nil, false
		}
		fields[i] = af
		fields[i].Type = widened
	}
	members := make([]typ.StaticMember, len(ar.StaticMembers))
	for i := range ar.StaticMembers {
		am, bm := ar.StaticMembers[i], br.StaticMembers[i]
		if typ.CompareStaticMembers(am, bm) != 0 ||
			am.Optional != bm.Optional ||
			am.Readonly != bm.Readonly {
			return nil, false
		}
		widened, ok := widenRecordMemberType(am.Type, bm.Type)
		if !ok {
			return nil, false
		}
		members[i] = am
		members[i].Type = widened
	}
	return typ.RebuildRecord(typ.RecordParts{
		Fields:        fields,
		StaticMembers: members,
		Metatable:     ar.Metatable,
		MapKey:        ar.MapKey,
		MapValue:      mapValue,
		Open:          ar.Open,
		AssumeSorted:  true,
	}), true
}

func widenRecordMemberType(prev, next typ.Type) (typ.Type, bool) {
	if sameWitnessType(prev, next) {
		return prev, true
	}
	if family, ok := widenedPrimitiveFamily(prev, next); ok {
		return family, true
	}
	if widened, ok := widenedStableRecord(prev, next); ok {
		return widened, true
	}
	return normalizeWitnessUnion(prev, next), true
}

func sameWitnessType(a, b typ.Type) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if a.Hash() != b.Hash() {
		return false
	}
	return typ.SameNodeOrAcyclicEqual(a, b)
}

func Meet(a, b Value) Value {
	if a.state == bottom || b.state == bottom {
		return Bottom()
	}
	if a.state == top {
		return b
	}
	if b.state == top {
		return a
	}
	if witnessTypeLeq(a.t, b.t) {
		return a
	}
	if witnessTypeLeq(b.t, a.t) {
		return b
	}
	// Greatest lower bound in the witness-containment lattice: for each pair of
	// alternatives, keep the smaller when they are comparable. Incomparable pairs
	// (e.g. number vs string) contribute no common lower bound. No kept
	// alternative means no shared witness evidence, i.e. Bottom.
	var kept []typ.Type
	for _, am := range witnessAlternatives(a.t) {
		for _, bm := range witnessAlternatives(b.t) {
			switch {
			case scalarLeq(am, bm):
				kept = append(kept, am)
			case scalarLeq(bm, am):
				kept = append(kept, bm)
			}
		}
	}
	if len(kept) == 0 {
		return Bottom()
	}
	return Of(normalize.UnionForEvidence(kept...))
}

// witnessAlternatives returns the top-level alternatives of a witness type: the
// members of a union, or the type itself.
func witnessAlternatives(t typ.Type) []typ.Type {
	if u, ok := unwrap.Annotated(t).(*typ.Union); ok {
		return u.Members
	}
	return []typ.Type{t}
}

// scalarLeq reports containment between two single (non-union) witness
// alternatives: structural equality, or a literal contained in its family base.
func scalarLeq(a, b typ.Type) bool {
	if typ.SameNodeOrAcyclicEqual(a, b) {
		return true
	}
	if typ.TypeEquals(a, b) {
		return true
	}
	if opt, ok := unwrap.Annotated(b).(*typ.Optional); ok {
		return typ.TypeEquals(a, typ.Nil) || scalarLeq(a, opt.Inner)
	}
	if base, ok := typelit.FamilyBase(a); ok && typ.SameNodeOrAcyclicEqual(base, b) {
		return true
	}
	return false
}

// alternativeLeqType reports whether a single alternative is contained in some
// alternative of t.
func alternativeLeqType(alt typ.Type, t typ.Type) bool {
	for _, member := range witnessAlternatives(t) {
		if scalarLeq(alt, member) {
			return true
		}
	}
	return false
}

// witnessTypeLeq reports whether every alternative of a is contained in some
// alternative of b.
func witnessTypeLeq(a, b typ.Type) bool {
	if typ.SameNodeOrAcyclicEqual(a, b) {
		return true
	}
	for _, alt := range witnessAlternatives(a) {
		if !alternativeLeqType(alt, b) {
			return false
		}
	}
	return true
}

// normalizeWitnessUnion builds the canonical union of two witnesses'
// alternatives. UnionForEvidence flattens, deduplicates, drops members subsumed
// by a more general alternative (e.g. a literal under its present base), and
// hash-orders the result, so it is deterministic and a pure canonical set union
// over the combined alternatives. Set union with subsumption is associative.
func normalizeWitnessUnion(a, b typ.Type) typ.Type {
	alts := append(append([]typ.Type{}, witnessAlternatives(a)...), witnessAlternatives(b)...)
	return normalize.UnionForEvidence(alts...)
}

func Equal(a, b Value) bool {
	if a.state != b.state {
		return false
	}
	if a.state != concrete {
		return true
	}
	return typ.SameNodeOrAcyclicEqual(a.t, b.t)
}

func (v Value) Hash() uint64 {
	h := internal.MixHash(internal.FnvString("typewitness"), uint64(v.state))
	if v.state == concrete && v.t != nil {
		h = internal.MixHash(h, typ.EqualityHash(v.t))
	}
	return h
}
