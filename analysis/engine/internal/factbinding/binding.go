// Package factbinding binds one declared typed Factor directly to the sealed
// heterogeneous carrier.  It is the only engine-private place where a
// admitted Factor algebra meets typed semantic storage; neither carrier
// State nor scheduling code learns this Factor's key or payload type.
package factbinding

import (
	"sync"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/diagram"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/scalar"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/semantic"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/stage"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/terminal"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/lattice"
)

// Measure is the immutable key-aware well-founded witness copied from the
// public FactorSpec at declaration time. It is retained here because the
// typed fact plane is the only execution boundary that can enforce a pointwise
// Widen or Narrow transition; Solver and transaction never recover V.
type Measure[K scalar.Key, V any] struct {
	Width int
	At    func(key K, value V, component int) uint64
}

func (measure Measure[K, V]) valid() bool { return measure.Width > 0 && measure.At != nil }

func (measure Measure[K, V]) absent() bool { return measure.Width == 0 && measure.At == nil }

func (measure Measure[K, V]) descends(key K, before, after V) bool {
	for component := 0; component < measure.Width; component++ {
		beforeRank, afterRank := measure.At(key, before, component), measure.At(key, after, component)
		switch {
		case afterRank < beforeRank:
			return true
		case afterRank > beforeRank:
			return false
		}
	}
	return false
}

// Algebra is a Factor's opaque, immutable admission proof. Admit is the only
// constructor; Bind accepts no raw configuration or trust flag. Its private
// seal prevents a zero value from becoming a binding authority.
type Algebra[K scalar.Key, V any] struct {
	seal        *algebraSeal
	keyEnd      uint64
	default_    V
	top_        V
	admitAt     func(K, V) bool
	equal       func(V, V) bool
	same        func(V, V) bool
	fingerprint func(V) uint64
	join        func(V, V) V
	widen       func(V, V) V
	narrow      func(V, V) V
	lessOrEq    func(V, V) bool
	widenRank   Measure[K, V]
	narrowRank  Measure[K, V]
}

type algebraSeal struct{}

// Admit proves the static Factor algebra once. In particular it proves that
// Default is admitted at every dense key before any guard-bound Binding
// exists. Dynamic value admission remains in Algebra and is checked by Patch.
func Admit[K scalar.Key, V any](keyEnd uint64, default_ V, values lattice.Lattice[V], admitAt func(K, V) bool, fingerprint func(V) uint64, widenRank Measure[K, V], narrowRank Measure[K, V]) (algebra *Algebra[K, V], admitted bool) {
	defer func() {
		if recover() != nil {
			algebra, admitted = nil, false
		}
	}()
	if values.Bottom == nil || values.Top == nil || values.Equal == nil || values.LessOrEq == nil || values.Join == nil || values.Widen == nil || admitAt == nil || fingerprint == nil || !widenRank.valid() && !widenRank.absent() || !narrowRank.valid() && !narrowRank.absent() || values.Narrow != nil && !narrowRank.valid() || values.Narrow == nil && !narrowRank.absent() {
		return nil, false
	}
	if keyEnd != 0 && uint64(K(keyEnd-1)) != keyEnd-1 {
		return nil, false
	}
	for raw := uint64(0); raw < keyEnd; raw++ {
		key := K(raw)
		if uint64(key) != raw || !admitAt(key, default_) {
			return nil, false
		}
	}
	algebra = &Algebra[K, V]{seal: new(algebraSeal), keyEnd: keyEnd, default_: default_, top_: values.Top(), admitAt: admitAt, equal: values.Equal, same: values.Same, fingerprint: fingerprint, join: values.Join, widen: values.Widen, narrow: values.Narrow, lessOrEq: values.LessOrEq, widenRank: widenRank, narrowRank: narrowRank}
	joined, widened := algebra.join(default_, default_), algebra.widen(default_, default_)
	if !algebra.sameValue(joined, default_) || !algebra.sameValue(widened, default_) || !algebra.lessOrEq(default_, joined) || !algebra.lessOrEq(joined, default_) || !algebra.lessOrEq(default_, widened) || !algebra.lessOrEq(widened, default_) || algebra.sameContradictsEqual(joined, default_) || algebra.sameContradictsEqual(widened, default_) {
		return nil, false
	}
	if algebra.narrow != nil {
		narrowed := algebra.narrow(default_, default_)
		if !algebra.sameValue(narrowed, default_) || !algebra.lessOrEq(default_, narrowed) || !algebra.lessOrEq(narrowed, default_) || algebra.sameContradictsEqual(narrowed, default_) {
			return nil, false
		}
		if fingerprint(narrowed) != fingerprint(default_) {
			return nil, false
		}
	}
	if fingerprint(joined) != fingerprint(default_) || fingerprint(widened) != fingerprint(default_) {
		return nil, false
	}
	return algebra, true
}

// KeyEnd exposes only the immutable structural bound needed by Factor schema
// checks; it cannot expose or mutate raw algebra callbacks.
func (algebra *Algebra[K, V]) KeyEnd() uint64 {
	if algebra == nil || algebra.seal == nil {
		return 0
	}
	return algebra.keyEnd
}

// Default returns the sealed sparse-value meaning for the typed Factor.  It
// is exposed only to the Factor-owner declaration layer so total carry maps
// can fence default preservation before sealing.
func (algebra *Algebra[K, V]) Default() (V, bool) {
	var zero V
	if algebra == nil || algebra.seal == nil {
		return zero, false
	}
	return algebra.default_, true
}

// Top returns the sealed greatest element of the typed Factor. It is the
// sound over-approximation the read boundary substitutes for a read whose
// dispatch alternative set is opaque and which declared widening.
func (algebra *Algebra[K, V]) Top() (V, bool) {
	var zero V
	if algebra == nil || algebra.seal == nil {
		return zero, false
	}
	return algebra.top_, true
}

// Equal is the admitted value equality used by typed query projection. It
// forwards the sealed algebra; callers cannot replace its implementation.
func (algebra *Algebra[K, V]) Equal(left, right V) bool {
	return algebra != nil && algebra.seal != nil && algebra.equal(left, right)
}

// Fingerprint returns the admitted semantic bucket for one value. Equality
// remains the authority: callers may use this only to avoid comparisons
// between values that cannot be equal, and must resolve collisions with
// Equal.
func (algebra *Algebra[K, V]) Fingerprint(value V) uint64 {
	if algebra == nil || algebra.seal == nil {
		return 0
	}
	return algebra.fingerprint(value)
}

func (algebra *Algebra[K, V]) valid() bool { return algebra != nil && algebra.seal != nil }

func (algebra *Algebra[K, V]) sameValue(left, right V) bool {
	return algebra.same != nil && algebra.same(left, right) || algebra.equal(left, right)
}

func (algebra *Algebra[K, V]) sameContradictsEqual(left, right V) bool {
	return algebra.same != nil && algebra.same(left, right) && !algebra.equal(left, right)
}

// admits is the one typed coordinate fence. It is deliberately separate
// from the Factor lattice: V is homogeneous across the Factor, while a
// coordinate may admit only a finite Link-derived subset of that lattice.
func (algebra *Algebra[K, V]) admits(key K, value V) bool {
	return algebra.valid() && uint64(key) < algebra.keyEnd && algebra.admitAt(key, value)
}

// joinStable is the sparse-ingress validity law. Same-root carrier Join may
// retain a published root without visiting its terminals, so every directly
// admitted value must be a fixed point of semantic Join. Widen and Narrow are
// recurrence strategies and are intentionally not invoked by ordinary writes.
//
// A value must first be equal to itself. That is not part of the Join law, it
// is what makes the value a fact at all: every consumer compares it - to the
// Factor default, to a predecessor, to another cell of the same vector - and a
// value the Factor's own equality will not call equal to itself gives all of
// them an answer that depends on nothing. Same cannot stand in for it here.
// Same says two values are the same object, not that the Factor considers them
// equal, so a Join fixed point that holds only under Same would admit exactly
// the value every reader would then have to re-authenticate for itself.
func (algebra *Algebra[K, V]) joinStable(value V) bool {
	return algebra.equal(value, value) && algebra.sameValue(algebra.join(value, value), value)
}

// validateWiden is invoked inside the synchronized typed FDD operation, once
// per exact key/terminal transition.  A Factor without a WidenRank belongs
// only to an acyclic compiled tuple and has no authority to widen here.
func (algebra *Algebra[K, V]) validateWiden(key K, previous, requested, output V) bool {
	if !algebra.widenRank.valid() || !algebra.admits(key, output) || !algebra.lessOrEq(previous, output) || !algebra.lessOrEq(requested, output) {
		return false
	}
	return algebra.sameValue(previous, output) || algebra.widenRank.descends(key, previous, output)
}

// validateNarrow enforces the dual pointwise bounds and its own well-founded
// descent witness.  Narrow is never substituted with Join at this boundary.
func (algebra *Algebra[K, V]) validateNarrow(key K, previous, desired, output V) bool {
	if algebra.narrow == nil || !algebra.narrowRank.valid() || !algebra.admits(key, output) || !algebra.lessOrEq(desired, output) || !algebra.lessOrEq(output, previous) {
		return false
	}
	return algebra.sameValue(previous, output) || algebra.narrowRank.descends(key, previous, output)
}

// Binding is one operation's active-epoch typed authority. It is both the one
// single-use cold FactorOperation and, after Attach receives its SlotOwner,
// the sealed SlotOperation. A refresh constructs another Binding/composition;
// no active Binding is ever retargeted to a new typed root store.
type Binding[K scalar.Key, V any] struct {
	lifecycle sync.Mutex
	algebra   *Algebra[K, V]
	plane     *plane[K, V]
	// stageConfig is the sealed staged law surface handed to every write
	// transaction. It is built once from the algebra at Bind.
	stageConfig stage.Config[K, V]
	issuer      carrier.Issuer
	// roots owns only the immutable composition-attached initial root.  Every
	// dynamic candidate/published root belongs instead to one bindingWork's
	// epoch-local store and is revoked with that Work.
	roots      *rootStore[planeFactor, K, V]
	initial    *rootReservation[planeFactor, K, V]
	units      map[carrier.Unit]declaredUnit[K]
	unitList   []carrier.Unit
	reverse    map[K][]carrier.Unit
	targets    map[carrier.Target]declaredTarget[K]
	targetList []carrier.Target
	// targetReverse is the immutable Target-incidence read index used by hot
	// sparse root operations. It never stores Guard regions or authorship:
	// SlotCoverage remains the sole dynamic presence authority.
	targetReverse    map[K][]int
	widenScopes      []widenScope[K]
	narrowScopes     []widenScope[K]
	declaring        bool
	sealed           bool
	prepared         bool
	bound            bool
	scopeFrozen      bool
	phase            declarationPhase
	lastExact        K
	hasExact         bool
	lastSummary      []K
	lastDistributive bool
	lastStrong       unitOrder
	lastWeak         []unitOrder
	summaries        uint64
	targetCount      uint64
}

type declarationPhase uint8

const (
	declareExact declarationPhase = iota
	declareSummary
	declareStrong
	declareWeak
)

// Canonical unit order is a tuple, never `KeyEnd + id`: maximal KeyEnd values
// must not wrap a summary into the exact-key range.
type unitOrder struct {
	kind carrier.UnitKind
	id   uint64
}

type targetOrder struct {
	mode carrier.TargetMode
	id   uint64
}

// The following descriptors are Binding-private typed authority.  They are
// neither carrier vocabulary nor a second evaluator: capability values point
// into these tables, and only Binding resolves their K surfaces.
type declaredUnit[K scalar.Key] struct {
	order    unitOrder
	position int
	keys     []K
	// distributive marks a summary whose reader folds each declared
	// coordinate independently.  It is sealed at declaration from the cold
	// read form and is never chosen per observation.
	distributive bool
}

type declaredTarget[K scalar.Key] struct {
	order         targetOrder
	position      int
	keys          []K
	units         []carrier.Unit
	notifications []carrier.Unit
}

type canonicalKeyCursor[K scalar.Key] struct {
	keys []K
	next int
}

// plane retains the exact typed identities for one binding. One diagram with
// one private factor column is intentional: the carrier SlotOwner supplies
// heterogeneous position while the FDD supplies this operation's key/guard/
// value algebra.
type plane[K scalar.Key, V any] struct {
	values  *terminal.Arena[V]
	diagram *diagram.Diagram[planeFactor, K, V]
	domain  *semantic.Domain[planeFactor, K, V]
}

// planeFactor is private to this one-unit diagram. The carrier SlotOwner,
// rather than a numeric external fact column, establishes outer identity.
type planeFactor uint64

const solePlaneFactor planeFactor = 1

// Bind creates one guard-bound Binding from a previously admitted immutable
// Algebra. It deliberately performs no KeyEnd-wide admission scan: Admit
// already proved Default for the complete dense universe.
func Bind[K scalar.Key, V any](algebra *Algebra[K, V], guards *guard.Manager, declare func(*Binding[K, V]) bool) (*Binding[K, V], bool) {
	if !algebra.valid() || guards == nil {
		return nil, false
	}
	issuer, ok := carrier.NewIssuer()
	if !ok {
		return nil, false
	}
	binding := &Binding[K, V]{
		algebra:   algebra,
		issuer:    issuer,
		units:     make(map[carrier.Unit]declaredUnit[K]),
		targets:   make(map[carrier.Target]declaredTarget[K]),
		declaring: true,
	}
	// The staged law surface is a property of the sealed algebra, not of one
	// write. It is compiled once here so opening a write transaction does not
	// build a closure over this Binding on every invocation.
	binding.stageConfig = stage.Config[K, V]{
		KeyEnd:     algebra.keyEnd,
		Default:    algebra.default_,
		AdmitAt:    algebra.admitAt,
		Equal:      algebra.equal,
		LessOrEq:   algebra.lessOrEq,
		JoinStable: algebra.joinStable,
		Join: func(left, right V) (V, bool) {
			return algebra.join(left, right), true
		},
	}
	if declare != nil && !declare(binding) {
		return nil, false
	}
	if !binding.sealReverse() {
		return nil, false
	}
	binding.declaring = false
	binding.sealed = true
	if !binding.bindPlane(guards) {
		return nil, false
	}
	return binding, true
}

// DeclareExact seals one exact typed dependency unit. It is legal only during
// Bind's declaration callback; afterward no API receives a raw K.
func (binding *Binding[K, V]) DeclareExact(key K) (carrier.Unit, bool) {
	if binding == nil || !binding.declaring || binding.phase > declareExact || binding.algebra == nil || uint64(key) >= binding.algebra.keyEnd || binding.hasExact && key <= binding.lastExact {
		return carrier.Unit{}, false
	}
	// Unit identities are carrier-opaque uint64 values; the zero-extension is
	// only at that authority boundary, never in typed key storage or order.
	id := uint64(key) + 1
	unit, ok := binding.issuer.IssueUnit(carrier.ExactUnit, id, id)
	if !ok {
		return carrier.Unit{}, false
	}
	binding.units[unit] = declaredUnit[K]{order: unitOrder{kind: carrier.ExactUnit, id: id}, position: len(binding.unitList), keys: []K{key}}
	binding.unitList = append(binding.unitList, unit)
	binding.lastExact, binding.hasExact = key, true
	return unit, true
}

// DeclareSummary seals one finite, canonical typed dependency surface.  Its
// key set is both the summary's coverage witness and its later invalidation
// closure; summaries never gain candidates after this cut.
// declaredKeyCount is the Binding's sealed declared-key inventory: every
// declared key of every Unit, counted once. It is the size of a table that
// gives each declared key its own coordinate.
func (binding *Binding[K, V]) declaredKeyCount() int {
	count := 0
	for _, declared := range binding.units {
		count += len(declared.keys)
	}
	return count
}

// declaredKeyOffsets is each Unit's base coordinate in that inventory, in
// declaration order, so a Unit's key at position index is addressed at the
// Unit's offset plus index.
func (binding *Binding[K, V]) declaredKeyOffsets() []int {
	offsets := make([]int, len(binding.unitList))
	base := 0
	for position, unit := range binding.unitList {
		offsets[position] = base
		base += len(binding.units[unit].keys)
	}
	return offsets
}

func (binding *Binding[K, V]) DeclareSummary(keys []K) (carrier.Unit, bool) {
	return binding.declareSummary(keys, false)
}

// DeclareDistributiveSummary seals a summary whose declared coordinates are
// folded independently by its reader. The two folds are distinct Units even
// over the same key vector: a correlated reader of those keys still receives
// the exact joint partition.
func (binding *Binding[K, V]) DeclareDistributiveSummary(keys []K) (carrier.Unit, bool) {
	return binding.declareSummary(keys, true)
}

func (binding *Binding[K, V]) declareSummary(keys []K, distributive bool) (carrier.Unit, bool) {
	if binding == nil || !binding.declaring || binding.phase > declareSummary || binding.algebra == nil {
		return carrier.Unit{}, false
	}
	// A zero-key summary is one Factor's sealed empty projection: the limit
	// of the proper coordinate subsets already admitted. It issues no
	// coordinate, so it carries no coordinate-level invalidation closure, and
	// its fold reads the declared constant.
	frozen := append([]K(nil), keys...)
	for index, key := range frozen {
		if binding.algebra == nil || uint64(key) >= binding.algebra.keyEnd || index > 0 && frozen[index-1] >= key {
			return carrier.Unit{}, false
		}
	}
	if binding.summaries != 0 && !summaryOrderLess(binding.lastSummary, binding.lastDistributive, frozen, distributive) {
		return carrier.Unit{}, false
	}
	binding.phase = declareSummary
	id, ok := nextOrdinal(binding.summaries)
	if !ok {
		return carrier.Unit{}, false
	}
	binding.summaries = id
	unit, ok := binding.issuer.IssueUnit(carrier.SummaryUnit, id, id)
	if !ok {
		return carrier.Unit{}, false
	}
	binding.units[unit] = declaredUnit[K]{order: unitOrder{kind: carrier.SummaryUnit, id: id}, position: len(binding.unitList), keys: frozen, distributive: distributive}
	binding.unitList = append(binding.unitList, unit)
	binding.lastSummary, binding.lastDistributive = frozen, distributive
	return unit, true
}

// DeclareStrong seals singleton replacement authority for one exact Unit.
// A summary cannot be made strong because it denotes more than one concrete
// target surface.
func (binding *Binding[K, V]) DeclareStrong(unit carrier.Unit) (carrier.Target, bool) {
	entry, ok := binding.unit(unit)
	if !ok || binding.phase > declareStrong || unit.Kind() != carrier.ExactUnit || len(entry.keys) != 1 || !unitOrderLess(binding.lastStrong, entry.order) {
		return carrier.Target{}, false
	}
	binding.phase, binding.lastStrong = declareStrong, entry.order
	return binding.declareTarget(carrier.StrongTarget, []carrier.Unit{unit})
}

// DeclareWeak seals finite weak-update authority over already declared Units.
func (binding *Binding[K, V]) DeclareWeak(units []carrier.Unit) (carrier.Target, bool) {
	if binding == nil || binding.phase > declareWeak {
		return carrier.Target{}, false
	}
	orders, ok := binding.unitOrders(units)
	if !ok || len(binding.lastWeak) != 0 && !unitOrdersLess(binding.lastWeak, orders) {
		return carrier.Target{}, false
	}
	binding.phase, binding.lastWeak = declareWeak, orders
	return binding.declareTarget(carrier.WeakTarget, units)
}

func (binding *Binding[K, V]) declareTarget(mode carrier.TargetMode, units []carrier.Unit) (carrier.Target, bool) {
	if binding == nil || !binding.declaring || len(units) == 0 {
		return carrier.Target{}, false
	}
	spans := make([][]K, 0, len(units))
	var prior unitOrder
	for _, unit := range units {
		entry, ok := binding.unit(unit)
		if !ok || !unitOrderLess(prior, entry.order) {
			return carrier.Target{}, false
		}
		prior = entry.order
		spans = append(spans, entry.keys)
	}
	keys, ok := mergeCanonicalKeySpans(spans)
	if !ok {
		return carrier.Target{}, false
	}
	id, ok := nextOrdinal(binding.targetCount)
	if !ok {
		return carrier.Target{}, false
	}
	binding.targetCount = id
	target, ok := binding.issuer.IssueTarget(id, mode)
	if !ok {
		return carrier.Target{}, false
	}
	binding.targets[target] = declaredTarget[K]{order: targetOrder{mode: mode, id: id}, position: len(binding.targetList), keys: keys, units: append([]carrier.Unit(nil), units...)}
	binding.targetList = append(binding.targetList, target)
	return target, true
}

// mergeCanonicalKeySpans returns the canonical union of non-empty, strictly
// ordered key spans. Unit declaration seals that local ordering before target
// declaration, while target units are ordered by Unit identity rather than by
// their first key. Consequently concatenation is not globally ordered: weak
// summary spans commonly interleave. A k-way merge keeps this cold cut at
// O(total keys * log span count), rather than insertion-sorting the flattened
// vector in O(total keys squared). The result owns its backing array.
func mergeCanonicalKeySpans[K scalar.Key](spans [][]K) ([]K, bool) {
	if len(spans) == 0 {
		return nil, false
	}
	total := 0
	for _, span := range spans {
		if len(span) == 0 || len(span) > int(^uint(0)>>1)-total {
			return nil, false
		}
		total += len(span)
	}
	if len(spans) == 1 {
		return append([]K(nil), spans[0]...), true
	}

	heap := make([]canonicalKeyCursor[K], len(spans))
	for index, span := range spans {
		heap[index] = canonicalKeyCursor[K]{keys: span}
	}
	for parent := len(heap)/2 - 1; parent >= 0; parent-- {
		siftCanonicalKeyCursor(heap, parent)
	}

	merged := make([]K, 0, total)
	for len(heap) != 0 {
		key := heap[0].keys[heap[0].next]
		if len(merged) == 0 || merged[len(merged)-1] != key {
			merged = append(merged, key)
		}
		heap[0].next++
		if heap[0].next == len(heap[0].keys) {
			last := len(heap) - 1
			heap[0] = heap[last]
			heap = heap[:last]
			if len(heap) == 0 {
				break
			}
		}
		siftCanonicalKeyCursor(heap, 0)
	}
	return merged, true
}

func siftCanonicalKeyCursor[K scalar.Key](heap []canonicalKeyCursor[K], parent int) {
	for {
		child := parent*2 + 1
		if child >= len(heap) {
			return
		}
		right := child + 1
		if right < len(heap) && heap[right].keys[heap[right].next] < heap[child].keys[heap[child].next] {
			child = right
		}
		if heap[parent].keys[heap[parent].next] <= heap[child].keys[heap[child].next] {
			return
		}
		heap[parent], heap[child] = heap[child], heap[parent]
		parent = child
	}
}

func (binding *Binding[K, V]) unit(unit carrier.Unit) (declaredUnit[K], bool) {
	if binding == nil || !binding.declaring {
		return declaredUnit[K]{}, false
	}
	entry, ok := binding.units[unit]
	return entry, ok
}

func (binding *Binding[K, V]) unitOrders(units []carrier.Unit) ([]unitOrder, bool) {
	if binding == nil || !binding.declaring || len(units) == 0 {
		return nil, false
	}
	orders := make([]unitOrder, len(units))
	for index, unit := range units {
		entry, ok := binding.unit(unit)
		if !ok || index > 0 && !unitOrderLess(orders[index-1], entry.order) {
			return nil, false
		}
		orders[index] = entry.order
	}
	return orders, true
}

func keysLess[K scalar.Key](left, right []K) bool {
	for index := 0; index < len(left) && index < len(right); index++ {
		if left[index] != right[index] {
			return left[index] < right[index]
		}
	}
	return len(left) < len(right)
}

// summaryOrderLess is the declaration order over summary Units. The key
// vector orders first; the fold breaks the tie so one key vector may carry
// both a correlated and a distributive Unit, in that order.
func summaryOrderLess[K scalar.Key](left []K, leftDistributive bool, right []K, rightDistributive bool) bool {
	if keysLess(left, right) {
		return true
	}
	if keysLess(right, left) {
		return false
	}
	return !leftDistributive && rightDistributive
}

func unitOrderLess(left, right unitOrder) bool {
	return left.kind < right.kind || left.kind == right.kind && left.id < right.id
}

func targetOrderLess(left, right targetOrder) bool {
	return left.mode < right.mode || left.mode == right.mode && left.id < right.id
}

func unitOrdersLess(left, right []unitOrder) bool {
	for index := 0; index < len(left) && index < len(right); index++ {
		if left[index] != right[index] {
			return unitOrderLess(left[index], right[index])
		}
	}
	return len(left) < len(right)
}

func nextOrdinal(current uint64) (uint64, bool) {
	if current == ^uint64(0) {
		return 0, false
	}
	return current + 1, true
}

// sealReverse freezes the Factor-owned dependency closure. Every concrete
// typed key points to all exact and summary Units invalidated by a semantic
// change at that key, in canonical declaration order.
func (binding *Binding[K, V]) sealReverse() bool {
	if binding == nil || !binding.declaring || binding.reverse != nil || binding.targetReverse != nil {
		return false
	}
	reverse := make(map[K][]carrier.Unit)
	targetReverse := make(map[K][]int)
	for position, unit := range binding.unitList {
		descriptor, ok := binding.units[unit]
		if !ok || descriptor.position != position {
			return false
		}
		for _, key := range descriptor.keys {
			reverse[key] = append(reverse[key], unit)
		}
	}
	for position, target := range binding.targetList {
		descriptor, declared := binding.targets[target]
		if !declared || descriptor.position != position {
			return false
		}
		for _, key := range descriptor.keys {
			if len(reverse[key]) == 0 {
				return false
			}
			targetReverse[key] = append(targetReverse[key], position)
		}
		notifications, ok := notificationUnion(reverse, descriptor.keys)
		if !ok {
			return false
		}
		descriptor.notifications = notifications
		binding.targets[target] = descriptor
	}
	for key, targets := range targetReverse {
		for index := 1; index < len(targets); index++ {
			if targets[index-1] >= targets[index] {
				return false
			}
		}
		targetReverse[key] = targets
	}
	binding.reverse = reverse
	binding.targetReverse = targetReverse
	return true
}

// notificationUnion derives the canonical reverse wake surface for one
// authored target scope. It is intentionally computed at declaration seal;
// runtime ChangeSets use the same reverse table for actual changed keys.
func notificationUnion[K scalar.Key](reverse map[K][]carrier.Unit, keys []K) ([]carrier.Unit, bool) {
	if len(keys) == 0 {
		return nil, false
	}
	result := make([]carrier.Unit, 0, len(keys))
	for _, key := range keys {
		units := reverse[key]
		if len(units) == 0 {
			return nil, false
		}
		for _, unit := range units {
			duplicate := false
			for _, prior := range result {
				if prior.Same(unit) {
					duplicate = true
					break
				}
			}
			if !duplicate {
				result = append(result, unit)
			}
		}
	}
	return result, len(result) != 0
}

// bindPlane constructs this Factor's immutable typed terminal, FDD, and
// semantic authority without selecting an outer heterogeneous carrier. Bind
// RootHandle composition uses exactly this authority; no second V store or
// payload adapter is introduced.
func (binding *Binding[K, V]) bindPlane(guards *guard.Manager) bool {
	if binding == nil || binding.plane != nil || guards == nil {
		return false
	}
	values, ok := terminal.New(terminal.Config[V]{
		Equal:       binding.algebra.equal,
		Fingerprint: binding.algebra.fingerprint,
	})
	if !ok {
		return false
	}
	// Sparse absence denotes Default.  Admit that one semantic value before
	// freezing the base page; every other value remains candidate-local until
	// its rule patch accepts.  This is a semantic distinguished value, not a
	// catalog or capacity bound.
	if _, ok := values.Admit(binding.algebra.default_); !ok || !values.Seal() {
		return false
	}
	factsDiagram, ok := diagram.New(diagram.Config[planeFactor, K, V]{
		Factors:   []planeFactor{solePlaneFactor},
		Terminals: values,
		Guards:    guards,
	})
	if !ok {
		return false
	}
	ops := semantic.Operations[V]{
		Default:     binding.algebra.default_,
		Equal:       binding.algebra.equal,
		Fingerprint: binding.algebra.fingerprint,
		Join:        func(left, right V) (V, bool) { return binding.algebra.join(left, right), true },
		Widen:       func(left, right V) (V, bool) { return binding.algebra.widen(left, right), true },
		LessOrEq:    binding.algebra.lessOrEq,
	}
	if binding.algebra.narrow != nil {
		ops.Narrow = func(left, right V) (V, bool) { return binding.algebra.narrow(left, right), true }
	}
	domain, ok := semantic.NewWithConvergence(factsDiagram, values, ops, semantic.Convergence[K, V]{
		Widen:  binding.algebra.validateWiden,
		Narrow: binding.algebra.validateNarrow,
	})
	if !ok {
		return false
	}
	binding.plane = &plane[K, V]{values: values, diagram: factsDiagram, domain: domain}
	return true
}
