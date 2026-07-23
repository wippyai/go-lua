package transformer

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/domain/formal"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
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

func TestChannelSelectLoweringAuditsExistingFactapplyExecution(t *testing.T) {
	owner := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte("channel-select-equation-audit")))
	occurrence := formal.NewOccurrenceID(owner, 1)
	contract, err := NewOperatorContract(OperatorChannelSelect, occurrence)
	if err != nil {
		t.Fatal(err)
	}
	contract.Reads = []ContractSelector{
		{Role: AccessFlow, Name: "predecessor"},
		{Role: AccessState, Name: "channel-facts"},
		{Role: AccessState, Name: "path-values"},
		{Role: AccessState, Name: "values"},
		{Role: AccessGuard, Name: "select-guard"},
	}
	contract.Writes = []ContractSelector{
		{Role: AccessState, Name: "channel-facts"},
		{Role: AccessState, Name: "result-values"},
	}
	contract.GuardAtoms = []string{"select-guard"}

	reg := standard.Registry()
	point := cfg.Point(17)
	path := pathdom.NewPath(symbol.ID(17), "channel")
	facts := factflow.NewFacts(factflow.FactsInput{ChannelSelects: map[cfg.Point]factflow.ChannelSelectSet{
		point: factflow.NewChannelSelectSet(factflow.NewChannelSelect(factflow.ChannelSelectConfig{
			SelectID: "audit-select", Kind: factflow.ChannelSelectReceive, Index: 0,
			CasePath: path, HasCasePath: true,
		})),
	}})
	prepared, err := factapply.PrepareChannelSelectTransaction(reg, factapply.PlanChannelSelectTransaction(facts, point),
		func(pathdom.Path) (pathaddr.StateKey, bool) { return pathaddr.StateKey("formal:channel"), true },
		func(cfg.Point, int) (string, bool) { return "result", true },
	)
	if err != nil || !prepared.Complete() {
		t.Fatalf("PrepareChannelSelectTransaction = %v, complete=%t", err, prepared.Complete())
	}
	evaluated, err := factapply.EvaluatePreparedChannelSelect(context.Background(), reg, nil, prepared, nil)
	if err != nil || len(evaluated.Facts()) != 1 {
		t.Fatalf("EvaluatePreparedChannelSelect = %v, facts=%d", err, len(evaluated.Facts()))
	}

	access := OperatorAccess{
		Kind:       OperatorChannelSelect,
		Occurrence: occurrence,
		Reads: []ContractSelector{
			{Role: AccessFlow, Name: "predecessor"},
			{Role: AccessState, Name: "channel-facts"},
			{Role: AccessGuard, Name: "select-guard"},
		},
		Writes: []ContractSelector{{Role: AccessState, Name: "channel-facts"}},
	}
	execution := equation.Execution{Complete: true, Published: true, Access: equation.AccessRecord{Payload: access}}
	if err := VerifyLoweredOperatorAccess(contract, execution); err != nil {
		t.Fatalf("channel-select audit: %v", err)
	}

	access.Reads = append(access.Reads, ContractSelector{Role: AccessState, Name: "undeclared"})
	execution.Access.Payload = access
	if err := VerifyLoweredOperatorAccess(contract, execution); err == nil {
		t.Fatal("channel-select audit accepted undeclared read")
	}

	execution = equation.Execution{Complete: false, Published: true, Access: equation.AccessRecord{Payload: OperatorAccess{Kind: OperatorChannelSelect, Occurrence: occurrence}}}
	if err := VerifyLoweredOperatorAccess(contract, execution); err == nil {
		t.Fatal("channel-select lowering published a partial transaction")
	}
}

func TestRootAssignmentLoweringAuditsRecordedExecution(t *testing.T) {
	owner := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte("root-assignment-equation-audit")))
	occurrence := formal.NewOccurrenceID(owner, 1)
	contract, err := NewOperatorContract(OperatorRootAssignment, occurrence)
	if err != nil {
		t.Fatal(err)
	}
	contract.Reads = []ContractSelector{
		{Role: AccessFlow, Name: "predecessor"},
		{Role: AccessNodeEntry, Name: "point-entry"},
		{Role: AccessState, Name: "values/current"},
		{Role: AccessGuard, Name: "assignment-guard"},
	}
	contract.Writes = []ContractSelector{{Role: AccessState, Name: "values/target"}}
	contract.GuardAtoms = []string{"assignment-guard"}
	contract.Outcomes = []OutcomeKind{OutcomeNormal}

	access := OperatorAccess{
		Kind: OperatorRootAssignment, Occurrence: occurrence,
		Reads: append([]ContractSelector(nil), contract.Reads...), Writes: append([]ContractSelector(nil), contract.Writes...),
		Outcomes: append([]OutcomeKind(nil), contract.Outcomes...),
	}
	execution := equation.Execution{Complete: true, Published: true, Access: equation.AccessRecord{Payload: access}}
	if err := VerifyLoweredOperatorAccess(contract, execution); err != nil {
		t.Fatalf("root-assignment audit: %v", err)
	}

	access.Reads = append(access.Reads, ContractSelector{Role: AccessState, Name: "undeclared-reduction"})
	execution.Access.Payload = access
	if err := VerifyLoweredOperatorAccess(contract, execution); err == nil {
		t.Fatal("root-assignment audit accepted undeclared read")
	}
}
