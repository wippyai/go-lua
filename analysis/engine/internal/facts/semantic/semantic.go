// Package semantic owns the algebra of one guarded typed fact plane.
//
// A Plane is one monomorphized sparse typed FDD column family.  Its callers
// pass the one outer shared Boolean support explicitly.  Join/Widen touch a
// terminal pair only on support overlap; Narrow requires subset support; Mu
// closes typed terminals at the same boundary where the outer carrier State
// existentially closes support.  The package has no rule, solver,
// dependency, or domain policy.
package semantic

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/diagram"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/scalar"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/terminal"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// Binary is one monomorphized typed lattice operation.  Returning false
// rejects the candidate fact operation before either its terminal page or FDD
// root publishes.
type Binary[V any] func(left, right V) (V, bool)

// Operations provides the domain-owned algebra for one typed terminal family.
// Default is the value denoted by an undefined FDD leaf *inside supported
// state*.  It is explicit because a Factor default need not be Go's zero
// value or lattice bottom.  The plane canonicalizes a result equal to Default
// back to the undefined terminal, preserving sparse storage.
type Operations[V any] struct {
	Default     V
	Equal       func(left, right V) bool
	Fingerprint func(value V) uint64
	Join        Binary[V]
	Widen       Binary[V]
	Narrow      Binary[V]
	LessOrEq    func(left, right V) bool
}

// Convergence supplies the typed proof obligations that are specific to a
// Factor key.  Semantic owns the synchronized FDD traversal; factbinding
// supplies these immutable callbacks from the declared FactorConfig measure.
// A nil callback deliberately leaves the operation available for standalone
// plane algebra tests, while a bound Factor always installs both applicable
// checks before a typed Plane is published.
type Convergence[K scalar.Key, V any] struct {
	Widen  func(key K, previous, requested, output V) bool
	Narrow func(key K, previous, desired, output V) bool
}

// Domain freezes the one fact diagram, its terminal semantic owner, and the
// typed closures for a single plane family.
type Domain[F ~uint64, K scalar.Key, V any] struct {
	diagram   *diagram.Diagram[F, K, V]
	terminals *terminal.Arena[V]
	ops       Operations[V]
	defaultID terminal.ID[V]
	checks    Convergence[K, V]
}

// Plane is one immutable published typed FDD root.  It deliberately stores
// no support: the heterogeneous carrier State is the sole owner of S.
type Plane[F ~uint64, K scalar.Key, V any] struct{ root diagram.Root[F, K, V] }

// New binds fact algebra to one sealed diagram and terminal semantic owner.
// BeginWithTerminals proves the same exact owner again at every operation;
// this initial check catches miswired construction before any candidate work.
func New[F ~uint64, K scalar.Key, V any](facts *diagram.Diagram[F, K, V], values *terminal.Arena[V], ops Operations[V]) (*Domain[F, K, V], bool) {
	return NewWithConvergence(facts, values, ops, Convergence[K, V]{})
}

// NewWithConvergence binds one typed plane to immutable key-aware Widen and
// Narrow law checks.  It is used by the Factor binding boundary; New remains
// available for typed algebra that intentionally has no Factor declaration.
func NewWithConvergence[F ~uint64, K scalar.Key, V any](facts *diagram.Diagram[F, K, V], values *terminal.Arena[V], ops Operations[V], checks Convergence[K, V]) (*Domain[F, K, V], bool) {
	if facts == nil || values == nil || facts.Terminals() != values || !values.Sealed() || facts.Guards() == nil || ops.Equal == nil || ops.Join == nil || ops.Widen == nil || ops.LessOrEq == nil {
		return nil, false
	}
	// Sparse absence is Default, so an absent/absent coordinate never reaches
	// the fused zipper.  These fixed-point laws are therefore admission
	// obligations, not optional optimization assumptions.
	stable := func(value V) bool {
		joined, joinOK := ops.Join(value, value)
		widened, widenOK := ops.Widen(value, value)
		if !joinOK || !widenOK || !ops.Equal(joined, value) || !ops.Equal(widened, value) {
			return false
		}
		if ops.Narrow == nil {
			return true
		}
		narrowed, narrowOK := ops.Narrow(value, value)
		return narrowOK && ops.Equal(narrowed, value)
	}
	if !stable(ops.Default) || !values.Every(stable) {
		return nil, false
	}
	if _, sole := facts.SoleFactor(); !sole {
		return nil, false
	}
	defaultID, present := values.Lookup(ops.Default)
	if !present {
		return nil, false
	}
	return &Domain[F, K, V]{diagram: facts, terminals: values, ops: ops, defaultID: defaultID, checks: checks}, true
}

// Plane accepts one published sparse root from this exact Diagram.
func (domain *Domain[F, K, V]) Plane(root diagram.Root[F, K, V]) (Plane[F, K, V], bool) {
	if !domain.validRoot(root) {
		return Plane[F, K, V]{}, false
	}
	return Plane[F, K, V]{root: root}, true
}

// Empty returns this typed plane's immutable sparse bottom root.  It carries
// no support; the heterogeneous carrier State alone owns that region.
func (domain *Domain[F, K, V]) Empty() (Plane[F, K, V], bool) {
	if domain == nil || domain.diagram == nil {
		return Plane[F, K, V]{}, false
	}
	return domain.Plane(domain.diagram.Empty())
}

// Restrict keeps this plane's exact terminal decisions only where region
// holds.  Region remains owned by the outer carrier State; this method merely
// constructs a transient masked view for an operation such as joint state
// restriction and never stores a second support beside the plane.
func (domain *Domain[F, K, V]) Restrict(input Plane[F, K, V], region support.Mask) (Plane[F, K, V], bool) {
	if !domain.validPlane(input) || !domain.validSupport(region) {
		return Plane[F, K, V]{}, false
	}
	builder := domain.diagram.Begin()
	if builder == nil {
		return Plane[F, K, V]{}, false
	}
	root, ok := builder.TransformSoleFactor(input.root, func(_ K, value diagram.Value[V]) (diagram.Value[V], bool) {
		return builder.Mask(value, region)
	})
	if !ok {
		builder.Discard()
		return Plane[F, K, V]{}, false
	}
	root, ok = builder.Seal(root)
	if !ok {
		return Plane[F, K, V]{}, false
	}
	return Plane[F, K, V]{root: root}, true
}

// ContributionRegions is the typed view of a carrier-owned authored surface.
// A missing key is Absent.  A present region whose sparse plane cell is
// undefined is explicitly Present(Default).  The relation deliberately lives
// at this boundary: carrier owns Target x Guard rows while the Binding alone
// expands Targets to K.
type ContributionRegions[K scalar.Key] func(K) (support.Mask, bool)

// CloseContribution removes every physical non-Default cell which is not in
// the contribution's authored surface.  It is the representation cut for a
// RuleContribution: sparse undefined under a supplied region means
// Present(Default), while sparse undefined outside it means Absent.  Raw
// State operations intentionally do not use this routine.
func (domain *Domain[F, K, V]) CloseContribution(input Plane[F, K, V], within support.Mask, regions ContributionRegions[K]) (Plane[F, K, V], bool) {
	if !domain.validPlane(input) || !domain.validSupport(within) || regions == nil {
		return Plane[F, K, V]{}, false
	}
	builder := domain.diagram.Begin()
	if builder == nil {
		return Plane[F, K, V]{}, false
	}
	root, ok := builder.TransformSoleFactor(input.root, func(key K, value diagram.Value[V]) (diagram.Value[V], bool) {
		region, present := regions(key)
		if !present {
			return builder.Constant(terminal.ID[V]{})
		}
		if !region.Valid() || region.Manager() != domain.guards() || !region.Entails(within) {
			return diagram.Value[V]{}, false
		}
		masked, ok := builder.Mask(value, region)
		if !ok {
			return diagram.Value[V]{}, false
		}
		return domain.eraseDefault(builder, masked)
	})
	if !ok {
		builder.Discard()
		return Plane[F, K, V]{}, false
	}
	root, ok = builder.Seal(root)
	if !ok {
		return Plane[F, K, V]{}, false
	}
	return Plane[F, K, V]{root: root}, true
}

// ContributionClosed proves the physical half of the closed contribution
// invariant.  It deliberately inspects the whole Boolean universe rather
// than only `within`: a hidden root outside outer support is still forbidden,
// because a later support expansion must not be able to revive it.
func (domain *Domain[F, K, V]) ContributionClosed(input Plane[F, K, V], within support.Mask, regions ContributionRegions[K]) bool {
	if !domain.validPlane(input) || !domain.validSupport(within) || regions == nil {
		return false
	}
	whole, ok := support.FromGuard(domain.guards(), domain.guards().True())
	if !ok {
		return false
	}
	return domain.ForEachNonDefault(input, whole, func(key K, _ V, cell support.Mask) bool {
		region, present := regions(key)
		return present && region.Valid() && region.Manager() == domain.guards() && region.Entails(within) && cell.Entails(region)
	})
}

// LessOrEqContribution compares values only on the left-authored surface.
// Outside that surface the left lifted cell is Absent, not Factor Default.
// Diagram synchronizes the two sparse FDDs and each key-local coverage BDD in
// one read-only traversal, avoiding the nested support partitions that this
// hot phase proof used to allocate.
func (domain *Domain[F, K, V]) LessOrEqContribution(left, right Plane[F, K, V], regions ContributionRegions[K], scratch *diagram.SoleScratch[K, V]) bool {
	if !domain.validPlane(left) || !domain.validPlane(right) || regions == nil || scratch == nil {
		return false
	}
	if left.root == right.root {
		return true
	}
	return domain.diagram.CompareSoleFactorRegions(left.root, right.root, scratch, regions, func(first, second terminal.ID[V]) bool {
		leftValue, leftOK := domain.ops.Default, true
		if first != (terminal.ID[V]{}) {
			leftValue, leftOK = domain.terminals.Value(first)
		}
		rightValue, rightOK := domain.ops.Default, true
		if second != (terminal.ID[V]{}) {
			rightValue, rightOK = domain.terminals.Value(second)
		}
		return leftOK && rightOK && domain.ops.LessOrEq(leftValue, rightValue)
	})
}

// Summary joins one key over the symbolic leaves of this plane.  An undefined
// FDD leaf is the typed Default.  Plane alone has no outer reachability
// region, so callers that summarize a carrier State must use SummaryUnder with
// the state's sole support mask.
func (domain *Domain[F, K, V]) Summary(input Plane[F, K, V], key K) (V, bool) {
	if domain == nil || domain.diagram == nil {
		var zero V
		return zero, false
	}
	factor, ok := domain.diagram.SoleFactor()
	if !ok {
		var zero V
		return zero, false
	}
	return domain.summaryAt(input, factor, key)
}

func (domain *Domain[F, K, V]) summaryAt(input Plane[F, K, V], factor F, key K) (V, bool) {
	var zero V
	if !domain.validPlane(input) {
		return zero, false
	}
	value, present, valid := domain.diagram.Get(input.root, factor, key)
	if !valid {
		return zero, false
	}
	if !present {
		return domain.ops.Default, true
	}
	return domain.summaryValue(value, true)
}

// SummaryUnder joins one key only over region.  It makes Default explicit
// inside region before traversing the FDD, then ignores undefined leaves
// outside region.  This keeps the outer State support the single source of
// feasibility rather than smuggling support into a Plane.
func (domain *Domain[F, K, V]) SummaryUnder(input Plane[F, K, V], key K, region support.Mask) (V, bool) {
	if domain == nil || domain.diagram == nil {
		var zero V
		return zero, false
	}
	factor, ok := domain.diagram.SoleFactor()
	if !ok {
		var zero V
		return zero, false
	}
	return domain.summaryUnderAt(input, factor, key, region)
}

// PartitionKey refines region by exactly one typed key and emits its stored
// terminal value together with whether that terminal is present. A sparse
// branch carries Default with present=false; a stored Default, if an adjacent
// typed plane admits one, remains distinguishable through present=true. This
// is the one-key read primitive for Binding observations: it walks the FDD
// value once and never scans another key or the complete plane.
func (domain *Domain[F, K, V]) PartitionKey(input Plane[F, K, V], key K, region support.Mask, visit func(value V, present bool, cell support.Mask) bool) bool {
	if domain == nil || domain.diagram == nil || !domain.validPlane(input) || !domain.validSupport(region) || visit == nil {
		return false
	}
	if support.Empty(region) {
		return true
	}
	factor, ok := domain.diagram.SoleFactor()
	if !ok {
		return false
	}
	value, present, valid := domain.diagram.Get(input.root, factor, key)
	if !valid {
		return false
	}
	if !present {
		return visit(domain.ops.Default, false, region)
	}
	completed, partitioned := domain.diagram.PartitionValueTerminals(value, region, func(id terminal.ID[V], cell support.Mask) bool {
		if id == (terminal.ID[V]{}) {
			return visit(domain.ops.Default, false, cell)
		}
		stored, readable := domain.terminals.Value(id)
		return readable && visit(stored, true, cell)
	})
	return completed && partitioned
}

// ForEachNonDefault visits the effective sparse presence of one plane under
// region.  A stored terminal equal to Default is not presence: sparse
// absence and an explicit Default have the same semantic value for the
// contribution boundary.  The callback receives only typed key/value data;
// the diagram and terminal identities remain private to semantic.
//
// This is deliberately a stream over the plane's existing sparse columns.
// It does not materialize a key union or a second fact representation.  The
// caller owns any coverage map used to combine these cells with authored
// Target rows.
func (domain *Domain[F, K, V]) ForEachNonDefault(input Plane[F, K, V], region support.Mask, visit func(key K, value V, cell support.Mask) bool) bool {
	if domain == nil || domain.diagram == nil || !domain.validPlane(input) || !domain.validSupport(region) || visit == nil {
		return false
	}
	if support.Empty(region) {
		return true
	}
	completed, traversed := domain.diagram.ForEach(input.root, func(fact diagram.Fact[F, K, V]) bool {
		return domain.PartitionKey(input, fact.Key, region, func(value V, present bool, cell support.Mask) bool {
			if !present || domain.ops.Equal(value, domain.ops.Default) {
				return true
			}
			return visit(fact.Key, value, cell)
		})
	})
	return completed && traversed
}

// Partition refines region by this plane's complete FDD tuple.  Every cell
// passed to visit has one exact value (or Default) for every factor/key in the
// plane.  It exposes only the shared support cell, never a raw terminal or
// FDD topology, so outer carrier code can form joint observations without another
// payload carrier.
func (domain *Domain[F, K, V]) Partition(input Plane[F, K, V], region support.Mask, visit func(support.Mask) bool) bool {
	if !domain.validPlane(input) || !domain.validSupport(region) || visit == nil {
		return false
	}
	completed, valid := domain.diagram.Partition(input.root, region, visit)
	return completed && valid
}

// EqualAt compares every explicit factor/key at two representative
// valuations.  All absent coordinates denote the same typed Default, so the
// finite union of populated coordinates is complete.  It is intended to
// confirm a collision-prone row bucket after Partition; it never exposes a
// terminal identity as semantic equality.
func (domain *Domain[F, K, V]) EqualAt(left Plane[F, K, V], leftValuation func(guard.Atom) bool, right Plane[F, K, V], rightValuation func(guard.Atom) bool) bool {
	if !domain.validPlane(left) || !domain.validPlane(right) || leftValuation == nil || rightValuation == nil {
		return false
	}
	type coordinate struct {
		factor F
		key    K
	}
	coordinates := make([]coordinate, 0)
	seen := make(map[coordinate]struct{})
	collect := func(root diagram.Root[F, K, V]) bool {
		completed, valid := domain.diagram.ForEach(root, func(fact diagram.Fact[F, K, V]) bool {
			coordinate := coordinate{factor: fact.Factor, key: fact.Key}
			if _, exists := seen[coordinate]; !exists {
				seen[coordinate] = struct{}{}
				coordinates = append(coordinates, coordinate)
			}
			return true
		})
		return completed && valid
	}
	if !collect(left.root) || !collect(right.root) {
		return false
	}
	for _, coordinate := range coordinates {
		leftValue, ok := domain.valueAt(left.root, coordinate.factor, coordinate.key, leftValuation)
		if !ok {
			return false
		}
		rightValue, ok := domain.valueAt(right.root, coordinate.factor, coordinate.key, rightValuation)
		if !ok || !domain.ops.Equal(leftValue, rightValue) {
			return false
		}
	}
	return true
}

// LessOrEqAt proves the domain's pointwise order for every explicit
// factor/key at two representative valuations.  Missing columns are the
// declared Default on supported state, so the finite union of populated
// coordinates is the complete comparison surface.  It is the order
// authority used by the heterogeneous carrier State; it does not infer order
// from Join or manufacture a second lattice carrier.
func (domain *Domain[F, K, V]) LessOrEqAt(left Plane[F, K, V], leftValuation func(guard.Atom) bool, right Plane[F, K, V], rightValuation func(guard.Atom) bool) bool {
	if !domain.validPlane(left) || !domain.validPlane(right) || leftValuation == nil || rightValuation == nil {
		return false
	}
	type coordinate struct {
		factor F
		key    K
	}
	coordinates := make([]coordinate, 0)
	seen := make(map[coordinate]struct{})
	collect := func(root diagram.Root[F, K, V]) bool {
		completed, valid := domain.diagram.ForEach(root, func(fact diagram.Fact[F, K, V]) bool {
			coordinate := coordinate{factor: fact.Factor, key: fact.Key}
			if _, exists := seen[coordinate]; !exists {
				seen[coordinate] = struct{}{}
				coordinates = append(coordinates, coordinate)
			}
			return true
		})
		return completed && valid
	}
	if !collect(left.root) || !collect(right.root) {
		return false
	}
	for _, coordinate := range coordinates {
		leftValue, ok := domain.valueAt(left.root, coordinate.factor, coordinate.key, leftValuation)
		if !ok {
			return false
		}
		rightValue, ok := domain.valueAt(right.root, coordinate.factor, coordinate.key, rightValuation)
		if !ok || !domain.ops.LessOrEq(leftValue, rightValue) {
			return false
		}
	}
	return true
}

// FingerprintAt is a non-authoritative row bucket key.  EqualAt remains the
// equality authority: this value is deterministic only to avoid comparing
// every disconnected Product cell quadratically.
func (domain *Domain[F, K, V]) FingerprintAt(input Plane[F, K, V], valuation func(guard.Atom) bool) (uint64, bool) {
	if !domain.validPlane(input) || valuation == nil || domain.ops.Fingerprint == nil {
		return 0, false
	}
	var result uint64 = 0x9e3779b97f4a7c15
	completed, valid := domain.diagram.ForEach(input.root, func(fact diagram.Fact[F, K, V]) bool {
		value, ok := domain.valueAt(input.root, fact.Factor, fact.Key, valuation)
		if !ok {
			return false
		}
		// FingerprintAt deliberately reduces this typed key to an opaque hash
		// bucket only here; exact equality and all fact storage retain K.
		keyHash := uint64(fact.Key)
		item := uint64(fact.Factor)*0x9e3779b185ebca87 ^ keyHash*0xc2b2ae3d27d4eb4f ^ domain.ops.Fingerprint(value)
		result ^= item + 0x9e3779b97f4a7c15 + (result << 6) + (result >> 2)
		return true
	})
	return result, completed && valid
}

func (domain *Domain[F, K, V]) valueAt(root diagram.Root[F, K, V], factor F, key K, valuation func(guard.Atom) bool) (V, bool) {
	id, present, valid := domain.diagram.At(root, factor, key, valuation)
	if !valid {
		var zero V
		return zero, false
	}
	if !present {
		return domain.ops.Default, true
	}
	return domain.terminals.Value(id)
}

func (domain *Domain[F, K, V]) summaryUnderAt(input Plane[F, K, V], factor F, key K, region support.Mask) (V, bool) {
	var zero V
	if !domain.validPlane(input) || !domain.validSupport(region) {
		return zero, false
	}
	builder := domain.diagram.Begin()
	if builder == nil {
		return zero, false
	}
	value, present, valid := domain.diagram.Get(input.root, factor, key)
	if !valid {
		builder.Discard()
		return zero, false
	}
	if !present {
		value, valid = builder.Constant(terminal.ID[V]{})
		if !valid {
			builder.Discard()
			return zero, false
		}
	}
	actual, valid := builder.Mask(value, region)
	if !valid {
		builder.Discard()
		return zero, false
	}
	defaultValue, valid := builder.Constant(domain.defaultID)
	if !valid {
		builder.Discard()
		return zero, false
	}
	defaults, valid := builder.Mask(defaultValue, region)
	if !valid {
		builder.Discard()
		return zero, false
	}
	total, valid := builder.Zip(actual, defaults, sparseOverlay[V])
	if !valid {
		builder.Discard()
		return zero, false
	}
	result, present := domain.summaryValue(total, false)
	builder.Discard()
	return result, present
}

func (domain *Domain[F, K, V]) summaryValue(value diagram.Value[V], includeUndefined bool) (V, bool) {
	var result V
	present := false
	completed, valid := domain.diagram.ForEachTerminal(value, includeUndefined, func(id terminal.ID[V]) bool {
		candidate := domain.ops.Default
		if id != (terminal.ID[V]{}) {
			var found bool
			candidate, found = domain.terminals.Value(id)
			if !found {
				return false
			}
		}
		if !present {
			result, present = candidate, true
			return true
		}
		next, ok := domain.ops.Join(result, candidate)
		if !ok {
			return false
		}
		result = next
		return true
	})
	return result, completed && valid && present
}

// Root returns Plane's published immutable sparse fact root.
func (plane Plane[F, K, V]) Root() diagram.Root[F, K, V] { return plane.root }

// Valid reports whether plane is a published root from this exact typed
// semantic domain.  It is an ownership check only: it neither reads nor
// transforms a fact value.  The outer carrier uses this boundary when
// admitting a plane into a sealed typed slot.
func (domain *Domain[F, K, V]) Valid(plane Plane[F, K, V]) bool {
	return domain.validPlane(plane)
}

// Same proves whole-plane semantic equality. Unlike EqualUnder, it has no
// support restriction: a recurrence closure must not retain an old root just
// because a difference currently lies outside the predecessor support.
func (domain *Domain[F, K, V]) Same(left, right Plane[F, K, V]) bool {
	return domain != nil && domain.diagram != nil && domain.validPlane(left) && domain.validPlane(right) && domain.diagram.Equal(left.root, right.root)
}

// Guards returns the one sealed Boolean universe shared by every plane of
// this domain.  It exposes no guard representation; adjacent outer storage
// uses pointer identity solely to prove that its sole support region and this
// typed plane range over the same valuations.
func (domain *Domain[F, K, V]) Guards() *guard.Manager { return domain.guards() }

// Mu existentially discharges atom from each typed column at a validated
// boundary.  Its caller closes the sole outer support at that same boundary.
func (domain *Domain[F, K, V]) Mu(input Plane[F, K, V], inputSupport support.Mask, atom guard.Atom) (Plane[F, K, V], bool) {
	if !domain.validPlane(input) || !domain.validSupport(inputSupport) {
		return Plane[F, K, V]{}, false
	}
	regions, ok := domain.muRegions(inputSupport, atom)
	if !ok {
		return Plane[F, K, V]{}, false
	}
	work := domain.terminals.Begin()
	builder := domain.diagram.BeginWithTerminals(work)
	if work == nil || builder == nil {
		return Plane[F, K, V]{}, false
	}
	root, ok := builder.TransformSoleFactor(input.root, func(key K, value diagram.Value[V]) (diagram.Value[V], bool) {
		return domain.muColumn(builder, work, key, value, atom, regions)
	})
	if !ok {
		builder.Discard()
		return Plane[F, K, V]{}, false
	}
	root, ok = builder.Seal(root)
	if !ok {
		return Plane[F, K, V]{}, false
	}
	return Plane[F, K, V]{root: root}, true
}

// Reindex transports a typed plane through one sealed source-to-target guard
// relation. For every stored column it first totalizes the column only on the
// source support, then combines exactly the source fibers that reach each
// target valuation. Thus an off-support FDD branch is never allowed to pollute
// a forgotten or non-injective target fiber.
func (domain *Domain[F, K, V]) Reindex(input Plane[F, K, V], source, target support.Mask, relation guard.Reindex) (Plane[F, K, V], bool) {
	if !domain.validPlane(input) || !domain.validSupport(source) || !domain.validSupport(target) || !relation.Valid() || relation.Source().Manager() != domain.guards() || relation.Target().Manager() != domain.guards() {
		return Plane[F, K, V]{}, false
	}
	sourceGuard, ok := source.Guard()
	if !ok || !relation.Source().Contains(sourceGuard) {
		return Plane[F, K, V]{}, false
	}
	targetGuard, ok := target.Guard()
	if !ok || !relation.Target().Contains(targetGuard) {
		return Plane[F, K, V]{}, false
	}
	if relation.Identity() {
		return input, true
	}
	values := domain.terminals.Begin()
	builder := domain.diagram.BeginWithTerminals(values)
	if values == nil || builder == nil {
		return Plane[F, K, V]{}, false
	}
	root, ok := builder.TransformSoleFactor(input.root, func(key K, value diagram.Value[V]) (diagram.Value[V], bool) {
		actual, okay := builder.Mask(value, source)
		if !okay {
			return diagram.Value[V]{}, false
		}
		defaultValue, okay := builder.Constant(domain.defaultID)
		if !okay {
			return diagram.Value[V]{}, false
		}
		defaults, okay := builder.Mask(defaultValue, source)
		if !okay {
			return diagram.Value[V]{}, false
		}
		total, okay := builder.Zip(actual, defaults, sparseOverlay[V])
		if !okay {
			return diagram.Value[V]{}, false
		}
		transported, okay := builder.Reindex(total, relation, domain.reindexFiberJoin(values, key))
		if !okay {
			return diagram.Value[V]{}, false
		}
		transported, okay = builder.Mask(transported, target)
		if !okay {
			return diagram.Value[V]{}, false
		}
		return domain.eraseDefault(builder, transported)
	})
	if !ok {
		builder.Discard()
		return Plane[F, K, V]{}, false
	}
	root, ok = builder.Seal(root)
	if !ok {
		return Plane[F, K, V]{}, false
	}
	return Plane[F, K, V]{root: root}, true
}

// ReindexContribution transports a closed partial contribution plane.  In
// contrast with Reindex, it totalizes only source-present fibers: undefined
// under a source region is Present(Default), and undefined outside that
// region is Absent.  This distinction is essential under forget/noninjective
// reindex, where treating an absent source fiber as Default can change a
// lower authored value into lower Join Default.
//
// targetRegions is the already transported and post-clipped authored
// surface.  The routine masks every output column to that exact surface, so
// no pre/post-hidden root can survive this publication boundary.
func (domain *Domain[F, K, V]) ReindexContribution(input Plane[F, K, V], source, target support.Mask, relation guard.Reindex, sourceRegions, targetRegions ContributionRegions[K]) (Plane[F, K, V], bool) {
	if !domain.validPlane(input) || !domain.validSupport(source) || !domain.validSupport(target) || !relation.Valid() || sourceRegions == nil || targetRegions == nil || relation.Source().Manager() != domain.guards() || relation.Target().Manager() != domain.guards() {
		return Plane[F, K, V]{}, false
	}
	sourceGuard, ok := source.Guard()
	if !ok || !relation.Source().Contains(sourceGuard) {
		return Plane[F, K, V]{}, false
	}
	targetGuard, ok := target.Guard()
	if !ok || !relation.Target().Contains(targetGuard) {
		return Plane[F, K, V]{}, false
	}
	values := domain.terminals.Begin()
	builder := domain.diagram.BeginWithTerminals(values)
	if values == nil || builder == nil {
		return Plane[F, K, V]{}, false
	}
	root, ok := builder.TransformSoleFactor(input.root, func(key K, value diagram.Value[V]) (diagram.Value[V], bool) {
		sourceRegion, sourcePresent := sourceRegions(key)
		if !sourcePresent {
			// A closed input has no meaningful payload here.  Ignoring it keeps
			// Absent as identity even if a hostile caller supplied a raw root.
			return builder.Constant(terminal.ID[V]{})
		}
		if !sourceRegion.Valid() || sourceRegion.Manager() != domain.guards() || !sourceRegion.Entails(source) {
			return diagram.Value[V]{}, false
		}
		targetRegion, targetPresent := targetRegions(key)
		if !targetPresent {
			// The post boundary killed this authored source surface.
			return builder.Constant(terminal.ID[V]{})
		}
		if !targetRegion.Valid() || targetRegion.Manager() != domain.guards() || !targetRegion.Entails(target) {
			return diagram.Value[V]{}, false
		}
		actual, okay := builder.Mask(value, sourceRegion)
		if !okay {
			return diagram.Value[V]{}, false
		}
		defaultValue, okay := builder.Constant(domain.defaultID)
		if !okay {
			return diagram.Value[V]{}, false
		}
		defaults, okay := builder.Mask(defaultValue, sourceRegion)
		if !okay {
			return diagram.Value[V]{}, false
		}
		total, okay := builder.Zip(actual, defaults, sparseOverlay[V])
		if !okay {
			return diagram.Value[V]{}, false
		}
		transported, okay := builder.Reindex(total, relation, domain.reindexFiberJoin(values, key))
		if !okay {
			return diagram.Value[V]{}, false
		}
		transported, okay = builder.Mask(transported, targetRegion)
		if !okay {
			return diagram.Value[V]{}, false
		}
		return domain.eraseDefault(builder, transported)
	})
	if !ok {
		builder.Discard()
		return Plane[F, K, V]{}, false
	}
	root, ok = builder.Seal(root)
	if !ok {
		return Plane[F, K, V]{}, false
	}
	return Plane[F, K, V]{root: root}, true
}

func (domain *Domain[F, K, V]) reindexFiberJoin(values *terminal.Work[V], key K) diagram.Combine[V] {
	join := domain.terminalsBinary(values, domain.ops.Join, binaryJoin, key)
	return func(left, right terminal.ID[V]) (terminal.ID[V], bool) {
		zero := terminal.ID[V]{}
		if left == zero {
			return right, true
		}
		if right == zero {
			return left, true
		}
		return join(left, right)
	}
}

// muRegions projects the two recurrence cofactors independently.  Their
// overlap is the exact set of remaining valuations for which both branches
// were supported; only there may Mu invoke the typed Join.  A one-sided
// projected cofactor carries its value unchanged.
type muRegions struct {
	low, high               support.Mask
	both, lowOnly, highOnly support.Mask
}

func (domain *Domain[F, K, V]) muRegions(input support.Mask, atom guard.Atom) (muRegions, bool) {
	work := support.New(domain.guards())
	if work == nil {
		return muRegions{}, false
	}
	low, ok := work.Conjoin(input, atom, false)
	if !ok {
		work.Discard()
		return muRegions{}, false
	}
	high, ok := work.Conjoin(input, atom, true)
	if !ok {
		work.Discard()
		return muRegions{}, false
	}
	lowProjected, ok := work.Exists(low, atom)
	if !ok {
		work.Discard()
		return muRegions{}, false
	}
	highProjected, ok := work.Exists(high, atom)
	if !ok {
		work.Discard()
		return muRegions{}, false
	}
	both, ok := work.And(lowProjected, highProjected)
	if !ok {
		work.Discard()
		return muRegions{}, false
	}
	notHigh, ok := work.Not(highProjected)
	if !ok {
		work.Discard()
		return muRegions{}, false
	}
	lowOnly, ok := work.And(lowProjected, notHigh)
	if !ok {
		work.Discard()
		return muRegions{}, false
	}
	notLow, ok := work.Not(lowProjected)
	if !ok {
		work.Discard()
		return muRegions{}, false
	}
	highOnly, ok := work.And(highProjected, notLow)
	if !ok || !work.Seal() {
		work.Discard()
		return muRegions{}, false
	}
	return muRegions{low: low, high: high, both: both, lowOnly: lowOnly, highOnly: highOnly}, true
}

func (domain *Domain[F, K, V]) muColumn(builder *diagram.Builder[F, K, V], values *terminal.Work[V], key K, input diagram.Value[V], atom guard.Atom, regions muRegions) (diagram.Value[V], bool) {
	low, ok := domain.projectMuCofactor(builder, input, regions.low, atom)
	if !ok {
		return diagram.Value[V]{}, false
	}
	high, ok := domain.projectMuCofactor(builder, input, regions.high, atom)
	if !ok {
		return diagram.Value[V]{}, false
	}
	// Mask before the typed closure.  Outside both, zero means an unsupported
	// cofactor rather than Default and therefore must not reach Join.
	lowBoth, ok := builder.Mask(low, regions.both)
	if !ok {
		return diagram.Value[V]{}, false
	}
	highBoth, ok := builder.Mask(high, regions.both)
	if !ok {
		return diagram.Value[V]{}, false
	}
	joined, ok := builder.Zip(lowBoth, highBoth, domain.muOverlapJoin(values, key))
	if !ok {
		return diagram.Value[V]{}, false
	}
	joined, ok = builder.Mask(joined, regions.both)
	if !ok {
		return diagram.Value[V]{}, false
	}
	low, ok = builder.Mask(low, regions.lowOnly)
	if !ok {
		return diagram.Value[V]{}, false
	}
	high, ok = builder.Mask(high, regions.highOnly)
	if !ok {
		return diagram.Value[V]{}, false
	}
	onesided, ok := builder.Zip(low, high, sparseOverlay[V])
	if !ok {
		return diagram.Value[V]{}, false
	}
	output, ok := builder.Zip(onesided, joined, sparseOverlay[V])
	if !ok {
		return diagram.Value[V]{}, false
	}
	return domain.eraseDefault(builder, output)
}

// projectMuCofactor first gives every supported undefined FDD leaf the typed
// Default, then discharges atom with sparse carry.  The carry sees zero only
// outside the selected support cofactor; it therefore preserves the fact
// value where exactly one branch is reachable.
func (domain *Domain[F, K, V]) projectMuCofactor(builder *diagram.Builder[F, K, V], input diagram.Value[V], region support.Mask, atom guard.Atom) (diagram.Value[V], bool) {
	actual, ok := builder.Mask(input, region)
	if !ok {
		return diagram.Value[V]{}, false
	}
	defaultValue, ok := builder.Constant(domain.defaultID)
	if !ok {
		return diagram.Value[V]{}, false
	}
	defaults, ok := builder.Mask(defaultValue, region)
	if !ok {
		return diagram.Value[V]{}, false
	}
	total, ok := builder.Zip(actual, defaults, sparseOverlay[V])
	if !ok {
		return diagram.Value[V]{}, false
	}
	return builder.Exists(total, atom, sparseCarry[V])
}

func (domain *Domain[F, K, V]) eraseDefault(builder *diagram.Builder[F, K, V], input diagram.Value[V]) (diagram.Value[V], bool) {
	zero, ok := builder.Constant(terminal.ID[V]{})
	if !ok {
		return diagram.Value[V]{}, false
	}
	return builder.Zip(input, zero, func(value, _ terminal.ID[V]) (terminal.ID[V], bool) {
		if value == domain.defaultID {
			return terminal.ID[V]{}, true
		}
		return value, true
	})
}

func (domain *Domain[F, K, V]) muOverlapJoin(values *terminal.Work[V], key K) diagram.Combine[V] {
	join := domain.terminalsBinary(values, domain.ops.Join, binaryJoin, key)
	return func(left, right terminal.ID[V]) (terminal.ID[V], bool) {
		zero := terminal.ID[V]{}
		if left == zero && right == zero {
			return zero, true
		}
		if left == zero || right == zero {
			// Both nonzero is an invariant of the both-support partition.  A
			// one-sided value here would conflate unsupported support with
			// Default, so reject the candidate rather than inventing a fact.
			return zero, false
		}
		return join(left, right)
	}
}

type binaryKind uint8

const (
	binaryJoin binaryKind = iota
	binaryWiden
	binaryNarrow
)

func (domain *Domain[F, K, V]) terminalsBinary(values *terminal.Work[V], operation Binary[V], kind binaryKind, key K) diagram.Combine[V] {
	return func(left, right terminal.ID[V]) (terminal.ID[V], bool) {
		zero := terminal.ID[V]{}
		// A zero FDD terminal has no payload identity.  At an operation over
		// common outer support it denotes this Factor's declared Default, not
		// a second bottom.  Mu handles its unsupported cofactor separately
		// with sparseCarry below; it must never call this closure for that
		// one-sided case.
		if left == zero {
			left = domain.defaultID
		}
		if right == zero {
			right = domain.defaultID
		}
		leftValue, leftValid := values.Value(left)
		rightValue, rightValid := values.Value(right)
		if !leftValid || !rightValid {
			return terminal.ID[V]{}, false
		}
		if kind == binaryNarrow && !domain.ops.LessOrEq(rightValue, leftValue) {
			return terminal.ID[V]{}, false
		}
		output, ok := operation(leftValue, rightValue)
		if !ok {
			return terminal.ID[V]{}, false
		}
		// A visited equal pair must preserve its semantic value. This makes
		// sparse omission and explicit Default observations equivalent for
		// Join/Widen/Narrow, including a later same-root fast path, without
		// re-invoking a domain operator merely to validate a fresh output.
		if domain.ops.Equal(leftValue, rightValue) && !domain.ops.Equal(output, leftValue) {
			return terminal.ID[V]{}, false
		}
		switch kind {
		case binaryJoin:
			// Join is the least implementation-independent upper-bound
			// contract at this layer.  The domain supplies its order, so the
			// typed zipper must reject a faulty closure before its output
			// terminal can publish.
			if !domain.ops.LessOrEq(leftValue, output) || !domain.ops.LessOrEq(rightValue, output) {
				return terminal.ID[V]{}, false
			}
		case binaryWiden:
			if !domain.ops.LessOrEq(leftValue, output) || !domain.ops.LessOrEq(rightValue, output) {
				return terminal.ID[V]{}, false
			}
			if domain.checks.Widen != nil && !domain.checks.Widen(key, leftValue, rightValue, output) {
				return terminal.ID[V]{}, false
			}
		case binaryNarrow:
			if domain.checks.Narrow != nil && !domain.checks.Narrow(key, leftValue, rightValue, output) {
				return terminal.ID[V]{}, false
			}
		}
		if domain.ops.Equal(output, domain.ops.Default) {
			return zero, true
		}
		// Any newly published terminal may later participate in a same-root
		// carrier Join, whose exact allocation-free identity path deliberately
		// skips the typed zipper.  Enforce semantic Join idempotence before that
		// terminal can enter a root; Widen/Narrow are recurrence strategies and
		// are checked only when their boundaries actually execute.
		if !domain.joinStable(output) {
			return terminal.ID[V]{}, false
		}
		return values.Admit(output)
	}
}

func (domain *Domain[F, K, V]) joinStable(value V) bool {
	if domain == nil || domain.ops.Equal == nil || domain.ops.Join == nil {
		return false
	}
	joined, joinOK := domain.ops.Join(value, value)
	return joinOK && domain.ops.Equal(joined, value)
}

func (domain *Domain[F, K, V]) equalTerminal(values *terminal.Work[V], left, right terminal.ID[V]) bool {
	if domain == nil || values == nil {
		return false
	}
	leftValue, leftOK := domain.ops.Default, true
	if left != (terminal.ID[V]{}) {
		leftValue, leftOK = values.Value(left)
	}
	rightValue, rightOK := domain.ops.Default, true
	if right != (terminal.ID[V]{}) {
		rightValue, rightOK = values.Value(right)
	}
	return leftOK && rightOK && domain.ops.Equal(leftValue, rightValue)
}

// sparseCarry is intentionally not a lattice operation.  It is used only
// while projecting one support cofactor through Mu: an absent cofactor means
// the state itself is unsupported and the other side must survive unchanged.
// Treating that absence as Default would incorrectly invoke Join(v, Default).
func sparseCarry[V any](left, right terminal.ID[V]) (terminal.ID[V], bool) {
	zero := terminal.ID[V]{}
	if left != zero {
		return left, true
	}
	return right, true
}

func sparseOverlay[V any](left, right terminal.ID[V]) (terminal.ID[V], bool) {
	return sparseCarry(left, right)
}

func (domain *Domain[F, K, V]) validRoot(root diagram.Root[F, K, V]) bool {
	return domain != nil && domain.diagram != nil && domain.diagram.Valid(root)
}

func (domain *Domain[F, K, V]) guards() *guard.Manager {
	if domain == nil || domain.diagram == nil {
		return nil
	}
	return domain.diagram.Guards()
}

func (domain *Domain[F, K, V]) validSupport(region support.Mask) bool {
	return domain != nil && region.Valid() && region.Manager() == domain.guards()
}

func (domain *Domain[F, K, V]) validPlane(plane Plane[F, K, V]) bool {
	return domain.validRoot(plane.root)
}
