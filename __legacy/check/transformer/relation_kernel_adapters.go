package transformer

import (
	"bytes"
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
)

// RelationOccurrenceKernel is the production-owned canonical transaction for
// one occurrence. The adapter supplies the already-bound operands and prior
// VM partition; it neither solves a relation program nor implements transfer
// semantics itself.
type RelationOccurrenceKernel func(BoundRelationOccurrence, equation.BoundEquation, equation.Partition) (equation.TransactionResult, error)

// KernelRegistry adapts one supplied canonical kernel per real occurrence to
// the VM's exact (kernel ID, contract ID) lookup. It is deliberately an
// adapter only: the supplied kernel remains the execution authority.
func (b *RealRelationBodyBinding) KernelRegistry(kernels map[uint64]RelationOccurrenceKernel) (*equation.KernelRegistry, error) {
	if b == nil || b.program == nil || len(kernels) != len(b.occurrences) {
		return nil, fmt.Errorf("transformer: relation kernel adapters are incomplete")
	}
	bindings := make([]equation.KernelBinding, 0, len(b.occurrences))
	for _, occurrence := range b.Occurrences() {
		kernel := kernels[occurrence.Ordinal]
		contract, ok := b.contractsForOccurrence(occurrence.Ordinal)
		if !ok || kernel == nil {
			return nil, fmt.Errorf("transformer: relation kernel adapter has no canonical kernel for occurrence %d", occurrence.Ordinal)
		}
		kernelID, ok := stage2KernelForKind(occurrence.Kind)
		if !ok {
			return nil, fmt.Errorf("transformer: relation kernel adapter has no stage-2 kernel for %q", occurrence.Kind)
		}
		boundOccurrence, boundContract, boundKernel := occurrence, contract, kernel
		bindings = append(bindings, equation.KernelBinding{
			KernelID:   kernelID,
			ContractID: equation.ContentID(boundContract.ContentID()),
			Kernel: equation.KernelFunc(func(bound equation.BoundEquation, partition equation.Partition) (equation.TransactionResult, error) {
				if err := validateRelationKernelOperands(boundOccurrence, bound); err != nil {
					return equation.TransactionResult{}, err
				}
				return boundKernel(boundOccurrence, bound, partition)
			}),
			Verify: func(access equation.AccessRecord) error {
				return VerifyLoweredOperatorAccess(boundContract, equation.Execution{Complete: true, Access: access})
			},
		})
	}
	return equation.NewKernelRegistry(bindings)
}

func stage2KernelForKind(kind OperatorKind) (string, bool) {
	for _, binding := range stage2EquationBindings {
		if binding.kind == kind {
			return binding.kernel, true
		}
	}
	return "", false
}

func validateRelationKernelOperands(occurrence BoundRelationOccurrence, bound equation.BoundEquation) error {
	if bound.Target != occurrence.Target || len(bound.Operands) != len(occurrence.Operands) {
		return fmt.Errorf("transformer: relation kernel received foreign bound occurrence %d", occurrence.Ordinal)
	}
	for index, expected := range occurrence.Operands {
		actual := bound.Operands[index]
		if actual.Role != string(expected.Role) {
			return fmt.Errorf("transformer: relation kernel occurrence %d has foreign %s operand", occurrence.Ordinal, expected.Role)
		}
		// Entry is deliberately substituted by equation.BindEntry; every other
		// runtime slot must remain byte-for-byte the production slot selected by
		// the total binder.
		if expected.Role != AccessEntry && !bytes.Equal(actual.Value, expected.Value) {
			return fmt.Errorf("transformer: relation kernel occurrence %d has unbound %s operand", occurrence.Ordinal, expected.Role)
		}
	}
	return nil
}
