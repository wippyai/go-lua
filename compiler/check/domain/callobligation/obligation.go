package callobligation

import (
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
)

// Source identifies why a concrete call argument has an obligation.
//
// Signature obligations come from a declared callable shape. Body obligations
// come from solved callee Summary.Params and are preconditions proved by the
// body. Both are checked as proof obligations: top-like `any` may remain in the
// type language, but it is not evidence for a concrete parameter type.
type Source uint8

const (
	SourceNone Source = iota
	SourceSignature
	SourceBody
)

// Shape is the optional domain witness behind an obligation's public type
// projection. It lets producers carry their native obligation algebra to proof
// sites without making callobligation depend on that domain package.
type Shape interface {
	ProjectValue() typ.Type
}

// Obligation is one call argument's caller-visible contract.
type Obligation struct {
	Type   typ.Type
	Source Source
	Shape  Shape
}

// Signature constructs a caller-visible signature obligation.
func Signature(t typ.Type) Obligation {
	if !InformativeType(t) {
		return Obligation{}
	}
	return Obligation{Type: t, Source: SourceSignature}
}

// Body constructs a strict body-summary obligation.
func Body(t typ.Type) Obligation {
	if !InformativeType(t) {
		return Obligation{}
	}
	return Obligation{Type: t, Source: SourceBody}
}

// BodyShape constructs a strict body-summary obligation with its native domain
// witness preserved for contract-aware proof.
func BodyShape(t typ.Type, shape Shape) Obligation {
	if !InformativeType(t) {
		return Obligation{}
	}
	if shape == nil {
		return Body(t)
	}
	return Obligation{Type: t, Source: SourceBody, Shape: shape}
}

// Informative reports whether this obligation carries a concrete caller check.
func (o Obligation) Informative() bool {
	return InformativeType(o.Type)
}

// AllowsGradualAny reports whether a true gradual-top any can satisfy this
// obligation at diagnostics. Current policy is strict: `any` is representation,
// not proof, so concrete call obligations require narrowing/assertion/cast.
func (o Obligation) AllowsGradualAny() bool {
	return false
}

// InformativeType reports whether t carries an enforceable caller obligation.
func InformativeType(t typ.Type) bool {
	if t == nil {
		return false
	}
	if obligationIsTopLike(t, make(obligationTypeSeen)) {
		return false
	}
	return !HasFreeVariable(t)
}

// HasFreeVariable reports whether t still contains an unbound symbolic type
// variable that cannot be enforced as a concrete caller/return obligation.
// Instantiated generics are inspected through their type arguments rather than
// their template body, so a closed `Box<string>` is not rejected merely because
// the generic declaration contains `T`.
func HasFreeVariable(t typ.Type) bool {
	return hasFreeObligationVariable(t, make(obligationTypeSeen))
}

type obligationTypeSeen map[uint64][]typ.Type

func (s obligationTypeSeen) contains(t typ.Type) bool {
	if t == nil || s == nil {
		return false
	}
	for _, existing := range s[obligationTypeSeenKey(t)] {
		if typ.SameNodeOrAcyclicEqual(existing, t) || typ.SameProductFamily(existing, t) {
			return true
		}
	}
	return false
}

func (s obligationTypeSeen) remember(t typ.Type) {
	if t == nil || s == nil {
		return
	}
	key := obligationTypeSeenKey(t)
	s[key] = append(s[key], t)
}

func obligationTypeSeenKey(t typ.Type) uint64 {
	if t == nil {
		return 0
	}
	if typ.ContainsRecursive(t) {
		return typ.ProductFamilyHash(t)
	}
	return t.Hash()
}

func obligationIsTopLike(t typ.Type, seen obligationTypeSeen) bool {
	if t == nil {
		return false
	}
	t = typ.UnwrapAnnotated(t)
	if t == nil {
		return false
	}
	if typ.IsAbsentOrUnknown(t) || typ.IsAny(t) {
		return true
	}
	if seen.contains(t) {
		return false
	}
	seen.remember(t)
	return typ.Visit(t, typ.Visitor[bool]{
		Optional: func(o *typ.Optional) bool {
			return obligationIsTopLike(o.Inner, seen)
		},
		Union: func(u *typ.Union) bool {
			for _, member := range u.Members {
				if obligationIsTopLike(member, seen) {
					return true
				}
			}
			return false
		},
		Alias: func(a *typ.Alias) bool {
			return obligationIsTopLike(a.Target, seen)
		},
		Meta: func(m *typ.Meta) bool {
			return obligationIsTopLike(m.Of, seen)
		},
		Recursive: func(r *typ.Recursive) bool {
			return obligationIsTopLike(r.Body, seen)
		},
	})
}

func hasFreeObligationVariable(t typ.Type, seen obligationTypeSeen) bool {
	if t == nil {
		return false
	}
	t = typ.UnwrapAnnotated(t)
	if t == nil {
		return false
	}
	if seen.contains(t) {
		return false
	}
	seen.remember(t)
	switch t.Kind() {
	case kind.Self, kind.Ref, kind.TypeParam, kind.TypeVar, kind.FieldAccess, kind.IndexAccess, kind.Generic:
		return true
	}
	return typ.Visit(t, typ.Visitor[bool]{
		Optional: func(o *typ.Optional) bool {
			return hasFreeObligationVariable(o.Inner, seen)
		},
		Union: func(u *typ.Union) bool {
			for _, member := range u.Members {
				if hasFreeObligationVariable(member, seen) {
					return true
				}
			}
			return false
		},
		Intersection: func(in *typ.Intersection) bool {
			for _, member := range in.Members {
				if hasFreeObligationVariable(member, seen) {
					return true
				}
			}
			return false
		},
		Array: func(a *typ.Array) bool {
			return hasFreeObligationVariable(a.Element, seen)
		},
		Map: func(m *typ.Map) bool {
			return hasFreeObligationVariable(m.Key, seen) || hasFreeObligationVariable(m.Value, seen)
		},
		ReadonlyMap: func(m *typ.ReadonlyMap) bool {
			return hasFreeObligationVariable(m.Key, seen) || hasFreeObligationVariable(m.Value, seen)
		},
		Tuple: func(tup *typ.Tuple) bool {
			for _, elem := range tup.Elements {
				if hasFreeObligationVariable(elem, seen) {
					return true
				}
			}
			return false
		},
		Function: func(fn *typ.Function) bool {
			for _, param := range fn.Params {
				if hasFreeObligationVariable(param.Type, seen) {
					return true
				}
			}
			if hasFreeObligationVariable(fn.Variadic, seen) {
				return true
			}
			for _, ret := range fn.Returns {
				if hasFreeObligationVariable(ret, seen) {
					return true
				}
			}
			return len(fn.TypeParams) > 0
		},
		Record: func(r *typ.Record) bool {
			if hasFreeObligationVariable(r.MapKey, seen) ||
				hasFreeObligationVariable(r.MapValue, seen) ||
				hasFreeObligationVariable(r.Metatable, seen) {
				return true
			}
			for _, field := range r.Fields {
				if hasFreeObligationVariable(field.Type, seen) {
					return true
				}
			}
			return false
		},
		Alias: func(a *typ.Alias) bool {
			return hasFreeObligationVariable(a.Target, seen)
		},
		Meta: func(m *typ.Meta) bool {
			return hasFreeObligationVariable(m.Of, seen)
		},
		Instantiated: func(i *typ.Instantiated) bool {
			for _, arg := range i.TypeArgs {
				if hasFreeObligationVariable(arg, seen) {
					return true
				}
			}
			return false
		},
		Sum: func(s *typ.Sum) bool {
			for _, variant := range s.Variants {
				for _, t := range variant.Types {
					if hasFreeObligationVariable(t, seen) {
						return true
					}
				}
			}
			return false
		},
		Recursive: func(r *typ.Recursive) bool {
			return hasFreeObligationVariable(r.Body, seen)
		},
		Interface: func(*typ.Interface) bool {
			return false
		},
	})
}

// Normalize canonicalizes an all-empty vector to nil and clones non-empty input.
func Normalize(in []Obligation) []Obligation {
	any := false
	for _, obligation := range in {
		if obligation.Informative() {
			any = true
			break
		}
	}
	if !any {
		return nil
	}
	out := make([]Obligation, len(in))
	copy(out, in)
	return out
}

// Types projects obligations to their concrete type vector.
func Types(in []Obligation) []typ.Type {
	if len(in) == 0 {
		return nil
	}
	out := make([]typ.Type, len(in))
	any := false
	for i, obligation := range in {
		if !obligation.Informative() {
			continue
		}
		out[i] = obligation.Type
		any = true
	}
	if !any {
		return nil
	}
	return out
}

// JoinSource preserves strictness when any joined target contributes a body
// precondition.
func JoinSource(a, b Source) Source {
	if a == SourceBody || b == SourceBody {
		return SourceBody
	}
	if a == SourceSignature || b == SourceSignature {
		return SourceSignature
	}
	return SourceNone
}
