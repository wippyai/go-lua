package event

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/read"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/store"
	"github.com/wippyai/go-lua/analysis/engine/relation/tuple"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// Event is one immutable before/after tuple transition.  It retains the
// exact aggregate roots and producer range authority that signed it.  A side
// is optional because sparse state has no tuple at that support extent; a
// present side is never replaced by a synthetic absence tuple.
//
// Event values are issued only by Bind.  In particular, there is no public
// event constructor, ID, or local ordinal: authored order is represented by
// the containing Batch vector itself.
type Event struct {
	base     database.Version
	next     database.Version
	range_   arrangement.RangeBinding
	layout   arrangement.Layout
	fence    binding.Fence
	mount    identity.ContentID
	before   tuple.Tuple
	after    tuple.Tuple
	beforeOK bool
	afterOK  bool
	semantic bool
	lineage  bool
	sealed   bool
}

// Available reports whether this event is a complete materialized
// transition.  It is intentionally constant time after Bind; tuple and root
// certificates are redeemed at the only external boundary (ValidFor).
func (event Event) Available() bool {
	return event.sealed && event.base.Available() && event.next.Available() &&
		event.next.SuccessorOf(event.base) && event.range_.Available() &&
		event.layout.Available() && event.fence.Available() && event.mount.Available() &&
		event.base.Fence().Same(event.fence) && event.base.MountedDigest() == event.mount &&
		(event.beforeOK || event.afterOK)
}

// ValidFor redeems the event against the exact mounted runtime that issued
// both tuple sides.  The side tuples are immutable copies, not callback
// borrows from state/read.
func (event Event) ValidFor(mounted witness.Mounted) bool {
	if !event.Available() || !mounted.Available() || mounted.Digest() != event.mount ||
		!mounted.RuntimeFence().Same(event.fence) || !event.base.Fence().Same(mounted.RuntimeFence()) ||
		!event.next.Fence().Same(mounted.RuntimeFence()) || !event.range_.ValidFor(mounted.Fence()) ||
		!event.layout.ValidFor(mounted.Fence()) {
		return false
	}
	if event.beforeOK && !event.before.ValidFor(mounted) {
		return false
	}
	if event.afterOK && !event.after.ValidFor(mounted) {
		return false
	}
	return true
}

// Base returns the exact predecessor root retained by this event.
func (event Event) Base() database.Version {
	if !event.Available() {
		return database.Version{}
	}
	return event.base
}

// Next returns the exact successor root retained by this event.
func (event Event) Next() database.Version {
	if !event.Available() {
		return database.Version{}
	}
	return event.next
}

// Range returns the producer-owned range authority carried by this event.
func (event Event) Range() arrangement.RangeBinding {
	if !event.Available() {
		return arrangement.RangeBinding{}
	}
	return event.range_
}

// Layout returns the exact state layout used to redeem the before/after
// rows.  It is separate from Range: Input range authority intentionally uses
// a zero-column relation layout while its tuple payload has delivered cells.
func (event Event) Layout() arrangement.Layout {
	if !event.Available() {
		return arrangement.Layout{}
	}
	return event.layout
}

// Before returns the materialized predecessor tuple.  A false result is a
// sparse predecessor, not an explicit ProvenAbsent tuple.
func (event Event) Before() (tuple.Tuple, bool) {
	if !event.Available() || !event.beforeOK {
		return tuple.Tuple{}, false
	}
	return event.before, true
}

// After returns the materialized successor tuple.  A false result is a
// sparse successor and is the signed deletion form.  A present tuple may
// still carry a ProvenAbsent cell; that distinction is left intact in the
// returned tuple.
func (event Event) After() (tuple.Tuple, bool) {
	if !event.Available() || !event.afterOK {
		return tuple.Tuple{}, false
	}
	return event.after, true
}

// Scope returns the common normalized scope of the present tuple side.  A
// transition always has at least one side; if both sides are unexpectedly
// absent it is unavailable rather than inventing a scope.
func (event Event) Scope() witness.Scope {
	if !event.Available() {
		return witness.Scope{}
	}
	if event.afterOK {
		return event.after.Scope()
	}
	if event.beforeOK {
		return event.before.Scope()
	}
	return witness.Scope{}
}

// SemanticChanged reports the semantic component retained from the atomic
// database transition.  The flag is copied from Delta rather than inferred
// from tuple payloads, so lineage-only transitions remain distinguishable.
func (event Event) SemanticChanged() bool { return event.Available() && event.semantic }

// LineageChanged reports the proof-lineage component retained from the atomic
// database transition.  It is independent of SemanticChanged.
func (event Event) LineageChanged() bool { return event.Available() && event.lineage }

// Same compares the immutable event payload in authored order terms.  It is
// useful to differential laws and does not introduce an event identity.
func (event Event) Same(other Event) bool {
	if !event.Available() || !other.Available() || !event.base.Same(other.base) ||
		!event.next.Same(other.next) || event.range_.Producer() != other.range_.Producer() ||
		event.range_.Kind() != other.range_.Kind() || !event.layout.Equal(other.layout) ||
		event.semantic != other.semantic || event.lineage != other.lineage ||
		event.beforeOK != other.beforeOK || event.afterOK != other.afterOK {
		return false
	}
	if event.beforeOK && !event.before.Same(other.before) {
		return false
	}
	if event.afterOK && !event.after.Same(other.after) {
		return false
	}
	return true
}

// Batch is one immutable ordered vector of tuple events for one exact
// database Delta and one producer range authority.  Bind appends every
// callback event in state/read order; it never groups by scope or deduplicates
// rows, so repeated authored transitions remain repeated.
type Batch struct {
	base     database.Version
	next     database.Version
	range_   arrangement.RangeBinding
	layout   arrangement.Layout
	fence    binding.Fence
	mount    identity.ContentID
	semantic bool
	lineage  bool
	events   []Event
	sealed   bool
}

// Available reports whether the batch is a complete immutable event vector.
func (batch Batch) Available() bool {
	return batch.sealed && batch.base.Available() && batch.next.Available() &&
		batch.next.SuccessorOf(batch.base) && batch.range_.Available() &&
		batch.layout.Available() && batch.fence.Available() && batch.mount.Available() &&
		batch.base.Fence().Same(batch.fence) && batch.base.MountedDigest() == batch.mount &&
		batch.events != nil
}

// ValidFor redeems the batch against its exact mounted runtime and each
// already materialized event.  No state scan is performed here.
func (batch Batch) ValidFor(mounted witness.Mounted) bool {
	if !batch.Available() || !mounted.Available() || mounted.Digest() != batch.mount ||
		!mounted.RuntimeFence().Same(batch.fence) || !batch.base.Fence().Same(mounted.RuntimeFence()) ||
		!batch.next.Fence().Same(mounted.RuntimeFence()) || !batch.range_.ValidFor(mounted.Fence()) ||
		!batch.layout.ValidFor(mounted.Fence()) {
		return false
	}
	// Bind proves every event and seals the immutable vector before returning.
	// The post-bind envelope check deliberately does not walk that vector:
	// callers cannot mutate the private slice, and revalidating each event here
	// would turn the evaluator boundary back into a width-sized receipt scan.
	return true
}

// Base returns the exact predecessor aggregate root retained by the batch.
func (batch Batch) Base() database.Version {
	if !batch.Available() {
		return database.Version{}
	}
	return batch.base
}

// Next returns the exact successor aggregate root retained by the batch.
func (batch Batch) Next() database.Version {
	if !batch.Available() {
		return database.Version{}
	}
	return batch.next
}

// Range returns the producer-owned range authority retained by the batch.
func (batch Batch) Range() arrangement.RangeBinding {
	if !batch.Available() {
		return arrangement.RangeBinding{}
	}
	return batch.range_
}

// Layout returns the exact state layout used for row redemption.
func (batch Batch) Layout() arrangement.Layout {
	if !batch.Available() {
		return arrangement.Layout{}
	}
	return batch.layout
}

// SemanticChanged reports whether the aggregate transition contains semantic
// changes.  It remains available for an empty, authenticated batch.
func (batch Batch) SemanticChanged() bool { return batch.Available() && batch.semantic }

// LineageChanged reports whether the aggregate transition contains lineage
// changes.  It remains independent from SemanticChanged.
func (batch Batch) LineageChanged() bool { return batch.Available() && batch.lineage }

// Len returns the ordered event count.  Zero can denote either an empty
// authenticated transition or an unavailable batch; callers use Available to
// distinguish the two.
func (batch Batch) Len() int {
	if !batch.Available() {
		return 0
	}
	return len(batch.events)
}

// At returns one event in the exact state/read order produced by Bind.
func (batch Batch) At(index int) (Event, bool) {
	if !batch.Available() || index < 0 || index >= len(batch.events) {
		return Event{}, false
	}
	return batch.events[index], true
}

// Events returns a defensive copy of the ordered event vector.
func (batch Batch) Events() []Event {
	if !batch.Available() {
		return nil
	}
	return append([]Event(nil), batch.events...)
}

// Same compares exact immutable batch contents in authored order, including
// repeated events.  It deliberately does not compare or expose a local event
// ordinal because none exists in the ABI.
func (batch Batch) Same(other Batch) bool {
	if !batch.Available() || !other.Available() || !batch.base.Same(other.base) ||
		!batch.next.Same(other.next) || batch.range_.Producer() != other.range_.Producer() ||
		batch.range_.Kind() != other.range_.Kind() || !batch.layout.Equal(other.layout) ||
		batch.semantic != other.semantic || batch.lineage != other.lineage || len(batch.events) != len(other.events) {
		return false
	}
	for index := range batch.events {
		if !batch.events[index].Same(other.events[index]) {
			return false
		}
	}
	return true
}

// Bind is the sole constructor for the tuple-event ABI.  It authenticates an
// exact database Delta, state layout, producer RangeBinding, Geometry and
// reusable read scratch, then materializes every RowChange side before the
// state callback returns.  It never scans a root as a fallback and never
// deduplicates the ordered change stream.
func Bind(mounted witness.Mounted, delta database.Delta, layout arrangement.Layout, authority arrangement.RangeBinding, view geometry.Geometry, scratch *store.ReadScratch) (Batch, bool) {
	if !mounted.Available() || !delta.Available() || !layout.Available() || !authority.Available() || !view.Available() || scratch == nil || !scratch.Available() {
		return Batch{}, false
	}
	base, next := delta.Base(), delta.Next()
	if !base.Available() || !next.Available() || !next.SuccessorOf(base) ||
		base.MountedDigest() != mounted.Digest() || next.MountedDigest() != mounted.Digest() ||
		!base.Fence().Same(mounted.RuntimeFence()) || !next.Fence().Same(mounted.RuntimeFence()) ||
		!layout.ValidFor(mounted.Fence()) || !authority.ValidFor(mounted.Fence()) ||
		authority.Layout().Access().Relation() != layout.Access().Relation() ||
		!view.ValidFor(mounted) || scratch.Manager() != view.Manager() {
		return Batch{}, false
	}

	changes, ok := read.BindChanges(delta, layout, view, scratch)
	if !ok || !changes.Available() || !changes.Base().Same(base) || !changes.Next().Same(next) || !changes.Layout().Equal(layout) {
		return Batch{}, false
	}
	baseReader := changes.BaseReader()
	if !baseReader.Available() || !baseReader.Layout().Equal(layout) {
		return Batch{}, false
	}
	nextReader := changes.Reader()
	if !nextReader.Available() || !nextReader.Layout().Equal(layout) {
		return Batch{}, false
	}

	result := Batch{
		base: base, next: next, range_: authority, layout: layout,
		fence: mounted.RuntimeFence(), mount: mounted.Digest(),
		semantic: delta.SemanticChanged(), lineage: delta.LineageChanged(),
		events: make([]Event, 0), sealed: true,
	}
	completed, valid := changes.ScanChanges(func(change read.RowChange) bool {
		if !change.Available() || !change.Base().Same(base) || !change.Next().Same(next) || !change.Layout().Equal(layout) {
			return false
		}
		before, beforeOK := change.Before()
		after, afterOK := change.After()
		if afterOK && (after == nil || !after.Available() || !nextReader.Owns(after)) {
			return false
		}
		var beforeTuple tuple.Tuple
		if beforeOK {
			var tupleOK bool
			// RowChange's Before row is trimmed to the exact event refinement
			// and is owned by changes.BaseReader. Copy it while the callback is
			// live; do not replay the full RowID or require a wider row scope.
			beforeTuple, tupleOK = tuple.Input(mounted, baseReader, before)
			if !tupleOK || !beforeTuple.ValidFor(mounted) || !beforeTuple.Scope().Same(change.Scope()) {
				return false
			}
		}
		var afterTuple tuple.Tuple
		if afterOK {
			var tupleOK bool
			afterTuple, tupleOK = tuple.Input(mounted, nextReader, after)
			if !tupleOK || !afterTuple.ValidFor(mounted) || !afterTuple.Scope().Same(change.Scope()) {
				return false
			}
		}
		if !beforeOK && !afterOK {
			return false
		}
		candidate := Event{
			base: base, next: next, range_: authority, layout: layout,
			fence: mounted.RuntimeFence(), mount: mounted.Digest(),
			before: beforeTuple, after: afterTuple,
			beforeOK: beforeOK, afterOK: afterOK,
			semantic: change.SemanticChanged(), lineage: change.LineageChanged(), sealed: true,
		}
		if !candidate.Available() || !candidate.ValidFor(mounted) {
			return false
		}
		result.events = append(result.events, candidate)
		return true
	})
	if !completed || !valid || !result.Available() || !result.ValidFor(mounted) {
		return Batch{}, false
	}
	return result, true
}
