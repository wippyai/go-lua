package callpayload

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
)

func TestCallOutcomeAlgebraIsDescriptorTotal(t *testing.T) {
	descriptors := CallOutcomeDescriptors()
	if len(callOutcomeAlgebra) != len(descriptors) {
		t.Fatalf("algebra handlers = %d, descriptors = %d", len(callOutcomeAlgebra), len(descriptors))
	}
	for i, descriptor := range descriptors {
		if got, want := callOutcomeAlgebra[i].field, string(descriptor.Kind); got != want {
			t.Fatalf("algebra handler %d = %q, want %q", i, got, want)
		}
		if callOutcomeAlgebra[i].equal == nil || callOutcomeAlgebra[i].join == nil {
			t.Fatalf("algebra handler %q has incomplete equality/join", descriptor.Kind)
		}
	}
}

func TestJoinCallOutcomeIsExactDescriptorTotalLUB(t *testing.T) {
	reg := standard.Registry()
	p0, p1 := pathdom.NewPlaceholder(0), pathdom.NewPlaceholder(1)
	allocation := identity.LuaFunction(17)
	present := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	absent := product.NewWithPresence(reg, product.ShapeTop, presence.Absent())
	commonPresence := CallReturnPresenceRelation{
		TriggerIndex: 0, TriggerPresence: presence.Present(),
		TargetIndex: 1, TargetPresence: presence.Present(),
	}
	left := CallOutcome{
		Results:             []CallResult{{Index: 0, Value: present}},
		PostReturnAuthority: true,
		SuspensionKnown:     true,
		NormalReturnFacts: callboundary.NormalReturnFacts{NumFloors: []callboundary.NumFloorFact{
			{Path: p0, Floor: 4}, {Path: p1, Floor: 1},
		}},
		Placements:      map[identity.ID]placement.Value{allocation: placement.Stack},
		ParamPathWrites: []CallParamPathWrite{{Path: p0, Value: present}, {Path: p1, Value: present}},
		ParamConditions: []CallParamCondition{{ParamIndex: 0, Value: true}},
		ReturnPresenceRelations: []CallReturnPresenceRelation{commonPresence, {
			TriggerIndex: 1, TriggerPresence: presence.Present(),
			TargetIndex: 2, TargetPresence: presence.Absent(),
		}},
	}
	right := CallOutcome{
		Results:             []CallResult{{Index: 0, Value: absent}},
		PostReturnAuthority: false,
		SuspensionKnown:     true,
		MaySuspend:          true,
		NormalReturnFacts: callboundary.NormalReturnFacts{NumFloors: []callboundary.NumFloorFact{
			{Path: p0, Floor: 2},
		}},
		Placements:              map[identity.ID]placement.Value{allocation: placement.SharedHeap},
		ParamPathWrites:         []CallParamPathWrite{{Path: p0, Value: absent}},
		ParamConditions:         []CallParamCondition{{ParamIndex: 0, Value: true}, {ParamIndex: 1, Value: false}},
		ReturnPresenceRelations: []CallReturnPresenceRelation{commonPresence},
	}

	joined := JoinCallOutcome(reg, left, right)
	if joined.PostReturnAuthority || !joined.SuspensionKnown || !joined.MaySuspend {
		t.Fatalf("joined authority/suspension = authority:%v known:%v may:%v", joined.PostReturnAuthority, joined.SuspensionKnown, joined.MaySuspend)
	}
	if len(joined.Results) != 1 || !product.Equal(reg, joined.Results[0].Value, product.Join(reg, present, absent)) {
		t.Fatalf("joined results = %#v", joined.Results)
	}
	if len(joined.NormalReturnFacts.NumFloors) != 1 || joined.NormalReturnFacts.NumFloors[0].Floor != 2 || !joined.NormalReturnFacts.NumFloors[0].Path.Equal(p0) {
		t.Fatalf("joined normal-return floors = %#v", joined.NormalReturnFacts.NumFloors)
	}
	if joined.Placements[allocation] != placement.SharedHeap {
		t.Fatalf("joined placement = %v, want shared heap", joined.Placements[allocation])
	}
	if len(joined.ParamPathWrites) != 1 || !joined.ParamPathWrites[0].Path.Equal(p0) {
		t.Fatalf("must-write join = %#v, want only common path", joined.ParamPathWrites)
	}
	if len(joined.ParamConditions) != 1 || joined.ParamConditions[0] != (CallParamCondition{ParamIndex: 0, Value: true}) {
		t.Fatalf("must-condition join = %#v", joined.ParamConditions)
	}
	if len(joined.ReturnPresenceRelations) != 1 || !equalReturnPresenceForTest(joined.ReturnPresenceRelations[0], commonPresence) {
		t.Fatalf("must-presence join = %#v", joined.ReturnPresenceRelations)
	}
}

func TestJoinCallOutcomeAndCollapseSatisfySemilatticeLaws(t *testing.T) {
	reg := standard.Registry()
	p0 := pathdom.NewPlaceholder(0)
	a := CallOutcome{Results: []CallResult{{Index: 0, Value: product.Bottom(reg)}}, PostReturnAuthority: true}
	b := CallOutcome{Results: []CallResult{{Index: 0, Value: product.Top()}}, ParamLengthFloors: []CallParamLengthFloor{{Path: p0, Floor: 3}}}
	c := CallOutcome{SuspensionKnown: true, MaySuspend: true, ParamLengthFloors: []CallParamLengthFloor{{Path: p0, Floor: 1}}}

	if !EqualCallOutcome(reg, JoinCallOutcome(reg, a, a), a) {
		t.Fatal("CallOutcome join is not idempotent")
	}
	if !EqualCallOutcome(reg, JoinCallOutcome(reg, a, b), JoinCallOutcome(reg, b, a)) {
		t.Fatal("CallOutcome join is not commutative")
	}
	leftAssoc := JoinCallOutcome(reg, JoinCallOutcome(reg, a, b), c)
	rightAssoc := JoinCallOutcome(reg, a, JoinCallOutcome(reg, b, c))
	if !EqualCallOutcome(reg, leftAssoc, rightAssoc) {
		t.Fatal("CallOutcome join is not associative")
	}
	set := NewCallOutcomeAlternativeSet(reg, c, a, b)
	if !EqualCallOutcome(reg, set.Collapse(reg), leftAssoc) {
		t.Fatal("alternative collapse disagrees with CallOutcome LUB")
	}
}

func equalReturnPresenceForTest(a, b CallReturnPresenceRelation) bool {
	return a.TriggerIndex == b.TriggerIndex && a.TriggerPresence == b.TriggerPresence &&
		a.TargetIndex == b.TargetIndex && a.TargetPresence == b.TargetPresence
}

func TestCallOutcomeAlternativeSetDistinguishesAbsentFromExecutedEmpty(t *testing.T) {
	reg := standard.Registry()
	var absent CallOutcomeAlternativeSet
	executed := NewCallOutcomeAlternativeSet(reg, CallOutcome{})
	if !absent.Empty() || executed.Empty() {
		t.Fatalf("absent/executed empty distinction lost: absent=%v executed=%v", absent.Empty(), executed.Empty())
	}
	if len(executed.Outcomes()) != 1 || executed.Equal(reg, absent) {
		t.Fatal("executed empty outcome did not remain an explicit alternative")
	}
}

func TestCallOutcomeAlternativeJoinIsExactDeduplicatedUnion(t *testing.T) {
	reg := standard.Registry()
	first := CallOutcome{Results: []CallResult{{Index: 0, Value: product.Bottom(reg)}}, PostReturnAuthority: true}
	second := CallOutcome{Results: []CallResult{{Index: 0, Value: product.Top()}}, SuspensionKnown: true, MaySuspend: true}
	left := NewCallOutcomeAlternativeSet(reg, first, first)
	right := NewCallOutcomeAlternativeSet(reg, second)
	joined := left.Join(reg, right)
	if got := len(joined.Outcomes()); got != 2 {
		t.Fatalf("joined alternatives = %d, want 2", got)
	}
	if !joined.Equal(reg, right.Join(reg, left)) {
		t.Fatal("alternative union is not commutative")
	}
	if joined.Fingerprint(reg) != right.Join(reg, left).Fingerprint(reg) {
		t.Fatal("equal alternative sets have different fingerprints")
	}
	for _, outcome := range joined.Outcomes() {
		if outcome.PostReturnAuthority && outcome.MaySuspend {
			t.Fatal("join collapsed fields from distinct correlated alternatives")
		}
	}
}

func TestNormalizeCallOutcomeCanonicalizesResultSlotsAndDiagnostics(t *testing.T) {
	reg := standard.Registry()
	in := CallOutcome{
		Results: []CallResult{
			{Index: 2, Value: product.Top()},
			{Index: 0, Value: product.Bottom(reg)},
			{Index: 2, Value: product.Bottom(reg)},
		},
		SuspensionKnown: true,
	}
	out := NormalizeCallOutcome(reg, in)
	if len(out.Results) != 2 || out.Results[0].Index != 0 || out.Results[1].Index != 2 ||
		!product.Equal(reg, out.Results[1].Value, product.Top()) {
		t.Fatalf("normalized results = %#v", out.Results)
	}
	if !EqualCallOutcome(reg, out, NormalizeCallOutcome(reg, out)) {
		t.Fatal("outcome normalization is not idempotent")
	}
	if FingerprintCallOutcome(reg, out) != FingerprintCallOutcome(reg, NormalizeCallOutcome(reg, out)) {
		t.Fatal("normalized equal outcomes have different fingerprints")
	}
}
