package equation

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/identity"
)

// Relation is one atomically published accepted-activation set of a sealed
// Topology. It binds three inseparable parts: the accepted rows, the
// structural digest derived exactly once at that publication, and the
// topology-scoped Generation stamp that orders publications of this Topology.
//
// The split is deliberate. A stamp is compared, never recomputed: every stale
// fence in the engine asks whether a retained address still names the live
// publication, and that question is answered by comparing Generations. The
// digest is content identity, derived at the one publication point and stored;
// no reader may re-derive it, which is why the deriving routine is private and
// no constructor accepts a caller-supplied digest.
type Relation struct {
	owner      *Topology
	generation identity.Generation
	digest     composition.Key
	rows       []AcceptedMember
}

// Available reports whether relation names a published activation set.
// Publish and sealInitialRelation are the only constructors, and both prove
// the generation and digest available before ever setting owner, so a set
// owner already is the complete verdict.
func (relation Relation) Available() bool {
	return relation.owner != nil
}

// Generation returns this publication's monotone stamp within its Topology.
func (relation Relation) Generation() identity.Generation {
	if !relation.Available() {
		return 0
	}
	return relation.generation
}

// Digest returns the structural identity derived once when this Relation was
// published. Equal digests name structurally equal accepted sets of the same
// Topology; the stamp orders them.
func (relation Relation) Digest() composition.Key {
	if !relation.Available() {
		return composition.Key{}
	}
	return relation.digest
}

// Rows returns the retained accepted members in canonical order. The rows are
// immutable values published with this Relation; callers read them and never
// write through the returned slice.
func (relation Relation) Rows() []AcceptedMember {
	if !relation.Available() {
		return nil
	}
	return relation.rows
}

// OwnedBy proves this Relation was published by exactly this sealed Topology.
// It is the whole stale fence for a Relation: no digest is re-derived, and no
// equal-content Topology can lend its publications to another.
func (relation Relation) OwnedBy(topology *Topology) bool {
	return relation.Available() && topology != nil && relation.owner == topology
}

// Precedes reports whether relation is an earlier publication of the same
// Topology than other. Publications of different topologies are unordered.
func (relation Relation) Precedes(other Relation) bool {
	return relation.Available() && other.Available() && relation.owner == other.owner &&
		relation.generation.Precedes(other.generation)
}

// InitialRelation returns the sealed Topology's first publication: the empty
// accepted set at the first Generation, whose digest was derived once at seal.
func (topology *Topology) InitialRelation() (Relation, bool) {
	if topology == nil || !topology.initialRelation.OwnedBy(topology) {
		return Relation{}, false
	}
	return topology.initialRelation, true
}

// Publish issues the next publication of this Topology. It derives the
// structural digest exactly once, here, and stamps the result with the
// Generation following previous. A saturated Generation fails closed rather
// than reusing a live stamp, and a previous publication of another Topology,
// or rows this Topology does not own, are rejected before anything is derived.
func (topology *Topology) Publish(previous Relation, rows []AcceptedMember) (Relation, bool) {
	if topology == nil || !previous.OwnedBy(topology) || !topology.ValidAccepted(rows) {
		return Relation{}, false
	}
	generation := previous.generation.Next()
	if !generation.Available() {
		return Relation{}, false
	}
	digest, derived := topology.deriveRelationDigest(rows)
	if !derived {
		return Relation{}, false
	}
	return Relation{owner: topology, generation: generation, digest: digest, rows: append([]AcceptedMember(nil), rows...)}, true
}

// sealInitialRelation mints the first publication while the Topology is being
// sealed. It is the only constructor that does not require a predecessor.
func (topology *Topology) sealInitialRelation() bool {
	if topology == nil || topology.initialRelation.owner != nil {
		return false
	}
	digest, derived := topology.deriveRelationDigest(nil)
	if !derived {
		return false
	}
	first := identity.Generation(0).Next()
	if !first.Available() {
		return false
	}
	topology.initialRelation = Relation{owner: topology, generation: first, digest: digest}
	return true
}
