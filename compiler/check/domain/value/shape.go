package value

import (
	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// Equivalent reports structural equality or mutual subtyping.
func Equivalent(a, b typ.Type) bool {
	return typ.TypeEquals(a, b) || (subtype.IsSubtype(a, b) && subtype.IsSubtype(b, a))
}

// ElidesOptional reports whether candidate is inside baseline after nil is
// removed from baseline.
func ElidesOptional(candidate, baseline typ.Type) bool {
	if candidate == nil || baseline == nil {
		return false
	}
	nonNil := narrow.RemoveNil(baseline)
	if nonNil == nil || typ.TypeEquals(nonNil, baseline) {
		return false
	}
	return subtype.IsSubtype(candidate, nonNil)
}

// SplitNilable separates the non-nil component from an optional/nilable type.
func SplitNilable(t typ.Type) (typ.Type, bool) {
	t = unwrap.Alias(t)
	switch v := t.(type) {
	case nil:
		return nil, false
	case *typ.Optional:
		return v.Inner, true
	case *typ.Union:
		members := make([]typ.Type, 0, len(v.Members))
		nilable := false
		for _, member := range v.Members {
			member = unwrap.Alias(member)
			if unwrap.IsNilType(member) {
				nilable = true
				continue
			}
			members = append(members, member)
		}
		if !nilable {
			return t, false
		}
		return typ.NewUnion(members...), true
	default:
		if unwrap.IsNilType(t) {
			return nil, true
		}
		return t, false
	}
}

// IsTruthyRefinement reports whether candidate equals or subtypes the truthy
// refinement of baseline.
func IsTruthyRefinement(candidate, baseline typ.Type) bool {
	if candidate == nil || baseline == nil || typ.TypeEquals(candidate, baseline) {
		return false
	}
	refined := narrow.ToTruthy(baseline)
	if refined == nil || refined.Kind().IsNever() || typ.TypeEquals(refined, baseline) {
		return false
	}
	return typ.TypeEquals(candidate, refined) || subtype.IsSubtype(candidate, refined)
}

// PreferConcreteOverSoft selects a concrete observation over a soft placeholder
// while preserving explicit nilability.
func PreferConcreteOverSoft(a, b typ.Type) (typ.Type, bool) {
	aSoft := typ.IsSoft(a, typ.SoftPlaceholderPolicy)
	bSoft := typ.IsSoft(b, typ.SoftPlaceholderPolicy)
	switch {
	case aSoft && !bSoft && !unwrap.IsNilType(b):
		return b, true
	case bSoft && !aSoft && !unwrap.IsNilType(a):
		return a, true
	}
	if preferred, ok := preferConcreteOverNilableSoft(a, b); ok {
		return preferred, true
	}
	return nil, false
}

func preferConcreteOverNilableSoft(a, b typ.Type) (typ.Type, bool) {
	if preferred, ok := preferConcreteOverNilableSoftDirected(a, b); ok {
		return preferred, true
	}
	return preferConcreteOverNilableSoftDirected(b, a)
}

func preferConcreteOverNilableSoftDirected(softMaybeNil, concrete typ.Type) (typ.Type, bool) {
	inner, nilable := SplitNilable(softMaybeNil)
	if !nilable || inner == nil || !typ.IsSoft(inner, typ.SoftPlaceholderPolicy) {
		return nil, false
	}
	if concrete == nil || unwrap.IsNilType(concrete) {
		return nil, false
	}
	concreteInner, concreteNilable := SplitNilable(concrete)
	if concreteInner == nil {
		return nil, false
	}
	if typ.IsSoft(concreteInner, typ.SoftPlaceholderPolicy) {
		return nil, false
	}
	if concreteNilable {
		return concrete, true
	}
	return typ.NewOptional(concrete), true
}

// CanSelfEmbed reports whether t is a structural type that can recursively
// carry another type value below itself.
func CanSelfEmbed(t typ.Type) bool {
	if t == nil {
		return false
	}
	switch v := t.(type) {
	case *typ.Annotated:
		return CanSelfEmbed(v.Inner)
	case *typ.Alias:
		return CanSelfEmbed(v.Target)
	case *typ.Optional:
		return CanSelfEmbed(v.Inner)
	case *typ.Union:
		for _, member := range v.Members {
			if CanSelfEmbed(member) {
				return true
			}
		}
		return false
	case *typ.Intersection:
		for _, member := range v.Members {
			if CanSelfEmbed(member) {
				return true
			}
		}
		return false
	case *typ.Array, *typ.Map, *typ.Tuple, *typ.Record, *typ.Function:
		return true
	default:
		return false
	}
}

// ContainsEquivalent reports whether haystack contains a node equivalent to
// needle while walking structural type children.
func ContainsEquivalent(haystack, needle typ.Type) bool {
	if haystack == nil || needle == nil {
		return false
	}
	return Scan(haystack, typ.NewGuard(), func(node typ.Type) (bool, bool) {
		if typ.TypeEquals(node, needle) {
			return true, false
		}
		return false, true
	})
}

// ContainsUnion reports whether t contains any union node.
func ContainsUnion(t typ.Type) bool {
	if t == nil {
		return false
	}
	return Scan(t, typ.NewGuard(), func(node typ.Type) (bool, bool) {
		if _, ok := node.(*typ.Union); ok {
			return true, false
		}
		return false, true
	})
}

// Scan walks structural type children until visit stops traversal.
func Scan(
	t typ.Type,
	guard internal.RecursionGuard,
	visit func(node typ.Type) (stop bool, descend bool),
) bool {
	if t == nil {
		return false
	}
	next, ok := guard.Enter(t)
	if !ok {
		return false
	}

	node := t
	for {
		ann, ok := node.(*typ.Annotated)
		if !ok || ann.Inner == nil || ann.Inner == node {
			break
		}
		node = ann.Inner
	}

	if stop, descend := visit(node); stop {
		return true
	} else if !descend {
		return false
	}

	switch n := node.(type) {
	case *typ.Optional:
		return Scan(n.Inner, next, visit)
	case *typ.Union:
		for _, m := range n.Members {
			if Scan(m, next, visit) {
				return true
			}
		}
		return false
	case *typ.Intersection:
		for _, m := range n.Members {
			if Scan(m, next, visit) {
				return true
			}
		}
		return false
	case *typ.Array:
		return Scan(n.Element, next, visit)
	case *typ.Map:
		return Scan(n.Key, next, visit) || Scan(n.Value, next, visit)
	case *typ.Tuple:
		for _, e := range n.Elements {
			if Scan(e, next, visit) {
				return true
			}
		}
		return false
	case *typ.Function:
		for _, p := range n.Params {
			if Scan(p.Type, next, visit) {
				return true
			}
		}
		for _, r := range n.Returns {
			if Scan(r, next, visit) {
				return true
			}
		}
		return n.Variadic != nil && Scan(n.Variadic, next, visit)
	case *typ.Record:
		for _, f := range n.Fields {
			if Scan(f.Type, next, visit) {
				return true
			}
		}
		if n.Metatable != nil && Scan(n.Metatable, next, visit) {
			return true
		}
		if n.HasMapComponent() {
			return Scan(n.MapKey, next, visit) || Scan(n.MapValue, next, visit)
		}
		return false
	case *typ.Alias:
		return Scan(n.Target, next, visit)
	case *typ.Instantiated:
		for _, a := range n.TypeArgs {
			if Scan(a, next, visit) {
				return true
			}
		}
		return false
	case *typ.Interface:
		for _, m := range n.Methods {
			if m.Type != nil && Scan(m.Type, next, visit) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// ExtendsRecord reports whether a extends b by adding record fields. This
// treats record field supersets as refinements when b is a record or union of
// records.
func ExtendsRecord(a, b typ.Type) bool {
	if a == nil || b == nil {
		return false
	}
	ar, ok := a.(*typ.Record)
	if !ok {
		return false
	}
	switch br := b.(type) {
	case *typ.Record:
		return RecordSuperset(ar, br)
	case *typ.Union:
		return recordSupersetUnion(ar, br)
	default:
		return false
	}
}

// RecordSuperset reports whether newRec preserves oldRec and may add fields.
func RecordSuperset(newRec, oldRec *typ.Record) bool {
	if newRec == nil || oldRec == nil {
		return false
	}
	if oldRec.Metatable != nil {
		if newRec.Metatable == nil || !subtype.IsSubtype(newRec.Metatable, oldRec.Metatable) {
			return false
		}
	}
	if oldRec.HasMapComponent() {
		if !newRec.HasMapComponent() {
			return false
		}
		if !subtype.IsSubtype(newRec.MapKey, oldRec.MapKey) || !subtype.IsSubtype(newRec.MapValue, oldRec.MapValue) {
			return false
		}
	}
	oldFields := make(map[string]typ.Field, len(oldRec.Fields))
	for _, f := range oldRec.Fields {
		oldFields[f.Name] = f
	}
	for _, nf := range newRec.Fields {
		if of, ok := oldFields[nf.Name]; ok {
			if of.Optional && !nf.Optional {
				// ok: stronger requirement
			} else if !of.Optional && nf.Optional {
				return false
			}
			if of.Readonly && !nf.Readonly {
				return false
			}
			if of.Type != nil {
				if IsOpenTopRecord(nf.Type) && IsStructuredTableShape(of.Type) {
					return false
				}
				if nf.Type == nil || !subtype.IsSubtype(nf.Type, of.Type) {
					return false
				}
			}
			delete(oldFields, nf.Name)
		}
	}
	return len(oldFields) == 0
}

func recordSupersetUnion(newRec *typ.Record, oldUnion *typ.Union) bool {
	if newRec == nil || oldUnion == nil {
		return false
	}
	if len(oldUnion.Members) == 0 {
		return false
	}
	for _, member := range oldUnion.Members {
		oldRec, ok := member.(*typ.Record)
		if !ok {
			return false
		}
		if !RecordSuperset(newRec, oldRec) {
			return false
		}
	}
	return true
}

// IsOpenTopRecord reports whether t is an open record with no concrete fields
// or map component.
func IsOpenTopRecord(t typ.Type) bool {
	rec, ok := unwrap.Alias(t).(*typ.Record)
	if !ok || rec == nil {
		return false
	}
	return rec.Open && len(rec.Fields) == 0 && !rec.HasMapComponent()
}

// IsStructuredTableShape reports whether t carries table structure beyond an
// open-top placeholder.
func IsStructuredTableShape(t typ.Type) bool {
	switch v := unwrap.Alias(t).(type) {
	case *typ.Array:
		return true
	case *typ.Map:
		return true
	case *typ.Record:
		return v.HasMapComponent() || len(v.Fields) > 0
	default:
		return false
	}
}

// RefinesSoftContainer reports whether candidate preserves the same table shape
// while replacing a soft placeholder element/value with concrete evidence.
func RefinesSoftContainer(candidate, baseline typ.Type) (bool, bool) {
	candidate = UnwrapStructuralShape(candidate)
	baseline = UnwrapStructuralShape(baseline)
	if candidate == nil || baseline == nil {
		return candidate == baseline, false
	}
	if typ.TypeEquals(candidate, baseline) {
		return true, false
	}

	switch b := baseline.(type) {
	case *typ.Array:
		c, ok := candidate.(*typ.Array)
		if !ok {
			return false, false
		}
		return refinesSoftContainerSlot(c.Element, b.Element)
	case *typ.Map:
		c, ok := candidate.(*typ.Map)
		if !ok || !Equivalent(c.Key, b.Key) {
			return false, false
		}
		return refinesSoftContainerSlot(c.Value, b.Value)
	case *typ.Record:
		c, ok := candidate.(*typ.Record)
		if !ok || !sameRecordFrame(c, b) {
			return false, false
		}
		if !c.HasMapComponent() && !b.HasMapComponent() {
			return true, false
		}
		if !c.HasMapComponent() || !b.HasMapComponent() || !Equivalent(c.MapKey, b.MapKey) {
			return false, false
		}
		return refinesSoftContainerSlot(c.MapValue, b.MapValue)
	default:
		return false, false
	}
}

func refinesSoftContainerSlot(candidate, baseline typ.Type) (bool, bool) {
	if typ.TypeEquals(candidate, baseline) {
		return true, false
	}
	if (typ.IsAny(baseline) || typ.IsUnknown(baseline)) && CanSelfEmbed(candidate) {
		return false, false
	}
	preferred, ok := PreferConcreteOverSoft(baseline, candidate)
	return ok && typ.TypeEquals(preferred, candidate), ok
}

func sameRecordFrame(a, b *typ.Record) bool {
	if a == nil || b == nil || a.Open != b.Open || len(a.Fields) != len(b.Fields) {
		return false
	}
	if (a.Metatable == nil) != (b.Metatable == nil) {
		return false
	}
	if a.Metatable != nil && !typ.TypeEquals(a.Metatable, b.Metatable) {
		return false
	}
	for i, field := range a.Fields {
		other := b.Fields[i]
		if field.Name != other.Name || field.Optional != other.Optional || field.Readonly != other.Readonly {
			return false
		}
		if !typ.TypeEquals(field.Type, other.Type) {
			return false
		}
	}
	return true
}

// RefinesFalsyMapKey reports whether candidate is the same table-derived shape
// as baseline after removing stale falsy key members from baseline.
func RefinesFalsyMapKey(candidate, baseline typ.Type) (bool, bool) {
	candidate = UnwrapStructuralShape(candidate)
	baseline = UnwrapStructuralShape(baseline)
	if candidate == nil || baseline == nil {
		return candidate == baseline, false
	}
	if typ.TypeEquals(candidate, baseline) {
		return true, false
	}

	switch b := baseline.(type) {
	case *typ.Array:
		c, ok := candidate.(*typ.Array)
		if !ok {
			return false, false
		}
		return truthyElementRefinement(c.Element, b.Element)
	case *typ.Map:
		c, ok := candidate.(*typ.Map)
		if !ok {
			return false, false
		}
		return mapKeyTruthyRefinement(c.Key, c.Value, b.Key, b.Value)
	case *typ.Record:
		if c, ok := candidate.(*typ.Map); ok {
			if len(b.Fields) != 0 || b.Metatable != nil || !b.HasMapComponent() {
				return false, false
			}
			return mapKeyTruthyRefinement(c.Key, c.Value, b.MapKey, b.MapValue)
		}
		c, ok := candidate.(*typ.Record)
		if !ok || !c.HasMapComponent() || !b.HasMapComponent() {
			return false, false
		}
		if c.Open && !b.Open {
			return false, false
		}
		if len(c.Fields) != len(b.Fields) {
			return false, false
		}
		for _, bf := range b.Fields {
			cf := c.GetField(bf.Name)
			if cf == nil || cf.Optional != bf.Optional || cf.Readonly != bf.Readonly || !typ.TypeEquals(cf.Type, bf.Type) {
				return false, false
			}
		}
		if (c.Metatable == nil) != (b.Metatable == nil) || (c.Metatable != nil && !typ.TypeEquals(c.Metatable, b.Metatable)) {
			return false, false
		}
		return mapKeyTruthyRefinement(c.MapKey, c.MapValue, b.MapKey, b.MapValue)
	default:
		return false, false
	}
}

func mapKeyTruthyRefinement(candidateKey, candidateValue, baselineKey, baselineValue typ.Type) (bool, bool) {
	if !typ.TypeEquals(candidateValue, baselineValue) {
		return false, false
	}
	if IsTruthyRefinement(candidateKey, baselineKey) {
		return true, true
	}
	return false, false
}

func truthyElementRefinement(candidate, baseline typ.Type) (bool, bool) {
	if typ.TypeEquals(candidate, baseline) {
		return true, false
	}
	if IsTruthyRefinement(candidate, baseline) {
		return true, true
	}
	return false, false
}

// NestedNilOnlyRegression reports whether candidate's apparent refinement only
// adds nested nil facts over a more useful baseline shape.
func NestedNilOnlyRegression(candidate, baseline typ.Type) bool {
	candidate = UnwrapStructuralShape(candidate)
	baseline = UnwrapStructuralShape(baseline)
	if candidate == nil || baseline == nil || typ.TypeEquals(candidate, baseline) {
		return false
	}
	if unwrap.IsNilType(candidate) {
		return typ.IsAny(baseline) || typ.IsUnknown(baseline) || unwrap.IsOptionalLike(baseline)
	}

	switch c := candidate.(type) {
	case *typ.Record:
		b, ok := baseline.(*typ.Record)
		if !ok {
			return false
		}
		for _, cf := range c.Fields {
			bf := b.GetField(cf.Name)
			if bf == nil {
				continue
			}
			if unwrap.IsNilType(cf.Type) && (bf.Optional || typ.IsAny(bf.Type) || typ.IsUnknown(bf.Type) || unwrap.IsOptionalLike(bf.Type)) {
				return true
			}
			if NestedNilOnlyRegression(cf.Type, bf.Type) {
				return true
			}
		}
		if c.HasMapComponent() && b.HasMapComponent() {
			return NestedNilOnlyRegression(c.MapValue, b.MapValue)
		}
	case *typ.Array:
		if b, ok := baseline.(*typ.Array); ok {
			return NestedNilOnlyRegression(c.Element, b.Element)
		}
	case *typ.Map:
		if b, ok := baseline.(*typ.Map); ok {
			return NestedNilOnlyRegression(c.Value, b.Value)
		}
	case *typ.Tuple:
		b, ok := baseline.(*typ.Tuple)
		if !ok || len(c.Elements) != len(b.Elements) {
			return false
		}
		for i := range c.Elements {
			if NestedNilOnlyRegression(c.Elements[i], b.Elements[i]) {
				return true
			}
		}
	case *typ.Function:
		b, ok := baseline.(*typ.Function)
		if !ok || len(c.Returns) != len(b.Returns) {
			return false
		}
		for i := range c.Returns {
			if NestedNilOnlyRegression(c.Returns[i], b.Returns[i]) {
				return true
			}
		}
	}
	return false
}

// ContainsNestedStructuralShape reports whether haystack embeds the same
// shallow structural shape as needle below the root.
func ContainsNestedStructuralShape(haystack, needle typ.Type) bool {
	return containsNestedStructuralShapeDepth(haystack, needle, make(map[typ.Type]bool), false)
}

func containsNestedStructuralShapeDepth(haystack, needle typ.Type, seen map[typ.Type]bool, belowContainer bool) bool {
	if haystack == nil || needle == nil {
		return false
	}
	if seen[haystack] {
		return false
	}
	seen[haystack] = true

	node := UnwrapStructuralShape(haystack)
	if node == nil {
		return false
	}
	if belowContainer && ShallowStructuralShapeEquals(node, needle) {
		return true
	}

	descend := func(child typ.Type, childBelowContainer bool) bool {
		return containsNestedStructuralShapeDepth(child, needle, seen, childBelowContainer)
	}

	switch n := node.(type) {
	case *typ.Optional:
		return descend(n.Inner, belowContainer)
	case *typ.Union:
		for _, member := range n.Members {
			if descend(member, belowContainer) {
				return true
			}
		}
		return false
	case *typ.Intersection:
		for _, member := range n.Members {
			if descend(member, belowContainer) {
				return true
			}
		}
		return false
	case *typ.Array:
		return descend(n.Element, true)
	case *typ.Map:
		return descend(n.Key, true) || descend(n.Value, true)
	case *typ.Tuple:
		for _, elem := range n.Elements {
			if descend(elem, true) {
				return true
			}
		}
		return false
	case *typ.Record:
		for _, field := range n.Fields {
			if descend(field.Type, true) {
				return true
			}
		}
		if n.Metatable != nil && descend(n.Metatable, true) {
			return true
		}
		if n.HasMapComponent() {
			return descend(n.MapKey, true) || descend(n.MapValue, true)
		}
		return false
	case *typ.Function:
		for _, param := range n.Params {
			if descend(param.Type, true) {
				return true
			}
		}
		if n.Variadic != nil && descend(n.Variadic, true) {
			return true
		}
		for _, ret := range n.Returns {
			if descend(ret, true) {
				return true
			}
		}
		return false
	case *typ.Instantiated:
		for _, arg := range n.TypeArgs {
			if descend(arg, belowContainer) {
				return true
			}
		}
		return false
	case *typ.Interface:
		for _, method := range n.Methods {
			if method.Type != nil && descend(method.Type, true) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// ShallowStructuralShapeEquals reports whether a and b have the same root
// structural container shape.
func ShallowStructuralShapeEquals(a, b typ.Type) bool {
	a = UnwrapStructuralShape(a)
	b = UnwrapStructuralShape(b)
	if a == nil || b == nil {
		return a == b
	}

	switch av := a.(type) {
	case *typ.Union:
		for _, member := range av.Members {
			if ShallowStructuralShapeEquals(member, b) {
				return true
			}
		}
		return false
	case *typ.Intersection:
		for _, member := range av.Members {
			if ShallowStructuralShapeEquals(member, b) {
				return true
			}
		}
		return false
	}
	switch bv := b.(type) {
	case *typ.Union:
		for _, member := range bv.Members {
			if ShallowStructuralShapeEquals(a, member) {
				return true
			}
		}
		return false
	case *typ.Intersection:
		for _, member := range bv.Members {
			if ShallowStructuralShapeEquals(a, member) {
				return true
			}
		}
		return false
	}

	switch av := a.(type) {
	case *typ.Array:
		_, ok := b.(*typ.Array)
		return ok
	case *typ.Map:
		bv, ok := b.(*typ.Map)
		return ok && shallowMapKeyShapeEquals(av.Key, bv.Key)
	case *typ.Tuple:
		bv, ok := b.(*typ.Tuple)
		return ok && len(av.Elements) == len(bv.Elements)
	case *typ.Record:
		bv, ok := b.(*typ.Record)
		return ok && shallowRecordShapeEquals(av, bv)
	default:
		return typ.TypeEquals(a, b)
	}
}

// UnwrapStructuralShape strips transparent wrappers for structural comparison.
func UnwrapStructuralShape(t typ.Type) typ.Type {
	for t != nil {
		switch v := t.(type) {
		case *typ.Annotated:
			if v.Inner == nil || v.Inner == t {
				return t
			}
			t = v.Inner
		case *typ.Alias:
			if v.Target == nil || v.Target == t {
				return t
			}
			t = v.Target
		case *typ.Optional:
			if v.Inner == nil || v.Inner == t {
				return t
			}
			t = v.Inner
		default:
			return t
		}
	}
	return nil
}

func shallowMapKeyShapeEquals(a, b typ.Type) bool {
	if a == nil || b == nil {
		return a == b
	}
	if typ.TypeEquals(a, b) {
		return true
	}
	return typ.IsAny(a) || typ.IsAny(b) || typ.IsUnknown(a) || typ.IsUnknown(b)
}

func shallowRecordShapeEquals(a, b *typ.Record) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.HasMapComponent() != b.HasMapComponent() {
		return false
	}
	if a.HasMapComponent() && !shallowMapKeyShapeEquals(a.MapKey, b.MapKey) {
		return false
	}
	if len(a.Fields) != len(b.Fields) {
		return false
	}
	for _, field := range a.Fields {
		if b.GetField(field.Name) == nil {
			return false
		}
	}
	return true
}

// UnionMembers returns explicit union members after structural unwrapping.
func UnionMembers(t typ.Type) []typ.Type {
	switch v := UnwrapStructuralShape(t).(type) {
	case *typ.Union:
		return v.Members
	case *typ.Optional:
		return append([]typ.Type{typ.Nil}, UnionMembers(v.Inner)...)
	default:
		return nil
	}
}
