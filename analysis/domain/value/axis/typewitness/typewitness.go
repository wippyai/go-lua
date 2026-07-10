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
	state       state
	t           typ.Type
	recursiveOK bool
	recursive   typ.RecursiveIdentitySignature
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
	value := Value{state: concrete, t: t}
	if typ.ContainsRecursive(t) {
		if sig, ok := typ.RecursiveIdentitySignatureOf(t); ok {
			value.recursiveOK = true
			value.recursive = sig
		}
	}
	return value
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
	if widened, ok := widenedStableSequence(prev.t, next.t); ok {
		return Of(widened)
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

func widenedStableSequence(a, b typ.Type) (typ.Type, bool) {
	if element, ok := widenedArrayElement(a, b); ok {
		return typ.NewArray(element), true
	}
	if element, ok := widenedArrayElement(b, a); ok {
		return typ.NewArray(element), true
	}
	return nil, false
}

func widenedArrayElement(arrayLike, other typ.Type) (typ.Type, bool) {
	array, ok := unwrap.Annotated(arrayLike).(*typ.Array)
	if !ok {
		return nil, false
	}
	switch t := unwrap.Annotated(other).(type) {
	case *typ.Array:
		return widenRecordMemberType(array.Element, t.Element)
	case *typ.Tuple:
		return widenedArrayTupleElement(array.Element, t.Elements)
	case *typ.Record:
		if emptyStableRecord(t) {
			return widenedPrimitiveOrSelf(array.Element), true
		}
	}
	return nil, false
}

func widenedArrayTupleElement(element typ.Type, tuple []typ.Type) (typ.Type, bool) {
	out := widenedPrimitiveOrSelf(element)
	for _, tupleElement := range tuple {
		var ok bool
		out, ok = widenRecordMemberType(out, tupleElement)
		if !ok {
			return nil, false
		}
	}
	return out, true
}

func widenedPrimitiveOrSelf(t typ.Type) typ.Type {
	if base, ok := typelit.FamilyBase(t); ok {
		return base
	}
	return t
}

func emptyStableRecord(r *typ.Record) bool {
	return r != nil &&
		len(r.Fields) == 0 &&
		len(r.StaticMembers) == 0 &&
		r.Metatable == nil &&
		!r.Open &&
		!r.HasMapComponent()
}

func widenedStableRecord(a, b typ.Type) (typ.Type, bool) {
	ar, aOK := unwrap.Annotated(a).(*typ.Record)
	br, bOK := unwrap.Annotated(b).(*typ.Record)
	if !aOK || !bOK {
		return nil, false
	}
	if ar.Open != br.Open ||
		!sameWitnessType(ar.Metatable, br.Metatable) ||
		!sameWitnessType(ar.MapKey, br.MapKey) {
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
	fields, commonFields, ok := widenedStableRecordFields(ar.Fields, br.Fields)
	if !ok {
		return nil, false
	}
	members, commonMembers, ok := widenedStableRecordMembers(ar.StaticMembers, br.StaticMembers)
	if !ok {
		return nil, false
	}
	if commonFields+commonMembers == 0 {
		return nil, false
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

func widenedStableRecordFields(a, b []typ.Field) ([]typ.Field, int, bool) {
	out := make([]typ.Field, 0, len(a)+len(b))
	i, j := 0, 0
	common := 0
	for i < len(a) || j < len(b) {
		switch {
		case i >= len(a):
			out = append(out, optionalBranchField(b[j]))
			j++
		case j >= len(b):
			out = append(out, optionalBranchField(a[i]))
			i++
		case a[i].Name == b[j].Name:
			if a[i].Readonly != b[j].Readonly {
				return nil, 0, false
			}
			widened, ok := widenRecordMemberType(a[i].Type, b[j].Type)
			if !ok {
				return nil, 0, false
			}
			field := a[i]
			field.Type = widened
			field.Optional = a[i].Optional || b[j].Optional
			out = append(out, field)
			common++
			i++
			j++
		case a[i].Name < b[j].Name:
			out = append(out, optionalBranchField(a[i]))
			i++
		default:
			out = append(out, optionalBranchField(b[j]))
			j++
		}
	}
	return out, common, true
}

func optionalBranchField(field typ.Field) typ.Field {
	field.Optional = true
	return field
}

func widenedStableRecordMembers(a, b []typ.StaticMember) ([]typ.StaticMember, int, bool) {
	out := make([]typ.StaticMember, 0, len(a)+len(b))
	i, j := 0, 0
	common := 0
	for i < len(a) || j < len(b) {
		switch {
		case i >= len(a):
			out = append(out, optionalBranchStaticMember(b[j]))
			j++
		case j >= len(b):
			out = append(out, optionalBranchStaticMember(a[i]))
			i++
		default:
			cmp := typ.CompareStaticMembers(a[i], b[j])
			switch {
			case cmp == 0:
				if a[i].Readonly != b[j].Readonly {
					return nil, 0, false
				}
				widened, ok := widenRecordMemberType(a[i].Type, b[j].Type)
				if !ok {
					return nil, 0, false
				}
				member := a[i]
				member.Type = widened
				member.Optional = a[i].Optional || b[j].Optional
				out = append(out, member)
				common++
				i++
				j++
			case cmp < 0:
				out = append(out, optionalBranchStaticMember(a[i]))
				i++
			default:
				out = append(out, optionalBranchStaticMember(b[j]))
				j++
			}
		}
	}
	return out, common, true
}

func optionalBranchStaticMember(member typ.StaticMember) typ.StaticMember {
	member.Optional = true
	return member
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
	if emptyRecordArrayLeq(a, b) {
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

func emptyRecordArrayLeq(a, b typ.Type) bool {
	record, recordOK := unwrap.Annotated(a).(*typ.Record)
	if !recordOK || !emptyStableRecord(record) {
		return false
	}
	_, arrayOK := unwrap.Annotated(b).(*typ.Array)
	return arrayOK
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
	if a.recursiveOK && b.recursiveOK {
		if !a.recursive.Equal(b.recursive) {
			return false
		}
		return typ.TypeEquals(a.t, b.t)
	}
	return typ.SameNodeOrRecursiveIdentityEqual(a.t, b.t)
}

func (v Value) Hash() uint64 {
	h := internal.MixHash(internal.FnvString("typewitness"), uint64(v.state))
	if v.state == concrete && v.t != nil {
		if v.recursiveOK {
			h = internal.MixHash(h, internal.FnvString("recursive.identity"))
			h = internal.MixHash(h, uint64(v.recursive.SmallLen))
			for i := 0; i < v.recursive.SmallLen; i++ {
				h = internal.MixHash(h, v.recursive.Small[i])
			}
		}
		h = internal.MixHash(h, typ.EqualityHash(v.t))
	}
	return h
}
