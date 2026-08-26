package publish

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/apply"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/store"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/transaction"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// Door is the solve-local publication capability for one mounted runtime.
//
// The door captures only already-sealed runtime authorities. It does not hold
// a relation catalog, classify a plan, or choose a physical operator. A
// semantic worker supplies one application containing its leased
// ProposalBatch and exact input provenance; state/transaction performs the
// complete validation and the one atomic store replacement. Keeping this
// wrapper immutable makes the publication boundary explicit without
// introducing a second state or delta representation.
type Door struct {
	mounted  witness.Mounted
	geometry geometry.Geometry
}

// New binds the publication door to one complete mount and one solve-local
// geometry view. Schema planning is intentionally absent: the certificate
// has already been checked and mount has already admitted all destination
// denominators, columns, algebras, and lineage authorities.
//
// The door refuses mismatched runtime fences at construction. It never
// manufactures a denominator, cell, scope, lineage, algebra, or presence.
func New(
	mounted witness.Mounted,
	view geometry.Geometry,
) (Door, bool) {
	if !mounted.Available() || !view.ValidFor(mounted) {
		return Door{}, false
	}
	return Door{mounted: mounted, geometry: view}, true
}

// Available reports whether all authorities captured by the door remain
// complete and bound to the same runtime fence.
func (door Door) Available() bool {
	return door.mounted.Available() && door.geometry.ValidFor(door.mounted)
}

// Fence returns the exact solve-local runtime fence redeemed by this door.
func (door Door) Fence() binding.Fence {
	if !door.Available() {
		return binding.Fence{}
	}
	return door.mounted.RuntimeFence()
}

// WideningFor redeems the recurrence capability for one scheduled
// publication only when the application carries an actual proposal batch.
// The publication proposal is the write-intent authority; outcome names are
// deliberately not inspected here.  Non-publishing applications have no
// proposal batch, while any publishing outcome with an empty batch is an
// authenticated no-op, so neither can require or redeem a widening permit.
//
// The evaluator supplies only the sealed schedule entry and destination.  It
// must not reach into Mounted.Widening itself: recurrence admission belongs to
// this publication boundary, immediately before the state transaction can
// redeem a write-bearing batch.
func (door Door) WideningFor(entry arrangement.ScheduleEntry, destination model.RelationID, application apply.Application) (witness.WideningPermit, bool) {
	if !door.Available() || !application.Available() || !destination.Available() {
		return witness.WideningPermit{}, false
	}
	proposals, ok := application.Proposals()
	if !ok || proposals.Len() == 0 {
		return witness.WideningPermit{}, true
	}
	if !entry.Available() || !entry.WideningFor(destination) {
		return witness.WideningPermit{}, true
	}
	permit, ok := door.mounted.Widening(entry.Dependency(), destination)
	if !ok || !permit.Available() {
		return witness.WideningPermit{}, false
	}
	return permit, true
}

// acceptsBase redeems the complete root authority captured by this door.
// RuntimeFence is intentionally only the token namespace; it does not identify
// the mounted address book or physical arrangement. A root from a sibling
// mount can therefore share the same semantic fence while carrying different
// mounted/arrangement digests. Such a root must refuse before even a no-write
// outcome is converted into a settlement.
func (door Door) acceptsBase(base database.Version) bool {
	if !door.Available() || !base.Available() || !base.Fence().Same(door.Fence()) {
		return false
	}
	arrangement := door.mounted.Arrangement()
	return arrangement.Available() && base.MountedDigest() == door.mounted.Digest() && base.ArrangementDigest() == arrangement.Digest()
}

// Publish redeems one semantic application through the sole state transaction
// and root-commit door. The application is the authoritative outcome and
// provenance; the optional widening permit is the only other declared input
// to that same invocation.
//
// A valid non-publishing outcome returns a Settlement with the exact
// predecessor as both Base and Next and no Delta. In particular,
// NoCandidate, NoSelection, and Refused never become Bottom, Unknown,
// ProvenAbsent, or any other fabricated cell. An Opaque application carries
// authenticated rows through the same atomic path as Produced; an Opaque or
// Produced application with no proposals is the analogous exact-root no-op.
//
// The scratch buffer is caller-owned and may not be shared concurrently. It
// is borrowed only while the transaction prepares its private candidate. The
// application proposal lease remains live for the same duration; a stale or
// reset lease refuses.
func (door Door) Publish(
	base database.Version,
	scratch *store.ReadScratch,
	application apply.Application,
	widening witness.WideningPermit,
) Settlement {
	request, requestOK := newRequest(application, widening)
	if !requestOK || !door.acceptsBase(base) || !application.Fence().Same(door.mounted.RuntimeFence()) {
		return Settlement{}
	}
	result := application.Outcome()
	if !result.Code.Publishes() {
		if request.Widening().Available() {
			return Settlement{}
		}
		settlement, ok := noWrite(result, base)
		if !ok {
			return Settlement{}
		}
		return settlement
	}
	batch, batchOK := request.batch(door.mounted)
	if !batchOK {
		return Settlement{}
	}
	if batch.Len() == 0 {
		settlement, ok := publishingNoWrite(result, base)
		if !ok {
			return Settlement{}
		}
		return settlement
	}
	// State reads are needed only to stage a nonempty publishing batch. Exact
	// no-write outcomes above already hold the authenticated predecessor root;
	// requiring a scratch buffer for them would turn a semantic result into an
	// unrelated resource refusal.
	if scratch == nil || !scratch.Available() {
		return Settlement{}
	}
	prepared, preparedOK := transaction.Prepare(base, door.geometry, scratch, batch)
	if !preparedOK || !prepared.Available() {
		return Settlement{}
	}
	next, delta, committedOK := database.Commit(prepared)
	if !committedOK {
		return Settlement{}
	}
	if !delta.Available() {
		settlement, ok := publishingNoWrite(result, base)
		if !ok || !next.Same(base) {
			return Settlement{}
		}
		return settlement
	}
	settlement, ok := committed(result, base, next, delta)
	if !ok {
		return Settlement{}
	}
	return settlement
}
