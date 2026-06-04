package shapevalue

import (
	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
)

// Value is the Shape/Value axis abstraction of a runtime value's structural type.
//
// Value is immutable: it holds a single typ.Type node and exposes only lattice
// operations. The raw type is recovered through Project at the diagnostic/subtype
// boundary. Two Values that describe the same structural family compare Equal even
// when they hold distinct (but equivalent) type nodes.
type Value struct {
	t typ.Type
}

// Of lifts a structural type into the Shape/Value axis.
//
// A nil type is normalized to Top because the absence of structural evidence is
// the fully-dynamic value. A recursive product family is canonicalized to its
// single interned representative, so two observations of the same family hold the
// same node pointer and recursive Equal reduces to pointer identity, consistent
// with the family hash by construction.
func Of(t typ.Type) Value {
	if t == nil {
		return Top()
	}
	return Value{t: value.CanonicalRecursiveFamily(t)}
}

// Bottom is the uninhabited type (never): the empty set of runtime values.
func Bottom() Value {
	return Value{t: typ.Never}
}

// Top is the fully-dynamic type (any): every runtime value.
func Top() Value {
	return Value{t: typ.Any}
}

// IsBottom reports whether the value is the uninhabited type.
func (v Value) IsBottom() bool {
	return v.t != nil && v.t.Kind().IsNever()
}

// IsTop reports whether the value is the fully-dynamic type.
func (v Value) IsTop() bool {
	if v.t == nil {
		return true
	}
	return v.t.Kind() == kind.Any
}

// Join is the least upper bound under convergence widening.
//
// Join over-approximates the union of the two value sets. It delegates to the
// proven convergence merge so recursive product families fold coinductively
// instead of unfolding by node identity. When one operand already covers the
// other, the covering operand is returned unchanged so Join is the exact least
// upper bound under the lattice order rather than a structurally larger node.
//
// Top-level aliases are resolved by law before the lattice merge, so the result is
// deterministic and commutative regardless of interner order: the result carries
// an alias only when both operands are the same alias (same name over Equal
// targets); otherwise it is the alias-free join of the unwrapped targets. Joining
// an alias with its own target, or with a different alias, therefore drops to the
// unaliased representative — the merged observation is no longer unambiguously the
// alias, and Covers (alias-transparent) still relates the result to either operand.
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
	aa, aIsAlias := topLevelAlias(a.t)
	ba, bIsAlias := topLevelAlias(b.t)
	if aIsAlias && bIsAlias && aa.Name == ba.Name && sameShapeFamily(aa.Target, ba.Target) {
		return a
	}
	if aIsAlias || bIsAlias {
		return joinUnaliased(unaliasTopLevel(a.t, aa, aIsAlias), unaliasTopLevel(b.t, ba, bIsAlias))
	}
	return joinUnaliased(a.t, b.t)
}

// joinUnaliased is the alias-free lattice join over two structural types.
//
// Covers is a preorder, not antisymmetric with carrier identity, so returning
// either operand under mutual coverage is unsound for a commutative join:
// when a.Covers(b) AND b.Covers(a) but the carrier values differ (alias vs
// bare, recursive vs union with the recursive arm), returning the first one
// makes Join order-sensitive. The fix is to canonicalize through the
// convergence merge whenever both Covers checks succeed without Equal.
func joinUnaliased(at, bt typ.Type) Value {
	if typ.ContainsRecursive(at) {
		at = value.CanonicalRecursiveFamily(at)
	}
	if typ.ContainsRecursive(bt) {
		bt = value.CanonicalRecursiveFamily(bt)
	}
	if at == bt {
		return Value{t: at}
	}
	a := Value{t: at}
	b := Value{t: bt}
	aCovB := a.Covers(b)
	bCovA := b.Covers(a)
	// A record that width-covers another (it requires a strict subset of fields)
	// is an upper bound but not the LEAST one: the least upper bound carries the
	// union of fields with every non-shared field made optional ({id} join
	// {id,name} = {id, name?}, strictly below {id} in the order). Returning the
	// covering operand here would diverge from the record-into-union join and break
	// associativity, so width-differing records always route through the
	// optionalizing structural join below.
	if !value.RecordWidthDiffer(at, bt) {
		if aCovB && !bCovA {
			return a
		}
		if bCovA && !aCovB {
			return b
		}
		if aCovB && bCovA && Equal(a, b) {
			return a
		}
	}
	return Value{t: canonical(value.JoinForConvergence(at, bt))}
}

// unaliasTopLevel returns t with its outermost alias wrapper removed.
func unaliasTopLevel(t typ.Type, a *typ.Alias, isAlias bool) typ.Type {
	if isAlias {
		return a.Target
	}
	return t
}

// Widen accelerates an ascending chain toward a fixed point.
//
// prev is the previous iterate and next the freshly computed one. The result is
// a sound upper bound of next that bounds unbounded structural growth. Widening
// is applied to the joined value so it never sits below the join.
//
// When Join preserved a top-level alias (both operands the same alias), widening
// keeps the alias carrier and widens only its target, so a stable aliased chain
// does not lose its name to the alias-transparent convergence widener.
func Widen(prev, next Value) Value {
	joined := Join(prev, next)
	if joined.IsTop() || joined.IsBottom() {
		return joined
	}
	if a, ok := topLevelAlias(joined.t); ok {
		widenedTarget := canonical(value.WidenForConvergence(a.Target))
		return Value{t: typ.NewAlias(a.Name, widenedTarget)}
	}
	return Value{t: canonical(value.WidenForConvergence(joined.t))}
}

// Equal is the value-domain convergence equivalence: the two shapes are the same
// point in the convergence lattice the flow engine folds over.
//
// For non-recursive shapes this is value.SameConvergedFact, the flow engine's
// no-op/change-detection relation, not mutual coverage. Mutual coverage is too
// coarse for a flow-state carrier: it conflates gradual-consistent types because
// typ.Any and typ.Unknown cover each other, so it would collapse a placeholder
// unknown into a dynamic any. SameConvergedFact keeps them distinct, so a value
// seeded unknown round-trips losslessly. Structurally identical nodes and
// reordered unions (canonical member order from typ.NewUnion) still compare equal
// because they are the same converged fact. Covers remains the lattice order
// (value.Covers); only Equal is the finer convergence relation.
//
// A recursive product family carries its single canonical representative (Of and
// the merge output canonicalize it), so two observations of the same family hold
// the same node pointer. Recursive Equal is therefore canonical-rep identity (==),
// which is the kernel of the family hash by construction; it does not re-prove a
// structural bisimulation per comparison.
//
// A top-level type alias is a distinct carrier identity, not its unwrapped target.
// SameConvergedFact is alias-transparent (typ.TypeEquals unwraps the alias), so it
// would collapse an aliased flow value onto its bare target and lose the alias name
// through interning. Equal therefore first compares the top-level alias wrapper:
// two aliased shapes are equal only when they carry the same alias name over Equal
// targets, and an aliased shape never equals its unwrapped target. This is the same
// split as the unknown/any distinction: Equal is the finer carrier/convergence
// identity, Covers stays the alias-transparent lattice order.
func Equal(a, b Value) bool {
	at := a.Project()
	bt := b.Project()
	aa, aIsAlias := topLevelAlias(at)
	ba, bIsAlias := topLevelAlias(bt)
	if aIsAlias || bIsAlias {
		if !aIsAlias || !bIsAlias {
			return false
		}
		return aa.Name == ba.Name && sameShapeFamily(aa.Target, ba.Target)
	}
	return sameShapeFamily(at, bt)
}

// sameShapeFamily is the value-domain convergence equivalence over an unaliased
// shape. A recursive family is compared by canonical-rep identity: both operands
// pass through CanonicalRecursiveFamily at admission, so the same family is the
// same pointer and Equal is consistent with the family hash without unfolding the
// cycle. Non-recursive shapes use the structural converged-fact relation.
func sameShapeFamily(a, b typ.Type) bool {
	if a == b {
		return true
	}
	if typ.ContainsRecursive(a) || typ.ContainsRecursive(b) {
		return value.CanonicalRecursiveFamily(a) == value.CanonicalRecursiveFamily(b)
	}
	return value.SameConvergedFact(a, b)
}

// topLevelAlias reports whether t is a top-level alias node and returns it. Only
// the outermost wrapper is inspected: aliases nested inside a structural node ride
// along inside that node and are compared by the alias-transparent convergence
// relation, matching Covers.
func topLevelAlias(t typ.Type) (*typ.Alias, bool) {
	a, ok := t.(*typ.Alias)
	return a, ok && a != nil
}

// Hash is a stable, cycle-safe semantic hash consistent with Equal: equal values
// hash identically.
//
// Union and intersection members fold commutatively (order-independent) so two
// Equal values whose only difference is member order hash identically. Recursive
// families fold coinductively via the product-family hash. The leading kind keys
// the fold, so gradual placeholders that Equal keeps distinct (unknown vs any)
// also hash distinctly.
//
// A top-level alias keys the hash on its name combined with the target hash, so an
// aliased shape hashes distinctly from its unwrapped target and from a different
// alias over the same target, keeping Equal => EqualHash for the alias carrier
// distinction.
func (v Value) Hash() uint64 {
	if v.t == nil {
		return uint64(kind.Any)
	}
	if a, ok := topLevelAlias(v.t); ok {
		h := internal.HashCombine(uint64(kind.Alias), internal.FnvString(a.Name))
		return internal.HashCombine(h, semanticHash(a.Target, typ.NewGuard()))
	}
	return semanticHash(v.t, typ.NewGuard())
}

// Covers reports whether the receiver is at least as high as other in the lattice.
//
// Covers is the proven value-domain order: product-family coverage for recursive
// values, ordinary subtype for acyclic ones. Join is defined in terms of Covers,
// keeping the two consistent.
func (v Value) Covers(other Value) bool {
	return value.Covers(v.Project(), other.Project())
}

// Project recovers the underlying structural type at the diagnostic/subtype
// boundary. This is the only legal egress of typ.Type from the axis.
func (v Value) Project() typ.Type {
	if v.t == nil {
		return typ.Any
	}
	return v.t
}

// String renders the axis value for diagnostics.
func (v Value) String() string {
	return v.Project().String()
}

// canonical normalizes union member order at every depth so the convergence merge
// output is a canonical representative of its value set, then hash-conses a
// recursive product family to its single interned representative.
//
// The convergence merge composes unions in operand order, so joining the same
// inputs in opposite orders can yield unions whose members differ only in order.
// Equal compares unions positionally (typ.NewUnion gives a canonical member order
// at admission), so the axis restores that canonical order on merge output to keep
// Join and Widen commutative up to Equal. Re-admitting each union node through
// typ.NewUnion is a semantic no-op on a set of members and is cycle-safe via
// Rewrite.
//
// A merged recursive family is then routed through CanonicalRecursiveFamily so the
// fresh node the merge built collapses onto the family's canonical representative.
// Without this, Join/Widen would emit a hash-equal but pointer-distinct recursive
// node every iteration and recursive Equal (canonical-rep identity) would never
// see a fixed point.
func canonical(t typ.Type) typ.Type {
	if t == nil {
		return nil
	}
	normalized := typ.Rewrite(t, func(node typ.Type) (typ.Type, bool) {
		if u, ok := node.(*typ.Union); ok {
			return typ.NewUnion(u.Members...), true
		}
		return nil, false
	})
	return value.CanonicalRecursiveFamily(normalized)
}

// semanticHash is order-independent over union and intersection members and
// cycle-safe via the guard, so it never distinguishes values that Equal treats
// as equivalent. Recursive families collapse to a stable family marker.
func semanticHash(t typ.Type, guard internal.RecursionGuard) uint64 {
	if t == nil {
		return uint64(kind.Nil)
	}
	next, ok := guard.Enter(t)
	if !ok {
		return internal.HashCombine(uint64(t.Kind()), internal.FnvString("$cycle"))
	}
	switch v := t.(type) {
	case *typ.Union:
		return commutativeMemberHash(kind.Union, v.Members, next)
	case *typ.Intersection:
		return commutativeMemberHash(kind.Intersection, v.Members, next)
	default:
		if typ.ContainsRecursive(t) {
			return typ.ProductFamilyHash(t)
		}
		return internal.HashCombine(uint64(t.Kind()), t.Hash())
	}
}

// commutativeMemberHash folds member hashes with XOR so member order does not
// affect the result, then mixes in the member count and the collection kind.
func commutativeMemberHash(k kind.Kind, members []typ.Type, guard internal.RecursionGuard) uint64 {
	var fold uint64
	for _, m := range members {
		fold ^= semanticHash(m, guard)
	}
	h := internal.HashCombine(uint64(k), uint64(len(members)))
	return internal.HashCombine(h, fold)
}
