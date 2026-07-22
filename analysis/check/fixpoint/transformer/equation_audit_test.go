package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/domain/formal"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

func TestEnvironmentWriteLoweringAuditsRecordedExecution(t *testing.T) {
	owner := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte("environment-write-equation-audit")))
	occurrence := formal.NewOccurrenceID(owner, 1)
	contract, err := NewOperatorContract(OperatorEnvironmentWrite, occurrence)
	if err != nil {
		t.Fatal(err)
	}
	contract.Reads = []ContractSelector{{Role: AccessFlow, Name: "flow"}, {Role: AccessState, Name: "values"}}
	contract.Writes = []ContractSelector{{Role: AccessState, Name: "target"}}

	access := OperatorAccess{Kind: OperatorEnvironmentWrite, Occurrence: occurrence, Reads: append([]ContractSelector(nil), contract.Reads...), Writes: append([]ContractSelector(nil), contract.Writes...)}
	execution := equation.Execution{Complete: true, Published: true, Access: equation.AccessRecord{Payload: access}}
	if err := VerifyLoweredOperatorAccess(contract, execution); err != nil {
		t.Fatalf("environment-write audit: %v", err)
	}

	access.Writes = append(access.Writes, ContractSelector{Role: AccessState, Name: "undeclared"})
	execution.Access.Payload = access
	if err := VerifyLoweredOperatorAccess(contract, execution); err == nil {
		t.Fatal("environment-write audit accepted undeclared write")
	}
}

func TestApplyLoweringAuditsCompleteFactApplyAccess(t *testing.T) {
	owner := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte("apply-equation-audit")))
	occurrence := formal.NewOccurrenceID(owner, 1)
	contract, err := NewOperatorContract(OperatorApply, occurrence)
	if err != nil {
		t.Fatal(err)
	}
	// These selectors are the complete frozen Apply boundary: the caller
	// predecessor is correlated with stabilized callee outcomes through the
	// application guard and frame, then publishes caller-owned results and
	// residual diagnostics in one transaction.
	contract.Reads = []ContractSelector{
		{Role: AccessFlow, Name: "caller-predecessor"},
		{Role: AccessCalleeOutcome, Name: "stabilized-callee-outcome"},
		{Role: AccessGuard, Name: "application-guard"},
		{Role: AccessBoundary, Name: "apply-frame"},
		{Role: AccessAllocation, Name: "boundary-allocation"},
	}
	contract.Writes = []ContractSelector{
		{Role: AccessState, Name: "caller-result"},
		{Role: AccessState, Name: "caller-heap"},
		{Role: AccessDiagnostic, Name: "boundary-diagnostic"},
	}
	contract.Outcomes = []OutcomeKind{OutcomeNormal}
	contract.Dependencies = []ContractDependency{{Kind: "operator-contract-catalog", ID: FrozenOperatorContractCatalog().ContentID()}}

	access := OperatorAccess{
		Kind:         OperatorApply,
		Occurrence:   occurrence,
		Reads:        append([]ContractSelector(nil), contract.Reads...),
		Writes:       append([]ContractSelector(nil), contract.Writes...),
		Outcomes:     append([]OutcomeKind(nil), contract.Outcomes...),
		Dependencies: append([]ContractDependency(nil), contract.Dependencies...),
	}
	execution := equation.Execution{Complete: true, Published: true, Access: equation.AccessRecord{Payload: access}}
	if err := VerifyLoweredOperatorAccess(contract, execution); err != nil {
		t.Fatalf("apply audit: %v", err)
	}

	access.Reads = append(access.Reads, ContractSelector{Role: AccessBoundary, Name: "undeclared-boundary"})
	execution.Access.Payload = access
	if err := VerifyLoweredOperatorAccess(contract, execution); err == nil {
		t.Fatal("apply audit accepted undeclared boundary read")
	}

	// A factapply transaction is full-result-or-nothing: the audit harness must
	// reject a record that claims to publish any incomplete transaction.
	execution = equation.Execution{Complete: false, Published: true, Access: equation.AccessRecord{Payload: OperatorAccess{Kind: OperatorApply, Occurrence: occurrence}}}
	if err := VerifyLoweredOperatorAccess(contract, execution); err == nil {
		t.Fatal("apply audit accepted partial publication")
	}
}
