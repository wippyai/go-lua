package callpayload

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

var benchmarkCallOutcomeRepresentationEqualResult bool

// BenchmarkCallOutcomeRepresentationEqual covers the raw provider payload
// shape used by formal external-call alternatives: result values, diagnostic
// facts, value/path refinements, and placement facts are all present while the
// two independently-owned payloads retain equal representation.
func BenchmarkCallOutcomeRepresentationEqual(b *testing.B) {
	reg := standard.Registry()
	left := benchmarkCallOutcomeRepresentation(reg)
	right := left.Clone()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		benchmarkCallOutcomeRepresentationEqualResult = CallOutcomeRepresentationEqual(left, right)
	}
}

func benchmarkCallOutcomeRepresentation(reg *axis.Registry) CallOutcome {
	value := product.Bottom(reg)
	p0, p1 := pathdom.NewPlaceholder(0), pathdom.NewPlaceholder(1)
	return CallOutcome{
		Results:             []CallResult{{Index: 0, Value: value}, {Index: 1, Value: product.Top()}},
		PostReturnAuthority: true,
		SuspensionKnown:     true,
		MaySuspend:          true,
		Placements: map[identity.ID]placement.Value{
			identity.LuaFunction(1): placement.OwnedHeap,
		},
		ParamObligations: []CallParamObligation{{
			ParamIndex: 0, Value: value,
			Origin:           CallParamObligationOrigin{HasOrigin: true, ReceiverParam: 1, ArgParam: 0, SubjectLabel: "subject", ProviderLabel: "provider"},
			SignatureSurface: true,
		}},
		PathObligations:            []CallPathObligation{{Path: p0, Value: value}},
		ParamPathRefinements:       []CallParamPathRefinement{{Path: p0, Value: value}},
		ParamPathWrites:            []CallParamPathWrite{{Path: p1, Value: value}},
		ParamLengthFloors:          []CallParamLengthFloor{{Path: p0, Floor: 2}},
		ParamPathInvalidations:     []CallParamPathInvalidation{{Path: p1, PreserveStructuralWitness: true}},
		ParamConditions:            []CallParamCondition{{ParamIndex: 0, Value: true}},
		ParamPathRelations:         []CallParamPathRelation{{Kind: CallPathRelationEqual, Left: p0, Right: p1}},
		ReturnConditionRefinements: []CallReturnConditionRefinement{{ReturnIndex: 0, ReturnValue: true, Target: p0, Value: value}},
		ReturnConditionSlots:       []CallReturnConditionSlotRefinement{{ReturnIndex: 0, ReturnValue: false, TargetIndex: 1, Value: value}},
		ReturnPresenceRelations:    []CallReturnPresenceRelation{{TriggerIndex: 0, TargetIndex: 1}},
		ParamExposures:             []CallParamExposure{{Source: p0, Contract: value}},
	}
}
