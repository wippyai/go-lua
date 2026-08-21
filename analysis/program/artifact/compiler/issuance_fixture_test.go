package compiler

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/artifact/issuance"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

func transportDirectory(t *testing.T, placements ...issuance.Placement) issuance.Directory {
	t.Helper()
	directory, ok := issuance.NewDirectory(placements,
		map[issuance.Form]string{
			issuance.FormLocal:            pinnedLocalStageFraming,
			issuance.FormComputation:      pinnedLocalComputationStageFraming,
			issuance.FormLocalPredecessor: pinnedLocalPredecessorStageFraming,
			issuance.FormLocalSuccessor:   "analysis/program-artifact/local-successor-stage",
		},
		map[programschema.RuleStage]string{
			programschema.RuleStageCallDispatch: pinnedCallDispatchStageFraming,
			programschema.RuleStageCallSummary:  pinnedCallSummaryStageFraming,
			programschema.RuleStageCallEffect:   pinnedCallEffectStageFraming,
		})
	if !ok {
		t.Fatal("the declared placements and framings were refused admission")
	}
	return directory
}
