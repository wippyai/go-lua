package typewitness

import (
	"sync"

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
		Key:           Key,
		Bottom:        Bottom,
		Top:           Top,
		Equal:         Equal,
		LessOrEq:      LessOrEq,
		Join:          Join,
		Meet:          Meet,
		Widen:         Widen,
		Hash:          Value.Hash,
		Retention:     axis.ValidatedRetention(retentionSafe),
		Canonical:     canonicalDescriptor(),
		Boundary:      axis.PortableIdentity,
		Reducer:       reduceByRuntimeKind,
		ReducerReads:  []string{Key.ID(), runtimekind.Key.ID()},
		ReducerWrites: []string{Key.ID()},
	}
}

// retentionSafe admits only exact package-owned singleton identities. Kind is
// intentionally insufficient: Type is an open interface, so a caller can forge
// kind.String while retaining mutable state. Every pointer-backed literal,
// recursive, nominal, and composite remains transaction-local until portable
// type ownership and sealing provides a mechanical retention proof.
func retentionSafe(value Value) bool {
	if value.t == nil {
		return value.recursive == nil || value.recursive == bottomSignature
	}
	return value.t == typ.Nil || value.t == typ.Boolean || value.t == typ.Number ||
		value.t == typ.Integer || value.t == typ.String || value.t == typ.Any ||
		value.t == typ.Unknown || value.t == typ.Never
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
	if !ok || tw.t == nil {
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
	t         typ.Type
	recursive *recursiveSignature
}

// recursiveSignature is interned because recursive identity sets are immutable
// canonical values. Keeping the 80-byte typ signature behind this pointer keeps
// the frequently erased Value small while preserving the exact, stable hash
// representation used outside this package.
type recursiveSignature struct {
	signature typ.RecursiveIdentitySignature
}

var recursiveSignatures = struct {
	mu      sync.Mutex
	byValue map[typ.RecursiveIdentitySignature]*recursiveSignature
}{
	byValue: make(map[typ.RecursiveIdentitySignature]*recursiveSignature),
}

// bottomSignature is a private sentinel. Concrete values always have a type,
// Top has neither field, and Bottom has this sentinel; that encodes all three
// states without growing Value past its type interface plus one pointer.
var bottomSignature = &recursiveSignature{}

func internRecursiveSignature(signature typ.RecursiveIdentitySignature) *recursiveSignature {
	recursiveSignatures.mu.Lock()
	defer recursiveSignatures.mu.Unlock()
	if interned := recursiveSignatures.byValue[signature]; interned != nil {
		return interned
	}
	interned := &recursiveSignature{signature: signature}
	recursiveSignatures.byValue[signature] = interned
	return interned
}

func Bottom() Value { return Value{recursive: bottomSignature} }
func Top() Value    { return Value{} }

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
	value := Value{t: t}
	if typ.ContainsRecursive(t) {
		if sig, ok := typ.RecursiveIdentitySignatureOf(t); ok {
			value.recursive = internRecursiveSignature(sig)
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

func (v Value) IsBottom() bool { return v.t == nil && v.recursive == bottomSignature }
func (v Value) IsTop() bool    { return v.t == nil && v.recursive == nil }

func (v Value) Type() (typ.Type, bool) {
	if v.t == nil {
		return nil, false
	}
	return v.t, true
}

func Join(a, b Value) Value {
	if a.IsBottom() {
		return b
	}
	if b.IsBottom() {
		return a
	}
	if a.IsTop() || b.IsTop() {
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
	if a.IsBottom() || b.IsTop() {
		return true
	}
	if a.IsTop() || b.IsBottom() {
		return false
	}
	return witnessTypeLeq(a.t, b.t)
}

// Widen is a true widening: once a concrete witness strictly grows it jumps to
// a stable primitive family when possible, otherwise Top, so ascending chains
// under repeated Widen are eventually stationary. Join synthesizes unions, so
// Widen cannot remain Join without risking unbounded witness growth at loop
// heads; but literal scalar growth such as 0, 1, 2 should widen to integer, not
// erase the proof entirely. A record shape expansion widens to an open record:
// it keeps the known optional fields from the two inputs while admitting later
// arbitrary fields without extending the record shape again.
func Widen(prev, next Value) Value {
	if prev.IsBottom() {
		return next
	}
	if next.IsBottom() {
		return prev
	}
	if prev.IsTop() || next.IsTop() {
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
		return widenSequenceElement(array.Element, t.Element), true
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
		out = widenSequenceElement(out, tupleElement)
	}
	return out, true
}

// widenSequenceElement preserves nested collection precision where the same
// stable abstraction exists. All other element-shape changes use the existing
// nested any type: unlike a growing union, it is a stationary upper bound while
// retaining the outer array witness.
func widenSequenceElement(prev, next typ.Type) typ.Type {
	if sameWitnessType(prev, next) {
		return prev
	}
	if family, ok := widenedPrimitiveFamily(prev, next); ok {
		return family
	}
	if widened, ok := widenedStableSequence(prev, next); ok {
		return widened
	}
	if widened, ok := widenedStableRecord(prev, next); ok {
		return widened
	}
	return typ.Any
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
	if !sameWitnessType(ar.Metatable, br.Metatable) {
		return nil, false
	}
	sameMap := ar.HasMapComponent() && br.HasMapComponent() && sameWitnessType(ar.MapKey, br.MapKey)
	var mapKey, mapValue typ.Type
	if sameMap {
		widened, ok := widenRecordMemberType(ar.MapValue, br.MapValue)
		if !ok {
			return nil, false
		}
		mapKey = ar.MapKey
		mapValue = widened
	} else {
		// With no shared map shape, leave both map pieces nil. When either input
		// has a differing map component, the open result below carries unknown
		// access evidence instead of incorrectly requiring either branch's map.
	}
	fields, ok := widenedStableRecordFields(ar.Fields, br.Fields)
	if !ok {
		return nil, false
	}
	members, ok := widenedStableRecordMembers(ar.StaticMembers, br.StaticMembers)
	if !ok {
		return nil, false
	}
	shapeExpanded := len(fields) > len(ar.Fields) || len(fields) > len(br.Fields) ||
		len(members) > len(ar.StaticMembers) || len(members) > len(br.StaticMembers) ||
		ar.HasMapComponent() != br.HasMapComponent() ||
		(ar.HasMapComponent() && br.HasMapComponent() && !sameMap)
	return typ.RebuildRecord(typ.RecordParts{
		Fields:        fields,
		StaticMembers: members,
		Metatable:     ar.Metatable,
		MapKey:        mapKey,
		MapValue:      mapValue,
		Open:          ar.Open || br.Open || shapeExpanded,
		AssumeSorted:  true,
	}), true
}

func widenedStableRecordFields(a, b []typ.Field) ([]typ.Field, bool) {
	out := make([]typ.Field, 0, len(a)+len(b))
	i, j := 0, 0
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
				return nil, false
			}
			widened, ok := widenRecordMemberType(a[i].Type, b[j].Type)
			if !ok {
				return nil, false
			}
			field := a[i]
			field.Type = widened
			field.Optional = a[i].Optional || b[j].Optional
			out = append(out, field)
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
	return out, true
}

func optionalBranchField(field typ.Field) typ.Field {
	field.Optional = true
	return field
}

func widenedStableRecordMembers(a, b []typ.StaticMember) ([]typ.StaticMember, bool) {
	out := make([]typ.StaticMember, 0, len(a)+len(b))
	i, j := 0, 0
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
					return nil, false
				}
				widened, ok := widenRecordMemberType(a[i].Type, b[j].Type)
				if !ok {
					return nil, false
				}
				member := a[i]
				member.Type = widened
				member.Optional = a[i].Optional || b[j].Optional
				out = append(out, member)
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
	return out, true
}

func optionalBranchStaticMember(member typ.StaticMember) typ.StaticMember {
	member.Optional = true
	return member
}

func widenRecordMemberType(prev, next typ.Type) (typ.Type, bool) {
	if sameWitnessType(prev, next) {
		return prev, true
	}
	if witnessTypeLeq(next, prev) {
		return prev, true
	}
	if witnessTypeLeq(prev, next) {
		return next, true
	}
	if optional, ok := unwrap.Annotated(prev).(*typ.Optional); ok {
		widened, _ := widenRecordMemberType(optional.Inner, next)
		return typ.MaterializeOptional(widened), true
	}
	if optional, ok := unwrap.Annotated(next).(*typ.Optional); ok {
		widened, _ := widenRecordMemberType(prev, optional.Inner)
		return typ.MaterializeOptional(widened), true
	}
	if typ.TypeEquals(prev, typ.Nil) {
		return typ.MaterializeOptional(next), true
	}
	if typ.TypeEquals(next, typ.Nil) {
		return typ.MaterializeOptional(prev), true
	}
	if family, ok := widenedPrimitiveFamily(prev, next); ok {
		return family, true
	}
	if widened, ok := widenedStableRecord(prev, next); ok {
		return widened, true
	}
	if widened, ok := widenedStableSequence(prev, next); ok {
		return widened, true
	}
	// A widening must not retain an unbounded series of unrelated member
	// alternatives. The surrounding record preserves the access-path evidence;
	// nested any is the stable upper bound for the member itself.
	return typ.Any, true
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
	if a.IsBottom() || b.IsBottom() {
		return Bottom()
	}
	if a.IsTop() {
		return b
	}
	if b.IsTop() {
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
	if typ.IsAny(b) {
		return true
	}
	if emptyRecordArrayLeq(a, b) {
		return true
	}
	if sequenceWitnessLeq(a, b) {
		return true
	}
	if recordWitnessLeq(a, b) {
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

// sequenceWitnessLeq is the structural containment needed by the array
// widenings. Array elements are covariant evidence, tuples are contained when
// each position is, and an empty closed record is the table literal witness for
// an empty array.
func sequenceWitnessLeq(a, b typ.Type) bool {
	target, ok := unwrap.Annotated(b).(*typ.Array)
	if !ok {
		return false
	}
	switch source := unwrap.Annotated(a).(type) {
	case *typ.Array:
		return witnessTypeLeq(source.Element, target.Element)
	case *typ.Tuple:
		for _, element := range source.Elements {
			if !witnessTypeLeq(element, target.Element) {
				return false
			}
		}
		return true
	case *typ.Record:
		return emptyStableRecord(source)
	default:
		return false
	}
}

// recordWitnessLeq recognizes the structural upper bounds constructed by
// widenedStableRecord. A target open record absorbs source-only members; known
// target members still have to be present when required and contain the source
// evidence. The relation deliberately keeps metatable, map-key, and readonly
// identities exact because widening never changes them while retaining a map
// component.
func recordWitnessLeq(a, b typ.Type) bool {
	source, sourceOK := unwrap.Annotated(a).(*typ.Record)
	target, targetOK := unwrap.Annotated(b).(*typ.Record)
	if !sourceOK || !targetOK ||
		(source.Open && !target.Open) ||
		!sameWitnessType(source.Metatable, target.Metatable) ||
		!recordMapWitnessLeq(source, target) {
		return false
	}
	return recordFieldsWitnessLeq(source.Fields, target.Fields, target.Open) &&
		recordStaticMembersWitnessLeq(source.StaticMembers, target.StaticMembers, target.Open)
}

func recordMapWitnessLeq(source, target *typ.Record) bool {
	switch {
	case !source.HasMapComponent() && !target.HasMapComponent():
		return true
	case source.HasMapComponent() && target.HasMapComponent():
		return sameWitnessType(source.MapKey, target.MapKey) &&
			witnessTypeLeq(source.MapValue, target.MapValue)
	case source.HasMapComponent() && !target.HasMapComponent():
		return target.Open
	default:
		return false
	}
}

func recordFieldsWitnessLeq(source, target []typ.Field, targetOpen bool) bool {
	i, j := 0, 0
	for i < len(source) || j < len(target) {
		switch {
		case i >= len(source):
			if !target[j].Optional {
				return false
			}
			j++
		case j >= len(target):
			if !targetOpen {
				return false
			}
			i++
		case source[i].Name < target[j].Name:
			if !targetOpen {
				return false
			}
			i++
		case source[i].Name > target[j].Name:
			if !target[j].Optional {
				return false
			}
			j++
		default:
			if (source[i].Optional && !target[j].Optional) ||
				source[i].Readonly != target[j].Readonly ||
				!witnessTypeLeq(source[i].Type, target[j].Type) {
				return false
			}
			i++
			j++
		}
	}
	return true
}

func recordStaticMembersWitnessLeq(source, target []typ.StaticMember, targetOpen bool) bool {
	i, j := 0, 0
	for i < len(source) || j < len(target) {
		switch {
		case i >= len(source):
			if !target[j].Optional {
				return false
			}
			j++
		case j >= len(target):
			if !targetOpen {
				return false
			}
			i++
		default:
			cmp := typ.CompareStaticMembers(source[i], target[j])
			switch {
			case cmp < 0:
				if !targetOpen {
					return false
				}
				i++
			case cmp > 0:
				if !target[j].Optional {
					return false
				}
				j++
			default:
				if (source[i].Optional && !target[j].Optional) ||
					source[i].Readonly != target[j].Readonly ||
					!witnessTypeLeq(source[i].Type, target[j].Type) {
					return false
				}
				i++
				j++
			}
		}
	}
	return true
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
	if a.t == nil || b.t == nil {
		return a.t == nil && b.t == nil && a.recursive == b.recursive
	}
	if a.recursive != nil && b.recursive != nil {
		if !a.recursive.signature.Equal(b.recursive.signature) {
			return false
		}
		return typ.TypeEquals(a.t, b.t)
	}
	return typ.SameNodeOrRecursiveIdentityEqual(a.t, b.t)
}

func (v Value) Hash() uint64 {
	kind := top
	if v.t != nil {
		kind = concrete
	} else if v.IsBottom() {
		kind = bottom
	}
	h := internal.MixHash(internal.FnvString("typewitness"), uint64(kind))
	if v.t != nil {
		if v.recursive != nil {
			h = internal.MixHash(h, internal.FnvString("recursive.identity"))
			h = internal.MixHash(h, uint64(v.recursive.signature.SmallLen))
			for i := 0; i < v.recursive.signature.SmallLen; i++ {
				h = internal.MixHash(h, v.recursive.signature.Small[i])
			}
		}
		h = internal.MixHash(h, typ.EqualityHash(v.t))
	}
	return h
}
