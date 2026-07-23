package calloutcome

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	callpayload "github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestHasAuthoritativePostReturnEvidenceCountsPlacementFacts(t *testing.T) {
	tableID := identity.ID{Kind: "table", Site: "compose-placement", Index: 3}
	outcome := callpayload.CallOutcome{
		Placements: map[identity.ID]placement.Value{
			tableID: placement.Stack,
		},
	}
	if !HasAuthoritativePostReturnEvidence(standard.Registry(), outcome) {
		t.Fatal("HasAuthoritativePostReturnEvidence = false, want true for placement facts")
	}
}

func TestHasAuthoritativePostReturnEvidenceCountsParamLengthFloors(t *testing.T) {
	outcome := callpayload.CallOutcome{
		ParamLengthFloors: []callpayload.CallParamLengthFloor{
			{Path: pathdom.NewPlaceholder(0), Floor: 2},
		},
	}
	if !HasAuthoritativePostReturnEvidence(standard.Registry(), outcome) {
		t.Fatal("HasAuthoritativePostReturnEvidence = false, want true for param length floors")
	}
}

func TestHasAuthoritativePostReturnEvidenceCountsLifecycleFacts(t *testing.T) {
	outcome := callpayload.CallOutcome{
		NormalReturnFacts: callboundary.NormalReturnFacts{
			LifecycleFacts: []callboundary.LifecycleFact{
				{
					Target:   pathdom.NewPlaceholder(0),
					Kind:     callboundary.LifecycleTransition,
					Protocol: typestate.Protocol("transaction"),
					From:     typestate.State("active"),
					To:       typestate.State("finished"),
				},
			},
		},
	}
	if !HasAuthoritativePostReturnEvidence(standard.Registry(), outcome) {
		t.Fatal("HasAuthoritativePostReturnEvidence = false, want true for lifecycle facts")
	}
}

func TestHasAuthoritativePostReturnEvidenceRejectsWeakResultSlots(t *testing.T) {
	reg := standard.Registry()
	for name, value := range map[string]product.Value{
		"top":     product.Top(),
		"bottom":  product.Bottom(reg),
		"any":     typevalue.FromType(reg, typ.Any),
		"unknown": typevalue.FromType(reg, typ.Unknown),
	} {
		outcome := callpayload.CallOutcome{Results: []callpayload.CallResult{{Index: 0, Value: value}}}
		if HasAuthoritativePostReturnEvidence(reg, outcome) {
			t.Fatalf("%s result slot was authoritative; weak result evidence must remain supplemental", name)
		}
	}
}

func TestHasAuthoritativePostReturnEvidenceAcceptsSpecificResultSlot(t *testing.T) {
	reg := standard.Registry()
	outcome := callpayload.CallOutcome{
		Results: []callpayload.CallResult{{Index: 0, Value: typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)}},
	}
	if !HasAuthoritativePostReturnEvidence(reg, outcome) {
		t.Fatal("specific string result slot was not authoritative")
	}
}
