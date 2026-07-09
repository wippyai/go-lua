package factapply

import (
	"os"
	"strings"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestNormalReturnApplyLaneRegistryCoversStorageLanes(t *testing.T) {
	storage := callboundary.NormalReturnFactLanes()
	if len(normalReturnApplyLanes) != len(storage) {
		t.Fatalf("normal-return apply lane count = %d, want storage lane count %d", len(normalReturnApplyLanes), len(storage))
	}
	for _, lane := range normalReturnApplyLanes {
		if lane.Value.apply == nil {
			t.Fatal("normal-return apply lane has no apply function")
		}
	}
}

func TestNormalReturnApplyLanesUseCallBoundaryPathBindings(t *testing.T) {
	for _, file := range []string{"normal_return_apply_lanes.go", "call_outcome_apply.go", "call_return_slot_facts.go"} {
		srcBytes, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		src := string(srcBytes)
		for _, forbidden := range []string{
			"substituteCallBoundaryPath",
			"callBoundaryReturnSlotIndex",
			"callBoundaryConcreteSymbolPath",
			"ctx.bindings",
			"ctx.returnBindings",
		} {
			if strings.Contains(src, forbidden) {
				t.Fatalf("%s contains %q; normal-return apply lanes must use callboundary.PathBindings for boundary paths", file, forbidden)
			}
		}
	}
}

func TestNormalReturnBranchProofsUseBranchPathRelationApply(t *testing.T) {
	srcBytes, err := os.ReadFile("normal_return_branch.go")
	if err != nil {
		t.Fatalf("read normal_return_branch.go: %v", err)
	}
	src := string(srcBytes)
	for _, forbidden := range []string{
		"applyBranchPathEquality(",
		"applyBranchPathInequality(",
	} {
		if strings.Contains(src, forbidden) {
			t.Fatalf("normal_return_branch.go contains %q; normal-return branch proofs must flow through applyBranchPathRelation", forbidden)
		}
	}
	if !strings.Contains(src, "applyBranchPathRelation(") {
		t.Fatal("normal_return_branch.go does not call applyBranchPathRelation")
	}
}

func TestNormalReturnPathPresenceImplicationAppliesTargetValue(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(64)
	result := symbol.ID(366)
	resultPath := pathdom.NewPath(result, "result")
	channelPath := resultPath.Field("channel")
	valuePath := resultPath.Field("value")

	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, result, "result")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	triggerKey, triggerOK := visibility.AddressAt(resolver, point, channelPath).RootOrVisibleKeyspaceKey()
	targetKey, targetOK := visibility.AddressAt(resolver, point, valuePath).RootOrVisibleKeyspaceKey()
	if !triggerOK || !targetOK {
		t.Fatalf("missing implication keys: trigger=%v target=%v", triggerOK, targetOK)
	}
	triggerType := typ.LiteralString("recv")
	triggerValue := typevalue.WithWitness(reg, typevalue.FromType(reg, triggerType), triggerType)
	payloadType := typetable.NewRecord().Field("data", typ.String).Build()
	payloadValue := typevalue.WithWitness(reg, typevalue.FromType(reg, payloadType), payloadType)
	ctx := normalReturnApplyContext{
		node:          transfer.NodeContext{Registry: reg, Point: point},
		resolver:      resolver,
		point:         point,
		boundaryPaths: callboundary.NewPathBindings(nil, nil),
		normalFacts: callboundary.NormalReturnFacts{
			PathPresenceImplications: []callboundary.PathPresenceImplicationFact{{
				Trigger:         channelPath,
				TriggerValue:    triggerValue,
				HasTriggerValue: true,
				Target:          valuePath,
				TargetValue:     payloadValue,
				HasTargetValue:  true,
			}},
		},
	}
	ks := resolver.KeySpace()
	in := state.State{}.WritePathKey(reg, ks, ks.Format(triggerKey), triggerValue)

	got := applyNormalReturnPathPresenceImplications(ctx, in)

	assertPathValue(t, reg, ks, got, ks.Format(targetKey), payloadValue)
}
