package factapply

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestRootAssignmentStableImplicationRewritePreservesOnlyProvenFacts(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(611)
	target := symbol.ID(611)
	other := symbol.ID(612)
	targetPath := path.NewPath(target, "target")
	resolverBuilder := visibility.NewBuilder()
	resolverBuilder.Define(point, target, "target")
	resolverBuilder.Define(point, other, "other")
	resolver := visibility.NewResolver(resolverBuilder.Build())
	keys := resolver.KeySpace()
	targetKey := keys.FromPath(path.NewPath(target, ""))
	otherKey := keys.FromPath(path.NewPath(other, ""))
	preserved := pathevidence.NewPathPresenceImplication(otherKey, presence.Present(), targetKey, presence.Present())
	dropped := pathevidence.NewPathPresenceImplication(targetKey, presence.Present(), otherKey, presence.Present())
	selfRoot := pathevidence.NewPathPresenceImplication(targetKey, presence.Present(), targetKey, presence.Present())
	unrelated := pathevidence.NewPathPresenceImplication(otherKey, presence.Present(), otherKey, presence.Absent())
	in := state.State{}.
		AddPathPresenceImplication(preserved).
		AddPathPresenceImplication(dropped).
		AddPathPresenceImplication(selfRoot).
		AddPathPresenceImplication(unrelated)
	got := writeRootSymbol(transfer.NodeContext{Registry: reg, Point: point}, resolver, in, target, targetPath, presentValue(reg), false)
	if !got.HasPathPresenceImplication(preserved) || got.HasPathPresenceImplication(dropped) || got.HasPathPresenceImplication(selfRoot) || !got.HasPathPresenceImplication(unrelated) {
		t.Fatal("root assignment did not atomically retain exactly the still-valid stable implications")
	}
}

func BenchmarkRootAssignmentStableImplicationRewrite(b *testing.B) {
	reg := standard.Registry()
	for _, width := range []int{64, 1024, 4096} {
		for _, mode := range []struct {
			name       string
			idempotent bool
		}{{name: "idempotent", idempotent: true}, {name: "changed"}} {
			b.Run(fmt.Sprintf("%s/total-%d/matching-2", mode.name, width), func(b *testing.B) {
				point := cfg.Point(701)
				target := symbol.ID(701)
				targetPath := path.NewPath(target, "target")
				resolverBuilder := visibility.NewBuilder()
				resolverBuilder.Define(point, target, "target")
				resolver := visibility.NewResolver(resolverBuilder.Build())
				keys := resolver.KeySpace()
				targetKey := keys.FromPath(path.NewPath(target, ""))
				in := state.State{}
				for index := 0; index < width; index++ {
					trigger := symbol.ID(10000 + index*2)
					other := symbol.ID(10001 + index*2)
					in = in.AddPathPresenceImplication(pathevidence.NewPathPresenceImplication(
						keys.FromPath(path.NewPath(trigger, "")), presence.Present(),
						keys.FromPath(path.NewPath(other, "")), presence.Present(),
					))
				}
				trigger := keys.FromPath(path.NewPath(symbol.ID(9001), ""))
				in = in.AddPathPresenceImplication(pathevidence.NewPathPresenceImplication(trigger, presence.Present(), targetKey, presence.Present()))
				in = in.AddPathPresenceImplication(pathevidence.NewPathPresenceImplication(targetKey, presence.Present(), trigger, presence.Present()))
				ctx := transfer.NodeContext{Registry: reg, Point: point}
				assigned := presentValue(reg)
				if mode.idempotent {
					in = in.WriteValue(reg, key.SymbolValue(target), assigned)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					_ = writeRootSymbol(ctx, resolver, in, target, targetPath, assigned, false)
				}
			})
		}
	}
}

func TestPathPresenceImplicationActivationAcceptsRootTarget(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(62)
	trigger := symbol.ID(261)
	target := symbol.ID(262)
	triggerPath := path.NewPath(trigger, "use_template")
	targetPath := path.NewPath(target, "executor")

	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, trigger, "use_template")
	visibilityBuilder.Define(point, target, "executor")
	resolver := visibility.NewResolver(visibilityBuilder.Build())

	triggerKey := resolver.KeySpace().FromPath(triggerPath)
	targetKey := resolver.KeySpace().FromPath(targetPath)
	falseValue := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.False), typ.False)
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(trigger), falseValue).
		WriteValue(reg, key.SymbolValue(target), product.Top()).
		AddPathPresenceImplication(pathevidence.NewPathValuePresenceImplication(
			triggerKey,
			falseValue,
			targetKey,
			presence.Present(),
		))
	got := activatePathPresenceImplications(reg, resolver, point, in)

	assertValue(t, reg, got, key.SymbolValue(target), presentValue(reg))
}

func TestPathPresenceImplicationActivationAcceptsPathEqualityTrigger(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(63)
	result := symbol.ID(361)
	events := symbol.ID(362)
	resultPath := path.NewPath(result, "selected")
	channelPath := resultPath.Field("channel")
	valuePath := resultPath.Field("value")
	eventsPath := path.NewPath(events, "events")

	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, result, "selected")
	visibilityBuilder.Define(point, events, "events")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	triggerKey, triggerOK := visibility.AddressAt(resolver, point, channelPath).RootOrVisibleKeyspaceKey()
	otherKey, otherOK := visibility.AddressAt(resolver, point, eventsPath).RootOrVisibleKeyspaceKey()
	targetKey, targetOK := visibility.AddressAt(resolver, point, valuePath).RootOrVisibleKeyspaceKey()
	if !triggerOK || !otherOK || !targetOK {
		t.Fatalf("missing implication keys: trigger=%v other=%v target=%v", triggerOK, otherOK, targetOK)
	}
	payloadType := typetable.NewRecord().Field("id", typ.String).Build()
	payloadValue := typevalue.WithWitness(reg, typevalue.FromType(reg, payloadType), payloadType)
	implication := pathevidence.NewPathEqualValueRefinementImplication(
		triggerKey,
		otherKey,
		targetKey,
		payloadValue,
	)
	in := state.State{}.
		AddBranchProof(pathevidence.BranchProof{
			Kind:  pathevidence.BranchProofPathEqual,
			Path:  triggerKey,
			Other: otherKey,
		}).
		AddPathPresenceImplication(implication)

	got := activatePathPresenceImplications(reg, resolver, point, in)

	assertPathValue(t, reg, resolver.KeySpace(), got, resolver.KeySpace().Format(targetKey), payloadValue)
}

func TestPathPresenceImplicationActivationRefinesEquivalentTargetPath(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(64)
	result := symbol.ID(363)
	events := symbol.ID(364)
	msg := symbol.ID(365)
	resultPath := path.NewPath(result, "selected")
	channelPath := resultPath.Field("channel")
	valuePath := resultPath.Field("value")
	eventsPath := path.NewPath(events, "events")
	msgPath := path.NewPath(msg, "msg")

	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, result, "selected")
	visibilityBuilder.Define(point, events, "events")
	visibilityBuilder.Define(point, msg, "msg")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	triggerKey, triggerOK := visibility.AddressAt(resolver, point, channelPath).RootOrVisibleKeyspaceKey()
	otherKey, otherOK := visibility.AddressAt(resolver, point, eventsPath).RootOrVisibleKeyspaceKey()
	targetKey, targetOK := visibility.AddressAt(resolver, point, valuePath).RootOrVisibleKeyspaceKey()
	msgKey, msgOK := visibility.AddressAt(resolver, point, msgPath).VisibleLocalKeyspaceKey()
	if !triggerOK || !otherOK || !targetOK || !msgOK {
		t.Fatalf("missing implication keys: trigger=%v other=%v target=%v msg=%v", triggerOK, otherOK, targetOK, msgOK)
	}
	payloadType := typetable.NewRecord().Field("data", typ.String).Build()
	payloadValue := typevalue.WithWitness(reg, typevalue.FromType(reg, payloadType), payloadType)
	implication := pathevidence.NewPathEqualValueRefinementImplication(
		triggerKey,
		otherKey,
		targetKey,
		payloadValue,
	)
	in := state.State{}.
		AddBranchProof(pathevidence.BranchProof{
			Kind:  pathevidence.BranchProofPathEqual,
			Path:  triggerKey,
			Other: otherKey,
		}).
		AddBranchProof(pathevidence.BranchProof{
			Kind:  pathevidence.BranchProofPathEqual,
			Path:  targetKey,
			Other: msgKey,
		}).
		AddPathPresenceImplication(implication)

	got := activatePathPresenceImplications(reg, resolver, point, in)

	assertPathValue(t, reg, resolver.KeySpace(), got, resolver.KeySpace().Format(targetKey), payloadValue)
	assertValue(t, reg, got, key.SymbolValue(msg), payloadValue)
}
