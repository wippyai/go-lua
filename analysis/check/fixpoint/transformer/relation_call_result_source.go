package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
)

func bindLinkedCallResultSourcePath(body *relationProgramBody, assignment rootAssignmentTerm, plan factapply.ResolvedRootAssignmentPlan) (factapply.ResolvedRootAssignmentPlan, error) {
	if body == nil || body.keys == nil || !body.keys.Valid() || !plan.Valid() || !assignment.transaction.Valid() {
		return factapply.ResolvedRootAssignmentPlan{}, fmt.Errorf("transformer: linked call-result source path is unowned")
	}
	source, present := assignment.transaction.Source(0)
	if !present || source.Kind != factflow.ValueSourceCall {
		return plan, nil
	}
	if !source.HasCallPoint || source.CallPoint == 0 || source.TargetIndex < 0 {
		return factapply.ResolvedRootAssignmentPlan{}, fmt.Errorf("transformer: linked call-result source is malformed")
	}
	_, path, err := frameCallResultCarrier(body.keys, body.body, source.CallPoint, uint32(source.TargetIndex))
	if err != nil {
		return factapply.ResolvedRootAssignmentPlan{}, err
	}
	return plan.BindResolvedSourcePath(path)
}
