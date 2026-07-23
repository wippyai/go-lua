package summaryinstance

import (
	"bytes"
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/interproc"
)

func TestPortableClosedOutcomeSealRoundTripCarriesClosureResidualsAndAllocationTransport(t *testing.T) {
	schema, outcome := portableOutcomeFixture(t)
	artifact, err := Seal(context.Background(), schema, outcome)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(context.Background(), schema, artifact)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.DemandedArtifactID != outcome.DemandedArtifactID || decoded.InstanceProjectionID != outcome.InstanceProjectionID ||
		!bytes.Equal(decoded.InstanceProjectionBytes, outcome.InstanceProjectionBytes) || decoded.ResultDigest != outcome.ResultDigest {
		t.Fatalf("identity round trip = %#v, want %#v", decoded, outcome)
	}
	if len(decoded.Values) != 2 || decoded.Values[0].Key != "value-a" || decoded.Values[1].Key != "value-b" ||
		len(decoded.Outcomes) != 1 || decoded.Outcomes[0].Key != "return" {
		t.Fatalf("semantic closure was not canonically transported: %#v", decoded)
	}
	if len(decoded.AllocationTransport) != 1 || decoded.AllocationTransport[0].TemplateID != content("allocation-template") ||
		decoded.AllocationTransport[0].ResultID != content("allocation-result") {
		t.Fatalf("allocation transport = %#v", decoded.AllocationTransport)
	}
	if len(decoded.ApplicationResiduals) != 2 || decoded.ApplicationResiduals[0].Decision != ResidualUndetermined ||
		decoded.ApplicationResiduals[1].Decision != ResidualFailing || !decoded.ApplicationResiduals[1].BoundStateID.Valid() {
		t.Fatalf("application residuals = %#v", decoded.ApplicationResiduals)
	}
	if len(decoded.CalleeInstanceKeys) != 1 || !bytes.Equal(decoded.CalleeInstanceKeys[0].InstanceProjectionBytes, outcome.CalleeInstanceKeys[0].InstanceProjectionBytes) ||
		len(decoded.DependencyIDs) != 2 {
		t.Fatalf("callee or dependency transport = %#v", decoded)
	}
	if !artifact.Valid() {
		t.Fatal("sealed artifact is not self-authenticating")
	}
}

func TestPortableClosedOutcomeCanonicalizesInputOrderToByteIdenticalProduction(t *testing.T) {
	schema, left := portableOutcomeFixture(t)
	right := left
	right.Values = []Fact{left.Values[1], left.Values[0]}
	right.DependencyIDs = []interproc.ContentID{left.DependencyIDs[1], left.DependencyIDs[0]}
	right.ApplicationResiduals = []ApplicationResidual{left.ApplicationResiduals[1], left.ApplicationResiduals[0]}
	var err error
	right.ResultDigest, err = ResultDigestFor(right)
	if err != nil {
		t.Fatal(err)
	}
	leftArtifact, err := Seal(context.Background(), schema, left)
	if err != nil {
		t.Fatal(err)
	}
	rightArtifact, err := Seal(context.Background(), schema, right)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(leftArtifact.Bytes, rightArtifact.Bytes) || leftArtifact.Semantic != rightArtifact.Semantic {
		t.Fatal("equal portable results produced different bytes")
	}
}

func TestPortableClosedOutcomeRejectsUnknownOrNonPositiveFeasibility(t *testing.T) {
	_, outcome := portableOutcomeFixture(t)
	outcome.ApplicationResiduals[0].Decision = ResidualFailing
	outcome.ApplicationResiduals[0].BoundStateID = interproc.ContentID{}
	if _, err := ResultDigestFor(outcome); err == nil {
		t.Fatal("failing residual without positive feasibility proof was accepted")
	}

	outcome.ApplicationResiduals[0].Decision = ResidualUndetermined
	outcome.ApplicationResiduals[0].BoundStateID = content("would-be-possible")
	if _, err := ResultDigestFor(outcome); err == nil {
		t.Fatal("undetermined feasibility was accepted as a positive proof")
	}

	otherSchema, err := NewFormatSchema(content("other-registry"), content("domain"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Encode(context.Background(), otherSchema, outcome); err == nil {
		t.Fatal("foreign registry/domain schema was accepted")
	}
}

func TestPortableClosedOutcomeRejectsProjectionDigestMismatch(t *testing.T) {
	schema, outcome := portableOutcomeFixture(t)
	outcome.InstanceProjectionBytes = append([]byte(nil), outcome.InstanceProjectionBytes...)
	outcome.InstanceProjectionBytes = append(outcome.InstanceProjectionBytes, 0)
	outcome.InstanceProjectionID = interproc.ContentIDFromCanonicalBytes(outcome.InstanceProjectionBytes)
	outcome.ResultDigest, _ = ResultDigestFor(outcome)
	if _, err := Encode(context.Background(), schema, outcome); err == nil {
		t.Fatal("malformed retained projection bytes were accepted")
	}
}

func portableOutcomeFixture(t *testing.T) (FormatSchema, PortableClosedOutcome) {
	t.Helper()
	schema, err := NewFormatSchema(content("registry"), content("domain"))
	if err != nil {
		t.Fatal(err)
	}
	projection := mustProjection(t, "entry", "entry-value")
	calleeProjection := mustProjection(t, "callee-entry", "callee-value")
	outcome := PortableClosedOutcome{
		FormatSchemaID:          schema.ID(),
		DemandedArtifactID:      content("demanded-body"),
		InstanceProjectionBytes: projection,
		InstanceProjectionID:    interproc.ContentIDFromCanonicalBytes(projection),
		Values: []Fact{
			{Key: "value-b", Value: []byte("b")},
			{Key: "value-a", Value: []byte("a")},
		},
		Outcomes:            []Fact{{Key: "return", Value: []byte("normal")}},
		AllocationTransport: []AllocationTransport{{TemplateID: content("allocation-template"), ResultID: content("allocation-result")}},
		ApplicationResiduals: []ApplicationResidual{
			{DescriptorID: content("descriptor-b"), PredicateID: content("predicate-b"), EvidenceID: content("evidence-b"), GuardID: content("guard-b"), BoundaryID: content("boundary-b"), Decision: ResidualFailing, BoundStateID: content("bound-state-b")},
			{DescriptorID: content("descriptor-a"), PredicateID: content("predicate-a"), EvidenceID: content("evidence-a"), GuardID: content("guard-a"), BoundaryID: content("boundary-a"), Decision: ResidualUndetermined},
		},
		CalleeInstanceKeys: []InstanceKey{{DemandedArtifactID: content("callee-body"), InstanceProjectionBytes: calleeProjection, InstanceProjectionID: interproc.ContentIDFromCanonicalBytes(calleeProjection)}},
		DependencyIDs:      []interproc.ContentID{content("provider"), content("source")},
	}
	outcome.ResultDigest, err = ResultDigestFor(outcome)
	if err != nil {
		t.Fatal(err)
	}
	return schema, outcome
}

func mustProjection(t *testing.T, selector, value string) []byte {
	t.Helper()
	certificate, err := interproc.NewReadProjectionCertificate("demand", interproc.ReadCertificateInputs{Semantic: []interproc.EntrySelector{interproc.EntrySelector(selector)}})
	if err != nil {
		t.Fatal(err)
	}
	entry, err := interproc.NewEntryBinding([]interproc.EntryValue{{Selector: interproc.EntrySelector(selector), Encoding: []byte(value)}})
	if err != nil {
		t.Fatal(err)
	}
	projection, err := certificate.Project(entry)
	if err != nil {
		t.Fatal(err)
	}
	return projection.CanonicalBytes()
}

func content(value string) interproc.ContentID {
	return interproc.ContentIDFromCanonicalBytes([]byte(value))
}
