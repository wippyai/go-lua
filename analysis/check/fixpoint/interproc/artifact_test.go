package interproc

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/engine/solve"
)

func TestDemandedBodyArtifactCanonicalContentIncludesEnvelopeAuthorities(t *testing.T) {
	artifact := demandedBodyArtifactFixture(t)
	first := artifact.CanonicalBytes()
	if len(first) == 0 || !artifact.ContentID().Valid() {
		t.Fatal("complete envelope has no content identity")
	}

	changedCertificate, err := NewReadProjectionCertificate("point-state", ReadCertificateInputs{
		Semantic:   []EntrySelector{"value"},
		Guards:     []EntrySelector{"guard"},
		Diagnostic: []EntrySelector{"diagnostic"},
	})
	if err != nil {
		t.Fatal(err)
	}
	changedDemand, err := NewDemandedBodyArtifact(artifact.Body(), artifact.ParameterSchema(), "point-state", changedCertificate, artifact.SolverPolicyID(), artifact.Dependencies(), artifact.DiagnosticReadSets())
	if err != nil {
		t.Fatal(err)
	}
	if artifact.ContentID() == changedDemand.ContentID() {
		t.Fatal("demand key was omitted from artifact identity")
	}

	dependencies, err := NewDependencyManifest([]Dependency{
		{Kind: "codec", ID: testContentID("codec-v1")},
		{Kind: "source", ID: testContentID("source-v2")},
		{Kind: "registry", ID: testContentID("registry-v1")},
	})
	if err != nil {
		t.Fatal(err)
	}
	changedDependency, err := NewDemandedBodyArtifact(artifact.Body(), artifact.ParameterSchema(), artifact.DemandKey(), artifact.ReadCertificate(), artifact.SolverPolicyID(), dependencies, artifact.DiagnosticReadSets())
	if err != nil {
		t.Fatal(err)
	}
	if artifact.ContentID() == changedDependency.ContentID() {
		t.Fatal("dependency content identity was omitted from artifact identity")
	}
}

func TestDemandedBodyArtifactRejectsUncertifiedDiagnosticRead(t *testing.T) {
	body := interprocCyclicFixture(t)
	schema, err := NewParameterSchema("entry", []EntrySelector{"value", "diagnostic"})
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := NewReadProjectionCertificate("normal-return", ReadCertificateInputs{Semantic: []EntrySelector{"value"}})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := NewDependencyManifest([]Dependency{{Kind: "source", ID: testContentID("source")}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewDemandedBodyArtifact(body, schema, "normal-return", certificate, testContentID("solver"), manifest,
		[]DiagnosticReadSet{{Descriptor: testContentID("descriptor"), Reads: []EntrySelector{"diagnostic"}}})
	if err == nil {
		t.Fatal("artifact accepted diagnostic read absent from certificate")
	}
}

func TestDependencyManifestUsesContentChainsNotOrder(t *testing.T) {
	left, err := NewDependencyManifest([]Dependency{{Kind: "source", ID: testContentID("source")}, {Kind: "registry", ID: testContentID("registry")}})
	if err != nil {
		t.Fatal(err)
	}
	right, err := NewDependencyManifest([]Dependency{{Kind: "registry", ID: testContentID("registry")}, {Kind: "source", ID: testContentID("source")}})
	if err != nil {
		t.Fatal(err)
	}
	if string(left.CanonicalBytes()) != string(right.CanonicalBytes()) || left.ContentID() != right.ContentID() {
		t.Fatal("dependency manifest identity depends on declaration order")
	}
}

func TestDemandedBodyArtifactDetachesItsBodyCertificate(t *testing.T) {
	artifact := demandedBodyArtifactFixture(t)
	before := artifact.ContentID()
	body := artifact.Body()
	delete(body.CellForTarget, body.Artifact.Equations[0].Target)
	if artifact.ContentID() != before || artifact.Body().CanonicalBytes() == nil {
		t.Fatal("caller mutation leaked into demanded body artifact")
	}
}

func demandedBodyArtifactFixture(t testing.TB) DemandedBodyArtifact {
	t.Helper()
	body := interprocCyclicFixture(t)
	schema, err := NewParameterSchema("entry", []EntrySelector{"value", "guard", "diagnostic", "unread"})
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := NewReadProjectionCertificate("normal-return", ReadCertificateInputs{
		Semantic:   []EntrySelector{"value"},
		Guards:     []EntrySelector{"guard"},
		Diagnostic: []EntrySelector{"diagnostic"},
	})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := NewDependencyManifest([]Dependency{
		{Kind: "registry", ID: testContentID("registry-v1")},
		{Kind: "source", ID: testContentID("source-v1")},
		{Kind: "codec", ID: testContentID("codec-v1")},
	})
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := NewDemandedBodyArtifact(body, schema, "normal-return", certificate, testContentID("solver-v1"), manifest,
		[]DiagnosticReadSet{{Descriptor: testContentID("descriptor-v1"), Reads: []EntrySelector{"diagnostic"}}})
	if err != nil {
		t.Fatal(err)
	}
	return artifact
}

func interprocCyclicFixture(t testing.TB) equation.CyclicArtifact {
	t.Helper()
	var body equation.BodyID
	body[0] = 73
	entry := equation.EntryParameter{Body: body, Name: "entry"}
	var contract equation.ContentID
	contract[0] = 74
	plain := equation.Artifact{Equations: []equation.Equation{{
		Target: equation.Coordinate{Body: body, Name: "result"}, Entry: entry,
		Occurrence: equation.Occurrence{Kind: "entry", ContractID: contract}, KernelID: "seed",
		Operands: []equation.Operand{{Role: "entry", Term: equation.EntryTerm(entry)}},
	}}}
	plan, err := solve.FreezeWTOPlan([]equation.CellID{"result"}, []solve.WTOElement[equation.CellID]{{Vertex: "result"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	cyclic, err := equation.NewCyclicArtifact(plain, map[equation.Coordinate]equation.CellID{plain.Equations[0].Target: "result"}, plan,
		nil, []equation.OutputSelector{{ID: "normal-return", Cells: []equation.CellID{"result"}}}, []equation.CellID{"result"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return cyclic
}

func testContentID(text string) ContentID { return ContentIDFromCanonicalBytes([]byte(text)) }
