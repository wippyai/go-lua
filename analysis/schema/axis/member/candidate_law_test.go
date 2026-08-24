package program

import (
	"encoding/hex"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
)

// axisCandidateDigestHex is the sealed content identity of the law program on
// the axis arm. It is the byte-identity fence of this cut: the tagged choice
// writes the issued arm as its own record and leaves the axis arm's stream
// untouched, so this value must survive the change that introduced it.
const axisCandidateDigestHex = "989eb3cf0de275173972bb5da2f496a09c8ab3d5201de91327c957b6d1738af5"

const issuedCandidateRelation schema.Key = "program-relation/occurrence-law-row"

// TestCandidateStatesExactlyOneAuthority holds the tagged choice to its
// closure. A declaration with no arm names no candidate at all, and a
// declaration with both names two, which is the drift this choice exists to
// remove.
func TestCandidateStatesExactlyOneAuthority(t *testing.T) {
	none := CandidateDecl{}
	if none.Declared() || none.Available() || none.Issued() {
		t.Fatalf("empty candidate accepted: %+v", none)
	}
	both := CandidateDecl{AxisRelation: lawRelation("candidate"), IssuedRow: issuedCandidateRelation}
	if !both.Declared() || both.Available() {
		t.Fatalf("two-armed candidate accepted: %+v", both)
	}
	axisArm := AxisRelationCandidate(lawRelation("candidate"))
	if !axisArm.Declared() || !axisArm.Available() || axisArm.Issued() {
		t.Fatalf("axis arm rejected: %+v", axisArm)
	}
	issuedArm := IssuedRowCandidate(issuedCandidateRelation)
	if !issuedArm.Declared() || !issuedArm.Available() || !issuedArm.Issued() {
		t.Fatalf("issued arm rejected: %+v", issuedArm)
	}
}

// TestProgramRefusesUnarmedCandidate proves the choice is enforced through the
// Program's own seal rather than only by the accessor above.
func TestProgramRefusesUnarmedCandidate(t *testing.T) {
	for name, candidate := range map[string]CandidateDecl{
		"none": {},
		"both": {AxisRelation: lawRelation("candidate"), IssuedRow: issuedCandidateRelation},
	} {
		program := lawProgram(t)
		program.Candidate = candidate
		problem, valid := program.Check()
		if valid || problem.Kind != ProblemCandidate {
			t.Fatalf("%s candidate admitted: valid=%t problem=%+v", name, valid, problem)
		}
	}
}

// TestIssuedCandidateResolvesThroughIssuanceSurface proves the issued arm is
// held to the same upward resolution as the axis arm: the relation it names is
// published as a reference the seal subsystem must find on the issuance
// surface, so an unpublished relation cannot reach a compiled plan.
func TestIssuedCandidateResolvesThroughIssuanceSurface(t *testing.T) {
	program := lawProgram(t)
	program.Candidate = IssuedRowCandidate(issuedCandidateRelation)
	references := program.References()
	if len(references) == 0 {
		t.Fatal("issued candidate published no reference")
	}
	reference := references[0]
	if reference.Surface != schema.SurfaceKindIssuance || reference.Key != issuedCandidateRelation {
		t.Fatalf("issued candidate reference misrouted: %+v", reference)
	}
	axis := lawProgram(t)
	axisReferences := axis.References()
	if len(axisReferences) == 0 || axisReferences[0].Surface != schema.SurfaceKindAxis {
		t.Fatalf("axis candidate reference misrouted: %+v", axisReferences)
	}
}

// TestStatingTheCandidateChoiceRemintsNoAxisProgram is the content-identity
// law of this cut. The axis arm emits exactly the stream it emitted before the
// choice existed, so every program that keeps the arm it already had keeps its
// digest; only a program that takes the new arm mints a new one.
func TestStatingTheCandidateChoiceRemintsNoAxisProgram(t *testing.T) {
	axis := lawProgram(t)
	digest := axis.Digest()
	if !digest.Available() {
		t.Fatal("axis program published no digest")
	}
	if got := hex.EncodeToString(digest[:]); got != axisCandidateDigestHex {
		t.Fatalf("axis candidate stream reminted: %s", got)
	}
	issued := lawProgram(t)
	issued.Candidate = IssuedRowCandidate(issuedCandidateRelation)
	issuedDigest := issued.Digest()
	if !issuedDigest.Available() {
		t.Fatal("issued program published no digest")
	}
	if issuedDigest == digest {
		t.Fatal("the two candidate arms mint one digest")
	}
}
