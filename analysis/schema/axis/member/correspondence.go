package member

import "github.com/wippyai/go-lua/analysis/schema"

// correspondencesComplete states the catalog-local laws of one relation's
// correspondences.
//
// A correspondence names a foreign axis's relation whose candidate order
// enumerates the same subjects this relation's own order does: for every
// occurrence both directories are addressed from, each resolves one candidate,
// and those two candidates name one subject. That is why the statement carries
// nothing beside the foreign relation - the two orders are already addressed
// by the same occurrence, so the correspondence is a projection of two sealed
// orders rather than a fact either owner publishes.
//
// It deliberately carries no key projection. A projection's local lives in the
// key space of the axis that issued it, so no relation can publish a foreign
// coordinate as one, and a key that could be stated here would be a coordinate
// of this axis saying nothing about the foreign order.
//
// The law that both directories are addressed the same way needs both
// catalogs, so it is stated at the altitude that can see every axis.
func correspondencesComplete(relation Relation) bool {
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
		if !correspondence.Available() {
			return false
		}
		// A same-axis correspondence is the identity the candidate provider
		// already spells, and two statements about one foreign order are two
		// answers to one question with no declared way to choose.
		foreign := correspondence.Axis.Key
		if foreign == ownAxis {
			return false
		}
		if _, duplicate := axes[foreign]; duplicate {
			return false
		}
		axes[foreign] = struct{}{}
	}
	return true
}

// cloneCorrespondences returns an independent list, preserving nil.
func cloneCorrespondences(correspondences []RelationRef) []RelationRef {
	if correspondences == nil {
		return nil
	}
	return append([]RelationRef(nil), correspondences...)
}
