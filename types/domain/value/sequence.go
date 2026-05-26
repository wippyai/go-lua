package value

import (
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// JoinSequenceShape joins array-like table observations in their sequence
// domain. Tuples are exact local table-literal shapes; when they meet arrays at
// a call boundary, the canonical upper bound is an array with the joined element
// type. Same-arity tuples can keep exact positional precision.
func JoinSequenceShape(a, b typ.Type, join func(typ.Type, typ.Type) typ.Type) (typ.Type, bool) {
	if join == nil {
		join = typ.JoinPreferNonSoft
	}
	aInner, aNilable := SplitNilable(a)
	bInner, bNilable := SplitNilable(b)
	if aNilable {
		a = aInner
	}
	if bNilable {
		b = bInner
	}
	left, okLeft := sequenceShapeOf(a)
	right, okRight := sequenceShapeOf(b)
	if !okLeft || !okRight {
		return nil, false
	}
	if upper, ok := SelfEmbeddingUpperBound(a, b); ok {
		return upper, true
	}
	wrap := func(t typ.Type) typ.Type {
		if aNilable || bNilable {
			return typ.NewOptional(t)
		}
		return t
	}
	if left.array && right.array {
		return wrap(typ.NewArray(join(left.elementUnion(join), right.elementUnion(join)))), true
	}
	if !left.array && !right.array && len(left.elements) == len(right.elements) {
		elements := make([]typ.Type, len(left.elements))
		for i := range left.elements {
			elements[i] = join(left.elements[i], right.elements[i])
		}
		return wrap(typ.NewTuple(elements...)), true
	}
	return wrap(typ.NewArray(join(left.elementUnion(join), right.elementUnion(join)))), true
}

// CollapseSequenceUnion folds unions whose non-nil members are exact tuple and
// array observations into one canonical sequence shape.
func CollapseSequenceUnion(t typ.Type, join func(typ.Type, typ.Type) typ.Type) typ.Type {
	if t == nil {
		return nil
	}
	if join == nil {
		join = typ.JoinPreferNonSoft
	}
	switch v := unwrap.Alias(t).(type) {
	case *typ.Optional:
		inner := CollapseSequenceUnion(v.Inner, join)
		if inner == nil || typ.TypeEquals(inner, v.Inner) {
			return t
		}
		return typ.NewOptional(inner)
	case *typ.Union:
		return collapseSequenceUnionMembers(v, join)
	default:
		return t
	}
}

func collapseSequenceUnionMembers(u *typ.Union, join func(typ.Type, typ.Type) typ.Type) typ.Type {
	var sequence typ.Type
	residual := make([]typ.Type, 0, len(u.Members))
	changed := false
	for _, member := range u.Members {
		collapsed := CollapseSequenceUnion(member, join)
		if !typ.TypeEquals(collapsed, member) {
			changed = true
		}
		if _, ok := sequenceShapeOf(collapsed); ok {
			if sequence == nil {
				sequence = collapsed
			} else if joined, ok := JoinSequenceShape(sequence, collapsed, join); ok {
				sequence = joined
				changed = true
			}
			continue
		}
		residual = append(residual, collapsed)
	}
	if sequence == nil {
		if changed {
			return typ.NewUnion(residual...)
		}
		return u
	}
	residual = append(residual, sequence)
	if len(residual) == 1 {
		return sequence
	}
	return typ.NewUnion(residual...)
}

type sequenceShape struct {
	array    bool
	elements []typ.Type
}

func sequenceShapeOf(t typ.Type) (sequenceShape, bool) {
	switch v := unwrap.Alias(t).(type) {
	case *typ.Array:
		return sequenceShape{array: true, elements: []typ.Type{v.Element}}, true
	case *typ.Tuple:
		if len(v.Elements) == 0 {
			return sequenceShape{}, false
		}
		return sequenceShape{elements: v.Elements}, true
	default:
		return sequenceShape{}, false
	}
}

func (s sequenceShape) elementUnion(join func(typ.Type, typ.Type) typ.Type) typ.Type {
	var out typ.Type
	for _, elem := range s.elements {
		out = join(out, elem)
	}
	if out == nil {
		return typ.Unknown
	}
	return out
}
