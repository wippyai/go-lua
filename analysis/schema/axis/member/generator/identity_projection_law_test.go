package generator

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
)

// identity_projection_law_test.go states what the emitted owner does with a
// projection declared in the Identity role: it answers it from its own
// surface, and it does NOT answer it from Project, because a digest reduced to
// a uint32 is not the identity the declaration published.

// identityProjectionDefinition adds two identity columns to the self-provided
// specimen: an unframed module identity, and a framed semantic axis of the
// same candidate row.
func identityProjectionDefinition() definition.Definition {
	source := selfProviderDefinition()
	owner := definition.GoType{PackagePath: "example/self", Name: "Schema"}
	candidate := definition.GoType{PackagePath: "example/self", Name: "Candidate"}
	axis := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "self"}
	provider := member.AxisRelationCandidate(member.RelationRef{Axis: axis, Member: "self/candidates"})
	method := func(name string, receiver definition.GoType) definition.GoSymbol {
		return definition.GoSymbol{PackagePath: owner.PackagePath, Name: name, Receiver: receiver, ResultIndex: 0}
	}
	identityType := func(name string) definition.GoType {
		return definition.GoType{PackagePath: "github.com/wippyai/go-lua/analysis/identity", Name: name}
	}
	source.Carriers = append(source.Carriers,
		definition.Carrier{Name: "Module", Key: "carrier/self/module", Type: identityType("ContentID")},
		definition.Carrier{Name: "Endpoint", Key: "carrier/self/endpoint", Type: identityType("SemanticKey")})
	source.Projections = append(source.Projections,
		definition.Projection{
			Name: "BodyModule", Key: "self/body-module", Relation: "Candidates", Role: member.Identity,
			Result: "Module", CandidateProvider: provider, Accessor: method("BodyModule", candidate),
		},
		definition.Projection{
			Name: "Endpoint", Key: "self/endpoint", Relation: "Candidates", Role: member.Identity,
			Result: "Endpoint", CandidateProvider: provider, Accessor: method("Endpoint", candidate),
		})
	return source
}

func renderIdentityOwner(t testing.TB, source definition.Definition) string {
	t.Helper()
	rendered, err := Render("self", source)
	if err != nil {
		t.Fatal(err)
	}
	return string(rendered.Relations)
}

// TestAnOwnerDeclaringIdentityRowsClaimsTheIdentitySurface is the visible half
// of the cut. The generated assertion is what makes the optional interface
// checkable at compile time in the owner's own package rather than by a type
// switch somewhere downstream.
func TestAnOwnerDeclaringIdentityRowsClaimsTheIdentitySurface(t *testing.T) {
	owner := renderIdentityOwner(t, identityProjectionDefinition())
	if !strings.Contains(owner, "var _ memberrelation.IdentityProjection = (*RelationOwner)(nil)") {
		t.Fatal("an owner with identity rows does not assert the identity surface")
	}
	if !strings.Contains(owner, "func (owner *RelationOwner) ProjectIdentity(relationOrdinal, projectionOrdinal, candidateOrdinal uint32) (identity.ContentID, uint64, bool) {") {
		t.Fatalf("the identity surface is not emitted:\n%s", owner)
	}
}

// TestAnOwnerWithoutIdentityRowsDoesNotGrowTheSurface is the reason the
// interface is optional at all. Every axis but the one that declares an
// identity column stays exactly as it was.
func TestAnOwnerWithoutIdentityRowsDoesNotGrowTheSurface(t *testing.T) {
	owner := renderIdentityOwner(t, selfProviderDefinition())
	if strings.Contains(owner, "IdentityProjection") || strings.Contains(owner, "ProjectIdentity") {
		t.Fatal("an owner of only local relations grew the identity surface")
	}
}

// TestTheFrameIsTheOwnersOwn states the one derivation the emitter performs.
// A content identity is issued under no frame and the emitted arm answers
// zero; a semantic key already carries the frame its owner minted it at, and
// the arm reads it off the value rather than inventing a constant.
func TestTheFrameIsTheOwnersOwn(t *testing.T) {
	owner := renderIdentityOwner(t, identityProjectionDefinition())
	surface := owner[strings.Index(owner, "func (owner *RelationOwner) ProjectIdentity"):]
	if !strings.Contains(surface, "return projected, 0, true") {
		t.Fatalf("an unframed content identity does not answer frame zero:\n%s", surface)
	}
	if !strings.Contains(surface, "return identity.ContentID(projected.Digest()), projected.Version(), true") {
		t.Fatalf("a semantic axis does not answer its own minted frame:\n%s", surface)
	}
}

// TestTheLocalProjectionRefusesAnIdentityColumn is the fence that makes the
// two surfaces one answer each. Project publishes locals; an identity row
// reached through it could only be a truncation, so the emitted local switch
// has no arm for one at all.
func TestTheLocalProjectionRefusesAnIdentityColumn(t *testing.T) {
	owner := renderIdentityOwner(t, identityProjectionDefinition())
	start := strings.Index(owner, "func (owner *RelationOwner) Project(")
	end := strings.Index(owner, "func (owner *RelationOwner) ProjectIdentity(")
	if start < 0 || end < 0 || end < start {
		t.Fatal("the emitted owner does not carry both projection surfaces")
	}
	local := owner[start:end]
	if strings.Contains(local, "BodyModule") || strings.Contains(local, "Endpoint") {
		t.Fatalf("an identity column was emitted into the local projection:\n%s", local)
	}
}
