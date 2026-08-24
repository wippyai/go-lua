package member

import "github.com/wippyai/go-lua/analysis/schema"

// Correspondence is one relation's statement that its own sealed candidate
// order and a foreign axis's sealed candidate order enumerate the same
// subjects.
//
// A rule addresses one candidate directory. Every relation it joins is
// indexed by the dense ordinal that directory issued, which is sound only
// while every joined relation is provided by that same authority. Two axes
// that describe one subject enumerate independently, so without a stated
// correspondence a join across their directories is either inexpressible or
// resolved against the wrong order.
//
// The statement is owned by the axis whose rows it is about: the declaring
// relation's rows correspond to Foreign's candidates, not the other way
// round, so an axis that needs a correlation declares it and the axis it
// names publishes nothing new and imports nothing.
//
// Coordinate is the key the two orders agree on: a Key projection over the
// declaring relation whose result carrier is the foreign axis's key. Without
// it a correspondence is a claim that two enumerations happen to line up,
// which nothing can check; with it the claim is that this relation's rows are
// addressed in the foreign axis's own key space, which the seal proves.
type Correspondence struct {
	Foreign    RelationRef
	Coordinate schema.Key
}

// Available reports whether both halves of the statement are present. A
// correspondence naming a relation but no key states a correlation nothing
// can resolve; a key naming no relation correlates with nothing.
func (correspondence Correspondence) Available() bool {
	return correspondence.Foreign.Available() && correspondence.Coordinate.Available()
}

// Declared reports whether either half is stated. It separates an omitted
// correspondence from a half-written one, which Available then refuses.
func (correspondence Correspondence) Declared() bool {
	return correspondence.Foreign.Declared() || correspondence.Coordinate.Available()
}

// References returns the upward reference this statement carries: the axis
// whose candidate order it corresponds to. The same seal machinery that
// proves a candidate provider's axis exists proves a correspondent's does.
func (correspondence Correspondence) References() schema.EntryReferences {
	if !correspondence.Foreign.Declared() {
		return nil
	}
	return schema.EntryReferences{correspondence.Foreign.EntryReference()}
}

// correspondencesComplete states the catalog-local laws of one relation's
// correspondences. Each is already known complete - Relation.Available is the
// row-local law that half a statement is not a declaration - and
// keyProjections maps every Key projection's key to the relation whose rows it
// projects.
//
// The cross-catalog law - that the coordinate's result carrier is the foreign
// axis's own key carrier - belongs to the altitude that can see every axis,
// and is stated there rather than guessed at from one catalog.
func correspondencesComplete(relation Relation, keyProjections map[schema.Key]schema.Key) bool {
	if len(relation.Correspondences) == 0 {
		return true
	}
	// A correspondence pairs two sealed orders, so the declaring relation must
	// own one. A relation addressed through another authority's candidate has
	// no enumeration of its own to pair, and admitting it would let an axis
	// claim a correlation between two directories it owns neither side of.
	if relation.CandidateProvider.Issued() || relation.CandidateProvider.AxisRelation.Member != relation.Key {
		return false
	}
	ownAxis := relation.CandidateProvider.AxisRelation.Axis.Key
	axes := make(map[schema.Key]struct{}, len(relation.Correspondences))
	for _, correspondence := range relation.Correspondences {
		// A same-axis correspondence is the identity the candidate provider
		// already spells, and two statements about one foreign order are two
		// answers to one question with no declared way to choose.
		foreign := correspondence.Foreign.Axis.Key
		if foreign == ownAxis {
			return false
		}
		if _, duplicate := axes[foreign]; duplicate {
			return false
		}
		axes[foreign] = struct{}{}
		owner, held := keyProjections[correspondence.Coordinate]
		if !held || owner != relation.Key {
			return false
		}
	}
	return true
}

// cloneCorrespondences returns an independent list, preserving nil.
func cloneCorrespondences(correspondences []Correspondence) []Correspondence {
	if correspondences == nil {
		return nil
	}
	return append([]Correspondence(nil), correspondences...)
}
