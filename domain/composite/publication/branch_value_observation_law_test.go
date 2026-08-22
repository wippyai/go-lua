package publication

import (
	"crypto/sha256"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
)

func publicationLawID(label string) identity.ContentID {
	return identity.ContentID(sha256.Sum256([]byte("publication-law/" + label)))
}

const branchValueObservationLawProducer schema.Key = "value-summary"

func sealedBranchValueObservationAttachment() BranchValueObservationAttachment {
	attachment := BranchValueObservationAttachment{
		mount: publicationLawID("branch-observation/mount"),
		point: publicationLawID("branch-observation/point"),
	}
	attachment.producer = branchValueObservationLawProducer
	attachment.context, _ = executioncontext.NewContext(
		publicationLawID("branch-observation/link"), attachment.mount,
		publicationLawID("branch-observation/actor"), publicationLawID("branch-observation/representative"),
	)
	attachment.id, _ = BranchValueObservationID(attachment.mount, attachment.point, attachment.producer, attachment.context)
	return attachment
}

// The attachment's identity is what authorizes its retained Engine handle, so a
// scalar rewrite must leave the attachment unusable even before the handle is
// consulted.
func TestBranchValueObservationAttachmentScalarSealLaw(t *testing.T) {
	attachment := sealedBranchValueObservationAttachment()
	if !attachment.id.Available() {
		t.Fatal("sealed attachment identity")
	}
	foreign := publicationLawID("branch-observation/foreign")
	mutations := map[string]func(BranchValueObservationAttachment) BranchValueObservationAttachment{
		"id": func(value BranchValueObservationAttachment) BranchValueObservationAttachment {
			value.id = foreign
			return value
		},
		"mount": func(value BranchValueObservationAttachment) BranchValueObservationAttachment {
			value.mount = foreign
			return value
		},
		"point": func(value BranchValueObservationAttachment) BranchValueObservationAttachment {
			value.point = foreign
			return value
		},
		"producer": func(value BranchValueObservationAttachment) BranchValueObservationAttachment {
			value.producer = "effect-exact"
			return value
		},
		"context": func(value BranchValueObservationAttachment) BranchValueObservationAttachment {
			value.context, _ = executioncontext.NewContext(
				publicationLawID("branch-observation/link-foreign"), value.mount,
				publicationLawID("branch-observation/actor"), publicationLawID("branch-observation/representative"),
			)
			return value
		},
	}
	for name, mutate := range mutations {
		if want, ok := BranchValueObservationID(attachment.mount, attachment.point, attachment.producer, attachment.context); !ok || want != attachment.id {
			t.Fatal("attachment identity derivation")
		}
		mutated := mutate(attachment)
		if want, ok := BranchValueObservationID(mutated.mount, mutated.point, mutated.producer, mutated.context); ok && want == mutated.id {
			t.Fatalf("attachment scalar mutation kept its identity binding field=%s", name)
		}
		if mutated.Valid() {
			t.Fatalf("attachment scalar mutation survived field=%s", name)
		}
	}
}

// An attachment is only ever issued together with the Engine observation its
// identity authorized. A handle-free attachment therefore publishes no identity,
// and admits no member, which is what keeps a forged attachment out of every
// reader of this evidence point.
func TestBranchValueObservationAttachmentObservationLaw(t *testing.T) {
	attachment := sealedBranchValueObservationAttachment()
	if attachment.Valid() {
		t.Fatal("attachment sealed without an authenticated observation handle")
	}
	if _, ok := attachment.ContentID(); ok {
		t.Fatal("attachment published an identity without an authenticated observation handle")
	}
	if attachment.observation.Available() || attachment.observation.ID == attachment.id {
		t.Fatal("absent observation handle authenticated an attachment identity")
	}
	if _, published := attachment.Observation(); published {
		t.Fatal("attachment published a declared observation row without an authenticated handle")
	}
	if MemberPublished(nil, engine.RuleSlotCapability{}, attachment.mount, attachment.point, publicationLawID("branch-observation/occurrence")) {
		t.Fatal("attachment admitted a member without a committed program")
	}
}

// The declaration is the only issuer of this attachment, and it refuses every
// incomplete binding: a committed program, a Value summary query, and the
// published member that owns the point are all required before an identity is
// derived.
func TestBranchValueObservationAttachmentIssuanceLaw(t *testing.T) {
	mount, point, occurrence := publicationLawID("issuance/mount"), publicationLawID("issuance/point"), publicationLawID("issuance/occurrence")
	attachment, failure, ok := DeclareBranchValueObservation(nil, nil, engine.RuleSlotCapability{}, branchValueObservationLawProducer, mount, point, occurrence, executioncontext.Context{})
	if ok {
		t.Fatal("attachment issued without a committed program and query")
	}
	if failure != engine.ObservationSealArguments() {
		t.Fatalf("incomplete binding reported failure=%v", failure)
	}
	if attachment.Valid() {
		t.Fatal("refused issuance returned a valid attachment")
	}
	if _, published := attachment.ContentID(); published {
		t.Fatal("refused issuance published an identity")
	}
}

// The evidence point coordinates are the attachment identity: an absent
// coordinate names no point, and the mount and the point each move the identity
// so one mount's evidence cannot answer for another's.
func TestBranchValueObservationAttachmentCoordinateLaw(t *testing.T) {
	mount, point := publicationLawID("coordinate/branch-mount"), publicationLawID("coordinate/branch-point")
	context, contextOK := executioncontext.NewContext(publicationLawID("coordinate/link"), mount, publicationLawID("coordinate/actor"), publicationLawID("coordinate/representative"))
	if !contextOK {
		t.Fatal("coordinate context")
	}
	for index, absent := range [][2]identity.ContentID{
		{identity.ContentID{}, point},
		{mount, identity.ContentID{}},
		{identity.ContentID{}, identity.ContentID{}},
	} {
		if _, ok := BranchValueObservationID(absent[0], absent[1], branchValueObservationLawProducer, context); ok {
			t.Fatalf("absent coordinate derived an attachment identity index=%d", index)
		}
	}
	first, firstOK := BranchValueObservationID(mount, point, branchValueObservationLawProducer, context)
	swappedContext, swappedContextOK := executioncontext.NewContext(publicationLawID("coordinate/swapped-link"), point, publicationLawID("coordinate/actor"), publicationLawID("coordinate/representative"))
	swapped, swappedOK := BranchValueObservationID(point, mount, branchValueObservationLawProducer, swappedContext)
	if !firstOK || !swappedContextOK || !swappedOK || first == swapped {
		t.Fatal("attachment identity ignored the coordinate positions")
	}
}

func TestBranchValueObservationContextAddressIsDistinct(t *testing.T) {
	mount := publicationLawID("context/mount")
	point := publicationLawID("context/point")
	first := publicationLawID("context/first")
	second := publicationLawID("context/second")
	firstContext, firstOK := executioncontext.NewContext(publicationLawID("context/link"), mount, first, publicationLawID("context/first-representative"))
	secondContext, secondOK := executioncontext.NewContext(firstContext.LinkID(), mount, second, publicationLawID("context/second-representative"))
	if !firstOK || !secondOK {
		t.Fatal("construct context-qualified observation rows")
	}
	left, leftOK := BranchValueObservationID(mount, point, branchValueObservationLawProducer, firstContext)
	right, rightOK := BranchValueObservationID(mount, point, branchValueObservationLawProducer, secondContext)
	if !leftOK || !rightOK || left == right {
		t.Fatal("same-module contexts collapsed the observation address")
	}
}
