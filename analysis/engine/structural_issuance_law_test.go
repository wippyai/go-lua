package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/identity"
	memberrelation "github.com/wippyai/go-lua/analysis/schema/axis/member/relation"
)

// structural_issuance_law_test.go states the two derivations the generated
// activation arm performs that no owner is asked for: the transport issuer,
// which comes from the descriptor's own vector, and the frame discipline that
// keeps a semantic axis and a content identity from standing in for one
// another.

// sealIssuanceLawBinding brings one generated fixture to the phase issuance
// actually runs at. A generated rule seals only once it has installed the
// family that executes it and once every axis its plan addresses publishes a
// relation owner, so both are supplied here rather than worked around.
func sealIssuanceLawBinding(t *testing.T, fixture generatedBindingLawFixture) {
	t.Helper()
	if !BindRuleFamily[uint64](fixture.binding, fixture.slot, fixture.factors[0].Ref(), lawRuleFamilyInstaller{}) {
		t.Fatal("the fixture could not install its family")
	}
	for index, factor := range fixture.factors {
		if !BindRelationOwner(fixture.binding, factor, &generatedBindingLawOwner{acceptCandidate: true}) {
			t.Fatalf("relation owner %d", index)
		}
	}
	if !fixture.binding.Seal() {
		refusal, named := fixture.binding.Refusal()
		t.Fatalf("the fixture binding did not seal: refusal=%q named=%t poisoned=%t", refusal, named, fixture.binding.Poisoned())
	}
}

// TestTheTransportIssuerIsDerivedFromTheDeclaredVector is the piece that
// retires activation.BindHot's six FactorRef parameters.
//
// The hand lane is handed two lists and seals their symmetry: an export naming
// an axis no import named is a defect the issuer has to catch. A declared
// vector cannot express that at all - one row is one axis, its existence is
// the import and Exported is the return direction - so the symmetry becomes a
// property of the shape and the check has nothing left to reject.
func TestTheTransportIssuerIsDerivedFromTheDeclaredVector(t *testing.T) {
	fixture := structuralLawFixture(t, 0)
	descriptor, descriptorOK := fixture.schema.generatedProgramAt(0)
	if !descriptorOK || descriptor.TransportCount() == 0 {
		t.Fatal("the structural descriptor carries no transport vector")
	}
	ordinal, ordinalOK := fixture.slot.Ordinal()
	shape, shapeOK := fixture.schema.ruleShapeAt(ordinal)
	semantic := fixture.schema.ruleSemanticAt(ordinal)
	if !ordinalOK || !shapeOK || !semantic.Available() {
		t.Fatal("the sealed structural rule row")
	}
	sealIssuanceLawBinding(t, fixture)
	issuer, issued := declaredGeneratedActivationIssuer(bindingState(fixture.binding), descriptor, semantic, shape.ActivationFamily)
	if !issued || issuer == nil {
		t.Fatal("a declared transport vector issued no transport")
	}
	if issuer.rule != semantic || issuer.family != shape.ActivationFamily {
		t.Fatal("the issuer names a rule or family other than the one that declared it")
	}
	// Every row is an import; the exported subset is the return direction.
	if len(issuer.imports) != descriptor.TransportCount() {
		t.Fatalf("imports = %d, want one per declared row (%d)", len(issuer.imports), descriptor.TransportCount())
	}
	if len(issuer.exports) == 0 || len(issuer.exports) > len(issuer.imports) {
		t.Fatalf("exports = %d, want a non-empty subset of the imports", len(issuer.exports))
	}
	imported := make(map[composition.Key]struct{}, len(issuer.imports))
	for _, port := range issuer.imports {
		imported[port] = struct{}{}
	}
	for _, port := range issuer.exports {
		if _, carried := imported[port]; !carried {
			t.Fatal("an exported axis was never imported, which the declared vector cannot express")
		}
	}
}

// TestAFactWritingDescriptorIssuesNoTransport keeps the derivation on the same
// biconditional every other half of the A form is held to.
func TestAFactWritingDescriptorIssuesNoTransport(t *testing.T) {
	fixture := openGeneratedBindingSpareLaw(t, newGeneratedRuleLawFixture(t, generatedRuleLawExact, generatedRuleLawRuleRole), 0)
	descriptor, descriptorOK := fixture.schema.generatedProgramAt(0)
	if !descriptorOK || descriptor.TransportCount() != 0 {
		t.Fatal("the fact-writing fixture carries a transport vector")
	}
	ordinal, _ := fixture.slot.Ordinal()
	shape, _ := fixture.schema.ruleShapeAt(ordinal)
	sealIssuanceLawBinding(t, fixture)
	if issuer, issued := declaredGeneratedActivationIssuer(bindingState(fixture.binding), descriptor, fixture.schema.ruleSemanticAt(ordinal), shape.ActivationFamily); issued || issuer != nil {
		t.Fatal("a fact-writing descriptor issued a transport")
	}
}

// frameLawOwner answers one identity per projection ordinal so the frame
// discipline can be stated without a whole axis behind it.
type frameLawOwner struct {
	digest identity.ContentID
}

func (frameLawOwner) CandidateCount(uint32, identity.ContentID, identity.ContentID) (int, bool) {
	return 0, false
}

func (frameLawOwner) CandidateAt(uint32, identity.ContentID, identity.ContentID, int) (uint32, bool) {
	return 0, false
}
func (frameLawOwner) MemberCount(uint32, uint32) (int, bool)      { return 0, false }
func (frameLawOwner) MemberAt(uint32, uint32, int) (uint32, bool) { return 0, false }
func (frameLawOwner) Project(uint32, uint32, uint32) (uint32, bool) {
	return 0, false
}

func (owner frameLawOwner) ProjectIdentity(_, projection, _ uint32) (identity.ContentID, uint64, bool) {
	switch projection {
	case 0:
		return owner.digest, 0, true // a content identity
	case 1:
		return owner.digest, 4, true // a semantic axis under its own frame
	default:
		return identity.ContentID{}, 0, false
	}
}

func frameLawDigest() identity.ContentID {
	var digest identity.ContentID
	for index := range digest {
		digest[index] = byte(index + 3)
	}
	return digest
}

// TestASemanticAxisAndAContentIdentityDoNotStandInForOneAnother is the frame
// discipline the issuance arm reads its branch identities under.
//
// A target is a semantic axis: an unframed answer in that position is a
// DIFFERENT identity, not the same one under a default frame, so inventing a
// frame there would mount a branch under a key its owner never issued. A body
// module is a content identity: a framed answer there is a semantic axis, and
// truncating it to its digest would silently merge two axes of one subject.
func TestASemanticAxisAndAContentIdentityDoNotStandInForOneAnother(t *testing.T) {
	var owner memberrelation.IdentityProjection = frameLawOwner{digest: frameLawDigest()}

	semantic, semanticOK := projectedSemanticIdentity(owner, 0, 1, 0)
	if !semanticOK || !semantic.Available() || semantic.Version() != 4 {
		t.Fatalf("a framed column did not read as its owner's semantic axis: %v/%t", semantic.Version(), semanticOK)
	}
	content, contentOK := projectedContentIdentity(owner, 0, 0, 0)
	if !contentOK || content != frameLawDigest() {
		t.Fatal("an unframed column did not read as a content identity")
	}

	if _, ok := projectedSemanticIdentity(owner, 0, 0, 0); ok {
		t.Fatal("an unframed column was read as a semantic axis under an invented frame")
	}
	if _, ok := projectedContentIdentity(owner, 0, 1, 0); ok {
		t.Fatal("a framed column was truncated to a content identity")
	}
	if _, ok := projectedSemanticIdentity(owner, 0, 9, 0); ok {
		t.Fatal("an undeclared column answered a semantic axis")
	}
	if _, ok := projectedContentIdentity(owner, 0, 9, 0); ok {
		t.Fatal("an undeclared column answered a content identity")
	}
}
