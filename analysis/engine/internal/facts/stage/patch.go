// Package stage owns one private, one-shot typed fact patch.
//
// A Patch has exactly one publication candidate: a mutable output root above
// an immutable input root. It owns one terminal.Work for candidate values and
// no copy of the shared Boolean support. There are no read observations,
// scheduling, Fiber, or transaction-policy concepts here; factbinding is the
// only intended consumer.
package stage

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/diagram"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/scalar"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/terminal"
)

// Config is the small domain-owned law surface needed by a staged typed
// plane.  Default is the semantic value of an absent sparse key; it is not
// assumed to be a lattice bottom or a zero Go value.  Join is used only by
// WeakJoin, and may reject an invalid candidate result.
type Config[K scalar.Key, V any] struct {
	KeyEnd   uint64
	Default  V
	AdmitAt  func(K, V) bool
	Equal    func(V, V) bool
	Join     func(V, V) (V, bool)
	LessOrEq func(V, V) bool
	// JoinStable accepts values fixed by the Factor's semantic Join.  Sparse
	// roots and same-root carrier joins may retain an admitted terminal without
	// invoking Join again, so every newly staged value must satisfy this law.
	// Widen and Narrow are recurrence strategies, not ingress validators.
	JoinStable func(V) bool
}

// Patch is a single-writer, write-only candidate update. Neither its candidate
// FDD root nor terminal Work escape before Accept.
type Patch[F ~uint64, K scalar.Key, V any] struct {
	diagram   *diagram.Diagram[F, K, V]
	values    *terminal.Arena[V]
	terminals *terminal.Work[V]
	builder   *diagram.Builder[F, K, V]
	regions   *support.Work
	scratch   diagram.SoleScratch[K, V]
	config    Config[K, V]
	factor    F
	within    support.Mask
	base      diagram.Root[F, K, V]
	root      diagram.Root[F, K, V]
	changes   []keyChange[K, V]
	// transformMemo belongs to this one candidate Patch.  It prevents one
	// carried terminal shared by many keys or Product rows from being mapped
	// repeatedly, and is discarded with the candidate before publication.
	transformMemo map[terminal.ID[V]]terminal.ID[V]
	closed        bool
}

// KeyChanges is the exact net typed-key difference produced by one accepted
// Patch.  It has no public constructor: entries arise only from the
// synchronized base/current rewrite below.  K remains inside the typed
// Binding boundary and never enters the heterogeneous carrier.
type KeyChanges[K scalar.Key] struct {
	rows []KeyChange[K]
}

// KeyChange names one changed typed key and the exact Guard region on which
// its accepted value differs from the immutable patch predecessor.
type KeyChange[K scalar.Key] struct {
	key    K
	region support.Mask
}

type keyChange[K scalar.Key, V any] struct {
	key    K
	base   diagram.Value[V]
	region support.Mask
}

func (changes KeyChanges[K]) Count() int { return len(changes.rows) }

func (changes KeyChanges[K]) At(index int) (K, support.Mask, bool) {
	if index < 0 || index >= len(changes.rows) {
		return 0, support.Mask{}, false
	}
	row := changes.rows[index]
	return row.key, row.region, true
}

// Begin opens one candidate terminal page and one candidate FDD builder above
// base.  Both seal at the same Accept cut through BeginWithTerminals; a value
// admitted during the patch cannot become readable through a published root
// beforehand.
func Begin[F ~uint64, K scalar.Key, V any](facts *diagram.Diagram[F, K, V], base diagram.Root[F, K, V], within support.Mask, config Config[K, V]) *Patch[F, K, V] {
	if facts == nil || !facts.Valid(base) || !within.Valid() || within.Manager() != facts.Guards() || config.AdmitAt == nil || config.Equal == nil || config.Join == nil || config.LessOrEq == nil {
		return nil
	}
	values := facts.Terminals()
	factor, sole := facts.SoleFactor()
	if !sole {
		return nil
	}
	terminals := values.Begin()
	if terminals == nil {
		return nil
	}
	builder := facts.BeginWithTerminals(terminals)
	if builder == nil {
		return nil
	}
	return &Patch[F, K, V]{
		diagram: facts, values: values, terminals: terminals, builder: builder,
		config: config, factor: factor, within: within, base: base, root: base,
	}
}

// Set strongly overwrites key precisely where when holds.  A Default result
// becomes sparse undefined in that region, preserving the declared absent-key
// law without inventing a terminal for semantic absence.
func (patch *Patch[F, K, V]) Set(key K, when support.Mask, value V) bool {
	if !patch.open() || !patch.validKey(key) || !patch.admits(key, value) || !when.Valid() || when.Manager() != patch.diagram.Guards() || !when.Entails(patch.within) {
		return false
	}
	if patch.config.JoinStable != nil && !patch.config.JoinStable(value) {
		return false
	}
	if empty, valid := maskEmpty(when); !valid {
		return false
	} else if empty {
		return true
	}
	output := terminal.ID[V]{}
	if patch.config.Equal(value, patch.config.Default) {
		// Sparse undefined is the typed Default.
	} else {
		id, admitted := patch.terminals.Admit(value)
		if !admitted {
			return false
		}
		output = id
	}
	return patch.rewrite(key, when, func(terminal.ID[V]) (terminal.ID[V], bool) { return output, true })
}

// WeakJoin applies the domain's exact Join to the prior semantic value and
// incoming value only where when holds. Undefined FDD leaves resolve to
// Config.Default, and a resulting Default is re-sparsified. Outside when,
// the exact prior FDD is retained unchanged. Incoming values are not admitted
// as terminals merely to perform the join: only surviving output values enter
// this candidate terminal page.
func (patch *Patch[F, K, V]) WeakJoin(key K, when support.Mask, incoming V) bool {
	if !patch.open() || !patch.validKey(key) || !patch.admits(key, incoming) {
		return false
	}
	return patch.rewrite(key, when, func(prior terminal.ID[V]) (terminal.ID[V], bool) {
		return patch.joinWithin(key, prior, incoming)
	})
}

// WeakJoinMany applies one finite presealed target surface atomically with
// respect to this Patch root.  Every key is evaluated against a local
// candidate root; a failure leaves the publication candidate unchanged.  It
// is intentionally private staging machinery, not a raw-key engine API.
func (patch *Patch[F, K, V]) WeakJoinMany(keys []K, when support.Mask, incoming V) bool {
	if !patch.open() || len(keys) == 0 {
		return false
	}
	// Validate the whole presealed target surface before its first rewrite.
	// A heterogeneous weak target therefore fails atomically rather than
	// publishing an admitted prefix before a later key rejects the value.
	for _, key := range keys {
		if !patch.validKey(key) || !patch.admits(key, incoming) {
			return false
		}
	}
	for _, key := range keys {
		if !patch.rewrite(key, when, func(prior terminal.ID[V]) (terminal.ID[V], bool) {
			return patch.joinWithin(key, prior, incoming)
		}) {
			// A failed multi-key target never leaves its partially explored
			// candidate terminal/FDD work available for a later Accept.  There
			// is no rollback path: this one-shot Patch is consumed and callers
			// rebuild from the immutable predecessor.
			patch.closeDiscarded()
			return false
		}
	}
	return true
}

// Transform applies one total, default-preserving Factor-owned map to each
// distinct presealed key under the same guard. The owner proves monotonicity,
// same-operand idempotence, and join homomorphism; transforms are not required
// to be extensive. The
// callback is
// evaluated at most once per candidate terminal ID across the whole key
// batch; this is the only terminal memo and it dies with this Patch.
//
// It is intentionally not a general state traversal.  Its key vector is
// compiled by factbinding from the Rule's finite carried-target closure.
func (patch *Patch[F, K, V]) Transform(keys []K, when support.Mask, apply func(V) (V, bool)) bool {
	return patch.TransformSortedUnion(keys, nil, when, apply)
}

// TransformSortedUnion applies the transform over the sorted union of two
// already-authenticated key vectors. It is the zero-allocation composition
// primitive for one authored carry closure plus one shared Factor route
// closure: no per-call union slice or key map is built.
func (patch *Patch[F, K, V]) TransformSortedUnion(left, right []K, when support.Mask, apply func(V) (V, bool)) bool {
	if !patch.open() || apply == nil || !when.Valid() || when.Manager() != patch.diagram.Guards() || !when.Entails(patch.within) {
		return false
	}
	defaultValue, defaultOK := apply(patch.config.Default)
	if !defaultOK || !patch.config.Equal(defaultValue, patch.config.Default) {
		patch.closeDiscarded()
		return false
	}
	if len(left) == 0 && len(right) == 0 {
		return true
	}
	if patch.transformMemo == nil {
		patch.transformMemo = make(map[terminal.ID[V]]terminal.ID[V])
	}
	memo := patch.transformMemo
	leftIndex, rightIndex := 0, 0
	var prior K
	havePrior := false
	for leftIndex < len(left) || rightIndex < len(right) {
		var key K
		if rightIndex >= len(right) || leftIndex < len(left) && left[leftIndex] < right[rightIndex] {
			key, leftIndex = left[leftIndex], leftIndex+1
		} else if leftIndex >= len(left) || right[rightIndex] < left[leftIndex] {
			key, rightIndex = right[rightIndex], rightIndex+1
		} else {
			key = left[leftIndex]
			leftIndex++
			rightIndex++
		}
		if havePrior && key == prior {
			continue
		}
		prior, havePrior = key, true
		if !patch.validKey(key) {
			patch.closeDiscarded()
			return false
		}
		operation := func(prior terminal.ID[V]) (terminal.ID[V], bool) {
			if prior == (terminal.ID[V]{}) {
				return terminal.ID[V]{}, true
			}
			if next, found := memo[prior]; found {
				if next == (terminal.ID[V]{}) {
					return next, true
				}
				nextValue, nextOK := patch.terminals.Value(next)
				return next, nextOK && patch.admits(key, nextValue)
			}
			value, valueOK := patch.terminals.Value(prior)
			if !valueOK {
				return terminal.ID[V]{}, false
			}
			nextValue, transformed := apply(value)
			if !transformed || !patch.admits(key, nextValue) || patch.config.JoinStable != nil && !patch.config.JoinStable(nextValue) {
				return terminal.ID[V]{}, false
			}
			var next terminal.ID[V]
			if !patch.config.Equal(nextValue, patch.config.Default) {
				var admitted bool
				next, admitted = patch.terminals.Admit(nextValue)
				if !admitted {
					return terminal.ID[V]{}, false
				}
			}
			memo[prior] = next
			return next, true
		}
		if !patch.rewrite(key, when, operation) {
			patch.closeDiscarded()
			return false
		}
	}
	return true
}

func (patch *Patch[F, K, V]) rewrite(key K, when support.Mask, operation diagram.Transform[V]) bool {
	if !patch.open() || !patch.validKey(key) || !when.Valid() || when.Manager() != patch.diagram.Guards() || !when.Entails(patch.within) {
		return false
	}
	if !patch.ensureRegions() {
		return false
	}
	current, present, valid := patch.builder.Get(patch.root, patch.factor, key)
	if !valid {
		return false
	}
	if !present {
		var ok bool
		current, ok = patch.builder.Constant(terminal.ID[V]{})
		if !ok {
			return false
		}
	}
	index, base, ok := patch.baseFor(key)
	if !ok {
		return false
	}
	nextValue, changed, ok := patch.builder.TrackedTransform(base, current, when, patch.within, patch.regions, &patch.scratch, operation, patch.equalTerminal)
	if !ok {
		return false
	}
	next, ok := patch.builder.Put(patch.root, patch.factor, key, nextValue)
	if !ok {
		return false
	}
	patch.root = next
	patch.changes[index].region = changed
	return true
}

func (patch *Patch[F, K, V]) baseFor(key K) (int, diagram.Value[V], bool) {
	for index, change := range patch.changes {
		if change.key == key {
			return index, change.base, true
		}
		if change.key > key {
			break
		}
	}
	base, present, valid := patch.builder.Get(patch.base, patch.factor, key)
	if !valid {
		return 0, diagram.Value[V]{}, false
	}
	if !present {
		var ok bool
		base, ok = patch.builder.Constant(terminal.ID[V]{})
		if !ok {
			return 0, diagram.Value[V]{}, false
		}
	}
	index := len(patch.changes)
	for candidate, change := range patch.changes {
		if change.key > key {
			index = candidate
			break
		}
	}
	patch.changes = append(patch.changes, keyChange[K, V]{})
	copy(patch.changes[index+1:], patch.changes[index:])
	patch.changes[index] = keyChange[K, V]{key: key, base: base, region: patch.regions.False()}
	return index, base, true
}

func (patch *Patch[F, K, V]) equalTerminal(left, right terminal.ID[V]) bool {
	leftValue, leftOK := patch.config.Default, true
	if left != (terminal.ID[V]{}) {
		leftValue, leftOK = patch.terminals.Value(left)
	}
	rightValue, rightOK := patch.config.Default, true
	if right != (terminal.ID[V]{}) {
		rightValue, rightOK = patch.terminals.Value(right)
	}
	return leftOK && rightOK && patch.config.Equal(leftValue, rightValue)
}

// Accept hands the exact candidate KeyChanges and the Patch-owned Boolean
// transaction to consume before anything is published. consume may expand
// typed closure regions in that same Work; failure discards terminal, FDD,
// and support candidates together. There is no post-seal delta work.
func (patch *Patch[F, K, V]) Accept(consume func(KeyChanges[K], *support.Work) bool) (root diagram.Root[F, K, V], ok bool) {
	if !patch.open() || consume == nil {
		return diagram.Root[F, K, V]{}, false
	}
	if !patch.ensureRegions() {
		patch.closeDiscarded()
		return diagram.Root[F, K, V]{}, false
	}
	rows := make([]KeyChange[K], 0, len(patch.changes))
	for _, change := range patch.changes {
		view, valid := patch.regions.Decompose(change.region)
		if !valid {
			patch.closeDiscarded()
			return diagram.Root[F, K, V]{}, false
		}
		if !view.Terminal || view.Value {
			rows = append(rows, KeyChange[K]{key: change.key, region: change.region})
		}
	}
	changes := KeyChanges[K]{rows: rows}
	if !consume(changes, patch.regions) {
		patch.closeDiscarded()
		return diagram.Root[F, K, V]{}, false
	}
	root, ok = patch.builder.Seal(patch.root)
	if !ok || !patch.regions.Seal() {
		patch.closeDiscarded()
		return diagram.Root[F, K, V]{}, false
	}
	patch.closed = true
	patch.builder = nil
	patch.terminals = nil
	patch.regions = nil
	patch.scratch.Clear()
	patch.within = support.Mask{}
	patch.base = diagram.Root[F, K, V]{}
	patch.root = diagram.Root[F, K, V]{}
	patch.changes = nil
	patch.transformMemo = nil
	return root, true
}

// Discard revokes the candidate FDD and terminal page exactly once.
func (patch *Patch[F, K, V]) Discard() bool {
	if !patch.open() {
		return false
	}
	patch.closeDiscarded()
	return true
}

func (patch *Patch[F, K, V]) open() bool {
	return patch != nil && !patch.closed && patch.diagram != nil && patch.values != nil && patch.terminals != nil && patch.builder != nil && (patch.regions == nil || patch.regions.Open()) && patch.builder.Valid(patch.root)
}

func (patch *Patch[F, K, V]) ensureRegions() bool {
	if patch.regions != nil {
		return patch.regions.Open()
	}
	patch.regions = support.New(patch.diagram.Guards())
	return patch.regions != nil
}

func (patch *Patch[F, K, V]) validKey(key K) bool { return uint64(key) < patch.config.KeyEnd }

func (patch *Patch[F, K, V]) admits(key K, value V) bool {
	return patch != nil && patch.config.AdmitAt != nil && patch.config.AdmitAt(key, value)
}

func (patch *Patch[F, K, V]) joinWithin(key K, left terminal.ID[V], rightValue V) (terminal.ID[V], bool) {
	leftValue := patch.config.Default
	if left != (terminal.ID[V]{}) {
		value, valid := patch.terminals.Value(left)
		if !valid {
			return terminal.ID[V]{}, false
		}
		leftValue = value
	}
	if !patch.admits(key, leftValue) || !patch.admits(key, rightValue) {
		return terminal.ID[V]{}, false
	}
	value, valid := patch.config.Join(leftValue, rightValue)
	if !valid {
		return terminal.ID[V]{}, false
	}
	// A weak update is a Join, not an arbitrary replacement.  Reject a
	// declared Join that fails to cover either operand before admitting its
	// candidate terminal, so no unsound fact root can cross Accept.
	if !patch.admits(key, value) || !patch.config.LessOrEq(leftValue, value) || !patch.config.LessOrEq(rightValue, value) {
		return terminal.ID[V]{}, false
	}
	if patch.config.JoinStable != nil && !patch.config.JoinStable(value) {
		return terminal.ID[V]{}, false
	}
	if patch.config.Equal(value, patch.config.Default) {
		return terminal.ID[V]{}, true
	}
	return patch.terminals.Admit(value)
}

func (patch *Patch[F, K, V]) closeDiscarded() {
	if patch == nil || patch.closed {
		return
	}
	if patch.builder != nil {
		patch.builder.Discard()
	}
	if patch.regions != nil {
		patch.regions.Discard()
	}
	patch.closed = true
	patch.builder = nil
	patch.terminals = nil
	patch.regions = nil
	patch.scratch.Clear()
	patch.within = support.Mask{}
	patch.base = diagram.Root[F, K, V]{}
	patch.root = diagram.Root[F, K, V]{}
	patch.changes = nil
	patch.transformMemo = nil
}

func maskEmpty(mask support.Mask) (bool, bool) {
	view, valid := mask.Decompose()
	return valid && view.Terminal && !view.Value, valid
}
