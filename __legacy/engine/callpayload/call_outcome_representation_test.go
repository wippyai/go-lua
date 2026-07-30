package callpayload

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
)

// TestCallOutcomeRepresentationEqualMatchesPreChangeBehavior exercises each
// public CallOutcome field class. reflect.DeepEqual is the behavior this API
// exposed before the allocation-free fast path; this corpus therefore guards
// raw representation equality rather than normalized lattice equality.
func TestCallOutcomeRepresentationEqualMatchesPreChangeBehavior(t *testing.T) {
	reg := standard.Registry()
	base := benchmarkCallOutcomeRepresentation(reg)
	id := identity.LuaFunction(9)
	p0 := pathdom.NewPlaceholder(0)
	value := product.Bottom(reg)

	type pair struct{ left, right CallOutcome }
	cases := map[string]pair{
		"equal detached payload": {base, base.Clone()},
		"results":                mutationPair(base, func(out *CallOutcome) { out.Results[0].Index++ }),
		"post return authority":  mutationPair(base, func(out *CallOutcome) { out.PostReturnAuthority = false }),
		"suspension known":       mutationPair(base, func(out *CallOutcome) { out.SuspensionKnown = false }),
		"may suspend":            mutationPair(base, func(out *CallOutcome) { out.MaySuspend = false }),
		"normal return facts": {
			CallOutcome{NormalReturnFacts: callboundary.NormalReturnFacts{PathRefinements: []callboundary.PathValueFact{{Path: p0, Value: value}}}},
			CallOutcome{NormalReturnFacts: callboundary.NormalReturnFacts{PathRefinements: []callboundary.PathValueFact{{Path: p0, Value: product.Top()}}}},
		},
		"normal return nil versus empty lane": {
			CallOutcome{},
			CallOutcome{NormalReturnFacts: callboundary.NormalReturnFacts{PathRefinements: []callboundary.PathValueFact{}}},
		},
		"protected typestate": {
			CallOutcome{ProtectedCallTypestate: callboundary.ProtectedCallTypestate{HasNormal: true}},
			CallOutcome{ProtectedCallTypestate: callboundary.ProtectedCallTypestate{HasExceptional: true}},
		},
		"heap table objects": {
			CallOutcome{HeapTableObjects: map[identity.ID]heapidentity.TableObject{id: heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: value})}},
			CallOutcome{HeapTableObjects: map[identity.ID]heapidentity.TableObject{id: heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: product.Top()})}},
		},
		"placements":        mutationPair(base, func(out *CallOutcome) { out.Placements[id] = out.Placements[identity.LuaFunction(1)] }),
		"param obligations": mutationPair(base, func(out *CallOutcome) { out.ParamObligations[0].ParamIndex++ }),
		"path obligations":  mutationPair(base, func(out *CallOutcome) { out.PathObligations[0].Path.Root = "$different" }),
		"typestate requirements": {
			CallOutcome{TypestateRequirements: []CallTypestateRequirement{{Target: p0, Protocol: typestate.Protocol("p"), State: typestate.State("a")}}},
			CallOutcome{TypestateRequirements: []CallTypestateRequirement{{Target: p0, Protocol: typestate.Protocol("p"), State: typestate.State("b")}}},
		},
		"param path refinements":       mutationPair(base, func(out *CallOutcome) { out.ParamPathRefinements[0].Value = product.Top() }),
		"param path writes":            mutationPair(base, func(out *CallOutcome) { out.ParamPathWrites[0].Value = product.Top() }),
		"param length floors":          mutationPair(base, func(out *CallOutcome) { out.ParamLengthFloors[0].Floor++ }),
		"param path invalidations":     mutationPair(base, func(out *CallOutcome) { out.ParamPathInvalidations[0].PreserveStructuralWitness = false }),
		"param conditions":             mutationPair(base, func(out *CallOutcome) { out.ParamConditions[0].Value = false }),
		"param path relations":         mutationPair(base, func(out *CallOutcome) { out.ParamPathRelations[0].Kind = 0 }),
		"return condition refinements": mutationPair(base, func(out *CallOutcome) { out.ReturnConditionRefinements[0].ReturnValue = false }),
		"return condition slots":       mutationPair(base, func(out *CallOutcome) { out.ReturnConditionSlots[0].TargetIndex++ }),
		"return presence relations":    mutationPair(base, func(out *CallOutcome) { out.ReturnPresenceRelations[0].TargetIndex++ }),
		"param exposures":              mutationPair(base, func(out *CallOutcome) { out.ParamExposures[0].Contract = product.Top() }),
		"nil versus empty slice":       {CallOutcome{}, CallOutcome{Results: []CallResult{}}},
		"nil versus empty map":         {CallOutcome{}, CallOutcome{Placements: map[identity.ID]placement.Value{}}},
		"symbol path display spelling": {
			CallOutcome{PathObligations: []CallPathObligation{{Path: pathdom.Path{Root: "first", Symbol: 1}, Value: value}}},
			CallOutcome{PathObligations: []CallPathObligation{{Path: pathdom.Path{Root: "second", Symbol: 1}, Value: value}}},
		},
	}

	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			assertCallOutcomeRepresentationMatchesReflect(t, test.left, test.right)
		})
		t.Run("equal/"+name, func(t *testing.T) {
			assertCallOutcomeRepresentationMatchesReflect(t, test.left, test.left.Clone())
		})
	}
}

func assertCallOutcomeRepresentationMatchesReflect(t *testing.T, left, right CallOutcome) {
	t.Helper()
	want := reflect.DeepEqual(left, right)
	if got := CallOutcomeRepresentationEqual(left, right); got != want {
		t.Fatalf("CallOutcomeRepresentationEqual() = %v, old public behavior = %v\nleft: %#v\nright: %#v", got, want, left, right)
	}
}

func mutationPair(base CallOutcome, mutate func(*CallOutcome)) struct{ left, right CallOutcome } {
	right := base.Clone()
	mutate(&right)
	return struct{ left, right CallOutcome }{left: base, right: right}
}
