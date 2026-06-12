package summary

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestNormalReturnFactsNormalizeDropsNonPlaceholderPaths(t *testing.T) {
	reg := mustRegistry(t)
	placeholder := pathdom.NewPlaceholder(0).Field("field")
	concrete := pathdom.NewPath(symbol.ID(10), "arg").Field("field")
	value := presentProduct(reg)

	got := Normalize(reg, Summary{NormalReturnFacts: NormalReturnFacts{
		PathRefinements: []PathValueFact{
			{Path: concrete, Value: value},
			{Path: placeholder, Value: value},
		},
		PathStaticMembers: []PathStaticMemberFact{
			{Path: concrete, Value: value},
			{Path: placeholder, Value: value},
		},
		DynamicIndexFacts: []DynamicIndexFact{
			{Table: concrete, Site: "caller.dynamic.ignored", KeyPresence: presence.Present()},
			{Table: placeholder, Site: "caller.dynamic.1", KeyPresence: presence.Present()},
		},
		BranchProofs: []BranchProof{
			{Kind: BranchProofPathPresence, Path: concrete, Presence: presence.Present()},
			{Kind: BranchProofPathPresence, Path: placeholder, Presence: presence.Present()},
			{Kind: BranchProofPathEqual, Path: placeholder, Other: concrete},
			{Kind: BranchProofPathEqual, Path: placeholder, Other: pathdom.NewPlaceholder(1)},
		},
		ChannelSelects: []ChannelSelectFact{
			{Select: "select-concrete", Kind: ChannelSelectFactReceive, Result: concrete, Index: 0},
			{Select: "select-placeholder", Kind: ChannelSelectFactReceive, Result: placeholder, Index: 0},
		},
		EffectDeltas: []EffectDelta{
			{Target: concrete, Site: "caller.effect.ignored", Kind: EffectDeltaMutation, Before: value, After: value, Change: EffectDeltaChangeChanged},
			{Target: placeholder, Site: "caller.effect.1", Kind: EffectDeltaMutation, Before: value, After: value, Change: EffectDeltaChangeChanged},
		},
	}})

	facts := got.NormalReturnFacts
	if len(facts.PathRefinements) != 1 || !facts.PathRefinements[0].Path.Equal(placeholder) {
		t.Fatalf("PathRefinements = %#v, want only placeholder fact", facts.PathRefinements)
	}
	if len(facts.PathStaticMembers) != 1 || !facts.PathStaticMembers[0].Path.Equal(placeholder) {
		t.Fatalf("PathStaticMembers = %#v, want only placeholder fact", facts.PathStaticMembers)
	}
	if len(facts.DynamicIndexFacts) != 1 || facts.DynamicIndexFacts[0].Site != "caller.dynamic.1" {
		t.Fatalf("DynamicIndexFacts = %#v, want stable caller site placeholder fact", facts.DynamicIndexFacts)
	}
	if len(facts.BranchProofs) != 2 {
		t.Fatalf("BranchProofs = %#v, want placeholder presence and equality proofs", facts.BranchProofs)
	}
	if len(facts.ChannelSelects) != 1 || facts.ChannelSelects[0].Select != "select-placeholder" {
		t.Fatalf("ChannelSelects = %#v, want only placeholder result fact", facts.ChannelSelects)
	}
	if len(facts.EffectDeltas) != 1 || facts.EffectDeltas[0].Site != "caller.effect.1" {
		t.Fatalf("EffectDeltas = %#v, want stable caller site placeholder fact", facts.EffectDeltas)
	}
}

func TestNormalReturnFactsCloneIsolatesPayload(t *testing.T) {
	reg := mustRegistry(t)
	original := Summary{NormalReturnFacts: NormalReturnFacts{
		PathRefinements: []PathValueFact{{Path: pathdom.NewPlaceholder(0).Field("value"), Value: presentProduct(reg)}},
		DynamicIndexFacts: []DynamicIndexFact{{
			Table:       pathdom.NewPlaceholder(0),
			Site:        "caller.dynamic.clone",
			KeyPresence: presence.Present(),
			KeyValue:    presentProduct(reg),
			Value:       presentProduct(reg),
			Admission:   DynamicIndexAdmissionAdmitted,
		}},
		BranchProofs: []BranchProof{{
			Kind:     BranchProofPathPresence,
			Path:     pathdom.NewPlaceholder(0).Field("ok"),
			Presence: presence.Present(),
		}},
	}}

	cloned := original.Clone()
	cloned.NormalReturnFacts.PathRefinements[0].Path = pathdom.NewPlaceholder(1)
	cloned.NormalReturnFacts.DynamicIndexFacts[0].Site = "caller.dynamic.changed"
	cloned.NormalReturnFacts.BranchProofs[0].Presence = presence.Absent()

	if !original.NormalReturnFacts.PathRefinements[0].Path.Equal(pathdom.NewPlaceholder(0).Field("value")) {
		t.Fatalf("mutating cloned path refinement changed original")
	}
	if original.NormalReturnFacts.DynamicIndexFacts[0].Site != "caller.dynamic.clone" {
		t.Fatalf("mutating cloned dynamic index changed original")
	}
	if !presence.Equal(original.NormalReturnFacts.BranchProofs[0].Presence, presence.Present()) {
		t.Fatalf("mutating cloned branch proof changed original")
	}
}

func TestNormalReturnFactsJoinUsesStateLaneSemantics(t *testing.T) {
	reg := mustRegistry(t)
	p0 := pathdom.NewPlaceholder(0)
	leftOnly := p0.Field("left")
	commonPath := p0.Field("common")
	leftValue := presentProduct(reg)
	rightValue := absentProduct(reg)

	left := Summary{NormalReturnFacts: NormalReturnFacts{
		PathRefinements: []PathValueFact{
			{Path: commonPath, Value: leftValue},
			{Path: leftOnly, Value: leftValue},
		},
		PathStaticMembers: []PathStaticMemberFact{
			{Path: commonPath, Value: leftValue},
			{Path: leftOnly, Value: leftValue},
		},
		DynamicIndexFacts: []DynamicIndexFact{
			{Table: p0, Site: "caller.dynamic.common", KeyPresence: presence.Present(), KeyValue: leftValue, Value: leftValue, Admission: DynamicIndexAdmissionAdmitted},
			{Table: p0, Site: "caller.dynamic.left", KeyPresence: presence.Present(), KeyValue: leftValue, Value: leftValue, Admission: DynamicIndexAdmissionAdmitted},
		},
		BranchProofs: []BranchProof{
			{Kind: BranchProofPathPresence, Path: commonPath, Presence: presence.Present()},
			{Kind: BranchProofPathPresence, Path: leftOnly, Presence: presence.Present()},
		},
		ChannelSelects: []ChannelSelectFact{
			{Select: "select-common", Kind: ChannelSelectFactSelect, Result: commonPath, Index: 0},
			{Select: "select-left", Kind: ChannelSelectFactReceive, Case: leftOnly, Index: 1},
		},
		EffectDeltas: []EffectDelta{
			{Target: commonPath, Site: "caller.effect.common", Kind: EffectDeltaMutation, Before: leftValue, After: leftValue, Change: EffectDeltaChangeChanged},
			{Target: leftOnly, Site: "caller.effect.left", Kind: EffectDeltaMutation, Before: leftValue, After: leftValue, Change: EffectDeltaChangeChanged},
		},
	}}
	right := Summary{NormalReturnFacts: NormalReturnFacts{
		PathRefinements:   []PathValueFact{{Path: commonPath, Value: rightValue}},
		PathStaticMembers: []PathStaticMemberFact{{Path: commonPath, Value: rightValue}},
		DynamicIndexFacts: []DynamicIndexFact{{
			Table:       p0,
			Site:        "caller.dynamic.common",
			KeyPresence: presence.Absent(),
			KeyValue:    rightValue,
			Value:       rightValue,
			Admission:   DynamicIndexAdmissionRejected,
		}},
		BranchProofs: []BranchProof{
			{Kind: BranchProofPathPresence, Path: commonPath, Presence: presence.Present()},
			{Kind: BranchProofPathPresence, Path: p0.Field("right"), Presence: presence.Present()},
		},
		ChannelSelects: []ChannelSelectFact{
			{Select: "select-common", Kind: ChannelSelectFactSelect, Result: commonPath, Index: 0},
			{Select: "select-right", Kind: ChannelSelectFactCase, Case: p0.Field("right"), Index: 2},
		},
		EffectDeltas: []EffectDelta{{
			Target: commonPath,
			Site:   "caller.effect.common",
			Kind:   EffectDeltaMutation,
			Before: rightValue,
			After:  rightValue,
			Change: EffectDeltaChangeNone,
		}},
	}}

	got := Join(reg, left, right).NormalReturnFacts
	if len(got.PathRefinements) != 2 {
		t.Fatalf("PathRefinements = %#v, want common joined and left-only retained", got.PathRefinements)
	}
	if common := findPathRefinement(got.PathRefinements, commonPath); common == nil ||
		!presence.Equal(product.PresenceOf(common.Value), presence.Maybe()) {
		t.Fatalf("common path refinement did not join to maybe: %#v", common)
	}
	if len(got.PathStaticMembers) != 1 || !got.PathStaticMembers[0].Path.Equal(commonPath) ||
		!presence.Equal(product.PresenceOf(got.PathStaticMembers[0].Value), presence.Maybe()) {
		t.Fatalf("PathStaticMembers = %#v, want only common joined must fact", got.PathStaticMembers)
	}
	if len(got.DynamicIndexFacts) != 2 {
		t.Fatalf("DynamicIndexFacts = %#v, want common joined and left-only retained", got.DynamicIndexFacts)
	}
	if common := findDynamicIndexFact(got.DynamicIndexFacts, "caller.dynamic.common"); common == nil ||
		!presence.Equal(common.KeyPresence, presence.Maybe()) ||
		common.Admission != DynamicIndexAdmissionUnknown {
		t.Fatalf("common dynamic index fact did not pointwise join: %#v", common)
	}
	if leftOnlyFact := findDynamicIndexFact(got.DynamicIndexFacts, "caller.dynamic.left"); leftOnlyFact == nil ||
		!dynamicIndexFactEqual(reg, *leftOnlyFact, left.NormalReturnFacts.DynamicIndexFacts[1]) {
		t.Fatalf("left-only dynamic index fact = %#v, want original fact", leftOnlyFact)
	}
	if len(got.BranchProofs) != 1 || !got.BranchProofs[0].Path.Equal(commonPath) {
		t.Fatalf("BranchProofs = %#v, want only common must proof", got.BranchProofs)
	}
	if len(got.ChannelSelects) != 1 || got.ChannelSelects[0].Select != "select-common" {
		t.Fatalf("ChannelSelects = %#v, want only common must fact", got.ChannelSelects)
	}
	if len(got.EffectDeltas) != 2 {
		t.Fatalf("EffectDeltas = %#v, want common joined and left-only retained", got.EffectDeltas)
	}
	if common := findEffectDelta(got.EffectDeltas, "caller.effect.common"); common == nil ||
		!presence.Equal(product.PresenceOf(common.Before), presence.Maybe()) ||
		common.Change != EffectDeltaChangeUnknown {
		t.Fatalf("common effect delta did not pointwise join: %#v", common)
	}
	if leftOnlyDelta := findEffectDelta(got.EffectDeltas, "caller.effect.left"); leftOnlyDelta == nil ||
		!effectDeltaEqual(reg, *leftOnlyDelta, left.NormalReturnFacts.EffectDeltas[1]) {
		t.Fatalf("left-only effect delta = %#v, want original delta", leftOnlyDelta)
	}

	widened := Widen(reg, left, right).NormalReturnFacts
	if leftOnlyFact := findDynamicIndexFact(widened.DynamicIndexFacts, "caller.dynamic.left"); leftOnlyFact == nil ||
		!dynamicIndexFactEqual(reg, *leftOnlyFact, left.NormalReturnFacts.DynamicIndexFacts[1]) {
		t.Fatalf("widen left-only dynamic index fact = %#v, want original fact", leftOnlyFact)
	}
	if leftOnlyDelta := findEffectDelta(widened.EffectDeltas, "caller.effect.left"); leftOnlyDelta == nil ||
		!effectDeltaEqual(reg, *leftOnlyDelta, left.NormalReturnFacts.EffectDeltas[1]) {
		t.Fatalf("widen left-only effect delta = %#v, want original delta", leftOnlyDelta)
	}
}

func TestNormalReturnFactsEqualAndLessOrEqAccountForLane(t *testing.T) {
	reg := mustRegistry(t)
	p0 := pathdom.NewPlaceholder(0)
	left := Summary{NormalReturnFacts: NormalReturnFacts{
		PathRefinements: []PathValueFact{{Path: p0, Value: presentProduct(reg)}},
		BranchProofs:    []BranchProof{{Kind: BranchProofPathPresence, Path: p0, Presence: presence.Present()}},
	}}
	right := Summary{NormalReturnFacts: NormalReturnFacts{
		PathRefinements: []PathValueFact{{Path: p0, Value: product.Top()}},
	}}

	if Equal(reg, left, right) {
		t.Fatalf("Equal ignored normal return facts")
	}
	if !LessOrEq(reg, left, right) {
		t.Fatalf("LessOrEq should account for product weakening and must-proof removal")
	}
	if LessOrEq(reg, right, left) {
		t.Fatalf("LessOrEq should reject stronger product and extra must proof")
	}
	if !Equal(reg, left, Normalize(reg, left)) {
		t.Fatalf("Equal should compare normalized normal return facts")
	}
}

func findPathRefinement(facts []PathValueFact, path pathdom.Path) *PathValueFact {
	for i := range facts {
		if facts[i].Path.Equal(path) {
			return &facts[i]
		}
	}
	return nil
}

func findDynamicIndexFact(facts []DynamicIndexFact, site string) *DynamicIndexFact {
	for i := range facts {
		if facts[i].Site == site {
			return &facts[i]
		}
	}
	return nil
}

func findEffectDelta(deltas []EffectDelta, site string) *EffectDelta {
	for i := range deltas {
		if deltas[i].Site == site {
			return &deltas[i]
		}
	}
	return nil
}

func presentProduct(reg *axis.Registry) product.Value {
	return product.NewWithPresence(reg, product.ShapeTop, presence.Present())
}

func absentProduct(reg *axis.Registry) product.Value {
	return product.NewWithPresence(reg, product.ShapeTop, presence.Absent())
}
