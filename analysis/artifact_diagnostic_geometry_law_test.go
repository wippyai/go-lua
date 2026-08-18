package analysis

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// TestDiagnosticBranchGeometryUsesExecutionPointAndBaseAnchor proves that
// local storage and equality rules attach at their Local execution point while
// the geometry/collector continues to index the Program-issued base evidence.
// The two coordinates are related only by the sealed full-environment
// LocalTransfer row; no lower layer reconstructs the relationship.
func TestDiagnosticBranchGeometryUsesExecutionPointAndBaseAnchor(t *testing.T) {
	cases := []struct {
		name   string
		source string
		key    schema.Key
	}{
		{
			name: "storage-read",
			key:  "value-transfer",
			source: `local flag = true
if flag then
    return 1
end
return 0`,
		},
		{
			name: "binary-arithmetic",
			key:  "value-binary-arithmetic",
			source: `local cap = 3
if cap + 2 then
    return 1
end
return 0`,
		},
		{
			// The arithmetic sits in the initializer instead of the condition.
			// A branch producer is the condition's own evidence, never a
			// transitive dataflow closure over the values that reached it, so
			// this shape mounts the storage transfer of cap and no arithmetic
			// producer at all: the addition commits before the branch and is
			// the assignment's evidence, not the branch's. The row is kept
			// because the transfer chain it exercises starts from a local whose
			// value was computed rather than authored.
			name: "binary-arithmetic-initializer",
			key:  "value-transfer",
			source: `local cap = 3 + 2
if cap then
    return 1
end
return 0`,
		},
		{
			name: "binary-equality",
			key:  "value-binary-equality",
			source: `local function check(value: string | number): boolean
    if type(value) == "string" then
        return true
    end
			return false
end
return check("ok")`,
		},
		{
			name: "binary-order",
			key:  "value-binary-order",
			source: `local cap = 3
if cap > 5 then
    return 1
end
return 0`,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			contract, err := testfixture.StandardLibraryTarget()
			if err != nil {
				t.Fatal(err)
			}
			linked := mustLink(t, testCase.source, contract)
			plan, status, diagnostics := CompileWithDiagnostics(linked)
			if status != CompileComplete || plan == nil || plan.state == nil || plan.state.artifacts == nil {
				t.Fatalf("CompileWithDiagnostics = %v/%v diagnostics=%+v", status, plan, diagnostics)
			}
			defer plan.Close()

			found := false
			for _, mount := range plan.state.artifacts.mounts {
				transfers, transfersOK := diagnosticLocalTransfersByDestination(mount.artifact)
				if !transfersOK {
					t.Fatal("sealed LocalTransfer index unavailable")
				}
				for _, observation := range mustResultGeometry(t, plan.state).branchObservations {
					if observation.mount != mount.moduleKey {
						continue
					}
					for _, producer := range observation.producers {
						if producer.key != testCase.key {
							continue
						}
						found = true
						if producer.point == producer.anchor {
							t.Fatalf("producer %x was not retained at its Local execution point", producer.occurrence)
						}
						edge, edgeOK := transfers[producer.point]
						anchor, anchorOK := diagnosticEvidenceAnchor(observation.points, producer.point, transfers)
						if !edgeOK || !edge.FullEnvironment() || edge.To() != producer.point || !anchorOK || anchor != producer.anchor {
							t.Fatalf("producer geometry lost exact Program->Local chain: producer=%x anchor=%x/%x execution=%x edge=%+v/%t", producer.occurrence, producer.anchor, anchor, producer.point, edge, edgeOK)
						}
						baseEvidence := false
						for _, point := range observation.points {
							if point == producer.anchor {
								baseEvidence = true
								break
							}
						}
						if !baseEvidence {
							t.Fatalf("producer anchor %x is not a Program evidence point", producer.anchor)
						}
					}
				}
			}
			if !found {
				t.Fatalf("no mounted branch producer for key %q", testCase.key)
			}
		})
	}
}

// TestDiagnosticBranchGeometryRejectsUnprovenTransfer keeps the fail-closed
// rule local to the geometry wrapper: missing transfer, partial transport,
// and duplicate full destinations cannot be guessed into a branch geometry.
func TestDiagnosticBranchGeometryRejectsUnprovenTransfer(t *testing.T) {
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	linked := mustLink(t, `local flag = true
if flag then
    return 1
end
return 0`, contract)
	receipt, receiptOK := composite.Global()
	if !receiptOK || !receipt.Available() {
		t.Fatal("global schema unavailable")
	}
	artifacts, artifactsOK := compileProgramArtifacts(linked, receipt)
	if !artifactsOK || artifacts == nil || len(artifacts.mounts) == 0 {
		t.Fatal("artifact compile unavailable")
	}
	artifact := artifacts.mounts[0].artifact
	var full programartifact.LocalTransfer
	fullOK := false
	for index := 0; index < artifact.LocalTransferCount(); index++ {
		candidate, candidateOK := artifact.LocalTransferAt(index)
		if candidateOK && candidate.FullEnvironment() {
			full, fullOK = candidate, true
			break
		}
	}
	if !fullOK {
		t.Fatal("fixture did not issue a full LocalTransfer")
	}
	evidence := []identity.ContentID{full.From()}
	if anchor, ok := diagnosticEvidenceAnchor(evidence, full.To(), nil); ok || anchor.Available() {
		t.Fatal("missing transfer was accepted")
	}
	transfers := make(map[identity.ContentID]programartifact.LocalTransfer)
	if !addDiagnosticFullLocalTransfer(transfers, full) {
		t.Fatal("exact transfer index rejected its own sealed row")
	}
	if anchor, ok := diagnosticEvidenceAnchor(evidence, full.To(), transfers); !ok || anchor != full.From() {
		t.Fatalf("exact transfer was not accepted: anchor=%x/%t", anchor, ok)
	}
	if addDiagnosticFullLocalTransfer(transfers, full) {
		t.Fatal("ambiguous duplicate full destination was accepted")
	}
}

// TestDiagnosticBranchWithoutMountedProducerRemainsSupported proves that an
// optional Program branch family is omitted from the mounted projector rather
// than making the entire CompileWithDiagnostics boundary unsupported.
func TestDiagnosticBranchWithoutMountedProducerRemainsSupported(t *testing.T) {
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	linked := mustLink(t, `local function negate(value: boolean): boolean
    if not value then
        return true
    end
    return false
end
return negate(true)`, contract)
	plan, status, diagnostics := CompileWithDiagnostics(linked)
	if status != CompileComplete || plan == nil || plan.state == nil {
		t.Fatalf("unsupported producer branch rejected plan: %v/%v diagnostics=%+v", status, plan, diagnostics)
	}
	defer plan.Close()
	if len(mustResultGeometry(t, plan.state).branchObservations) != 0 {
		t.Fatalf("unsupported producer branch escaped mounted result: %d rows", len(mustResultGeometry(t, plan.state).branchObservations))
	}
}
