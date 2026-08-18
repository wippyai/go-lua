package artifact

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	programsource "github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// TestArtifactIDSealsEveryDiagnosticObservationPayload states the identity law
// for the diagnostic-observation column: the ArtifactID preimage carries the
// full payload of every observation kind it hashes. A kind the digest walk
// does not carry must therefore refuse, because a truncated preimage gives two
// artifacts with distinct payloads one content address.
func TestArtifactIDSealsEveryDiagnosticObservationPayload(t *testing.T) {
	span := programsource.Span{File: "observation.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 2}
	observation := func(kind structure.DiagnosticObservationKind, decision identity.ContentID) *Artifact {
		return &Artifact{diagnosticObservations: []DiagnosticObservationRow{{
			id:       identity.ContentID{1},
			kind:     kind,
			location: span,
			branch: diagnosticBranchConditionRow{
				decision: decision,
				value:    identity.ContentID{2},
				points:   []identity.ContentID{{3}},
			},
		}}}
	}

	known := artifactID(observation(structure.DiagnosticObservationBranchCondition, identity.ContentID{4}))
	distinct := artifactID(observation(structure.DiagnosticObservationBranchCondition, identity.ContentID{5}))
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
	left, right := observation(unknown, identity.ContentID{4}), observation(unknown, identity.ContentID{5})
	if left.diagnosticObservations[0].Available() {
		t.Fatal("row admission accepted an unknown observation kind")
	}
	if id := diagnosticObservationID(identity.ContentID{6}, unknown, span, left.diagnosticObservations[0].branch,
		diagnosticUnresolvedTypeReferenceRow{}, diagnosticUnresolvedValueReferenceRow{}, diagnosticTypeConformanceRow{}); id.Available() {
		t.Fatal("observation identity sealed an unknown kind")
	}
	if artifactID(left).Available() || artifactID(right).Available() {
		t.Fatal("artifact identity sealed an unknown observation kind on a truncated preimage")
	}
}
