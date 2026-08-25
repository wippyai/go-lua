package publish

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/apply"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/store"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/transaction"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
)

// Door is the solve-local publication capability for one mounted runtime.
//
// The door captures only already-sealed runtime authorities. It does not hold
// a relation catalog, classify a plan, or choose a physical operator. A
// semantic worker supplies a leased ProposalBatch and its proof sidecars;
// state/transaction performs the complete validation and the one atomic store
// replacement. Keeping this wrapper immutable makes the publication boundary
// explicit without introducing a second state or delta representation.
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
	if !mounted.Available() || !view.Available() {
		return Door{}, false
	}
	if !view.Fence().Same(mounted.RuntimeFence()) {
		return Door{}, false
	}
	return Door{mounted: mounted, geometry: view}, true
}

// Available reports whether all authorities captured by the door remain
// complete and bound to the same runtime fence.
func (door Door) Available() bool {
	return door.mounted.Available() && door.geometry.Available() && door.geometry.Fence().Same(door.mounted.RuntimeFence())
}

// Fence returns the exact solve-local runtime fence redeemed by this door.
func (door Door) Fence() binding.Fence {
	if !door.Available() {
		return binding.Fence{}
	}
	return door.mounted.RuntimeFence()
}

// Publish redeems one semantic application through the sole state transaction
// and root-commit door. The application is the authoritative outcome;
// sidecars and the optional widening permit are declared inputs to that same
// invocation, not a second result channel.
//
// A valid non-produced outcome returns a Settlement with the exact
// predecessor as both Base and Next and no Delta. In particular,
// NoCandidate, NoSelection, Opaque, and Refused never become Bottom, Unknown,
// ProvenAbsent, or any other fabricated cell. A Produced application with no
// proposals is the analogous exact-root no-op.
//
// The scratch buffer is caller-owned and may not be shared concurrently. It
// is borrowed only while the transaction prepares its private candidate. The
// application proposal lease remains live for the same duration; a stale or
// reset lease refuses.
func (door Door) Publish(
	base database.Version,
	scratch *store.ReadScratch,
	application apply.Application,
	sidecars []transaction.Submission,
	widening witness.WideningPermit,
) Settlement {
	request, requestOK := newRequest(application, sidecars, widening)
	if !requestOK || !door.Available() || !base.Available() || !base.Fence().Same(door.Fence()) {
		return Settlement{}
	}
	result := application.Outcome()
	if result.Code != outcome.Produced {
		if request.Widening().Available() {
			return Settlement{}
		}
		settlement, ok := noWrite(result, base)
		if !ok {
			return Settlement{}
		}
		return settlement
	}
	batch, batchOK := request.batch()
	if !batchOK {
		return Settlement{}
	}
	if batch.Proposals.Len() == 0 {
		if batch.Widening.Available() {
			return Settlement{}
		}
		settlement, ok := producedNoWrite(result, base)
		if !ok {
			return Settlement{}
		}
		return settlement
	}
	// State reads are needed only to stage a nonempty produced batch. Exact
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
		settlement, ok := producedNoWrite(result, base)
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
