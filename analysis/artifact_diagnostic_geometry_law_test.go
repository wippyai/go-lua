package analysis

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/result"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/internal/testfixture"
)

func mustResultGeometry(t *testing.T, state *compiledState) result.Geometry {
	t.Helper()
	if state == nil {
		t.Fatal("compiled state")
	}
	geometry, ok := state.resultGeometry()
	if !ok {
		t.Fatal("result geometry")
	}
	return geometry
}

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
				transfers, transfersOK := diagnosticLocalTransfersByDestination(mount.snapshot)
				if !transfersOK {
					t.Fatal("sealed LocalTransfer index unavailable")
				}
				for _, observation := range mustResultGeometry(t, plan.state).BranchObservations {
					if observation.Mount != mount.moduleKey {
						continue
					}
					for _, producer := range observation.Producers {
						if producer.Key != testCase.key {
							continue
						}
						found = true
						if producer.Point == producer.Anchor {
							t.Fatalf("producer %x was not retained at its Local execution point", producer.Occurrence)
						}
						edge, edgeOK := transfers[producer.Point]
						anchor, anchorOK := diagnosticEvidenceAnchor(observation.Points, producer.Point, transfers)
						if !edgeOK || !edge.Full() || edge.To() != producer.Point || !anchorOK || anchor != producer.Anchor {
							t.Fatalf("producer geometry lost exact Program->Local chain: producer=%x anchor=%x/%x execution=%x edge=%+v/%t", producer.Occurrence, producer.Anchor, anchor, producer.Point, edge, edgeOK)
						}
						baseEvidence := false
						for _, point := range observation.Points {
							if point == producer.Anchor {
								baseEvidence = true
								break
							}
						}
						if !baseEvidence {
							t.Fatalf("producer anchor %x is not a Program evidence point", producer.Anchor)
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
	snapshot := artifacts.mounts[0].snapshot
	var full ingress.LocalTransfer
	fullOK := false
	for index := 0; index < snapshot.LocalTransferCount(); index++ {
		candidate, candidateOK := snapshot.LocalTransferAt(index)
		if candidateOK && candidate.Full() {
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
	transfers := make(map[identity.ContentID]ingress.LocalTransfer)
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
	if len(mustResultGeometry(t, plan.state).BranchObservations) != 0 {
		t.Fatalf("unsupported producer branch escaped mounted result: %d rows", len(mustResultGeometry(t, plan.state).BranchObservations))
	}
}

// diagnosticEvidenceAnchor resolves a mounted producer's execution point to
// the Program-issued base evidence point used by the branch observation. A
// producer may execute directly at that base point, or after an acyclic chain
// of exact Local stages whose sealed full-environment transfers lead back to
// that base point.
func diagnosticEvidenceAnchor(evidence []identity.ContentID, execution identity.ContentID, transfers map[identity.ContentID]ingress.LocalTransfer) (identity.ContentID, bool) {
	if !execution.Available() || len(evidence) == 0 {
		return identity.ContentID{}, false
	}
	for _, point := range evidence {
		if execution == point {
			return point, true
		}
	}
	seen := make(map[identity.ContentID]struct{}, len(transfers))
	current := execution
	for steps := 0; steps <= len(transfers); steps++ {
		if _, duplicate := seen[current]; duplicate {
			return identity.ContentID{}, false
		}
		seen[current] = struct{}{}
		edge, found := transfers[current]
		if !found || !edge.Available() || !edge.Full() || edge.To() != current {
			return identity.ContentID{}, false
		}
		current = edge.From()
		for _, point := range evidence {
			if current == point {
				return point, true
			}
		}
	}
	return identity.ContentID{}, false
}

func diagnosticLocalTransfersByDestination(snapshot *ingress.Snapshot) (map[identity.ContentID]ingress.LocalTransfer, bool) {
	if snapshot == nil || !snapshot.Available() {
		return nil, false
	}
	transfers := make(map[identity.ContentID]ingress.LocalTransfer, snapshot.LocalTransferCount())
	for index := 0; index < snapshot.LocalTransferCount(); index++ {
		edge, edgeOK := snapshot.LocalTransferAt(index)
		if !edgeOK || !addDiagnosticFullLocalTransfer(transfers, edge) {
			return nil, false
		}
	}
	return transfers, true
}

func addDiagnosticFullLocalTransfer(transfers map[identity.ContentID]ingress.LocalTransfer, edge ingress.LocalTransfer) bool {
	if transfers == nil || !edge.Available() {
		return false
	}
	if !edge.Full() {
		return true
	}
	if _, duplicate := transfers[edge.To()]; duplicate {
		return false
	}
	transfers[edge.To()] = edge
	return true
}
