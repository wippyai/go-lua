package definition

import "github.com/wippyai/go-lua/analysis/schema/axis/member"

// correspondencesAgree reports whether two declarations of one relation name
// the same foreign orders. Relation carries them as a slice, so identity is
// stated here rather than left to comparison.
func correspondencesAgree(left, right []member.RelationRef) bool {
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

func cloneCorrespondences(correspondences []member.RelationRef) []member.RelationRef {
	if correspondences == nil {
		return nil
	}
	return append([]member.RelationRef(nil), correspondences...)
}
