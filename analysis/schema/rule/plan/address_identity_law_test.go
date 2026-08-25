package plan

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/rule/program"
)

// The addressing-identity laws. A corresponded read is resolved through the
// occurrence both directories are addressed by. That is the candidate's own
// occurrence whenever the candidate row IS the subject, and a directory whose
// rows all come from one occurrence family never needs to say otherwise.
//
// A directory may hold rows from several families, where one family's row is an
// interpretation of a subject sealed elsewhere - it NAMES a subject rather than
// being one. Asking the corresponded directory under such a row's own
// occurrence asks for a subject it never enumerated, and the plan would compile
// a read that resolves nothing. The declaration names the identity instead.

const (
	addressIdentityKey     schema.Key     = "plan/address-identity"
	addressIdentityCarrier member.Carrier = "carrier/plan/address-identity"
	siblingRelationKey     schema.Key     = "plan/other/sibling"
)

// correspondenceWithAddressIdentity is the corresponded fixture with an
// identity projection published on the CANDIDATE relation, and the join
// declaring it as the address of its foreign directory.
func correspondenceWithAddressIdentity(t *testing.T, role member.Role, relation schema.Key, declare bool) *planFixture {
	t.Helper()
	fixture := configureCorrespondenceFixture(t, true)
	occurrenceAxis := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: planOtherAxisKey}
	candidate := member.AxisRelationCandidate(member.RelationRef{Axis: occurrenceAxis, Member: heteroCandidateRelation})
	fixture.otherCatalog.Projections = append(fixture.otherCatalog.Projections, member.Projection{
		Key: addressIdentityKey, Relation: relation, Role: role,
		Result: addressIdentityCarrier, CandidateProvider: candidate,
	})
	if declare {
		fixture.declaration.Joins[0].AddressIdentity = member.ProjectionRef{Axis: occurrenceAxis, Member: addressIdentityKey}
	}
	return fixture
}

// TestADeclaredAddressIdentityCompilesOntoTheCorrespondedJoin is the carrying
// law. The identity is compiled beside the foreign directory it addresses, so
// the consumer resolving that directory has the occurrence to resolve it at
// without re-deriving one from the row's address.
func TestADeclaredAddressIdentityCompilesOntoTheCorrespondedJoin(t *testing.T) {
	fixture := correspondenceWithAddressIdentity(t, member.Identity, heteroCandidateRelation, true)
	compiled, failure := Compile(fixture.seal(t))
	if failure.Available() {
		t.Fatalf("a declared address identity did not compile: contributor=%d law=%d disposition=%s", failure.Contributor, failure.Law, failure.Disposition)
	}
	planned, plannedOK := compiled.At(0)
	if !plannedOK || planned.JoinCount() != 1 {
		t.Fatalf("compiled plan = ok %t joins %d", plannedOK, planned.JoinCount())
	}
	join, joinOK := planned.JoinAt(0)
	if !joinOK || !join.AddressIdentityPresent {
		t.Fatalf("compiled join carries no address identity: %+v/%t", join, joinOK)
	}
	if join.AddressIdentity.Axis != planned.Candidate().Axis {
		t.Fatalf("address identity axis = %d, want the candidate's own %d", join.AddressIdentity.Axis, planned.Candidate().Axis)
	}
	if !join.AddressingPresent || join.Addressing == planned.Candidate() {
		t.Fatal("the address identity was compiled onto a join that addresses no foreign directory")
	}
}

// TestAnUndeclaredAddressIdentityLeavesTheCandidateItsOwnAddress keeps the
// ordinary arm exact. Absence is a statement: the candidate row IS the subject,
// so the corresponded directory is asked about the candidate's own occurrence.
func TestAnUndeclaredAddressIdentityLeavesTheCandidateItsOwnAddress(t *testing.T) {
	fixture := configureCorrespondenceFixture(t, true)
	compiled, failure := Compile(fixture.seal(t))
	if failure.Available() {
		t.Fatalf("the corresponded fixture stopped compiling: contributor=%d law=%d disposition=%s", failure.Contributor, failure.Law, failure.Disposition)
	}
	planned, _ := compiled.At(0)
	join, joinOK := planned.JoinAt(0)
	if !joinOK || join.AddressIdentityPresent {
		t.Fatalf("a join declaring no address identity carries one: %+v/%t", join, joinOK)
	}
}

// TestAnAddressIdentityBesideABorrowedDirectoryIsRefused is the first refusal.
// A read borrowing the rule's own candidate directory is already resolved at
// the ordinal the rule holds, so there is no second directory to address. An
// identity there would name an address nothing asks for, which is a declaration
// disagreeing with its own geometry.
func TestAnAddressIdentityBesideABorrowedDirectoryIsRefused(t *testing.T) {
	fixture := newPlanFixture(t)
	mainAxis := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: planAxisKey}
	candidate := member.AxisRelationCandidate(member.RelationRef{Axis: mainAxis, Member: planCandidateRelation})
	fixture.catalog.Projections = append(fixture.catalog.Projections, member.Projection{
		Key: addressIdentityKey, Relation: planCandidateRelation, Role: member.Identity,
		Result: addressIdentityCarrier, CandidateProvider: candidate,
	})
	fixture.declaration.Joins[0].AddressIdentity = member.ProjectionRef{Axis: mainAxis, Member: addressIdentityKey}
	if _, failure := Compile(fixture.seal(t)); !failure.Available() {
		t.Fatal("an address identity was admitted beside a join that borrows the candidate's own directory")
	}
}

// TestAnAddressIdentityInAnotherRoleIsRefused is the second refusal. Only the
// Identity role names a subject the analyzer did not mint; a local projection
// answers an address this analyzer minted, and no dense width carries an
// occurrence. Admitting one would resolve the foreign directory at a number
// that means nothing in it.
func TestAnAddressIdentityInAnotherRoleIsRefused(t *testing.T) {
	for _, role := range []member.Role{member.Key, member.Destination, member.Predicate} {
		fixture := correspondenceWithAddressIdentity(t, role, heteroCandidateRelation, true)
		if _, failure := Compile(fixture.seal(t)); !failure.Available() {
			t.Fatalf("an address identity in the %v role was admitted", role)
		}
	}
}

// TestAnAddressIdentityOfAnotherRelationIsRefused is the third refusal. The
// occurrence is read off the CANDIDATE's row, because that is the row the
// invocation holds an ordinal for. A projection of a sibling relation on the
// same axis is a perfectly valid projection - the catalog holds it - and it is
// still refused here, because it would be read at an ordinal that does not
// index it.
func TestAnAddressIdentityOfAnotherRelationIsRefused(t *testing.T) {
	fixture := configureCorrespondenceFixture(t, true)
	occurrenceAxis := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: planOtherAxisKey}
	sibling := member.RelationRef{Axis: occurrenceAxis, Member: siblingRelationKey}
	fixture.otherCatalog.Relations = append(fixture.otherCatalog.Relations, member.Relation{
		Key: siblingRelationKey, Subject: heteroCandidateCarrier,
		CandidateProvider: member.AxisRelationCandidate(sibling),
	})
	fixture.otherCatalog.Projections = append(fixture.otherCatalog.Projections, member.Projection{
		Key: addressIdentityKey, Relation: siblingRelationKey, Role: member.Identity,
		Result: addressIdentityCarrier, CandidateProvider: member.AxisRelationCandidate(sibling),
	})
	fixture.declaration.Joins[0].AddressIdentity = member.ProjectionRef{Axis: occurrenceAxis, Member: addressIdentityKey}
	if _, failure := Compile(fixture.seal(t)); !failure.Available() {
		t.Fatal("an address identity projected from a relation that is not the candidate was admitted")
	}
}

// TestAnAddressIdentityIsCheckedBeforeItIsSealed states that the declaration's
// own Check is not what admits this term - the catalog is. A declaration naming
// a projection no axis publishes is refused at compile rather than carried.
func TestAnAddressIdentityIsCheckedBeforeItIsSealed(t *testing.T) {
	fixture := configureCorrespondenceFixture(t, true)
	occurrenceAxis := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: planOtherAxisKey}
	fixture.declaration.Joins[0].AddressIdentity = member.ProjectionRef{Axis: occurrenceAxis, Member: "plan/no-such-identity"}
	if problem, valid := fixture.declaration.Check(); !valid {
		t.Fatalf("the declaration refused a term the catalog is meant to settle: %+v", problem)
	}
	if _, failure := Compile(fixture.seal(t)); !failure.Available() {
		t.Fatal("an address identity naming a projection no axis publishes was admitted")
	}
}

var _ = program.JoinDecl{}
