package publication

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/domain/effect/callsite"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

func sealedMembershipAttachment() DirectAllocationMembershipAttachment {
	attachment := DirectAllocationMembershipAttachment{
		mount: publicationLawID("attachment/mount"),
		point: publicationLawID("attachment/point"),
		call:  publicationLawID("attachment/call"),
		width: 7,
	}
	attachment.id, _ = directAllocationMembershipAttachmentID(attachment.mount, attachment.point, attachment.call, attachment.width)
	return attachment
}

func sealedMembershipProof(membership valuedomain.AllocationMembership) DirectAllocationMembershipProof {
	proof := DirectAllocationMembershipProof{
		attachment:  publicationLawID("proof/attachment"),
		correlation: publicationLawID("proof/correlation"),
		direct:      publicationLawID("proof/direct"),
		membership:  membership,
	}
	proof.id, _ = directAllocationMembershipProofID(proof.attachment, proof.correlation, proof.direct, proof.membership)
	return proof
}

// The attachment's identity is what authorizes its retained Engine handle, so
// a scalar rewrite must leave the attachment unusable even before the handle
// is consulted.
func TestDirectAllocationMembershipAttachmentScalarSealLaw(t *testing.T) {
	attachment := sealedMembershipAttachment()
	if !attachment.id.Available() {
		t.Fatal("sealed attachment identity")
	}
	foreign := publicationLawID("attachment/foreign")
	mutations := map[string]func(DirectAllocationMembershipAttachment) DirectAllocationMembershipAttachment{
		"id": func(value DirectAllocationMembershipAttachment) DirectAllocationMembershipAttachment {
			value.id = foreign
			return value
		},
		"mount": func(value DirectAllocationMembershipAttachment) DirectAllocationMembershipAttachment {
			value.mount = foreign
			return value
		},
		"point": func(value DirectAllocationMembershipAttachment) DirectAllocationMembershipAttachment {
			value.point = foreign
			return value
		},
		"call": func(value DirectAllocationMembershipAttachment) DirectAllocationMembershipAttachment {
			value.call = foreign
			return value
		},
		"width": func(value DirectAllocationMembershipAttachment) DirectAllocationMembershipAttachment {
			value.width++
			return value
		},
	}
	for name, mutate := range mutations {
		if want, ok := directAllocationMembershipAttachmentID(attachment.mount, attachment.point, attachment.call, attachment.width); !ok || want != attachment.id {
			t.Fatal("attachment identity derivation")
		}
		if mutate(attachment).Valid() {
			t.Fatalf("attachment scalar mutation survived field=%s", name)
		}
	}
}

// An attachment is only ever issued together with the Engine observation its
// identity authorized. A retained handle that does not authenticate that
// identity therefore leaves the attachment invalid, which is what keeps a
// handle from one selected member out of another member's attachment.
func TestDirectAllocationMembershipAttachmentObservationLaw(t *testing.T) {
	attachment := sealedMembershipAttachment()
	if attachment.Valid() {
		t.Fatal("attachment sealed without an authenticated observation handle")
	}
	if _, ok := attachment.ContentID(); ok {
		t.Fatal("attachment published an identity without an authenticated observation handle")
	}
	if attachment.observation.MatchesID(attachment.id) {
		t.Fatal("absent observation handle authenticated an attachment identity")
	}
	if _, ok := AttachSelectedDirectAllocationMembership(nil, nil, engine.RuleSlotCapability{}, attachment.mount, attachment.point, attachment.call, attachment.width); ok {
		t.Fatal("attachment issued without a compilation, query, and graph")
	}
	// Prove is the relation's one cross-owner admission of a direct receipt, so
	// zero transition, correlation, binding, and direct evidence must fabricate
	// nothing even before an authenticated handle exists.
	if _, proven := attachment.Prove(nil, nil, callsite.PublicationTransitionProof{}, callsite.PublicationPlacementCorrelationCandidate{}, packdomain.RuntimeAllocationContextBinding{}, DirectAllocationSubject{}); proven {
		t.Fatal("missing transition, correlation, runtime, and direct evidence proved a membership")
	}
}

// Width is part of the attachment identity because the proof reads exactly
// that many summary cells. A zero width names no vector, and an absent
// coordinate names no member.
func TestDirectAllocationMembershipAttachmentCoordinateLaw(t *testing.T) {
	mount, point, call := publicationLawID("coordinate/mount"), publicationLawID("coordinate/point"), publicationLawID("coordinate/call")
	if _, ok := directAllocationMembershipAttachmentID(mount, point, call, 0); ok {
		t.Fatal("zero width derived an attachment identity")
	}
	for index, absent := range [][3]identity.ContentID{
		{identity.ContentID{}, point, call},
		{mount, identity.ContentID{}, call},
		{mount, point, identity.ContentID{}},
	} {
		if _, ok := directAllocationMembershipAttachmentID(absent[0], absent[1], absent[2], 1); ok {
			t.Fatalf("absent coordinate derived an attachment identity index=%d", index)
		}
	}
	first, firstOK := directAllocationMembershipAttachmentID(mount, point, call, 1)
	second, secondOK := directAllocationMembershipAttachmentID(mount, point, call, 2)
	if !firstOK || !secondOK || first == second {
		t.Fatal("width left the attachment identity")
	}
}

// The proof is the narrowest evidence in this relation: one attachment, one
// correlation, one direct receipt, one exact membership. Each is sealed, and
// an inexact membership is not evidence at all.
func TestDirectAllocationMembershipProofSealLaw(t *testing.T) {
	proof := sealedMembershipProof(valuedomain.MembershipRecent)
	id, idOK := proof.ContentID()
	if !proof.Valid() || !idOK || !id.Available() || proof.Membership() != valuedomain.MembershipRecent {
		t.Fatal("sealed membership proof invalid")
	}
	foreign := publicationLawID("proof/foreign")
	mutations := map[string]func(DirectAllocationMembershipProof) DirectAllocationMembershipProof{
		"id": func(value DirectAllocationMembershipProof) DirectAllocationMembershipProof {
			value.id = foreign
			return value
		},
		"attachment": func(value DirectAllocationMembershipProof) DirectAllocationMembershipProof {
			value.attachment = foreign
			return value
		},
		"correlation": func(value DirectAllocationMembershipProof) DirectAllocationMembershipProof {
			value.correlation = foreign
			return value
		},
		"direct": func(value DirectAllocationMembershipProof) DirectAllocationMembershipProof {
			value.direct = foreign
			return value
		},
		"membership": func(value DirectAllocationMembershipProof) DirectAllocationMembershipProof {
			value.membership = valuedomain.MembershipSummary
			return value
		},
	}
	for name, mutate := range mutations {
		mutated := mutate(proof)
		if mutated.Valid() {
			t.Fatalf("membership proof scalar mutation survived field=%s", name)
		}
		if _, ok := mutated.ContentID(); ok {
			t.Fatalf("mutated membership proof published an identity field=%s", name)
		}
	}
	if summary := sealedMembershipProof(valuedomain.MembershipSummary); !summary.Valid() {
		t.Fatal("summary membership proof invalid")
	}
	if mixed := sealedMembershipProof(valuedomain.MembershipMixedOrUnknown); mixed.Valid() {
		t.Fatal("mixed or unknown membership sealed a proof")
	}
	if _, ok := directAllocationMembershipProofID(identity.ContentID{}, proof.correlation, proof.direct, valuedomain.MembershipRecent); ok {
		t.Fatal("absent attachment derived a membership proof identity")
	}
}
