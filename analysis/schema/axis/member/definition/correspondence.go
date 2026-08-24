package definition

import "github.com/wippyai/go-lua/analysis/schema/axis/member"

// RelationCorrespondence is the owner-source spelling of one relation's
// statement that its sealed candidate order and a foreign axis's sealed
// candidate order enumerate the same subjects.
//
// It carries no Go symbol. A correspondence is not a fact an owner computes:
// it is a claim about two orders that already exist, and the seal proves it
// from what both owners already publish. An owner that had to answer a
// correspondence would be a third authority over a correlation the two
// directories already determine.
//
// Coordinate names a projection of the DECLARING contribution by its source
// name, the way every other definition row names its members; the composed
// catalog carries that projection's key.
type RelationCorrespondence struct {
	Foreign    member.RelationRef
	Coordinate string
}

// Available reports whether both halves of the statement are present.
func (correspondence RelationCorrespondence) Available() bool {
	return correspondence.Foreign.Available() && correspondence.Coordinate != ""
}

// Declared reports whether either half is stated, separating an omitted
// correspondence from a half-written one.
func (correspondence RelationCorrespondence) Declared() bool {
	return correspondence.Foreign.Declared() || correspondence.Coordinate != ""
}

// correspondencesAgree reports whether two declarations of one relation state
// the same correspondence list. Relation carries this as a slice, so identity
// is stated here rather than left to comparison.
func correspondencesAgree(left, right []RelationCorrespondence) bool {
	if len(left) != len(right) {
		return false
	}
	for index, correspondence := range left {
		if correspondence != right[index] {
			return false
		}
	}
	return true
}

func cloneCorrespondences(correspondences []RelationCorrespondence) []RelationCorrespondence {
	if correspondences == nil {
		return nil
	}
	return append([]RelationCorrespondence(nil), correspondences...)
}
