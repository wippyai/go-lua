package summary

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
	"github.com/wippyai/go-lua/analysis/engine/state/dynamicindex"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestNormalReturnFactsNormalizeDropsNonPlaceholderPaths(t *testing.T) {
	reg := mustRegistry(t)
	placeholder := pathdom.NewPlaceholder(0).Field("field")
	concrete := pathdom.NewPath(symbol.ID(10), "arg").Field("field")
	value := presentProduct(reg)

	got := Normalize(reg, Summary{NormalReturnFacts: callboundary.NormalReturnFacts{
		PathRefinements: []callboundary.PathValueFact{
			{Path: concrete, Value: value},
			{Path: placeholder, Value: value},
		},
		PathStaticMembers: []callboundary.PathStaticMemberFact{
			{Path: concrete, Value: value},
			{Path: placeholder, Value: value},
		},
		DynamicIndexFacts: []callboundary.DynamicIndexFact{
			{Table: concrete, Site: "caller.dynamic.ignored", Value: dynamicindex.Fact{KeyPresence: presence.Present()}},
			{Table: placeholder, Site: "caller.dynamic.1", Value: dynamicindex.Fact{KeyPresence: presence.Present()}},
		},
		BranchProofs: []callboundary.BranchProof{
			{Kind: pathevidence.BranchProofPathPresence, Path: concrete, Presence: presence.Present()},
			{Kind: pathevidence.BranchProofPathPresence, Path: placeholder, Presence: presence.Present()},
			{Kind: pathevidence.BranchProofPathEqual, Path: placeholder, Other: concrete},
			{Kind: pathevidence.BranchProofPathEqual, Path: placeholder, Other: pathdom.NewPlaceholder(1)},
		},
		ChannelSelects: []callboundary.ChannelSelectFact{
			{Select: channelselectfact.ID("select-concrete"), Kind: channelselectfact.FactReceive, Result: concrete, Index: 0},
			{Select: channelselectfact.ID("select-placeholder"), Kind: channelselectfact.FactReceive, Result: placeholder, Index: 0},
		},
		EffectDeltas: []callboundary.EffectDelta{
			{Target: concrete, Site: "caller.effect.ignored", Kind: effectdelta.Mutation, Value: effectdelta.Value{Before: value, After: value, Change: effectdelta.ChangeChanged}},
			{Target: placeholder, Site: "caller.effect.1", Kind: effectdelta.Mutation, Value: effectdelta.Value{Before: value, After: value, Change: effectdelta.ChangeChanged}},
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
	original := Summary{NormalReturnFacts: callboundary.NormalReturnFacts{
		PathRefinements: []callboundary.PathValueFact{{Path: pathdom.NewPlaceholder(0).Field("value"), Value: presentProduct(reg)}},
		DynamicIndexFacts: []callboundary.DynamicIndexFact{{
			Table: pathdom.NewPlaceholder(0),
			Site:  "caller.dynamic.clone",
			Value: dynamicindex.Fact{
				KeyPresence: presence.Present(),
				KeyValue:    presentProduct(reg),
				Value:       presentProduct(reg),
				Admission:   dynamicindex.AdmissionAdmitted,
			},
		}},
		BranchProofs: []callboundary.BranchProof{{
			Kind:     pathevidence.BranchProofPathPresence,
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

	left := Summary{NormalReturnFacts: callboundary.NormalReturnFacts{
		PathRefinements: []callboundary.PathValueFact{
			{Path: commonPath, Value: leftValue},
			{Path: leftOnly, Value: leftValue},
		},
		PathStaticMembers: []callboundary.PathStaticMemberFact{
			{Path: commonPath, Value: leftValue},
			{Path: leftOnly, Value: leftValue},
		},
		DynamicIndexFacts: []callboundary.DynamicIndexFact{
			{Table: p0, Site: "caller.dynamic.common", Value: dynamicindex.Fact{KeyPresence: presence.Present(), KeyValue: leftValue, Value: leftValue, Admission: dynamicindex.AdmissionAdmitted}},
			{Table: p0, Site: "caller.dynamic.left", Value: dynamicindex.Fact{KeyPresence: presence.Present(), KeyValue: leftValue, Value: leftValue, Admission: dynamicindex.AdmissionAdmitted}},
		},
		BranchProofs: []callboundary.BranchProof{
			{Kind: pathevidence.BranchProofPathPresence, Path: commonPath, Presence: presence.Present()},
			{Kind: pathevidence.BranchProofPathPresence, Path: leftOnly, Presence: presence.Present()},
		},
		ChannelSelects: []callboundary.ChannelSelectFact{
			{Select: channelselectfact.ID("select-common"), Kind: channelselectfact.FactSelect, Result: commonPath, Index: 0},
			{Select: channelselectfact.ID("select-left"), Kind: channelselectfact.FactReceive, Case: leftOnly, Index: 1},
		},
		EffectDeltas: []callboundary.EffectDelta{
			{Target: commonPath, Site: "caller.effect.common", Kind: effectdelta.Mutation, Value: effectdelta.Value{Before: leftValue, After: leftValue, Change: effectdelta.ChangeChanged}},
			{Target: leftOnly, Site: "caller.effect.left", Kind: effectdelta.Mutation, Value: effectdelta.Value{Before: leftValue, After: leftValue, Change: effectdelta.ChangeChanged}},
		},
	}}
	right := Summary{NormalReturnFacts: callboundary.NormalReturnFacts{
		PathRefinements:   []callboundary.PathValueFact{{Path: commonPath, Value: rightValue}},
		PathStaticMembers: []callboundary.PathStaticMemberFact{{Path: commonPath, Value: rightValue}},
		DynamicIndexFacts: []callboundary.DynamicIndexFact{{
			Table: p0,
			Site:  "caller.dynamic.common",
			Value: dynamicindex.Fact{
				KeyPresence: presence.Absent(),
				KeyValue:    rightValue,
				Value:       rightValue,
				Admission:   dynamicindex.AdmissionRejected,
			},
		}},
		BranchProofs: []callboundary.BranchProof{
			{Kind: pathevidence.BranchProofPathPresence, Path: commonPath, Presence: presence.Present()},
			{Kind: pathevidence.BranchProofPathPresence, Path: p0.Field("right"), Presence: presence.Present()},
		},
		ChannelSelects: []callboundary.ChannelSelectFact{
			{Select: channelselectfact.ID("select-common"), Kind: channelselectfact.FactSelect, Result: commonPath, Index: 0},
			{Select: channelselectfact.ID("select-right"), Kind: channelselectfact.FactCase, Case: p0.Field("right"), Index: 2},
		},
		EffectDeltas: []callboundary.EffectDelta{{
			Target: commonPath,
			Site:   "caller.effect.common",
			Kind:   effectdelta.Mutation,
			Value:  effectdelta.Value{Before: rightValue, After: rightValue, Change: effectdelta.ChangeNone},
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
		!presence.Equal(common.Value.KeyPresence, presence.Maybe()) ||
		common.Value.Admission != dynamicindex.AdmissionUnknown {
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
		!presence.Equal(product.PresenceOf(common.Value.Before), presence.Maybe()) ||
		common.Value.Change != effectdelta.ChangeUnknown {
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
	left := Summary{NormalReturnFacts: callboundary.NormalReturnFacts{
		PathRefinements: []callboundary.PathValueFact{{Path: p0, Value: presentProduct(reg)}},
		BranchProofs:    []callboundary.BranchProof{{Kind: pathevidence.BranchProofPathPresence, Path: p0, Presence: presence.Present()}},
	}}
	right := Summary{NormalReturnFacts: callboundary.NormalReturnFacts{
		PathRefinements: []callboundary.PathValueFact{{Path: p0, Value: product.Top()}},
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

func findPathRefinement(facts []callboundary.PathValueFact, path pathdom.Path) *callboundary.PathValueFact {
	for i := range facts {
		if facts[i].Path.Equal(path) {
			return &facts[i]
		}
	}
	return nil
}

func findDynamicIndexFact(facts []callboundary.DynamicIndexFact, site string) *callboundary.DynamicIndexFact {
	for i := range facts {
		if string(facts[i].Site) == site {
			return &facts[i]
		}
	}
	return nil
}

func findEffectDelta(deltas []callboundary.EffectDelta, site string) *callboundary.EffectDelta {
	for i := range deltas {
		if string(deltas[i].Site) == site {
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
