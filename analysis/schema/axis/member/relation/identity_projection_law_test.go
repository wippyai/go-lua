package relation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

// identity_projection_law_test.go states that an owner-issued identity crosses
// this boundary through one optional surface, on the same law as
// OccurrenceDirectory: an owner declares the surface exactly when it declares
// a relation that needs it, and the Owner every axis implements does not grow
// a method six of seven axes have no answer for.

// localOwner is an owner of ordinary keyed relations. It publishes locals and
// nothing else, which is what almost every axis is.
type localOwner struct{}

func (localOwner) CandidateCount(uint32, identity.ContentID, identity.ContentID) (int, bool) {
	return 1, true
}

func (localOwner) CandidateAt(uint32, identity.ContentID, identity.ContentID, int) (uint32, bool) {
	return 0, true
}

func (localOwner) MemberCount(uint32, uint32) (int, bool)         { return 0, false }
func (localOwner) MemberAt(uint32, uint32, int) (uint32, bool)    { return 0, false }
func (localOwner) KeyVectorCount(uint32, uint32) (int, bool)      { return 0, false }
func (localOwner) KeyVectorAt(uint32, uint32, int) (uint32, bool) { return 0, false }
func (localOwner) Project(uint32, uint32, uint32) (uint32, bool) {
	return 0, true
}

// identityOwner additionally declares one relation whose rows carry
// owner-issued identities: an unframed content identity and a framed semantic
// axis of it.
type identityOwner struct {
	localOwner
	digest identity.ContentID
}

func (owner identityOwner) ProjectIdentity(_, projectionOrdinal, _ uint32) (identity.ContentID, uint64, bool) {
	switch projectionOrdinal {
	case 0:
		return owner.digest, 0, true
	case 1:
		return owner.digest, 7, true
	default:
		return identity.ContentID{}, 0, false
	}
}

func identityLawDigest() identity.ContentID {
	var digest identity.ContentID
	for index := range digest {
		digest[index] = byte(index + 1)
	}
	return digest
}

// TestTheIdentitySurfaceIsOptional is the whole reason it is a separate
// interface. An axis that publishes only locals is a complete Owner; requiring
// it to answer an identity question it has no relation for would make every
// generated owner carry a refusing method for a capability it never declared.
func TestTheIdentitySurfaceIsOptional(t *testing.T) {
	var owner Owner = localOwner{}
	if _, projects := owner.(IdentityProjection); projects {
		t.Fatal("an owner of only local relations claims the identity surface")
	}
	var declared Owner = identityOwner{digest: identityLawDigest()}
	if _, projects := declared.(IdentityProjection); !projects {
		t.Fatal("an owner declaring identity rows does not reach the identity surface")
	}
}

// TestOneCallAnswersTheDigestAndItsFrame states why the surface is not two
// calls. A digest without the frame it was issued under is not an identity -
// the analyzer's own SemanticKey refuses version zero - so a second call for
// the frame would be a second authority over one value, free to disagree with
// the first. A content identity is issued under no frame and answers zero.
func TestOneCallAnswersTheDigestAndItsFrame(t *testing.T) {
	owner := identityOwner{digest: identityLawDigest()}
	content, frame, ok := owner.ProjectIdentity(0, 0, 0)
	if !ok || content != owner.digest || frame != 0 {
		t.Fatalf("content identity = %v/%d/%t, want the digest under no frame", content.Available(), frame, ok)
	}
	semantic, version, semanticOK := owner.ProjectIdentity(0, 1, 0)
	if !semanticOK || semantic != owner.digest || version == 0 {
		t.Fatalf("semantic axis = %v/%d/%t, want the digest under its own frame", semantic.Available(), version, semanticOK)
	}
	key, keyOK := identity.NewSemanticKey([32]byte(semantic), version)
	if !keyOK || !key.Available() {
		t.Fatal("a framed identity does not reconstitute the semantic key its owner minted")
	}
}

// TestAnUndeclaredProjectionAnswersNoIdentity keeps the surface total: a pair
// the owner declares no identity row for refuses rather than answering a zero
// digest, because an absent identity and the identity of nothing are not the
// same statement.
func TestAnUndeclaredProjectionAnswersNoIdentity(t *testing.T) {
	owner := identityOwner{digest: identityLawDigest()}
	if content, frame, ok := owner.ProjectIdentity(0, 9, 0); ok || content.Available() || frame != 0 {
		t.Fatalf("an undeclared identity projection answered %v/%d/%t", content.Available(), frame, ok)
	}
}
