package engine

import (
	"crypto/sha256"
	"encoding/binary"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/identity"
)

func framingLawContentID(seed byte) identity.ContentID {
	var id identity.ContentID
	for index := range id {
		id[index] = seed
	}
	return id
}

// framingLawCapability issues a capability under one private binding state, so
// the identity laws below exercise the same availability fence production does.
func framingLawCapability(t *testing.T, kind ruleCapabilityKind, ordinal uint64, activation bool) RuleSlotCapability {
	t.Helper()
	authority := &schemaBindingAuthority{}
	state := &schemaBindingState{authority: authority}
	capability := RuleSlotCapability{state: state, authority: authority, ordinal: ordinal, kind: kind, activation: activation}
	if !capability.Available() {
		t.Fatal("rule slot capability fixture is unavailable")
	}
	return capability
}

// TestSummaryVectorDigestSeparatesKeyWidthAndDomain proves the summary key
// vector reaches its digest under its own domain and with its key width
// recorded: a narrow vector never digests as the wide vector spelling the same
// values, and the digest is not the bare concatenation of the keys.
func TestSummaryVectorDigestSeparatesKeyWidthAndDomain(t *testing.T) {
	narrow := SummaryVectorDigest([]uint32{1, 2, 3})
	wide := SummaryVectorDigest([]uint64{1, 2, 3})
	if narrow == ([32]byte{}) || wide == ([32]byte{}) {
		t.Fatal("summary vector digest is unavailable")
	}
	if narrow == wide {
		t.Fatal("a uint32 key vector digests as the uint64 vector spelling the same keys")
	}
	var scalar [8]byte
	hash := sha256.New()
	for _, key := range []uint64{1, 2, 3} {
		binary.BigEndian.PutUint64(scalar[:], key)
		if _, err := hash.Write(scalar[:]); err != nil {
			t.Fatal(err)
		}
	}
	var concatenated [32]byte
	copy(concatenated[:], hash.Sum(nil))
	if wide == concatenated {
		t.Fatal("summary vector digest is the bare key concatenation and carries no domain")
	}
	if SummaryVectorDigest([]uint64{1, 2}) == SummaryVectorDigest([]uint64{1, 2, 0}) {
		t.Fatal("summary vector length does not participate in its digest")
	}
}

// TestArtifactSourceDomainsDigestDistinctly proves the three cold source
// namespaces separate in the preimage. One artifact content identity names a
// point, an edge, and an occurrence key whose digests all differ, rather than
// one digest carrying three version labels.
func TestArtifactSourceDomainsDigestDistinctly(t *testing.T) {
	id := framingLawContentID(0x31)
	point, pointOK := artifactReceiptKey(artifactPointSource, id)
	edge, edgeOK := artifactReceiptKey(artifactEdgeSource, id)
	occurrence, occurrenceOK := artifactReceiptKey(artifactOccurrenceSource, id)
	if !pointOK || !edgeOK || !occurrenceOK {
		t.Fatal("artifact source key derivation rejected an available identity")
	}
	if point.ID == edge.ID || point.ID == occurrence.ID || edge.ID == occurrence.ID {
		t.Fatal("two artifact source namespaces share one digest and separate only by version")
	}
	if point.ID == composition.ID(id) {
		t.Fatal("an artifact source key reuses the content identity as its own digest")
	}
	if _, ok := artifactReceiptKey(artifactPointSource, identity.ContentID{}); ok {
		t.Fatal("an unavailable content identity produced an artifact source key")
	}
}

// TestOperandEntityDerivesItsOwnKeySpace proves the operand entity identity is
// derived from the issuer content digest rather than reinterpreting it, so an
// operand entity cannot coincide with another key carrying the same bytes.
func TestOperandEntityDerivesItsOwnKeySpace(t *testing.T) {
	digest := [32]byte{}
	for index := range digest {
		digest[index] = 0x47
	}
	entity, entityOK := operandEntityForContent(digest)
	if !entityOK {
		t.Fatal("operand entity derivation rejected an available digest")
	}
	if entity.ID == composition.ID(digest) {
		t.Fatal("the operand entity key reinterprets the issuer digest as its own identity")
	}
	other := digest
	other[31] ^= 1
	otherEntity, otherEntityOK := operandEntityForContent(other)
	if !otherEntityOK || otherEntity == entity {
		t.Fatal("two issuer digests produced one operand entity")
	}
	if _, ok := operandEntityForContent([32]byte{}); ok {
		t.Fatal("an empty digest produced an operand entity")
	}
}

// TestRuleSourceIdentitiesSeparateDomainAndCapability proves the rule source
// minters frame both their namespace and every capability field: the same
// inputs never cross domains, and two capabilities differing in one field
// never share an identity.
func TestRuleSourceIdentitiesSeparateDomainAndCapability(t *testing.T) {
	mounted := framingLawCapability(t, ruleCapabilityMounted, 3, false)
	link := framingLawCapability(t, ruleCapabilityLink, 3, false)
	mount, point, occurrence := framingLawContentID(0x11), framingLawContentID(0x22), framingLawContentID(0x33)

	member := mountedRuleMemberID(mounted, mount, point, occurrence)
	activation := mountedRuleActivationID(mounted, mount, point, occurrence)
	linkMember := linkRuleMemberID(link, mount, point, occurrence)
	if !member.Available() || !activation.Available() || !linkMember.Available() {
		t.Fatal("rule source identity derivation rejected available inputs")
	}
	if member == activation || member == linkMember || activation == linkMember {
		t.Fatal("two rule source namespaces produced one identity from the same inputs")
	}

	shifted := mountedRuleMemberID(mounted, point, mount, occurrence)
	if member == shifted {
		t.Fatal("the member identity does not distinguish its mount from its point")
	}
	if mountedRuleMemberID(framingLawCapability(t, ruleCapabilityMounted, 4, false), mount, point, occurrence) == member {
		t.Fatal("the capability ordinal does not participate in the member identity")
	}
	if mountedRuleMemberID(framingLawCapability(t, ruleCapabilityMounted, 3, true), mount, point, occurrence) == member {
		t.Fatal("the capability activation flag does not participate in the member identity")
	}

	occurrenceKey, occurrenceKeyOK := mountedRuleOccurrenceKey(mounted, occurrence)
	linkOccurrenceKey, linkOccurrenceKeyOK := linkRuleOccurrenceKey(link, occurrence)
	if !occurrenceKeyOK || !linkOccurrenceKeyOK || occurrenceKey.ID == linkOccurrenceKey.ID {
		t.Fatal("the mounted and link occurrence namespaces share one digest")
	}

	first, firstOK := mountedRuleInputKey(member, point, 1)
	second, secondOK := mountedRuleInputKey(member, point, 2)
	if !firstOK || !secondOK || first.ID == second.ID {
		t.Fatal("the input slot does not participate in the rule input key")
	}
}
