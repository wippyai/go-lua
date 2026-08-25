package member

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
)

// identity_projection_law_test.go states the Identity role's place in the
// projection vocabulary.
//
// Every other role publishes a LOCAL: a dense coordinate of some directory, or
// a declared vocabulary ordinal. A local is an address of a row this analyzer
// minted, and a uint32 carries one. An identity is not an address at all - it
// names a subject the analyzer did not mint, a module or a body path or the
// semantic axis a role is issued under - and no dense width carries one. The
// two are separate roles because they are read through separate owner
// surfaces, and collapsing them would make "the projected local" mean two
// things.

func identityLawProvider() CandidateRef {
	return AxisRelationCandidate(RelationRef{
		Axis:   schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "identity-law"},
		Member: "identity-law/candidates",
	})
}

func identityLawCatalog(role Role) Catalog {
	provider := identityLawProvider()
	return Catalog{
		Relations: []Relation{{
			Key: "identity-law/candidates", Subject: "carrier/identity-law/candidate", CandidateProvider: provider,
		}},
		Projections: []Projection{{
			Key: "identity-law/body-module", Relation: "identity-law/candidates", Role: role,
			Result: "carrier/identity-law/module", CandidateProvider: provider,
		}},
	}
}

// TestTheIdentityRoleIsDeclaredBesideAttribute holds the vocabulary open at
// exactly one new ordinal. Attribute stays the last LOCAL role and Identity is
// the one role read as an identity, so a reader that switches on the role
// cannot silently treat one as the other.
func TestTheIdentityRoleIsDeclaredBesideAttribute(t *testing.T) {
	if !Identity.Available() {
		t.Fatal("the Identity role is not a declared projection role")
	}
	if Identity == Attribute {
		t.Fatal("Identity and Attribute are one ordinal; a local and an identity would be one role")
	}
	if Attribute+1 != Identity {
		t.Fatalf("Identity = %d, want the ordinal directly after Attribute (%d)", Identity, Attribute)
	}
	if Role(Identity + 1).Available() {
		t.Fatal("the role vocabulary admits an ordinal past Identity")
	}
}

// TestACatalogAdmitsAnIdentityProjection states that the role reaches a sealed
// catalog on the same terms as every other: it is a projection of a declared
// relation, keyed once, and nothing about being an identity exempts it from
// the closure the catalog already holds every projection to.
func TestACatalogAdmitsAnIdentityProjection(t *testing.T) {
	catalog := identityLawCatalog(Identity)
	if !catalog.Available() {
		t.Fatal("a catalog refused a projection in the Identity role")
	}
	projection, found := catalog.Projection("identity-law/body-module")
	if !found || projection.Role != Identity {
		t.Fatalf("sealed projection role = %d/%t", projection.Role, found)
	}
}

// TestAnIdentityProjectionOfNoDeclaredRelationIsRefused keeps the new role
// inside the catalog's own closure. The role says how a column is read, never
// that the column may name a relation the catalog does not hold.
func TestAnIdentityProjectionOfNoDeclaredRelationIsRefused(t *testing.T) {
	catalog := identityLawCatalog(Identity)
	catalog.Projections[0].Relation = "identity-law/absent"
	if catalog.Available() {
		t.Fatal("an identity projection of an undeclared relation was admitted")
	}
}
