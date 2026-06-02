package value

import (
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
	typejoin "github.com/wippyai/go-lua/types/typ/join"
)

// AdmitObservation canonicalizes a newly observed value before it enters a
// value-domain product slot.
//
// Product slots need one recursive admission law across flow state,
// interprocedural facts, return summaries, and parameter evidence: scalar
// literals widen to their base type, table variants keep discriminant literals,
// and self-embedding structural growth is folded into an explicit recursive
// product instead of another unfolded level.
func AdmitObservation(t typ.Type) typ.Type {
	w := observationAdmission{
		memo: make(map[typ.Type]typ.Type),
	}
	return w.admit(t)
}

// JoinObservations joins two value-domain observations with the canonical
// product admission law. Callers own when products meet; this package owns how
// structural values converge.
func JoinObservations(existing, incoming typ.Type) typ.Type {
	w := observationAdmission{
		memo: make(map[typ.Type]typ.Type),
	}
	return w.join(existing, incoming)
}

// IsNilOnly reports whether a type denotes only nil values. This is a value
// domain predicate used by write/delete projections, not a diagnostic rule.
func IsNilOnly(t typ.Type) bool {
	if t == nil {
		return false
	}
	switch v := typ.UnwrapAnnotated(t).(type) {
	case *typ.Alias:
		return IsNilOnly(v.Target)
	case *typ.Union:
		if len(v.Members) == 0 {
			return false
		}
		for _, member := range v.Members {
			if !IsNilOnly(member) {
				return false
			}
		}
		return true
	default:
		return v != nil && v.Kind() == kind.Nil
	}
}

type observationAdmission struct {
	memo map[typ.Type]typ.Type
}

func (w *observationAdmission) join(existing, incoming typ.Type) typ.Type {
	if existing == nil {
		return w.admit(incoming)
	}
	if incoming == nil {
		return w.admit(existing)
	}
	existing = w.admit(existing)
	incoming = w.admit(incoming)
	if sameRecursiveObservationEvidence(existing, incoming) {
		return existing
	}
	if typ.SameNodeOrAcyclicEqual(existing, incoming) {
		return existing
	}
	if refined, ok := selectObservationRefinement(existing, incoming); ok {
		return refined
	}
	if upper, ok := SelfEmbeddingUpperBound(existing, incoming); ok {
		return upper
	}
	if upper, ok := SelfEmbeddingUpperBound(incoming, existing); ok {
		return upper
	}
	if joined, ok := JoinStructuralShape(existing, incoming, w.join); ok {
		return w.admit(joined)
	}
	if joined, ok := JoinStructuralUnionShape(existing, incoming, w.join); ok {
		return w.admit(joined)
	}
	joined := typejoin.Types(existing, incoming)
	if upper, ok := SelfEmbeddingUpperBound(existing, joined); ok {
		return upper
	}
	if upper, ok := SelfEmbeddingUpperBound(incoming, joined); ok {
		return upper
	}
	return w.admit(joined)
}

func sameRecursiveObservationEvidence(a, b typ.Type) bool {
	if !typ.ContainsRecursive(a) && !typ.ContainsRecursive(b) {
		return false
	}
	return recursiveEvidenceUpperBoundCovers(a, b) && recursiveEvidenceUpperBoundCovers(b, a)
}

func selectObservationRefinement(a, b typ.Type) (typ.Type, bool) {
	if refined, ok := selectObservationRefinementDirected(b, a); ok {
		return refined, true
	}
	return selectObservationRefinementDirected(a, b)
}

func selectObservationRefinementDirected(candidate, baseline typ.Type) (typ.Type, bool) {
	if !typ.IsRefinableAnnotation(baseline) {
		return nil, false
	}
	if refines, changed := RefinesSoftContainer(candidate, baseline); refines && changed {
		return candidate, true
	}
	if preferred, ok := PreferConcreteOverSoft(baseline, candidate); ok && typ.SameNodeOrAcyclicEqual(preferred, candidate) {
		return candidate, true
	}
	return nil, false
}

func (w *observationAdmission) admit(t typ.Type) typ.Type {
	if t == nil {
		return t
	}
	if widened, ok := w.memo[t]; ok {
		return widened
	}
	if typ.ContainsRecursive(t) {
		w.memo[t] = t
		return t
	}

	switch v := t.(type) {
	case *typ.Literal:
		widened := admitObservationLiteral(v)
		w.memo[t] = widened
		return widened
	case *typ.Optional:
		inner := w.admit(v.Inner)
		if inner == v.Inner {
			w.memo[t] = t
			return t
		}
		widened := typ.NewOptional(inner)
		w.memo[t] = widened
		return widened
	case *typ.Alias:
		if v.Target == nil {
			w.memo[t] = t
			return t
		}
		target := w.admit(v.Target)
		if target == v.Target {
			w.memo[t] = t
			return t
		}
		widened := typ.NewAlias(v.Name, target)
		w.memo[t] = widened
		return widened
	case *typ.Union:
		members := make([]typ.Type, len(v.Members))
		changed := false
		preserveFalsySentinel := unionHasStructuralMember(v)
		for i, m := range v.Members {
			members[i] = w.admitUnionMember(m, preserveFalsySentinel)
			if members[i] != m {
				changed = true
			}
		}
		if !changed {
			w.memo[t] = t
			return t
		}
		widened := typ.NewUnion(members...)
		w.memo[t] = widened
		return widened
	case *typ.Tuple:
		if folded, ok := FoldSelfEmbedding(v, v); ok {
			w.memo[t] = folded
			return folded
		}
		elements := make([]typ.Type, len(v.Elements))
		changed := false
		for i, elem := range v.Elements {
			elements[i] = w.admit(elem)
			if elements[i] != elem {
				changed = true
			}
		}
		if !changed {
			w.memo[t] = t
			return t
		}
		widened := typ.NewTuple(elements...)
		w.memo[t] = widened
		return widened
	case *typ.Array:
		if folded, ok := FoldSelfEmbedding(v, v); ok {
			w.memo[t] = folded
			return folded
		}
		elem := w.admit(v.Element)
		if elem == v.Element {
			w.memo[t] = t
			return t
		}
		widened := typ.NewArray(elem)
		w.memo[t] = widened
		return widened
	case *typ.Map:
		if folded, ok := FoldSelfEmbedding(v, v); ok {
			w.memo[t] = folded
			return folded
		}
		mapKey := w.admit(v.Key)
		val := w.admit(v.Value)
		if mapKey == v.Key && val == v.Value {
			w.memo[t] = t
			return t
		}
		widened := typ.NewMap(mapKey, val)
		w.memo[t] = widened
		return widened
	case *typ.Record:
		if folded, ok := FoldSelfEmbedding(v, v); ok {
			folded = w.admitFreshRecursiveBody(folded)
			w.memo[t] = folded
			return folded
		}
		builder := typ.NewRecord()
		changed := false
		preserveRecordLiterals := recordHasDiscriminantLiteral(v)
		if v.Open {
			builder.SetOpen(true)
		}
		for _, f := range v.Fields {
			fieldType := w.admitRecordField(f.Type, preserveRecordLiterals)
			if fieldType != f.Type {
				changed = true
			}
			switch {
			case f.Optional && f.Readonly:
				builder.OptReadonlyField(f.Name, fieldType)
			case f.Optional:
				builder.OptField(f.Name, fieldType)
			case f.Readonly:
				builder.ReadonlyField(f.Name, fieldType)
			default:
				builder.Field(f.Name, fieldType)
			}
		}
		if v.HasMapComponent() {
			mapKey := w.admit(v.MapKey)
			val := w.admit(v.MapValue)
			if mapKey != v.MapKey || val != v.MapValue {
				changed = true
			}
			builder.MapComponent(mapKey, val)
		}
		if v.Metatable != nil {
			mt := w.admit(v.Metatable)
			if mt != v.Metatable {
				changed = true
			}
			builder.Metatable(mt)
		}
		if !changed {
			w.memo[t] = t
			return t
		}
		widened := builder.Build()
		w.memo[t] = widened
		return widened
	default:
		w.memo[t] = t
		return t
	}
}

func admitObservationLiteral(lit *typ.Literal) typ.Type {
	if lit == nil {
		return nil
	}
	switch lit.Base {
	case kind.Boolean:
		return typ.Boolean
	case kind.Integer:
		return typ.Integer
	case kind.Number:
		return typ.Number
	case kind.String:
		return typ.String
	default:
		return lit
	}
}

// recordHasDiscriminantLiteral reports whether a record has the shape of a tagged
// variant whose literal fields admission keeps precise rather than widening to
// their base. Two or more required literal fields are a structural signal of
// correlated variant evidence. A single literal field is admitted only when it is
// on the checker's discriminator surface and the record has required non-literal
// payload; otherwise scalar config/data literals still widen at the admission
// boundary. Literals carried only by a recursive self-embedding node are repeated
// values, so they widen too.
func recordHasDiscriminantLiteral(r *typ.Record) bool {
	if r == nil {
		return false
	}
	requiredLiterals := 0
	hasNamedDiscriminant := false
	hasPayload := false
	for _, f := range r.Fields {
		if f.Optional {
			continue
		}
		if typ.ContainsRecursive(f.Type) {
			continue
		}
		if _, ok := f.Type.(*typ.Literal); ok {
			requiredLiterals++
			if requiredLiterals >= 2 {
				return true
			}
			if isRecordDiscriminantField(f.Name) {
				hasNamedDiscriminant = true
			}
			continue
		}
		hasPayload = true
	}
	return hasNamedDiscriminant && hasPayload
}

func isRecordDiscriminantField(name string) bool {
	switch name {
	case "kind", "type", "tag", "__tag", "ok", "role":
		return true
	default:
		return false
	}
}

func unionHasStructuralMember(u *typ.Union) bool {
	if u == nil {
		return false
	}
	for _, m := range u.Members {
		switch m.(type) {
		case *typ.Record, *typ.Map, *typ.Array, *typ.Tuple, *typ.Interface, *typ.Recursive:
			return true
		case *typ.Alias:
			if unionAliasHasStructuralTarget(m) {
				return true
			}
		}
	}
	return false
}

func unionAliasHasStructuralTarget(t typ.Type) bool {
	a, ok := t.(*typ.Alias)
	if !ok || a.Target == nil {
		return false
	}
	switch a.UnaliasedTarget().(type) {
	case *typ.Record, *typ.Map, *typ.Array, *typ.Tuple, *typ.Interface, *typ.Recursive:
		return true
	default:
		return false
	}
}

func (w *observationAdmission) admitUnionMember(t typ.Type, preserveFalsySentinel bool) typ.Type {
	if preserveFalsySentinel && t == typ.False {
		return t
	}
	return w.admit(t)
}

func (w *observationAdmission) admitRecordField(t typ.Type, preserveRecordLiterals bool) typ.Type {
	if _, ok := t.(*typ.Literal); ok && preserveRecordLiterals {
		return t
	}
	return w.admit(t)
}

func (w *observationAdmission) admitFreshRecursiveBody(t typ.Type) typ.Type {
	rec, ok := t.(*typ.Recursive)
	if !ok || rec.Body == nil {
		return t
	}
	body, ok := rec.Body.(*typ.Record)
	if !ok {
		return t
	}
	builder := typ.NewRecord()
	if body.Open {
		builder.SetOpen(true)
	}
	changed := false
	preserveRecordLiterals := recordHasDiscriminantLiteral(body)
	for _, field := range body.Fields {
		fieldType := field.Type
		if !typ.ContainsRecursive(fieldType) {
			fieldType = w.admitRecordField(fieldType, preserveRecordLiterals)
		}
		if fieldType != field.Type {
			changed = true
		}
		switch {
		case field.Optional && field.Readonly:
			builder.OptReadonlyField(field.Name, fieldType)
		case field.Optional:
			builder.OptField(field.Name, fieldType)
		case field.Readonly:
			builder.ReadonlyField(field.Name, fieldType)
		default:
			builder.Field(field.Name, fieldType)
		}
	}
	if body.Metatable != nil {
		metatable := body.Metatable
		if !typ.ContainsRecursive(metatable) {
			metatable = w.admit(metatable)
		}
		if metatable != body.Metatable {
			changed = true
		}
		builder.Metatable(metatable)
	}
	if body.HasMapComponent() {
		mapKey := body.MapKey
		mapValue := body.MapValue
		if !typ.ContainsRecursive(mapKey) {
			mapKey = w.admit(mapKey)
		}
		if !typ.ContainsRecursive(mapValue) {
			mapValue = w.admit(mapValue)
		}
		if mapKey != body.MapKey || mapValue != body.MapValue {
			changed = true
		}
		builder.MapComponent(mapKey, mapValue)
	}
	if changed {
		rec.SetBody(builder.Build())
	}
	return rec
}
