package static

import (
	"github.com/wippyai/go-lua/analysis/lattice"
	"github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	typeauthority "github.com/wippyai/go-lua/domain/type/authority"
)

// TypeFact is the solve carrier for Static's existing ClassSet lattice. The
// only extra state is the factor's empty bottom; every non-bottom fact is an
// owner-issued Class, including normalized unions and AnyValue top.
type TypeFact struct {
	owner   *ClassSet
	class   Class
	present bool
}

func (fact TypeFact) Valid() bool      { return fact.owner != nil && fact.owner.OwnsTypeFact(fact) }
func (fact TypeFact) Owner() *ClassSet { return fact.owner }
func (fact TypeFact) IsBottom() bool {
	return fact.owner != nil && fact.owner.OwnsTypeFact(fact) && !fact.present
}
func (fact TypeFact) IsTop() bool {
	return fact.owner != nil && fact.owner.OwnsTypeFact(fact) && fact.present && fact.owner.Equal(fact.class, fact.owner.AnyValue())
}

// Inner returns an exact Runtime row only for a concrete structural class.
func (fact TypeFact) Inner() typeauthority.RuntimeInner {
	if !fact.Valid() || !fact.present {
		return typeauthority.RuntimeInner{}
	}
	inner, _ := fact.owner.RuntimeForClass(fact.class)
	return inner
}
func (fact TypeFact) RuntimeInner() typeauthority.RuntimeInner { return fact.Inner() }

func (s *ClassSet) TypeBottom() TypeFact {
	if s == nil {
		return TypeFact{}
	}
	return TypeFact{owner: s}
}
func (s *ClassSet) TypeTop() TypeFact {
	if s == nil {
		return TypeFact{}
	}
	return TypeFact{owner: s, class: s.AnyValue(), present: true}
}
func (s *ClassSet) OwnsTypeFact(fact TypeFact) bool {
	return s != nil && fact.owner == s && ((!fact.present && fact.class == (Class{})) || (fact.present && s.Owns(fact.class)))
}

// TypeFactForRuntime projects an exact closed Runtime row through the scalar
// Runtime/Class directory. Structural child rows that are not Class values are
// deliberately not admitted.
func (s *ClassSet) TypeFactForRuntime(inner typeauthority.RuntimeInner) (TypeFact, bool) {
	class, ok := s.ClassForRuntime(inner)
	if !ok {
		return TypeFact{}, false
	}
	return TypeFact{owner: s, class: class, present: true}, true
}

func (s *ClassSet) EqualTypeFact(left, right TypeFact) bool {
	if !s.OwnsTypeFact(left) || !s.OwnsTypeFact(right) || left.present != right.present {
		return false
	}
	return !left.present || s.Equal(left.class, right.class)
}
func (s *ClassSet) LessOrEqTypeFact(left, right TypeFact) bool {
	if !s.OwnsTypeFact(left) || !s.OwnsTypeFact(right) {
		return false
	}
	if !left.present {
		return true
	}
	return right.present && s.LessOrEq(left.class, right.class)
}
func (s *ClassSet) JoinTypeFact(left, right TypeFact) TypeFact {
	if !s.OwnsTypeFact(left) || !s.OwnsTypeFact(right) {
		panic("static: foreign type fact")
	}
	if !left.present {
		return right
	}
	if !right.present {
		return left
	}
	return TypeFact{owner: s, class: s.Join(left.class, right.class), present: true}
}
func (s *ClassSet) TypeFactLattice() lattice.Lattice[TypeFact] {
	if s == nil {
		return lattice.Lattice[TypeFact]{}
	}
	return lattice.Lattice[TypeFact]{
		Bottom:   s.TypeBottom,
		Top:      s.TypeTop,
		Equal:    s.EqualTypeFact,
		Same:     s.EqualTypeFact,
		LessOrEq: s.LessOrEqTypeFact,
		Join:     s.JoinTypeFact,
		Widen:    s.JoinTypeFact,
	}
}
func (s *ClassSet) TypeFactFingerprint(fact TypeFact) uint64 {
	if !s.OwnsTypeFact(fact) || !fact.present {
		return 0
	}
	return s.Fingerprint(fact.class)
}
func (s *ClassSet) TypeFactWidenRank(fact TypeFact) uint64 {
	if !s.OwnsTypeFact(fact) {
		return 0
	}
	if !fact.present {
		return uint64(s.universeSize) + 2
	}
	return s.Rank(fact.class)
}

// TypeFactForTarget projects one concrete declaration from this exact Target.
// Scoped/opaque declarations remain unjudged rather than widening by default.
func (s *ClassSet) TypeFactForTarget(target *contract.Contract, value vocabulary.Type) (TypeFact, bool) {
	class, ok := s.ClassForTarget(target, value)
	if !ok {
		return TypeFact{}, false
	}
	if _, concrete := s.RuntimeForClass(class); !concrete {
		return TypeFact{}, false
	}
	return TypeFact{owner: s, class: class, present: true}, true
}
func (s *ClassSet) TypeFactForOwnTarget(value vocabulary.Type) (TypeFact, bool) {
	if s == nil {
		return TypeFact{}, false
	}
	return s.TypeFactForTarget(s.target, value)
}

// TypeFactField projects one direct field from an exact record. Derived or
// opaque classes have no single structural row and are refused. Missing fields
// remain absent; they never become AnyValue or language-level Unknown.
func (s *ClassSet) TypeFactField(base TypeFact, key string) (TypeFact, bool) {
	if !s.OwnsTypeFact(base) || !base.present {
		return TypeFact{}, false
	}
	if base.IsTop() {
		return s.TypeTop(), true
	}
	inner, ok := s.RuntimeForClass(base.class)
	if !ok || s.runtime == nil {
		return TypeFact{}, false
	}
	field, ok := s.runtime.Field(inner, key)
	if !ok {
		return TypeFact{}, false
	}
	child, ok := s.TypeFactForRuntime(field.Inner)
	if !ok {
		return TypeFact{}, false
	}
	if !field.Optional {
		return child, true
	}
	return TypeFact{owner: s, class: s.Join(child.class, s.Nil()), present: true}, true
}
