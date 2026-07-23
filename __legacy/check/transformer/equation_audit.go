package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
)

// VerifyLoweredOperatorAccess connects the Stage-2 audit harness to the
// frozen contract verifier.  The record is opaque until this adapter recovers
// the existing OperatorAccess; verification is strictly post-execution.
func VerifyLoweredOperatorAccess(contract OperatorContract, execution equation.Execution) error {
	return equation.RunAndVerify(func() (equation.Execution, error) {
		return execution, nil
	}, func(record equation.AccessRecord) error {
		access, ok := record.Payload.(OperatorAccess)
		if !ok {
			return fmt.Errorf("transformer: lowered audit did not record OperatorAccess")
		}
		return contract.VerifyAccess(access)
	})
}
