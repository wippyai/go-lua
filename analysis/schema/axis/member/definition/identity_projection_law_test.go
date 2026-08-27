package definition

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/carrier"
)

// identity_projection_law_test.go holds the Identity role to the two carriers
// the owner surface can answer. ProjectIdentity answers a digest and the frame
// it was issued under; the analyzer mints exactly two things shaped that way,
// and a projection declared in this role that publishes anything else has
// declared a column its own owner could not return.

func identityLawDefinition(role member.Role, result string) Definition {
	owner := GoType{PackagePath: "example/ident", Name: "Schema"}
	candidate := GoType{PackagePath: "example/ident", Name: "Candidate"}
	key := GoType{PackagePath: "example/ident", Name: "Key"}
	fact := GoType{PackagePath: "example/ident", Name: "Fact"}
	axis := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "ident"}
	provider := member.AxisRelationCandidate(member.RelationRef{Axis: axis, Member: "ident/candidates"})
	method := func(name string, receiver GoType) GoSymbol {
		return GoSymbol{PackagePath: owner.PackagePath, Name: name, Receiver: receiver, ResultIndex: 0}
	}
	return Definition{
		Name: "Ident", Axis: "ident",
		Binding:   Binding{Key: KeyNormalization{Carrier: "Key", Dense: GoType{Name: "uint32"}, Normalizer: method("KeyIndex", owner)}},
		Signature: Signature{Key: "Key", Fact: "Fact"},
		Carriers: []Carrier{
			{Name: "Candidate", Key: "carrier/ident/candidate", Type: candidate, Capability: carrier.Equatable},
			{Name: "Key", Key: "carrier/ident/key", Type: key, Capability: carrier.Equatable},
			{Name: "Fact", Key: "carrier/ident/fact", Type: fact, Capability: carrier.Equatable},
			{Name: "Module", Key: "carrier/ident/module", Type: GoType{PackagePath: identityPackagePath, Name: "ContentID"}, Capability: carrier.Equatable},
			{Name: "Endpoint", Key: "carrier/ident/endpoint", Type: GoType{PackagePath: identityPackagePath, Name: "SemanticKey"}, Capability: carrier.Equatable},
			{Name: "Ordinal", Key: "carrier/ident/ordinal", Type: GoType{Name: "uint32"}, Capability: carrier.Equatable},
		},
		Relations: []Relation{{
			Name: "Candidates", Key: "ident/candidates", Subject: "Candidate", CandidateProvider: provider,
			CandidateResolver: method("CandidateForOccurrence", owner), CandidateOrdinal: method("CandidateOrdinal", owner), CandidateAt: method("CandidateAt", owner),
		}},
		Projections: []Projection{{
			Name: "BodyModule", Key: "ident/body-module", Relation: "Candidates", Role: role, Result: result,
			CandidateProvider: provider, Accessor: method("BodyModule", candidate),
		}},
	}
}

// TestAnIdentityProjectionPublishesAnOwnerIssuedIdentity admits both shapes the
// surface answers: an unframed content identity, and a semantic key, which is
// the same digest plus the frame its owner minted it at.
func TestAnIdentityProjectionPublishesAnOwnerIssuedIdentity(t *testing.T) {
	for _, result := range []string{"Module", "Endpoint"} {
		t.Run(result, func(t *testing.T) {
			source := identityLawDefinition(member.Identity, result)
			if !source.Complete() {
				t.Fatal("an identity projection over an owner-issued identity carrier was refused")
			}
			catalog, sealed := source.Catalog()
			if !sealed {
				t.Fatal("the catalog refused a sealed identity projection")
			}
			projection, found := catalog.Projection("ident/body-module")
			if !found || projection.Role != member.Identity {
				t.Fatalf("sealed role = %d/%t", projection.Role, found)
			}
		})
	}
}

// TestAnIdentityProjectionOverALocalCarrierIsRefused is the fence. A local is
// an address of a row this analyzer minted and the owner returns it from
// Project; declaring it in the Identity role names a column ProjectIdentity
// would have to invent a digest for.
func TestAnIdentityProjectionOverALocalCarrierIsRefused(t *testing.T) {
	for _, result := range []string{"Ordinal", "Key", "Fact"} {
		t.Run(result, func(t *testing.T) {
			if identityLawDefinition(member.Identity, result).Complete() {
				t.Fatal("an identity projection over a local carrier was admitted")
			}
		})
	}
}

// TestALocalRoleOverAnIdentityCarrierIsRefused is the converse, and it is what
// keeps Attribute's own statement true. Attribute's projected value IS a
// declared vocabulary ordinal; a 32-byte identity spelled in that role would
// be read as one through Project and truncated to a coordinate of a directory
// it was never an index into.
func TestALocalRoleOverAnIdentityCarrierIsRefused(t *testing.T) {
	for _, role := range []member.Role{member.Key, member.Predicate, member.Destination, member.Attribute} {
		source := identityLawDefinition(role, "Module")
		if source.Complete() {
			t.Fatalf("role %d published an owner-issued identity as a local", role)
		}
	}
}
