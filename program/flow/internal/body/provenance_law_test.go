package body

import (
	"testing"

	"github.com/wippyai/go-lua/program/flow/internal/authored"
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
)

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
	defer flowFinish.Abort()
	defer staticFinish.Abort()
	defer sourceFinish.Abort()
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
	defer foreignSourceFinish.Abort()
	defer foreignSourceStaticFinish.Abort()
	defer foreignSourceFinalizer.Abort()
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
	defer foreignFlowFinish.Abort()
	defer foreignFlowStaticFinish.Abort()
	defer foreignFlowFinalizer.Abort()
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
