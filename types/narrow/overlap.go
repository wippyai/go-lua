package narrow

import (
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

type overlapPair struct {
	a typ.Type
	b typ.Type
}

func mayOverlap(a, b typ.Type, seen map[overlapPair]bool) bool {
	a = typ.UnwrapAnnotated(unwrap.Alias(a))
	b = typ.UnwrapAnnotated(unwrap.Alias(b))
	if ai, ok := a.(*typ.Instantiated); ok {
		if bi, ok := b.(*typ.Instantiated); ok {
			return instantiatedTypesOverlap(ai, bi)
		}
	}
	a = normalizeOverlapType(a)
	b = normalizeOverlapType(b)
	if a == nil || b == nil || typ.IsNever(a) || typ.IsNever(b) {
		return false
	}
	if a == b || typ.IsAny(a) || typ.IsAny(b) || typ.IsUnknown(a) || typ.IsUnknown(b) {
		return true
	}
	if seen == nil {
		seen = make(map[overlapPair]bool)
	}
	pair := overlapPair{a: a, b: b}
	if seen[pair] {
		return true
	}
	seen[pair] = true

	if u, ok := a.(*typ.Union); ok {
		for _, member := range u.Members {
			if mayOverlap(member, b, seen) {
				return true
			}
		}
		return false
	}
	if u, ok := b.(*typ.Union); ok {
		for _, member := range u.Members {
			if mayOverlap(a, member, seen) {
				return true
			}
		}
		return false
	}

	if i, ok := a.(*typ.Intersection); ok {
		for _, member := range i.Members {
			if !mayOverlap(member, b, seen) {
				return false
			}
		}
		return true
	}
	if i, ok := b.(*typ.Intersection); ok {
		for _, member := range i.Members {
			if !mayOverlap(a, member, seen) {
				return false
			}
		}
		return true
	}

	if r, ok := a.(*typ.Recursive); ok {
		return r.Body != nil && r.Body != r && mayOverlap(r.Body, b, seen)
	}
	if r, ok := b.(*typ.Recursive); ok {
		return r.Body != nil && r.Body != r && mayOverlap(a, r.Body, seen)
	}

	if o, ok := a.(*typ.Optional); ok {
		return b.Kind() == kind.Nil || mayOverlap(o.Inner, b, seen)
	}
	if o, ok := b.(*typ.Optional); ok {
		return a.Kind() == kind.Nil || mayOverlap(a, o.Inner, seen)
	}
	if a.Kind() == kind.Nil || b.Kind() == kind.Nil {
		return false
	}

	if lit, ok := a.(*typ.Literal); ok {
		return literalOverlaps(lit, b)
	}
	if lit, ok := b.(*typ.Literal); ok {
		return literalOverlaps(lit, a)
	}

	if a.Kind() != b.Kind() {
		return crossKindMayOverlap(a, b, seen)
	}

	switch av := a.(type) {
	case *typ.Record:
		bv, ok := b.(*typ.Record)
		return ok && recordsMayOverlap(av, bv, seen)
	case *typ.Array:
		bv, ok := b.(*typ.Array)
		return ok && mayOverlap(av.Element, bv.Element, seen)
	case *typ.Map:
		bv, ok := b.(*typ.Map)
		return ok && mayOverlap(av.Key, bv.Key, seen) && mayOverlap(av.Value, bv.Value, seen)
	case *typ.Tuple:
		bv, ok := b.(*typ.Tuple)
		if !ok || len(av.Elements) != len(bv.Elements) {
			return false
		}
		for i := range av.Elements {
			if !mayOverlap(av.Elements[i], bv.Elements[i], seen) {
				return false
			}
		}
		return true
	case *typ.Interface:
		bv, ok := b.(*typ.Interface)
		return ok && interfacesMayOverlap(av, bv, seen)
	case *typ.Function, *typ.Generic, *typ.Instantiated, *typ.TypeParam:
		return true
	default:
		return a.Kind() == b.Kind()
	}
}

func instantiatedTypesOverlap(a, b *typ.Instantiated) bool {
	if a == nil || b == nil || a.Generic == nil || b.Generic == nil {
		return false
	}
	if !typ.TypeEquals(a.Generic, b.Generic) || len(a.TypeArgs) != len(b.TypeArgs) {
		return false
	}
	for i := range a.TypeArgs {
		if !typ.TypeEquals(a.TypeArgs[i], b.TypeArgs[i]) {
			return false
		}
	}
	return true
}

func normalizeOverlapType(t typ.Type) typ.Type {
	t = unwrap.Alias(t)
	t = typ.UnwrapAnnotated(t)
	if expanded := unwrap.Instantiated(t); expanded != nil && expanded != t {
		return expanded
	}
	return t
}

func literalOverlaps(lit *typ.Literal, other typ.Type) bool {
	if lit == nil || other == nil {
		return false
	}
	if otherLit, ok := other.(*typ.Literal); ok {
		return typ.LiteralEquals(lit, otherLit)
	}
	switch lit.Base {
	case kind.Boolean:
		return other.Kind() == kind.Boolean
	case kind.String:
		return other.Kind() == kind.String
	case kind.Integer:
		return other.Kind() == kind.Integer || other.Kind() == kind.Number
	case kind.Number:
		return other.Kind() == kind.Number
	default:
		return false
	}
}

func crossKindMayOverlap(a, b typ.Type, seen map[overlapPair]bool) bool {
	if a.Kind() == kind.Integer && b.Kind() == kind.Number {
		return true
	}
	if a.Kind() == kind.Number && b.Kind() == kind.Integer {
		return true
	}
	if record, ok := a.(*typ.Record); ok {
		return recordMayOverlapTable(record, b, seen)
	}
	if record, ok := b.(*typ.Record); ok {
		return recordMayOverlapTable(record, a, seen)
	}
	if array, ok := a.(*typ.Array); ok {
		return arrayMayOverlapTable(array, b, seen)
	}
	if array, ok := b.(*typ.Array); ok {
		return arrayMayOverlapTable(array, a, seen)
	}
	if m, ok := a.(*typ.Map); ok {
		return mapMayOverlapTable(m, b, seen)
	}
	if m, ok := b.(*typ.Map); ok {
		return mapMayOverlapTable(m, a, seen)
	}
	return false
}

func recordsMayOverlap(a, b *typ.Record, seen map[overlapPair]bool) bool {
	for _, af := range a.Fields {
		bf := b.GetField(af.Name)
		if bf == nil {
			continue
		}
		if !mayOverlap(af.Type, bf.Type, seen) {
			return false
		}
	}
	return true
}

func recordMayOverlapTable(record *typ.Record, other typ.Type, seen map[overlapPair]bool) bool {
	switch o := other.(type) {
	case *typ.Map:
		for _, field := range record.Fields {
			if !mayOverlap(typ.LiteralString(field.Name), o.Key, seen) {
				continue
			}
			if !mayOverlap(field.Type, o.Value, seen) {
				return false
			}
		}
		return true
	case *typ.Interface:
		return recordMayImplementInterface(record, o, seen)
	case *typ.Array, *typ.Tuple:
		return true
	default:
		return false
	}
}

func arrayMayOverlapTable(array *typ.Array, other typ.Type, seen map[overlapPair]bool) bool {
	switch o := other.(type) {
	case *typ.Map:
		return mayOverlap(typ.Integer, o.Key, seen) && mayOverlap(array.Element, o.Value, seen)
	case *typ.Tuple:
		for _, elem := range o.Elements {
			if !mayOverlap(array.Element, elem, seen) {
				return false
			}
		}
		return true
	case *typ.Record, *typ.Interface:
		return true
	default:
		return false
	}
}

func mapMayOverlapTable(m *typ.Map, other typ.Type, seen map[overlapPair]bool) bool {
	switch o := other.(type) {
	case *typ.Tuple:
		return mayOverlap(typ.Integer, m.Key, seen) && tupleElementsOverlapMapValue(o, m.Value, seen)
	case *typ.Interface:
		return true
	default:
		return false
	}
}

func tupleElementsOverlapMapValue(tuple *typ.Tuple, value typ.Type, seen map[overlapPair]bool) bool {
	for _, elem := range tuple.Elements {
		if !mayOverlap(elem, value, seen) {
			return false
		}
	}
	return true
}

func interfacesMayOverlap(a, b *typ.Interface, seen map[overlapPair]bool) bool {
	if a == nil || b == nil {
		return false
	}
	if a.Name != "" && a.Name == b.Name {
		return true
	}
	for _, am := range a.Methods {
		for _, bm := range b.Methods {
			if am.Name != bm.Name {
				continue
			}
			return mayOverlap(am.Type, bm.Type, seen)
		}
	}
	return false
}

func recordMayImplementInterface(record *typ.Record, iface *typ.Interface, seen map[overlapPair]bool) bool {
	if record == nil || iface == nil {
		return false
	}
	for _, method := range iface.Methods {
		field := record.GetField(method.Name)
		if field == nil || !mayOverlap(field.Type, method.Type, seen) {
			return false
		}
	}
	return true
}
