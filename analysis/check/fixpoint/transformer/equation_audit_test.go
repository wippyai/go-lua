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
