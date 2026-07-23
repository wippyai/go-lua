package transformer

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestProjectLexicalParamOutcomeFactsUsesStabilizedRootsAndMustEquality(t *testing.T) {
	reg := standard.Registry()
	conditions, relations := projectLexicalParamOutcomeFacts(reg, state.BoundaryRoots{
		{Value: typevalue.LiteralBool(reg, true)},
		{Value: typevalue.LiteralBool(reg, false)},
	}, 2, callboundary.NormalReturnFacts{BranchProofs: []callboundary.BranchProof{{
		Kind: pathevidence.BranchProofPathEqual, Path: pathdom.NewPlaceholder(0), Other: pathdom.NewPlaceholder(1),
	}}})
	if len(conditions) != 2 || conditions[0] != (callpayload.CallParamCondition{ParamIndex: 0, Value: true}) ||
		conditions[1] != (callpayload.CallParamCondition{ParamIndex: 1, Value: false}) {
		t.Fatalf("ParamConditions = %#v", conditions)
	}
	if len(relations) != 1 || relations[0].Kind != callpayload.CallPathRelationEqual ||
		!relations[0].Left.Equal(pathdom.NewPlaceholder(0)) || !relations[0].Right.Equal(pathdom.NewPlaceholder(1)) {
		t.Fatalf("ParamPathRelations = %#v", relations)
	}
}
