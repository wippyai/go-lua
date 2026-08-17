package body

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestBodyContainsSiblingIntervals(t *testing.T) {
	root := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	left := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	right := keyspace.MakeTerm(keyspace.FamilyBody, 3)
	input := authoredInputForIntervals(3)
	view, staticView, finish, staticFinish, preimage, sourceFinish := prepare(t, [][]keyspace.Term{{left, right}, nil, nil}, input)
	defer func() { _ = finish.Abort() }()
	defer func() { _ = staticFinish.Abort() }()
	defer func() { _ = sourceFinish.Abort() }()

	result, err := Seal(preimage, view, staticView, root)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if !result.Contains(root, root) || !result.AncestorOrSelf(root, left) || !result.Contains(root, right) {
		t.Fatal("root interval does not contain itself and both children")
	}
	if result.Contains(left, right) || result.Contains(right, left) {
		t.Fatal("sibling Body intervals overlap")
	}
	if result.Contains(left, root) || result.AncestorOrSelf(right, left) {
		t.Fatal("Body interval direction is reversed")
	}
}

func TestBodyAncestryQueriesFailClosed(t *testing.T) {
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	var nilResult *Result
	if nilResult.Contains(body1, body1) || nilResult.AncestorOrSelf(body1, body1) {
		t.Fatal("nil Body Result returned positive ancestry")
	}
	cases := []*Result{
		{parents: []keyspace.Term{0, body1, body1}, pre: []uint32{0, 1}, post: []uint32{0, 4, 0}},
		{parents: []keyspace.Term{0, body1, body1}, pre: []uint32{0, 1, 2}, post: []uint32{0, 0, 4}},
		{parents: []keyspace.Term{0, body1, body1}, pre: []uint32{0, 3, 2}, post: []uint32{0, 4, 1}},
		{parents: []keyspace.Term{0, body1, body2}, pre: []uint32{0, 1, 2}, post: []uint32{0, 4, 5}},
	}
	for index, result := range cases {
		if result.Contains(body1, body2) || result.AncestorOrSelf(body1, body2) {
			t.Fatalf("malformed Result %d returned positive ancestry", index)
		}
	}
}

func TestBodyAncestryQueriesDoNotAllocate(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	input := authoredInputForIntervals(1)
	view, staticView, finish, staticFinish, preimage, sourceFinish := prepare(t, [][]keyspace.Term{nil}, input)
	defer func() { _ = finish.Abort() }()
	defer func() { _ = staticFinish.Abort() }()
	defer func() { _ = sourceFinish.Abort() }()
	result, err := Seal(preimage, view, staticView, body)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	var contains, ancestor bool
	allocs := testing.AllocsPerRun(100, func() {
		contains = result.Contains(body, body)
		ancestor = result.AncestorOrSelf(body, body)
	})
	if allocs != 0 || !contains || !ancestor {
		t.Fatalf("Body ancestry queries allocated %f times or returned false", allocs)
	}
}

func authoredInputForIntervals(bodyCount uint32) authored.Input {
	return authored.Input{Counts: [keyspace.FamilyCount]uint32{keyspace.FamilyBody: bodyCount}}
}
func TestBodyProvenanceRejectsEqualDenominatorForeignOwners(t *testing.T) {
	loop := keyspace.MakeTerm(keyspace.FamilyLoop, 1)
	rows := [][]keyspace.Term{{loop}, nil}
	parent := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	child := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	base := authored.Input{Counts: [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 2,
		keyspace.FamilyNil:  1,
		keyspace.FamilyLoop: 1,
	}, Control: authored.ControlInput{Loops: []authored.Loop{{
		Owner: parent, Body: child, Kind: kind.LoopWhile,
		Control: keyspace.MakeTerm(keyspace.FamilyNil, 1),
	}}}}

	flowView, staticView, flowFinish, staticFinish, preimage, sourceFinish := prepareNamed(t, rows, base, "body-provenance-a.lua")
	defer func() { _ = flowFinish.Abort() }()
	defer func() { _ = staticFinish.Abort() }()
	defer func() { _ = sourceFinish.Abort() }()
	first, err := Seal(preimage, flowView, staticView, parent)
	if err != nil {
		t.Fatalf("first body.Seal: %v", err)
	}
	sourceID := preimage.Identity().ContentID()
	flowID := flowView.Cold().ContentID()
	if !Matches(first, sourceID, flowID) {
		t.Fatal("Body result did not retain its exact Source/Flow identities")
	}

	foreignSourceView, foreignSourceStatic, foreignSourceFinish, foreignSourceStaticFinish, foreignSourcePreimage, foreignSourceFinalizer := prepareNamed(t, rows, base, "body-provenance-b.lua")
	defer func() { _ = foreignSourceFinish.Abort() }()
	defer func() { _ = foreignSourceStaticFinish.Abort() }()
	defer func() { _ = foreignSourceFinalizer.Abort() }()
	foreignSource, err := Seal(foreignSourcePreimage, foreignSourceView, foreignSourceStatic, parent)
	if err != nil {
		t.Fatalf("foreign Source body.Seal: %v", err)
	}
	foreignSourceID := foreignSourcePreimage.Identity().ContentID()
	if sourceID == foreignSourceID || preimage.Identity().TermCount() != foreignSourcePreimage.Identity().TermCount() {
		t.Fatal("foreign Source fixture did not preserve equal denominator with a distinct identity")
	}
	if Matches(first, foreignSourceID, flowID) || Matches(foreignSource, sourceID, flowID) {
		t.Fatal("Body provenance accepted an equal-denominator foreign Source")
	}

	foreignFlowInput := base
	foreignFlowInput.Control.Loops = []authored.Loop{{
		Owner: parent, Body: child, Kind: kind.LoopRepeat,
		Control: keyspace.MakeTerm(keyspace.FamilyNil, 1),
	}}
	foreignFlowView, foreignFlowStatic, foreignFlowFinish, foreignFlowStaticFinish, foreignFlowPreimage, foreignFlowFinalizer := prepareNamed(t, rows, foreignFlowInput, "body-provenance-a.lua")
	defer func() { _ = foreignFlowFinish.Abort() }()
	defer func() { _ = foreignFlowStaticFinish.Abort() }()
	defer func() { _ = foreignFlowFinalizer.Abort() }()
	foreignFlow, err := Seal(foreignFlowPreimage, foreignFlowView, foreignFlowStatic, parent)
	if err != nil {
		t.Fatalf("foreign Flow body.Seal: %v", err)
	}
	foreignFlowID := foreignFlowView.Cold().ContentID()
	if sourceID != foreignFlowPreimage.Identity().ContentID() || flowID == foreignFlowID || base.Counts != foreignFlowInput.Counts {
		t.Fatal("foreign Flow fixture did not preserve equal denominator with a distinct identity")
	}
	if Matches(first, sourceID, foreignFlowID) || Matches(foreignFlow, sourceID, flowID) {
		t.Fatal("Body provenance accepted an equal-denominator foreign Flow")
	}

	zero := &Result{parents: first.parents, roots: first.roots, rootOffsets: first.rootOffsets, activation: first.activation, nearestLoop: first.nearestLoop, pre: first.pre, post: first.post}
	if Matches(zero, sourceID, flowID) || zero.BodyCount() != 0 || zero.Contains(parent, parent) {
		t.Fatal("zero-ID Body result bypassed provenance fail-closed law")
	}
}
