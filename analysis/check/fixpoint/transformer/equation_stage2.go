package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
)

// stage2EquationBindings is the complete frozen catalog-to-kernel ownership
// table. Every entry identifies an existing transformer/factapply transaction;
// it does not provide transfer semantics or a State fallback.
var stage2EquationBindings = []struct {
	kind   OperatorKind
	kernel string
}{
	{OperatorApply, "transformer/formal-apply/v1"},
	{OperatorPathReplacement, "transformer/formal-path-replacement/v1"},
	{OperatorPathInvalidation, "transformer/formal-path-invalidation/v1"},
	{OperatorIndexMutation, "transformer/formal-index-mutation/v1"},
	{OperatorAllocationTemplate, "transformer/formal-allocation-template/v1"},
	{OperatorObjectMaterialization, "transformer/formal-object-materialization/v1"},
	{OperatorEnvironmentWrite, "transformer/formal-environment-write/v1"},
	{OperatorChannelSelect, "transformer/formal-channel-select/v1"},
	{OperatorBranchRelations, FormalBranchRelationsKernelID},
	{OperatorCallResults, "transformer/formal-call-results/v1"},
	{OperatorPresenceImplications, "transformer/formal-presence-implications/v1"},
	{OperatorLoopControl, formalLoopControlEquationKernel},
	{OperatorGenericFor, formalGenericForEquationKernel},
	{OperatorRootAssignment, FormalRootAssignmentEquationKernel},
	{OperatorCovariantExposure, "transformer/formal-covariant-exposure/v1"},
	{OperatorContribution, "transformer/formal-contribution/v1"},
	{OperatorExternalCall, ExternalCallEquationKernelID},
	{OperatorOutcome, "transformer/formal-outcome/v1"},
	{OperatorNonreturning, "transformer/formal-nonreturning/v1"},
	{OperatorDefinition, "transformer/formal-definition/v1"},
	{OperatorResource, "transformer/formal-resource/v1"},
	{OperatorEntry, "transformer/formal-entry/v1"},
	{OperatorPublication, "transformer/formal-publication/v1"},
}

// Stage2EquationCompiler installs one non-nil mechanical lowering for every
// frozen operation kind. It is the stage-2 surface used to verify that every
// concrete semantic call remains owned by exactly one existing kernel.
func Stage2EquationCompiler() (*equation.Compiler, error) {
	if len(stage2EquationBindings) != len(FrozenOperatorKinds()) {
		return nil, fmt.Errorf("transformer: stage-2 equation binding count %d, want %d", len(stage2EquationBindings), len(FrozenOperatorKinds()))
	}
	compiler := equation.Skeleton()
	for index, binding := range stage2EquationBindings {
		if binding.kind != FrozenOperatorKinds()[index] || binding.kernel == "" {
			return nil, fmt.Errorf("transformer: stage-2 equation binding %d is incomplete or out of catalog order", index)
		}
		var err error
		compiler, err = compiler.With(string(binding.kind), equation.BindExistingKernel(binding.kernel))
		if err != nil {
			return nil, err
		}
	}
	return compiler, nil
}
