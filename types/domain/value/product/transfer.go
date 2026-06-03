package product

import (
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/axis/numeric"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/narrow"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// transfer.go provides the AbstractValue-native primitives used by flow
// transfer. Each primitive takes and returns AbstractValue (or product types) so
// the flow engine computes in the value domain with no typ.Type round-trip in
// its carriers.
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
// It returns the field value and whether the field resolves; a non-resolving
// field read reports ok=false, matching query/core.Field.
func FieldOf(av AbstractValue, name string) (AbstractValue, bool) {
	t := av.ProjectValue()
	if t == nil {
		return Bottom(), false
	}
	ft, ok := querycore.Field(t, name)
	if !ok || ft == nil {
		return Bottom(), false
	}
	if av.IsGradualTop() && typ.IsAny(ft) {
		return GradualAny(), true
	}
	return FromType(ft), true
}

// MemberOf is the structural-keyed member read. Dot fields dispatch to FieldOf;
// static string/int indexes dispatch to IndexOf with an exact literal key. This
// keeps product callers from collapsing `.x` and `["x"]` before the value-domain
// boundary decides which operation is intended.
func MemberOf(av AbstractValue, key value.MemberKey) (AbstractValue, bool) {
	if !key.IsValid() {
		return Bottom(), false
	}
	switch key.Kind() {
	case value.MemberKindField:
		return FieldOf(av, key.Name())
	case value.MemberKindStringIndex:
		return IndexOf(av, FromType(typ.LiteralString(key.Name())))
	case value.MemberKindIntIndex:
		return IndexOf(av, FromType(typ.LiteralInt(int64(key.Index()))))
	default:
		return Bottom(), false
	}
}

// RuntimeMemberOf is the Lua-runtime read counterpart of MemberOf. MemberOf is a
// strict structural query: it reports ok=false for a precise table shape that does
// not declare the requested member, so diagnostics can still flag likely typos.
// RuntimeMemberOf is for transfer semantics: reading a missing slot from a
// table-like value produces nil, while a non-table miss remains unresolved.
func RuntimeMemberOf(av AbstractValue, key value.MemberKey) (AbstractValue, bool) {
	if !key.IsValid() {
		return Bottom(), false
	}
	t, ok := runtimeMemberType(av.ProjectValue(), key, 0)
	if !ok || t == nil {
		return Bottom(), false
	}
	if av.IsGradualTop() && typ.IsAny(t) {
		return GradualAny(), true
	}
	return FromType(t), true
}

// RuntimeIndexOf is the Lua-runtime read counterpart of IndexOf. It preserves
// strict index precision when available and falls back to nil for definitely
// absent table-like slots.
func RuntimeIndexOf(av AbstractValue, key AbstractValue) (AbstractValue, bool) {
	keyType := key.ProjectValue()
	t, ok := querycore.RuntimeIndex(av.ProjectValue(), keyType)
	if !ok || t == nil {
		return Bottom(), false
	}
	if av.IsGradualTop() && typ.IsAny(t) {
		return GradualAny(), true
	}
	return FromType(t), true
}

// RefineCallableValue sharpens av's callable arm to sig without changing its
// presence proof. Function identity is not a presence proof: a value known only
// as F? remains F? after signature projection, while a definitely-present
// callable becomes the solved signature.
func RefineCallableValue(av AbstractValue, sig typ.Type) AbstractValue {
	if typ.IsAbsentOrUnknown(sig) {
		return av
	}
	if av.IsZero() {
		return FromType(sig)
	}
	t := ProjectValueOrUnknown(av)
	if typ.IsAbsentOrUnknown(t) {
		return FromType(sig)
	}
	if inner, nilable := value.SplitNilable(t); nilable {
		if inner == nil || !callableValueCanRefine(inner) {
			return av
		}
		return FromType(typ.NewOptional(sig))
	}
	if callableValueCanRefine(t) {
		return FromType(sig)
	}
	return av
}

// RefineCallableRead overlays callable identity/signature evidence onto the
// result of a runtime slot read. Unlike RefineCallableValue, this function treats
// absence/nil as "maybe callable" rather than as must-present: FunctionRefs name
// a callable but do not prove that a map/table slot is present at this program
// point. Presence must come from the product value, StaticMembers, or
// KeyPresence/index-read proof.
func RefineCallableRead(read AbstractValue, sig typ.Type) AbstractValue {
	if typ.IsAbsentOrUnknown(sig) {
		return read
	}
	if read.IsZero() {
		return FromType(typ.NewOptional(sig))
	}
	t := ProjectValueOrUnknown(read)
	if typ.IsAbsentOrUnknown(t) || typ.IsUnknown(t) || typ.TypeEquals(t, typ.Nil) {
		return FromType(typ.NewOptional(sig))
	}
	return RefineCallableValue(read, sig)
}

func callableValueCanRefine(t typ.Type) bool {
	if t == nil {
		return false
	}
	return typ.IsUnknown(t) || typ.IsAny(t) || unwrap.Function(t) != nil
}

// WithMetatable is the AbstractValue-native form of Lua setmetatable(t, mt).
// It preserves the table value while attaching mt on the structural metatable
// axis, so subsequent field/method reads use the ordinary query-core metatable
// lookup. Non-record table values keep their original shape because the current
// structural domain can represent metatables only on records.
func WithMetatable(av, meta AbstractValue) (AbstractValue, bool) {
	if av.IsZero() || meta.IsZero() {
		return Bottom(), false
	}
	t := withMetatableType(av.ProjectValue(), meta.ProjectValue())
	if t == nil || typ.IsAbsentOrUnknown(t) {
		return Bottom(), false
	}
	if av.IsGradualTop() && typ.IsAny(t) {
		return GradualAny(), true
	}
	return FromType(t), true
}

const runtimeReadMaxDepth = 32

func runtimeMemberType(t typ.Type, key value.MemberKey, depth int) (typ.Type, bool) {
	if depth > runtimeReadMaxDepth || t == nil {
		return nil, false
	}
	if strict, ok := strictMemberType(t, key); ok {
		return strict, true
	}
	switch v := t.(type) {
	case *typ.Union:
		return runtimeUnionMemberType(v.Members, func(member typ.Type) (typ.Type, bool) {
			return runtimeMemberType(member, key, depth+1)
		})
	case *typ.Optional:
		return runtimeOptionalType(v.Inner, func(inner typ.Type) (typ.Type, bool) {
			return runtimeMemberType(inner, key, depth+1)
		})
	case *typ.Alias:
		return runtimeMemberType(v.Target, key, depth+1)
	case *typ.Generic:
		return runtimeMemberType(v.Body, key, depth+1)
	case *typ.TypeParam:
		return runtimeMemberType(v.Constraint, key, depth+1)
	case *typ.Recursive:
		if v.Body == v {
			return nil, false
		}
		return runtimeMemberType(v.Body, key, depth+1)
	case *typ.Instantiated:
		resolved, err := querycore.ResolveInstantiated(v)
		if err != nil {
			return nil, false
		}
		return runtimeMemberType(resolved, key, depth+1)
	}
	if querycore.MissingFieldReadsNil(t) {
		return typ.Nil, true
	}
	return nil, false
}

func strictMemberType(t typ.Type, key value.MemberKey) (typ.Type, bool) {
	switch key.Kind() {
	case value.MemberKindField:
		return querycore.Field(t, key.Name())
	case value.MemberKindStringIndex:
		return querycore.Index(t, typ.LiteralString(key.Name()))
	case value.MemberKindIntIndex:
		return querycore.Index(t, typ.LiteralInt(int64(key.Index())))
	default:
		return nil, false
	}
}

func runtimeUnionMemberType(members []typ.Type, read func(typ.Type) (typ.Type, bool)) (typ.Type, bool) {
	if len(members) == 0 || read == nil {
		return nil, false
	}
	out := make([]typ.Type, 0, len(members))
	for _, member := range typ.CoalesceProductUnionMembers(members) {
		t, ok := read(member)
		if !ok || t == nil {
			return nil, false
		}
		out = append(out, t)
	}
	if len(out) == 0 {
		return nil, false
	}
	return typ.CoalesceProductUnion(typ.NewUnion(out...)), true
}

func runtimeOptionalType(inner typ.Type, read func(typ.Type) (typ.Type, bool)) (typ.Type, bool) {
	t, ok := read(inner)
	if !ok || t == nil || runtimeContainsNil(t) {
		return t, ok
	}
	return typ.NewOptional(t), true
}

func runtimeContainsNil(t typ.Type) bool {
	if t == nil {
		return false
	}
	return typ.Visit(t, typ.Visitor[bool]{
		Optional: func(*typ.Optional) bool {
			return true
		},
		Union: func(u *typ.Union) bool {
			for _, member := range u.Members {
				if runtimeContainsNil(member) {
					return true
				}
			}
			return false
		},
		Default: func(t typ.Type) bool {
			return t.Kind() == kind.Nil
		},
	})
}

func withMetatableType(tableType, metaType typ.Type) typ.Type {
	tableType = unwrap.Alias(tableType)
	if tableType == nil {
		return typ.Unknown
	}

	switch t := tableType.(type) {
	case *typ.Record:
		return recordWithMetatableVariants(t, metaType)
	case *typ.Optional:
		return typ.NewOptional(withMetatableType(t.Inner, metaType))
	case *typ.Union:
		members := make([]typ.Type, 0, len(t.Members))
		for _, member := range t.Members {
			if member == nil || member.Kind() == kind.Nil {
				members = append(members, member)
				continue
			}
			members = append(members, withMetatableType(member, metaType))
		}
		return typ.NewUnion(members...)
	default:
		return tableType
	}
}

func recordWithMetatableVariants(rec *typ.Record, metaType typ.Type) typ.Type {
	var variants []typ.Type
	for _, meta := range metatableVariants(metaType) {
		variants = append(variants, rebuildRecordWithMetatable(rec, meta))
	}
	if len(variants) == 0 {
		return rebuildRecordWithMetatable(rec, nil)
	}
	if len(variants) == 1 {
		return variants[0]
	}
	return typ.NewUnion(variants...)
}

func metatableVariants(metaType typ.Type) []typ.Type {
	metaType = unwrap.Alias(metaType)
	if metaType == nil {
		return []typ.Type{nil}
	}
	switch m := metaType.(type) {
	case *typ.Optional:
		return []typ.Type{nil, unwrap.Alias(m.Inner)}
	case *typ.Union:
		var variants []typ.Type
		hasNil := false
		for _, member := range m.Members {
			member = unwrap.Alias(member)
			if member == nil || member.Kind() == kind.Nil {
				if !hasNil {
					variants = append(variants, nil)
					hasNil = true
				}
				continue
			}
			variants = append(variants, member)
		}
		return variants
	default:
		if metaType.Kind() == kind.Nil {
			return []typ.Type{nil}
		}
		return []typ.Type{metaType}
	}
}

func rebuildRecordWithMetatable(rec *typ.Record, meta typ.Type) typ.Type {
	if rec == nil {
		return typ.Unknown
	}
	builder := typ.NewRecord()
	for _, field := range rec.Fields {
		switch {
		case field.Optional && field.Readonly:
			builder.OptReadonlyField(field.Name, field.Type)
		case field.Optional:
			builder.OptField(field.Name, field.Type)
		case field.Readonly:
			builder.ReadonlyField(field.Name, field.Type)
		default:
			builder.Field(field.Name, field.Type)
		}
	}
	for _, member := range rec.StaticMembers {
		builder.AddStaticMember(member)
	}
	if rec.HasMapComponent() {
		builder.MapComponent(rec.MapKey, rec.MapValue)
	}
	if meta != nil {
		builder.Metatable(meta)
	}
	return builder.SetOpen(rec.Open).Build()
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

// WithMember is the structural-keyed member write. Dot-field writes preserve the
// existing record-field transfer; static index writes use the index-write law so
// they do not masquerade as dot fields before typ.Record grows a structural
// member carrier.
func WithMember(av AbstractValue, key value.MemberKey, fieldAV AbstractValue) AbstractValue {
	if !key.IsValid() {
		return av
	}
	switch key.Kind() {
	case value.MemberKindField:
		return WithField(av, key.Name(), fieldAV)
	case value.MemberKindStringIndex:
		return withStaticMember(av, key, fieldAV)
	case value.MemberKindIntIndex:
		return withStaticMember(av, key, fieldAV)
	default:
		return av
	}
}

func withStaticMember(av AbstractValue, key value.MemberKey, fieldAV AbstractValue) AbstractValue {
	current := av.ProjectValue()
	keyType := memberKeyType(key)
	valType := fieldAV.ProjectValue()
	written := value.AdmitForeignIndexedWrite(current, keyType, valType)
	return FromType(addStaticMemberToType(written, key, valType))
}

func memberKeyType(key value.MemberKey) typ.Type {
	switch key.Kind() {
	case value.MemberKindStringIndex:
		return typ.LiteralString(key.Name())
	case value.MemberKindIntIndex:
		return typ.LiteralInt(int64(key.Index()))
	default:
		return typ.Unknown
	}
}

func addStaticMemberToType(t typ.Type, key value.MemberKey, val typ.Type) typ.Type {
	if val == nil {
		val = typ.Unknown
	}
	switch v := t.(type) {
	case *typ.Alias:
		return typ.NewAlias(v.Name, addStaticMemberToType(v.Target, key, val))
	case *typ.Record:
		return addStaticMemberToRecord(v, key, val)
	case *typ.Map:
		builder := typ.NewRecord().MapComponent(v.Key, v.Value)
		addStaticMemberToBuilder(builder, key, val)
		return builder.Build()
	default:
		builder := typ.NewRecord().SetOpen(true)
		addStaticMemberToBuilder(builder, key, val)
		return builder.Build()
	}
}

func addStaticMemberToRecord(rec *typ.Record, key value.MemberKey, val typ.Type) typ.Type {
	builder := typ.NewRecord()
	if rec.Open {
		builder.SetOpen(true)
	}
	for _, f := range rec.Fields {
		switch {
		case f.Optional && f.Readonly:
			builder.OptReadonlyField(f.Name, f.Type)
		case f.Optional:
			builder.OptField(f.Name, f.Type)
		case f.Readonly:
			builder.ReadonlyField(f.Name, f.Type)
		default:
			builder.Field(f.Name, f.Type)
		}
	}
	for _, m := range rec.StaticMembers {
		if staticMemberMatchesKey(m, key) {
			continue
		}
		builder.AddStaticMember(m)
	}
	addStaticMemberToBuilder(builder, key, val)
	if rec.Metatable != nil {
		builder.Metatable(rec.Metatable)
	}
	if rec.HasMapComponent() {
		builder.MapComponent(rec.MapKey, rec.MapValue)
	}
	return builder.Build()
}

func staticMemberMatchesKey(member typ.StaticMember, key value.MemberKey) bool {
	switch key.Kind() {
	case value.MemberKindStringIndex:
		return member.Kind == typ.StaticMemberStringIndex && member.Name == key.Name()
	case value.MemberKindIntIndex:
		return member.Kind == typ.StaticMemberIntIndex && member.Index == int64(key.Index())
	default:
		return false
	}
}

func addStaticMemberToBuilder(builder *typ.RecordBuilder, key value.MemberKey, val typ.Type) {
	switch key.Kind() {
	case value.MemberKindStringIndex:
		builder.StaticStringIndex(key.Name(), val)
	case value.MemberKindIntIndex:
		builder.StaticIntIndex(int64(key.Index()), val)
	}
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
	if av.IsGradualTop() && typ.IsAny(et) {
		return GradualAny(), true
	}
	return FromType(et), true
}

// WriteIndex is the AbstractValue-native form of an indexed write av[keyAV] = valAV.
//
// The container is widened to admit the key/value, then convergence-merged
// against its prior shape so the result is a stable fixpoint iterate.
func WriteIndex(av AbstractValue, keyAV AbstractValue, valAV AbstractValue) AbstractValue {
	current := av.ProjectValue()
	key := keyAV.ProjectValue()
	val := valAV.ProjectValue()
	written := value.AdmitIndexedWrite(current, key, val)
	return FromType(value.MergeForConvergence(current, written))
}

// WriteSelfDerivedIndex is the AbstractValue-native form of a proven identity
// write av[key] = av[key].
//
// The caller has already proved that valAV is the value read from the same
// container at the same key, so the concrete store does not change. Keeping the
// container unchanged is strictly more precise than applying the generic indexed
// write widening to the union of possible keys and values.
func WriteSelfDerivedIndex(av AbstractValue, keyAV AbstractValue, valAV AbstractValue) AbstractValue {
	return av
}

// WriteIndexForeign is the AbstractValue-native form of an indexed write
// av[keyAV] = valAV whose value is a FOREIGN value — not provably drawn from av at
// the written key. It is WriteIndex over value.AdmitForeignIndexedWrite: a closed
// record's declared field the key could match is weakened by valAV (the foreign
// store can replace that field at runtime). A self-derived write (the value
// provably read from av at the same key) uses WriteSelfDerivedIndex instead, so
// its fields are not weakened.
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
// It reports whether the value domain can soundly admit av[keyAV] = valAV
// before WriteIndex computes the next container.
func IndexWriteAdmits(av AbstractValue, keyAV AbstractValue, valAV AbstractValue) bool {
	return value.IndexedWriteAdmits(av.ProjectValue(), keyAV.ProjectValue(), valAV.ProjectValue())
}

// SealedIndexWriteAdmits is the AbstractValue-native sealed-table admission
// predicate. It accepts only writes that satisfy the declared write slot or
// universal write obligation already present in the container; it never admits a
// write by widening the container shape.
func SealedIndexWriteAdmits(av AbstractValue, keyAV AbstractValue, valAV AbstractValue) bool {
	return value.SealedIndexedWriteAdmits(av.ProjectValue(), keyAV.ProjectValue(), valAV.ProjectValue())
}

// MutateIndex is the AbstractValue-native form of a structural mutation inside an
// indexed element, av[keyAV].field = ... .
//
// Unlike WriteIndex the incoming value is a patch joined into the element slot
// rather than a replacement.
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
// It reports whether the value domain can soundly admit the indexed value
// mutation.
func IndexMutateAdmits(av AbstractValue, keyAV AbstractValue, valAV AbstractValue) bool {
	return value.IndexedValueMutationAdmits(av.ProjectValue(), keyAV.ProjectValue(), valAV.ProjectValue())
}

// AppendElement is the AbstractValue-native form of an array-element mutation
// av[#av+1] = elemAV (table.insert-like).
//
// The array element domain is joined with elemAV's content and the result
// convergence-merged against the prior shape.
func AppendElement(av AbstractValue, elemAV AbstractValue) AbstractValue {
	current := av.ProjectValue()
	elem := elemAV.ProjectValue()
	widened := value.AdmitArrayElementMutation(current, elem, typ.JoinPreferNonSoft)
	return FromType(value.MergeForConvergence(current, widened))
}

// AppendMapElement is the AbstractValue-native form of an array-element mutation
// through a dynamic map key, av[keyAV][#...+1] = elemAV.
//
// The value slot of the map at keyAV is array-widened by elemAV.
func AppendMapElement(av AbstractValue, keyAV AbstractValue, elemAV AbstractValue) AbstractValue {
	current := av.ProjectValue()
	key := keyAV.ProjectValue()
	elem := elemAV.ProjectValue()
	widened := value.AdmitMapArrayElementMutation(current, key, elem)
	return FromType(value.MergeForConvergence(current, widened))
}

// ContainerElementUnion is the AbstractValue-native form of a spec-level
// ContainerElementUnion mutation such as channel.send(v). It widens the element
// slot of arrays, maps, instantiated generic containers, aliases, and unions.
func ContainerElementUnion(av AbstractValue, elemAV AbstractValue) AbstractValue {
	current := av.ProjectValue()
	elem := elemAV.ProjectValue()
	widened := value.AdmitContainerElementUnion(current, elem)
	return FromType(widened)
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

// NarrowLengthLowerBound is the AbstractValue-native form of a proven sequence
// length floor, such as the true edge of `#x > 0`. It filters impossible shapes
// (`{}` in `{}`|array) through the shared type-domain law and re-lifts the
// reduced value into the product.
func NarrowLengthLowerBound(av AbstractValue, lower int64) AbstractValue {
	base := av.ProjectValue()
	if lower > 0 {
		base = narrow.RemoveNil(base)
	}
	narrowed := narrow.RefineByLengthLowerBound(base, lower)
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
