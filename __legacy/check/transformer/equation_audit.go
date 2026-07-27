package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
)

type Execution struct {
	Complete  bool
	Published bool
	Access    equation.AccessRecord
}

// VerifyLoweredOperatorAccess connects the Stage-2 audit harness to the
// frozen contract verifier.  The record is opaque until this adapter recovers
// the existing OperatorAccess; verification is strictly post-execution.
func VerifyLoweredOperatorAccess(contract OperatorContract, execution Execution) error {
	if !execution.Complete {
		return fmt.Errorf("transformer: partial execution")
	}
	access, ok := execution.Access.Payload.(OperatorAccess)
	if !ok {
		return fmt.Errorf("transformer: lowered audit did not record OperatorAccess")
	}
	return contract.VerifyAccess(access)
}
