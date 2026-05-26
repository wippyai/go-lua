package product

import (
	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/axis/effectrows"
	"github.com/wippyai/go-lua/types/domain/value/axis/escape"
	"github.com/wippyai/go-lua/types/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/types/domain/value/axis/identityrecursion"
	"github.com/wippyai/go-lua/types/domain/value/axis/numeric"
	"github.com/wippyai/go-lua/types/domain/value/axis/ownership"
	"github.com/wippyai/go-lua/types/domain/value/axis/presence"
	"github.com/wippyai/go-lua/types/domain/value/axis/shapevalue"
	"github.com/wippyai/go-lua/types/typ"
)

// node is the canonical interned content of an AbstractValue: one value per axis.
//
// A node is deeply immutable once interned. It is never compared by Go == (it
// contains non-comparable axis carriers); equality is the component-wise lattice
// equivalence in nodeEqual, and its bucket key is nodeHash. Provenance is not part
// of the node: equal values share one node regardless of provenance.
type node struct {
	shape    shapevalue.Value
	presence presence.Value
	numeric  numeric.Value
	effects  effectrows.Value
	owner    ownership.Value
	escape   escape.Value
	identity identityrecursion.Value
	evidence evidence.Value
}

// AbstractValue is the opaque, deeply immutable, interned reduced product over the
// value-domain axes.
//
// It is a comparable handle: the node pointer is the canonical identity, so two
// AbstractValues built from equal content share one node and compare equal by
// pointer (the Equal fast path). Provenance is a diagnostic sidecar excluded from
// Equal and Hash. The zero AbstractValue is invalid; construct values through New
// or FromType.
type AbstractValue struct {
	n    *node
	prov *provenance
}

// New constructs an interned AbstractValue from explicit axis values.
//
// This is the axis-level admission boundary. The result is reduced (the registered
// cross-axis reducers run to a local fixed point) and interned, so equal inputs
// yield the same canonical node.
func New(
	shape shapevalue.Value,
	pres presence.Value,
	num numeric.Value,
	eff effectrows.Value,
	own ownership.Value,
	esc escape.Value,
	id identityrecursion.Value,
	ev evidence.Value,
) AbstractValue {
	n := &node{
		shape:    shape,
		presence: pres,
		numeric:  num,
		effects:  eff,
		owner:    own,
		escape:   esc,
		identity: id,
		evidence: ev,
	}
	return AbstractValue{n: intern(reduce(n))}
}

// FromType constructs an interned AbstractValue from a structural type at the
// admission boundary.
//
// Nilability is factored off the shape axis onto the presence axis, matching the
// value domain's convergence merge (which elides nil from structural joins):
//
//   - The Shape/Value axis carries the non-nil part of t. A purely-nil type has no
//     non-nil structural content, so its shape is Bottom; a dynamic type (any or
//     unknown) is Top.
//   - The Presence axis records nilability: a dynamic type is Maybe (it may be
//     nil), the uninhabited type is Bottom, a purely-nil type is Absent, a
//     nil-admitting type is Maybe, and any other concrete type is Present.
//   - The Identity/Recursion axis records recursive product-family membership: a
//     recursive type carries its family identity (so distinct families stay
//     distinct under Join), a non-recursive type carries Top. Identity is derived
//     from the same non-nil structural content as the shape, since nilability is
//     factored onto presence.
//
// The remaining axes carry their identity (Top) value.
func FromType(t typ.Type) AbstractValue {
	return New(
		shapeOf(t),
		presenceOf(t),
		numeric.Top(),
		effectrows.Top(),
		ownership.Top(),
		escape.Top(),
		identityOf(t),
		evidence.Top(),
	)
}

// Top is the most general AbstractValue: every axis at its Top.
func Top() AbstractValue {
	return New(
		shapevalue.Top(),
		presence.Top(),
		numeric.Top(),
		effectrows.Top(),
		ownership.Top(),
		escape.Top(),
		identityrecursion.Top(),
		evidence.Top(),
	)
}

// Bottom is the least AbstractValue: every axis at its Bottom (the empty value).
func Bottom() AbstractValue {
	return New(
		shapevalue.Bottom(),
		presence.Bottom(),
		numeric.Bottom(),
		effectrows.Bottom(),
		ownership.Bottom(),
		escape.Bottom(),
		identityrecursion.Bottom(),
		evidence.Bottom(),
	)
}

// Shape returns the Shape/Value axis component.
func (v AbstractValue) Shape() shapevalue.Value { return v.n.shape }

// Presence returns the Presence/Nilability axis component.
func (v AbstractValue) Presence() presence.Value { return v.n.presence }

// Numeric returns the Numeric/Interval axis component.
func (v AbstractValue) Numeric() numeric.Value { return v.n.numeric }

// Effects returns the EffectRows axis component.
func (v AbstractValue) Effects() effectrows.Value { return v.n.effects }

// Ownership returns the Ownership/Linearity axis component.
func (v AbstractValue) Ownership() ownership.Value { return v.n.owner }

// Escape returns the Escape/Allocation axis component.
func (v AbstractValue) Escape() escape.Value { return v.n.escape }

// Identity returns the Identity/Recursion axis component.
func (v AbstractValue) Identity() identityrecursion.Value { return v.n.identity }

// Evidence returns the SemanticEvidence axis component.
func (v AbstractValue) Evidence() evidence.Value { return v.n.evidence }

// Project recovers the underlying non-nil structural type carried on the Shape
// axis. It is the bare shape egress: nilability, which FromType factors onto the
// Presence axis, is not recombined. Callers that need the full value-domain type
// (the lossless inverse of FromType) use ProjectValue.
func (v AbstractValue) Project() typ.Type {
	return v.n.shape.Project()
}

// ProjectValue recombines the Shape and Presence axes into the full structural
// type, the lossless inverse of FromType at the value-domain boundary.
//
// FromType factors a type's nilability off the shape and onto the presence axis:
// the shape carries the non-nil content, the presence records whether the slot may
// be nil. ProjectValue restores that factoring so SameConvergedFact(t,
// FromType(t).ProjectValue()) holds for every t, including the gradual placeholders
// any and unknown (whose dynamic shape already admits nil, so the presence Maybe
// adds nothing and the shape is returned unchanged).
//
//   - Presence Bottom: the value is unreachable, so it projects to never.
//   - Presence Absent: the slot is definitely nil, so it projects to nil.
//   - Presence Maybe over a dynamic shape (any/unknown): the shape already admits
//     nil, so it is returned as is (re-wrapping would lose the placeholder).
//   - Presence Maybe over a concrete shape: the slot may be nil, so the non-nil
//     shape is wrapped optional.
//   - Presence Present (or any other state): the slot definitely holds the shape's
//     non-nil content, returned as is.
func (v AbstractValue) ProjectValue() typ.Type {
	shape := v.n.shape.Project()
	switch {
	case v.n.presence.IsBottom():
		return typ.Never
	case presence.Equal(v.n.presence, presence.Absent()):
		return typ.Nil
	case presence.Equal(v.n.presence, presence.Maybe()):
		if v.n.shape.IsTop() || typ.IsUnknown(shape) {
			return shape
		}
		if v.n.shape.IsBottom() {
			return typ.Nil
		}
		return typ.NewOptional(shape)
	default:
		return shape
	}
}

// IsZero reports whether the value is the zero AbstractValue: an uninitialized
// handle with no interned node. It is the value a Go map read returns for an absent
// key, distinct from Bottom (the interned empty value). Callers at a storage seam
// use it to tell an absent slot from a stored empty one.
func (v AbstractValue) IsZero() bool {
	return v.n == nil
}

// WithProvenance returns the same interned value carrying the given provenance.
//
// Provenance is a diagnostic sidecar: the node (hence Equal and Hash) is
// unchanged, so a value with provenance is Equal to the same value without it.
func (v AbstractValue) WithProvenance(p Provenance) AbstractValue {
	v.prov = newProvenance(p)
	return v
}

// Provenance returns the diagnostic provenance attached to the value, if any.
func (v AbstractValue) Provenance() (Provenance, bool) {
	if v.prov == nil {
		return Provenance{}, false
	}
	return v.prov.data, true
}

// Join is the component-wise least upper bound across all axes, reduced and
// interned. The component join runs first, then the registered cross-axis reducers
// refine the result to a local fixed point.
func Join(a, b AbstractValue) AbstractValue {
	n := &node{
		shape:    shapevalue.Join(a.n.shape, b.n.shape),
		presence: presence.Join(a.n.presence, b.n.presence),
		numeric:  numeric.Join(a.n.numeric, b.n.numeric),
		effects:  effectrows.Join(a.n.effects, b.n.effects),
		owner:    ownership.Join(a.n.owner, b.n.owner),
		escape:   escape.Join(a.n.escape, b.n.escape),
		identity: identityrecursion.Join(a.n.identity, b.n.identity),
		evidence: evidence.Join(a.n.evidence, b.n.evidence),
	}
	return AbstractValue{n: intern(reduce(n))}
}

// Widen is the component-wise widening from prev to next, reduced and interned.
func Widen(prev, next AbstractValue) AbstractValue {
	n := &node{
		shape:    shapevalue.Widen(prev.n.shape, next.n.shape),
		presence: presence.Widen(prev.n.presence, next.n.presence),
		numeric:  numeric.Widen(prev.n.numeric, next.n.numeric),
		effects:  effectrows.Widen(prev.n.effects, next.n.effects),
		owner:    ownership.Widen(prev.n.owner, next.n.owner),
		escape:   escape.Widen(prev.n.escape, next.n.escape),
		identity: identityrecursion.Widen(prev.n.identity, next.n.identity),
		evidence: evidence.Widen(prev.n.evidence, next.n.evidence),
	}
	return AbstractValue{n: intern(reduce(n))}
}

// Equal is the total, cycle-safe lattice equivalence: the component-wise per-axis
// equivalence across all axes. Provenance is excluded.
//
// Interning makes equal values share one node, so the pointer-identity check is
// the fast path; the component comparison is the cold path that interning relies
// on. Equal(a, b) implies a.Hash() == b.Hash().
func Equal(a, b AbstractValue) bool {
	if a.n == b.n {
		return true
	}
	if a.n == nil || b.n == nil {
		return a.n == b.n
	}
	return nodeEqual(a.n, b.n)
}

// Equal reports whether the receiver equals other under the product lattice
// equivalence.
func (v AbstractValue) Equal(other AbstractValue) bool {
	return Equal(v, other)
}

// AbstractValue is the value-level red-green firewall for the types/db Salsa
// database: db change detection bumps a memo entry's revision only when the
// stored value is not Equal to the recomputed one. Satisfying internal.Equaler
// lets a db.Query[K, AbstractValue] reuse Equal as its change predicate through
// the existing default-equality path (db's anyEqual), with no extra equal func
// and no parallel comparison.
var _ internal.Equaler = AbstractValue{}

// Equals satisfies internal.Equaler by delegating to the typed product
// equivalence. A non-AbstractValue operand is never equal, so the firewall
// stays sound for heterogeneous comparisons.
func (v AbstractValue) Equals(other any) bool {
	o, ok := other.(AbstractValue)
	if !ok {
		return false
	}
	return Equal(v, o)
}

// Covers reports whether the receiver is at least as high as other in the product
// order: it covers other on every axis.
func Covers(a, b AbstractValue) bool {
	if a.n == b.n {
		return true
	}
	return a.n.shape.Covers(b.n.shape) &&
		a.n.presence.Covers(b.n.presence) &&
		a.n.numeric.Covers(b.n.numeric) &&
		a.n.effects.Covers(b.n.effects) &&
		a.n.owner.Covers(b.n.owner) &&
		a.n.escape.Covers(b.n.escape) &&
		a.n.identity.Covers(b.n.identity) &&
		a.n.evidence.Covers(b.n.evidence)
}

// Covers reports whether the receiver covers other on every axis.
func (v AbstractValue) Covers(other AbstractValue) bool {
	return Covers(v, other)
}

// Hash is a stable, cycle-safe semantic hash consistent with Equal: it folds the
// per-axis hashes. Provenance is excluded. Two Equal values hash identically.
func (v AbstractValue) Hash() uint64 {
	return nodeHash(v.n)
}

// nodeEqual is the component-wise per-axis lattice equivalence.
func nodeEqual(a, b *node) bool {
	return shapevalue.Equal(a.shape, b.shape) &&
		presence.Equal(a.presence, b.presence) &&
		numeric.Equal(a.numeric, b.numeric) &&
		effectrows.Equal(a.effects, b.effects) &&
		ownership.Equal(a.owner, b.owner) &&
		escape.Equal(a.escape, b.escape) &&
		identityrecursion.Equal(a.identity, b.identity) &&
		evidence.Equal(a.evidence, b.evidence)
}

// nodeHash folds the per-axis hashes into a stable product hash. Each axis hash is
// already consistent with its own Equal, so the fold is consistent with nodeEqual.
func nodeHash(n *node) uint64 {
	h := internal.FnvString("product.AbstractValue")
	h = internal.HashCombine(h, n.shape.Hash())
	h = internal.HashCombine(h, n.presence.Hash())
	h = internal.HashCombine(h, n.numeric.Hash())
	h = internal.HashCombine(h, n.effects.Hash())
	h = internal.HashCombine(h, n.owner.Hash())
	h = internal.HashCombine(h, n.escape.Hash())
	h = internal.HashCombine(h, n.identity.Hash())
	h = internal.HashCombine(h, n.evidence.Hash())
	return h
}

// isUnsealedRecursivePlaceholder reports whether t is an unsealed recursive
// placeholder (NewRecursivePlaceholder, Body == nil): an uninferred recursion
// hole that carries no structural content. Each placeholder has a distinct
// auto-incrementing ID, but typ.TypeEquals / value.SameConvergedFact treat two
// nil-body recursive types as equal, so the value domain admits a placeholder as
// the gradual dynamic value rather than as a distinct recursive family. Without
// this, a fixpoint that re-creates a fresh placeholder each iteration would never
// reach a product-Equal fixed point.
func isUnsealedRecursivePlaceholder(t typ.Type) bool {
	rec, ok := t.(*typ.Recursive)
	return ok && rec != nil && rec.Body == nil
}

// identityOf derives the Identity/Recursion axis from a structural type at
// admission. It carries the non-nil part of t (nilability lives on the presence
// axis), matching the shape derivation, so a recursive product's family identity
// is read from the same structural content the shape axis carries.
func identityOf(t typ.Type) identityrecursion.Value {
	if isUnsealedRecursivePlaceholder(t) {
		return identityrecursion.Top()
	}
	if t == nil || typ.IsAny(t) || typ.IsUnknown(t) {
		return identityrecursion.Of(t)
	}
	nonNil, _ := value.SplitNilable(t)
	if nonNil == nil || typ.IsNever(nonNil) {
		return identityrecursion.Top()
	}
	return identityrecursion.Of(nonNil)
}

// shapeOf derives the Shape/Value axis from a structural type at admission. It
// carries the non-nil part of t (nilability lives on the presence axis). A purely
// nil type has no non-nil content, so its shape is Bottom.
//
// A top-level alias is a distinct carrier identity (P3.2), so its name is preserved
// on the shape rather than unwrapped: the shape carries the alias wrapping the
// non-nil content of its target, and presence still records the target's
// nilability. ProjectValue then recovers an alias-equal type, so FromType(Alias)
// round-trips the alias name losslessly.
func shapeOf(t typ.Type) shapevalue.Value {
	if isUnsealedRecursivePlaceholder(t) {
		// An uninferred recursion hole carries no structural content: admit the
		// dynamic (Top) shape so two distinct placeholders share one node.
		return shapevalue.Top()
	}
	if t == nil || typ.IsAny(t) || typ.IsUnknown(t) {
		return shapevalue.Of(t)
	}
	if alias, ok := t.(*typ.Alias); ok && alias != nil {
		nonNil, _ := value.SplitNilable(alias.Target)
		if nonNil == nil || typ.IsNever(nonNil) {
			return shapevalue.Bottom()
		}
		return shapevalue.Of(typ.NewAlias(alias.Name, nonNil))
	}
	nonNil, _ := value.SplitNilable(t)
	if nonNil == nil || typ.IsNever(nonNil) {
		return shapevalue.Bottom()
	}
	return shapevalue.Of(nonNil)
}

// presenceOf derives the Presence axis from a structural type at admission.
//
// A missing or dynamic type may be nil, so it is Maybe. The uninhabited type is
// Bottom. A purely-nil type is Absent. A type that admits nil alongside non-nil
// content is Maybe. Any other concrete type definitely holds a value, so it is
// Present.
func presenceOf(t typ.Type) presence.Value {
	if t == nil || typ.IsAny(t) || typ.IsUnknown(t) || isUnsealedRecursivePlaceholder(t) {
		// A placeholder is a dynamic, uninferred hole that may be nil.
		return presence.Top()
	}
	if t.Kind().IsNever() {
		return presence.Bottom()
	}
	nonNil, nilable := value.SplitNilable(t)
	switch {
	case nonNil == nil || typ.IsNever(nonNil):
		// Purely nil (no non-nil content): definitely absent.
		return presence.Absent()
	case nilable:
		return presence.Maybe()
	default:
		return presence.Present()
	}
}
