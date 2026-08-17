package analysis

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/composite"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/library/lualib/targetprofile"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
)

// TestDiagnosticBranchGeometryUsesExecutionPointAndBaseAnchor proves that
// local storage and equality rules attach at their Local execution point while
// the receipt/collector continues to index the Program-issued base evidence.
// The two coordinates are related only by the sealed full-environment
// LocalTransfer row; no lower layer reconstructs the relationship.
func TestDiagnosticBranchGeometryUsesExecutionPointAndBaseAnchor(t *testing.T) {
	cases := []struct {
		name   string
		source string
		role   programartifact.RuleRole
	}{
		{
			name: "storage-read",
			role: programartifact.RuleRoleValueStorageTransfer,
			source: `local flag = true
if flag then
    return 1
end
return 0`,
		},
		{
			name: "binary-arithmetic",
			role: programartifact.RuleRoleValueBinaryArithmetic,
			source: `local cap = 3
if cap + 2 then
    return 1
end
return 0`,
		},
		{
			name: "binary-equality",
			role: programartifact.RuleRoleValueBinaryEquality,
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
			role: programartifact.RuleRoleValueBinaryOrder,
			source: `local cap = 3
if cap > 5 then
    return 1
end
return 0`,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			contract, err := profile.Contract()
			if err != nil {
				t.Fatal(err)
			}
			linked := mustLink(t, testCase.source, contract)
			plan, status, diagnostics := CompileWithDiagnostics(linked)
			if status != CompileComplete || plan == nil || plan.state == nil || plan.state.artifacts == nil || plan.state.resultReceipt == nil {
				t.Fatalf("CompileWithDiagnostics = %v/%v diagnostics=%+v", status, plan, diagnostics)
			}
			defer plan.Close()

			found := false
			for _, mount := range plan.state.artifacts.mounts {
				transfers, transfersOK := diagnosticLocalTransfersByDestination(mount.artifact)
				if !transfersOK {
					t.Fatal("sealed LocalTransfer index unavailable")
				}
				for _, observation := range plan.state.resultReceipt.branchObservations {
					if observation.mount != mount.moduleKey {
						continue
					}
					for _, producer := range observation.producers {
						if producer.role != testCase.role {
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
				t.Fatalf("no mounted branch producer for role %d", testCase.role)
			}
		})
	}
}

// TestDiagnosticBranchGeometryRejectsUnprovenTransfer keeps the fail-closed
// rule local to the geometry wrapper: missing transfer, partial transport,
// and duplicate full destinations cannot be guessed into a branch receipt.
func TestDiagnosticBranchGeometryRejectsUnprovenTransfer(t *testing.T) {
	contract, err := profile.Contract()
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
	contract, err := profile.Contract()
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
	if status != CompileComplete || plan == nil || plan.state == nil || plan.state.resultReceipt == nil {
		t.Fatalf("unsupported producer branch rejected plan: %v/%v diagnostics=%+v", status, plan, diagnostics)
	}
	defer plan.Close()
	if len(plan.state.resultReceipt.branchObservations) != 0 {
		t.Fatalf("unsupported producer branch escaped mounted receipt: %d rows", len(plan.state.resultReceipt.branchObservations))
	}
}
