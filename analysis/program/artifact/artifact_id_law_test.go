package artifact

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	programsource "github.com/wippyai/go-lua/analysis/program/source"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	programdiagnostic "github.com/wippyai/go-lua/analysis/schema/program/programdiagnostic"
	programpublication "github.com/wippyai/go-lua/analysis/schema/program/publication"
	programstate "github.com/wippyai/go-lua/analysis/schema/program/state"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

// coldLawPublication seals the supplied cold publication. The identity walk
// reads the families that live there, so an artifact assembled for a law about
// one column still needs every declared family published as an empty plane.
func coldLawPublication(t *testing.T, publication programpublication.Publication) (snapshot.Frozen, identity.ContentID) {
	t.Helper()
	catalog, derived := programcatalog.CatalogID(identity.ContentID{0xC0, 0x1D})
	if !derived {
		t.Fatal("cold catalog")
	}
	frozen, sealed := publication.Seal(catalog, identity.StoreID(1))
	if !sealed {
		t.Fatal("seal cold publication")
	}
	return frozen, catalog
}

// TestArtifactIDSealsEveryDiagnosticObservationPayload states the identity law
// for the diagnostic-observation column: the ArtifactID preimage carries the
// full payload of every observation kind it hashes. A kind the digest walk
// does not carry must therefore refuse, because a truncated preimage gives two
// artifacts with distinct payloads one content address.
func TestArtifactIDSealsEveryDiagnosticObservationPayload(t *testing.T) {
	span := programsource.Span{File: "observation.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 2}
	observation := func(kind structure.DiagnosticObservationKind, decision identity.ContentID) (*Artifact, programdiagnostic.View) {
		evidence, evidenceOK := programdiagnostic.NewDiagnosticEvidence(identity.ContentID{3})
		if !evidenceOK {
			t.Fatal("diagnostic evidence")
		}
		if kind != structure.DiagnosticObservationBranchCondition {
			t.Fatalf("diagnostic observation kind %v", kind)
		}
		row, rowOK := programdiagnostic.NewDiagnosticObservationBranchCondition(
			identity.ContentID{1}, span, 0, 1, decision, identity.ContentID{2},
		)
		if !rowOK {
			t.Fatalf("diagnostic observation kind %v", kind)
		}
		frozen, catalog := coldLawPublication(t, programpublication.Publication{
			Diagnostic: programdiagnostic.Publication{
				DiagnosticObservations: []programdiagnostic.DiagnosticObservation{row},
				DiagnosticEvidence:     []programdiagnostic.DiagnosticEvidence{evidence},
			},
		})
		artifact := &Artifact{frozen: frozen, coldCatalog: catalog}
		state, stateOK := programstate.New(frozen, catalog)
		view, viewOK := programdiagnostic.NewView(state)
		if !stateOK || !viewOK {
			t.Fatal("diagnostic view")
		}
		return artifact, view
	}

	knownArtifact, knownProgram := observation(structure.DiagnosticObservationBranchCondition, identity.ContentID{4})
	distinctArtifact, distinctProgram := observation(structure.DiagnosticObservationBranchCondition, identity.ContentID{5})
	knownCount, knownPublished := knownProgram.DiagnosticObservationCount()
	distinctCount, distinctPublished := distinctProgram.DiagnosticObservationCount()
	if !knownPublished || !distinctPublished || knownCount != 1 || distinctCount != 1 {
		t.Fatalf("diagnostic observation counts = %d/%d", knownCount, distinctCount)
	}
	knownRow, knownHeld := knownProgram.DiagnosticObservationAt(0)
	distinctRow, distinctHeld := distinctProgram.DiagnosticObservationAt(0)
	if !knownHeld || !distinctHeld || knownRow.DecisionPathID() == distinctRow.DecisionPathID() {
		t.Fatal("direct Program read dropped the observation payload")
	}
	known := artifactID(knownArtifact)
	distinct := artifactID(distinctArtifact)
	if !known.Available() || !distinct.Available() {
		t.Fatal("artifact identity refused a declared observation kind")
	}
	if known == distinct {
		t.Fatal("artifact identity dropped the observation payload from its preimage")
	}

	unknown := structure.DiagnosticObservationTypeConformance + 1
	if unknown.Available() {
		t.Fatal("probe kind belongs to the canonical observation vocabulary")
	}
	if _, admitted := programdiagnostic.NewDiagnosticObservationTypeConformance(
		identity.ContentID{6}, span, 0, 0, programdiagnostic.DiagnosticObservationSiteInvalid,
		identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, 0,
	); admitted {
		t.Fatal("canonical conformance constructor accepted an invalid site/evidence span")
	}
}
