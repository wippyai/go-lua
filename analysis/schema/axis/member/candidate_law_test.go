package member

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
)

const (
	candidateLawAxisKey  schema.Key = "axis/candidate-law-owner"
	candidateLawRelation schema.Key = "relation/candidate-law"
	candidateLawIssued   schema.Key = "program-relation/candidate-law-issued-row"
)

func candidateLawAxisRelation() RelationRef {
	return RelationRef{
		Axis:   schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: candidateLawAxisKey},
		Member: candidateLawRelation,
	}
}

// TestCandidateStatesExactlyOneAuthority holds the tagged choice to its
// closure. A declaration with no arm names no candidate at all, and one with
// both names two, which is the drift this choice exists to remove.
func TestCandidateStatesExactlyOneAuthority(t *testing.T) {
	none := CandidateRef{}
	if none.Declared() || none.Available() || none.Issued() {
		t.Fatalf("empty candidate accepted: %+v", none)
	}
	both := CandidateRef{AxisRelation: candidateLawAxisRelation(), IssuedRow: candidateLawIssued}
	if !both.Declared() || both.Available() {
		t.Fatalf("two-armed candidate accepted: %+v", both)
	}
	axisArm := AxisRelationCandidate(candidateLawAxisRelation())
	if !axisArm.Declared() || !axisArm.Available() || axisArm.Issued() {
		t.Fatalf("axis arm rejected: %+v", axisArm)
	}
	issuedArm := IssuedRowCandidate(candidateLawIssued)
	if !issuedArm.Declared() || !issuedArm.Available() || !issuedArm.Issued() {
		t.Fatalf("issued arm rejected: %+v", issuedArm)
	}
}

// TestCandidateResolvesThroughTheSurfaceItsArmNames proves each arm is held to
// the same upward resolution: whichever authority is stated is published as a
// reference the seal subsystem must find on that authority's own surface.
func TestCandidateResolvesThroughTheSurfaceItsArmNames(t *testing.T) {
	issued := IssuedRowCandidate(candidateLawIssued).References()
	if len(issued) != 1 || issued[0].Surface != schema.SurfaceKindIssuance || issued[0].Key != candidateLawIssued {
		t.Fatalf("issued candidate reference misrouted: %+v", issued)
	}
	axis := AxisRelationCandidate(candidateLawAxisRelation()).References()
	if len(axis) != 1 || axis[0].Surface != schema.SurfaceKindAxis || axis[0].Key != candidateLawAxisKey {
		t.Fatalf("axis candidate reference misrouted: %+v", axis)
	}
	if references := (CandidateRef{}).References(); len(references) != 0 {
		t.Fatalf("an unarmed candidate published a reference: %+v", references)
	}
}
