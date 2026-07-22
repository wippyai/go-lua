package transformer

import "testing"

func TestDiagnosticPublicationRequiresPositiveApplicationFeasibility(t *testing.T) {
	descriptor := operatorContractFixture(t).DiagnosticOutputs[0].DiagnosticDescriptorID()
	declared := DeclaredCheckContext{Artifact: contentID([]byte("provider")), Body: registryTestOwner(51), Registry: contentID([]byte("registry"))}
	publication := DiagnosticPublication{
		Descriptor: descriptor,
		Owner:      DiagnosticOwnerApplication,
		Declared:   declared,
		Application: BoundApplicationContext{
			CallerArtifact: contentID([]byte("caller")),
			CallAnchor:     contentID([]byte("anchor")),
			Binding:        contentID([]byte("binding")),
		},
		Feasibility: FeasibilityCertificate{
			Descriptor: descriptor,
			BoundState: contentID([]byte("bound-state")),
			Guard:      contentID([]byte("guard")),
			Feasible:   true,
		},
	}
	if err := publication.Validate(); err != nil {
		t.Fatalf("positive application publication rejected: %v", err)
	}
	publication.Feasibility.Feasible = false
	if err := publication.Validate(); err == nil {
		t.Fatal("application publication with no positive feasibility certificate was accepted")
	}
}

func TestDiagnosticPublicationRejectsCallerStateForCalleeCheck(t *testing.T) {
	descriptor := operatorContractFixture(t).DiagnosticOutputs[0].DiagnosticDescriptorID()
	publication := DiagnosticPublication{
		Descriptor: descriptor,
		Owner:      DiagnosticOwnerCalleeCheck,
		Declared: DeclaredCheckContext{
			Artifact: contentID([]byte("provider")), Body: registryTestOwner(52), Registry: contentID([]byte("registry")),
		},
		Application: BoundApplicationContext{
			CallerArtifact: contentID([]byte("caller")), CallAnchor: contentID([]byte("anchor")), Binding: contentID([]byte("binding")),
		},
	}
	if err := publication.Validate(); err == nil {
		t.Fatal("callee check publication retained caller application state")
	}
}
