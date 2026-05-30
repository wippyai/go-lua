package product

import (
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/axis/numeric"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/narrow"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

// transfer.go provides the AbstractValue-native form of the flow transfer
// functions (types/flow/transfer.go, query.go, structured_carry.go,
// mutable_store.go). Each primitive takes and returns AbstractValue (or product
// types) so the flow engine can later compute in the value domain with no
// typ.Type round-trip in its carriers.
//
// A primitive recovers the structural type from the shape axis only internally,
// through ProjectValue at the shape boundary, feeds it to the proven
// value-domain / query-core / narrow logic, and re-lifts the result with
// FromType. ProjectValue (not the bare Project) is the shape egress because it
// recombines the Presence axis nilability the value-domain field/index/narrow
// laws expect to see on the structural type. The contract of every primitive is
// AbstractValue-in / AbstractValue-out; typ.Type never appears in a signature.

// FieldOf is the AbstractValue-native form of a field read av.name.
//
// It corresponds to the field-read path of the flow transfer query
// (Solution.TypeAt / derivedTypeAt deriving a child field through
// query/core.Field). It returns the field value and whether the field resolves;
// a non-resolving field read reports ok=false, matching query/core.Field.
func FieldOf(av AbstractValue, name string) (AbstractValue, bool) {
	t := av.ProjectValue()
	if t == nil {
		return Bottom(), false
	}
	ft, ok := querycore.Field(t, name)
	if !ok || ft == nil {
		return Bottom(), false
	}
	return FromType(ft), true
}

// WithField is the AbstractValue-native form of a field write/overlay
// av.name = fieldAV.
//
// It corresponds to the field-write transfer (flow setValueTemplateSlot's
// record/field branch and the structured field-overlay write) over the record
// shape carried by av. The field slot is replaced (not joined) with fieldAV's
// structural content via typ.ExtendRecordWithField, which adds the field when av
// carries none by that name yet, matching Lua field assignment that admits a
// fresh key, and preserves the record's open flag, metatable, and map component.
func WithField(av AbstractValue, name string, fieldAV AbstractValue) AbstractValue {
	base := av.ProjectValue()
	fieldType := fieldAV.ProjectValue()
	return FromType(typ.ExtendRecordWithField(base, name, fieldType))
}

// IndexOf is the AbstractValue-native form of a dynamic index read av[keyAV].
//
// It corresponds to the index-read transfer (flow mapElementTypeAt /
// lengthIndexReadType resolving t[k] through query/core.Index). It returns the
// element value and whether the index resolves; Lua missing-key nilability is
// already folded onto the result by query/core.Index, so it survives the
// FromType round-trip onto the presence axis.
func IndexOf(av AbstractValue, keyAV AbstractValue) (AbstractValue, bool) {
	t := av.ProjectValue()
	if t == nil {
		return Bottom(), false
	}
	key := keyAV.ProjectValue()
	et, ok := querycore.Index(t, key)
	if !ok || et == nil {
		return Bottom(), false
	}
	return FromType(et), true
}

// WriteIndex is the AbstractValue-native form of an indexed write av[keyAV] = valAV.
//
// It corresponds to the map-mutator write transfer
// (processMapMutatorAssignmentReturnKey with MapMutationValueWrite, which calls
// value.AdmitIndexedWrite then value.MergeForConvergence). The container is
// widened to admit the key/value, then convergence-merged against its prior
// shape so the result is a stable fixpoint iterate.
func WriteIndex(av AbstractValue, keyAV AbstractValue, valAV AbstractValue) AbstractValue {
	current := av.ProjectValue()
	key := keyAV.ProjectValue()
	val := valAV.ProjectValue()
	written := value.AdmitIndexedWrite(current, key, val)
	return FromType(value.MergeForConvergence(current, written))
}

// WriteIndexForeign is the AbstractValue-native form of an indexed write
// av[keyAV] = valAV whose value is a FOREIGN value — not provably drawn from av at
// the written key. It is WriteIndex over value.AdmitForeignIndexedWrite: a closed
// record's declared field the key could match is weakened by valAV (the foreign
// store can replace that field at runtime), where the plain WriteIndex would leave
// the field type intact and only add a map component. A self-derived write (the
// value provably read from av at the same key) keeps plain WriteIndex, so its fields
// are not weakened.
func WriteIndexForeign(av AbstractValue, keyAV AbstractValue, valAV AbstractValue) AbstractValue {
	current := av.ProjectValue()
	key := keyAV.ProjectValue()
	val := valAV.ProjectValue()
	written := value.AdmitForeignIndexedWrite(current, key, val)
	return FromType(value.MergeForConvergence(current, written))
}

// IndexWriteAdmits is the AbstractValue-native form of the indexed-write
// admission predicate.
//
// It corresponds to mapMutationAdmits with MapMutationValueWrite
// (value.IndexedWriteAdmits): whether the value domain can soundly admit
// av[keyAV] = valAV before WriteIndex computes the next container.
func IndexWriteAdmits(av AbstractValue, keyAV AbstractValue, valAV AbstractValue) bool {
	return value.IndexedWriteAdmits(av.ProjectValue(), keyAV.ProjectValue(), valAV.ProjectValue())
}

// MutateIndex is the AbstractValue-native form of a structural mutation inside an
// indexed element, av[keyAV].field = ... .
//
// It corresponds to the map-mutator update transfer
// (processMapMutatorAssignmentReturnKey with MapMutationValueUpdate, which calls
// value.AdmitIndexedValueMutation then value.MergeForConvergence). Unlike
// WriteIndex the incoming value is a patch joined into the element slot rather
// than a replacement.
func MutateIndex(av AbstractValue, keyAV AbstractValue, valAV AbstractValue) AbstractValue {
	current := av.ProjectValue()
	key := keyAV.ProjectValue()
	val := valAV.ProjectValue()
	mutated := value.AdmitIndexedValueMutation(current, key, val)
	return FromType(value.MergeForConvergence(current, mutated))
}

// IndexMutateAdmits is the AbstractValue-native form of the indexed value
// mutation admission predicate.
//
// It corresponds to mapMutationAdmits with MapMutationValueUpdate
// (value.IndexedValueMutationAdmits).
func IndexMutateAdmits(av AbstractValue, keyAV AbstractValue, valAV AbstractValue) bool {
	return value.IndexedValueMutationAdmits(av.ProjectValue(), keyAV.ProjectValue(), valAV.ProjectValue())
}

// AppendElement is the AbstractValue-native form of an array-element mutation
// av[#av+1] = elemAV (table.insert-like).
//
// It corresponds to the table-mutator transfer with a direct array target
// (processTableMutatorAssignmentReturnKey calling
// value.AdmitArrayElementMutation with typ.JoinPreferNonSoft, then
// value.MergeForConvergence). The array element domain is joined with elemAV's
// content and the result convergence-merged against the prior shape.
func AppendElement(av AbstractValue, elemAV AbstractValue) AbstractValue {
	current := av.ProjectValue()
	elem := elemAV.ProjectValue()
	widened := value.AdmitArrayElementMutation(current, elem, typ.JoinPreferNonSoft)
	return FromType(value.MergeForConvergence(current, widened))
}

// AppendMapElement is the AbstractValue-native form of an array-element mutation
// through a dynamic map key, av[keyAV][#...+1] = elemAV.
//
// It corresponds to the table-mutator transfer with a keyed target
// (processTableMutatorAssignmentReturnKey calling
// value.AdmitMapArrayElementMutation, then value.MergeForConvergence): the value
// slot of the map at keyAV is array-widened by elemAV.
func AppendMapElement(av AbstractValue, keyAV AbstractValue, elemAV AbstractValue) AbstractValue {
	current := av.ProjectValue()
	key := keyAV.ProjectValue()
	elem := elemAV.ProjectValue()
	widened := value.AdmitMapArrayElementMutation(current, key, elem)
	return FromType(value.MergeForConvergence(current, widened))
}

// PhiJoin is the AbstractValue-native form of a phi-node join across
// predecessors (processJoinReturnChangedKeys / joinPhiEquation lifting
// typ/join.Types over the operand types).
//
// Joining no operands yields Bottom; one operand returns it unchanged; otherwise
// the operands fold under the product Join, which is the component-wise least
// upper bound (its shape axis already delegates to the same convergence merge the
// flow join over-approximates with). Phi join is commutative and associative
// because product Join is.
func PhiJoin(operands ...AbstractValue) AbstractValue {
	if len(operands) == 0 {
		return Bottom()
	}
	out := operands[0]
	for _, op := range operands[1:] {
		out = Join(out, op)
	}
	return out
}

// CarryForward is the AbstractValue-native form of the structured-carry merge
// that seeds a new symbol version with predecessor facts
// (structured_carry.go / mutable_store.go joining predecessor root and suffix
// facts at a fixpoint boundary).
//
// It is value.MergeForConvergence lifted onto AbstractValue: the existing carried
// fact merged with the candidate predecessor fact to a stable iterate. It is the
// store-merge counterpart of PhiJoin, kept distinct because the store uses the
// convergence widening merge directly (joinPredecessorMutableState's
// ConvergenceWidening.Merge) rather than the plain phi join.
func CarryForward(prev AbstractValue, next AbstractValue) AbstractValue {
	return FromType(value.MergeForConvergence(prev.ProjectValue(), next.ProjectValue()))
}

// NarrowTruthy is the AbstractValue-native form of then-branch truthiness
// refinement (the value side of applyCondition for a Truthy constraint, which
// removes nil and false via narrow.ToTruthy).
//
// Removing nil from the shape also removes the nilability the presence axis
// carries, so the result is re-lifted through FromType, which re-factors the
// (now non-nilable) shape back onto the presence axis as Present.
func NarrowTruthy(av AbstractValue) AbstractValue {
	narrowed := narrow.ToTruthy(av.ProjectValue())
	if narrowed == nil {
		return Bottom()
	}
	return FromType(narrowed)
}

// NarrowFalsy is the AbstractValue-native form of else-branch falsy refinement
// (the value side of applyCondition for a Falsy constraint, narrow.ToFalsy
// keeping only nil and false).
func NarrowFalsy(av AbstractValue) AbstractValue {
	narrowed := narrow.ToFalsy(av.ProjectValue())
	if narrowed == nil {
		return Bottom()
	}
	return FromType(narrowed)
}

// NarrowPresent is the AbstractValue-native form of not-nil refinement (the value
// side of applyCondition for a NotNil constraint, narrow.RemoveNil), used for
// x ~= nil narrowing and for present-key map reads (mapElementPresenceTypeAt).
//
// It strips nil from the shape; FromType then re-factors the non-nilable shape
// onto the presence axis as Present, so the presence transition (Maybe -> Present)
// is recorded natively.
func NarrowPresent(av AbstractValue) AbstractValue {
	narrowed := narrow.RemoveNil(av.ProjectValue())
	if narrowed == nil {
		return Bottom()
	}
	return FromType(narrowed)
}

// FilterByKind is the AbstractValue-native form of positive typeof narrowing
// type(x) == k (the value side of applyCondition for a HasType/kind constraint,
// narrow.FilterByKind). It takes a Lua typeof kind, not a typ.Type, so the
// AbstractValue-in/AbstractValue-out contract holds.
func FilterByKind(av AbstractValue, k kind.Kind) AbstractValue {
	narrowed := narrow.FilterByKind(av.ProjectValue(), k)
	if narrowed == nil {
		return Bottom()
	}
	return FromType(narrowed)
}

// ExcludeByKind is the AbstractValue-native form of negative typeof narrowing
// type(x) ~= k (the value side of applyCondition for a NotHasType/kind
// constraint, narrow.ExcludeKind).
func ExcludeByKind(av AbstractValue, k kind.Kind) AbstractValue {
	narrowed := narrow.ExcludeKind(av.ProjectValue(), k)
	if narrowed == nil {
		return Bottom()
	}
	return FromType(narrowed)
}

// RefineNumeric is the AbstractValue-native form of numeric refinement (the
// numeric side of the transfer: applyPointNumericShapeProjection narrows the
// index/integer domain, e.g. from a proven length bound or a comparison guard).
//
// The refinement is the numeric-axis meet of av's interval with bound: meeting
// keeps only integers in both. The reduced product then propagates an empty
// (Bottom) numeric set to presence Bottom through reduceNumericPresence, so an
// unsatisfiable refinement makes the whole value unreachable. The shape and
// presence inputs are otherwise carried through unchanged except for that
// cross-axis reduction.
func RefineNumeric(av AbstractValue, bound numeric.Value) AbstractValue {
	refined := numericMeet(av.Numeric(), bound)
	return New(
		av.Shape(),
		av.Presence(),
		refined,
		av.Effects(),
		av.Ownership(),
		av.Escape(),
		av.Identity(),
		av.Evidence(),
	)
}

// numericMeet is the greatest lower bound of two numeric values: the intersection
// of their integer sets. The numeric axis exposes Join (least upper bound) and
// Covers but not meet, so the meet is derived from them. When one value covers
// the other, the covered (lower) value is the meet; otherwise the meet is the
// intersection of the two closed intervals, and Bottom when they do not overlap.
func numericMeet(a, b numeric.Value) numeric.Value {
	if a.IsBottom() || b.IsBottom() {
		return numeric.Bottom()
	}
	if a.Covers(b) {
		return b
	}
	if b.Covers(a) {
		return a
	}
	aLow, aHigh := a.Interval()
	bLow, bHigh := b.Interval()
	low := aLow
	if bLow > low {
		low = bLow
	}
	high := aHigh
	if bHigh < high {
		high = bHigh
	}
	if low > high {
		return numeric.Bottom()
	}
	return numeric.Range(low, high)
}
