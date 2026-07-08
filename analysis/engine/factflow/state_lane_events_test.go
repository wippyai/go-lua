package factflow

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestStateLaneEventConstructorsAndAccessorsCopyPaths(t *testing.T) {
	source := ValueSource{Kind: ValueSourceExpression, ExprRef: ExprRef(1), HasExpr: true}
	keySource := ValueSource{Kind: ValueSourceExpression, ExprRef: ExprRef(2), HasExpr: true}

	staticTarget := path.NewPath(symbol.ID(101), "table").Field("member")
	staticWrite := NewPathStaticMemberWrite(staticTarget, source)
	staticTarget.Segments[0].Name = "changed"
	assertDirectField(t, staticWrite.TargetPath(), "member")
	assertDirectField(t, staticWrite.TargetPathRef(), "member")
	gotStaticTarget := staticWrite.TargetPath()
	gotStaticTarget.Segments[0].Name = "changed-again"
	assertDirectField(t, staticWrite.TargetPath(), "member")
	if got := staticWrite.Source(); got != source {
		t.Fatalf("static member source = %#v, want %#v", got, source)
	}

	dynamicTable := path.NewPath(symbol.ID(102), "table").Field("dynamic")
	dynamicWrite := NewDynamicIndexWrite(
		dynamicTable,
		keySource,
		source,
		dynamicindex.AdmissionAdmitted,
		DynamicIndexReadbackKeyAndValue,
	)
	dynamicTable.Segments[0].Name = "changed"
	assertDirectField(t, dynamicWrite.TablePath(), "dynamic")
	assertDirectField(t, dynamicWrite.TablePathRef(), "dynamic")
	gotDynamicTable := dynamicWrite.TablePath()
	gotDynamicTable.Segments[0].Name = "changed-again"
	assertDirectField(t, dynamicWrite.TablePath(), "dynamic")
	if dynamicWrite.KeySource() != keySource || dynamicWrite.Source() != source {
		t.Fatalf("dynamic sources = %#v/%#v, want %#v/%#v", dynamicWrite.KeySource(), dynamicWrite.Source(), keySource, source)
	}
	if dynamicWrite.Admission() != dynamicindex.AdmissionAdmitted || dynamicWrite.ReadbackIntent() != DynamicIndexReadbackKeyAndValue {
		t.Fatalf("dynamic intent = %v/%v", dynamicWrite.Admission(), dynamicWrite.ReadbackIntent())
	}
	keyPath := path.NewPath(symbol.ID(202), "key").Field("name")
	valuePath := path.NewPath(symbol.ID(203), "value").Field("payload")
	dynamicWrite = dynamicWrite.WithKeyPath(keyPath).WithValuePath(valuePath)
	keyPath.Segments[0].Name = "changed"
	valuePath.Segments[0].Name = "changed"
	if got, ok := dynamicWrite.KeyPathRef(); !ok || !got.Equal(path.NewPath(symbol.ID(202), "key").Field("name")) {
		t.Fatalf("dynamic key path ref = %v/%v", got, ok)
	}
	if got, ok := dynamicWrite.ValuePathRef(); !ok || !got.Equal(path.NewPath(symbol.ID(203), "value").Field("payload")) {
		t.Fatalf("dynamic value path ref = %v/%v", got, ok)
	}

	evidencePath := path.NewPath(symbol.ID(103), "err")
	presenceEvidence := NewBranchPathPresenceEvidenceOnEdge(evidencePath, presence.Present(), true)
	presenceEvidenceFalse := NewBranchPathPresenceEvidenceOnEdge(evidencePath, presence.Present(), false)
	evidencePath.Root = "changed"
	assertPathEqual(t, presenceEvidence.Path(), path.NewPath(symbol.ID(103), "err"))
	if !presenceEvidence.ActiveOnEdge(true) || presenceEvidence.ActiveOnEdge(false) {
		t.Fatalf("true-edge presence evidence should only be active on the true edge")
	}
	if presenceEvidenceFalse.ActiveOnEdge(true) || !presenceEvidenceFalse.ActiveOnEdge(false) {
		t.Fatalf("false-edge presence evidence should only be active on the false edge")
	}
	if got, ok := presenceEvidence.Presence(); !ok || !presence.Equal(got, presence.Present()) {
		t.Fatalf("presence evidence = %s/%v, want present/true", got, ok)
	}
	if _, ok := presenceEvidence.OtherPath(); ok {
		t.Fatalf("presence evidence unexpectedly has other path")
	}
	gotEvidencePath := presenceEvidence.Path()
	gotEvidencePath.Root = "changed-again"
	assertPathEqual(t, presenceEvidence.Path(), path.NewPath(symbol.ID(103), "err"))

	leftPath := path.NewPath(symbol.ID(104), "left").Field("value")
	rightPath := path.NewPath(symbol.ID(105), "right").Field("value")
	equalityEvidence := NewBranchPathEqualityEvidenceOnEdge(leftPath, rightPath, true)
	inequalityEvidence := NewBranchPathInequalityEvidenceOnEdge(leftPath, rightPath, true)
	falseEdgeEvidence := NewBranchPathEqualityEvidenceOnEdge(leftPath, rightPath, false)
	frozenPath := path.NewPath(symbol.ID(106), "frozen")
	frozenEvidence := NewBranchFrozenTableEvidenceOnEdge(frozenPath, true)
	leftPath.Segments[0].Name = "changed"
	rightPath.Segments[0].Name = "changed"
	if equalityEvidence.Kind() != BranchPathEvidenceEqual || inequalityEvidence.Kind() != BranchPathEvidenceNotEqual {
		t.Fatalf("evidence kinds = %v/%v", equalityEvidence.Kind(), inequalityEvidence.Kind())
	}
	if falseEdgeEvidence.ActiveOnEdge(true) || !falseEdgeEvidence.ActiveOnEdge(false) {
		t.Fatalf("false-edge evidence active true/false = %v/%v, want false/true", falseEdgeEvidence.ActiveOnEdge(true), falseEdgeEvidence.ActiveOnEdge(false))
	}
	if frozenEvidence.Kind() != BranchPathEvidenceFrozenTable || !frozenEvidence.ActiveOnEdge(true) || frozenEvidence.ActiveOnEdge(false) {
		t.Fatalf("frozen evidence kind/active = %v/%v/%v, want frozen/true/false", frozenEvidence.Kind(), frozenEvidence.ActiveOnEdge(true), frozenEvidence.ActiveOnEdge(false))
	}
	assertDirectField(t, equalityEvidence.Path(), "value")
	otherPath, ok := equalityEvidence.OtherPath()
	if !ok {
		t.Fatalf("equality evidence other path missing")
	}
	assertDirectField(t, otherPath, "value")
	otherPath.Segments[0].Name = "changed-again"
	otherAgain, _ := equalityEvidence.OtherPath()
	assertDirectField(t, otherAgain, "value")
	evidenceSet := NewBranchPathEvidenceSet(presenceEvidence, equalityEvidence)
	evidence := evidenceSet.Evidence()
	evidence[0] = inequalityEvidence
	if got := evidenceSet.Evidence(); got[0].Kind() != BranchPathEvidencePresence {
		t.Fatalf("branch path evidence set exposed mutable slice, got %v", got[0].Kind())
	}
	frozenAgain := frozenEvidence.Path()
	frozenAgain.Root = "changed"
	assertPathEqual(t, frozenEvidence.Path(), frozenPath)

	resultPath := path.NewPath(symbol.ID(106), "select").Field("result")
	casePath := path.NewPath(symbol.ID(107), "select").Field("case")
	channel := NewChannelSelect(ChannelSelectConfig{
		SelectID:      ChannelSelectID("select-1"),
		Kind:          ChannelSelectReceive,
		ResultPath:    resultPath,
		HasResultPath: true,
		CasePath:      casePath,
		HasCasePath:   true,
		Index:         2,
	})
	resultPath.Segments[0].Name = "changed"
	casePath.Segments[0].Name = "changed"
	if channel.SelectID() != ChannelSelectID("select-1") || channel.Kind() != ChannelSelectReceive || channel.Index() != 2 {
		t.Fatalf("channel select id/kind/index = %q/%v/%d", channel.SelectID(), channel.Kind(), channel.Index())
	}
	gotResult, ok := channel.ResultPath()
	if !ok {
		t.Fatalf("channel result path missing")
	}
	assertDirectField(t, gotResult, "result")
	gotResult.Segments[0].Name = "changed-again"
	gotResultAgain, _ := channel.ResultPath()
	assertDirectField(t, gotResultAgain, "result")
	gotCase, ok := channel.CasePath()
	if !ok {
		t.Fatalf("channel case path missing")
	}
	assertDirectField(t, gotCase, "case")
	gotCase.Segments[0].Name = "changed-again"
	gotCaseAgain, _ := channel.CasePath()
	assertDirectField(t, gotCaseAgain, "case")
	channelSet := NewChannelSelectSet(channel)
	events := channelSet.Events()
	events[0] = NewChannelSelect(ChannelSelectConfig{SelectID: ChannelSelectID("changed"), Kind: ChannelSelectCase})
	if got := channelSet.Events(); got[0].SelectID() != ChannelSelectID("select-1") {
		t.Fatalf("channel select set exposed mutable slice, got %q", got[0].SelectID())
	}
}

func TestFactsStateLaneEventSnapshotsAreImmutable(t *testing.T) {
	point := cfg.Point(201)
	missing := cfg.Point(202)
	source := ValueSource{Kind: ValueSourceExpression, ExprRef: ExprRef(1), HasExpr: true}
	keySource := ValueSource{Kind: ValueSourceExpression, ExprRef: ExprRef(2), HasExpr: true}
	callSource := ValueSource{Kind: ValueSourceCall, ExprRef: ExprRef(3), HasExpr: true}

	input := FactsInput{
		PathStaticMemberWrites: map[cfg.Point]PathStaticMemberWrite{
			point: NewPathStaticMemberWrite(path.NewPath(symbol.ID(201), "table").Field("member"), source),
		},
		DynamicIndexWrites: map[cfg.Point]DynamicIndexWrite{
			point: NewDynamicIndexWrite(
				path.NewPath(symbol.ID(202), "table").Field("dynamic"),
				keySource,
				source,
				dynamicindex.AdmissionAdmitted,
				DynamicIndexReadbackKeyAndValue,
			),
		},
		BranchPathEvidence: map[cfg.Point]BranchPathEvidenceSet{
			point: NewBranchPathEvidenceSet(
				NewBranchPathPresenceEvidenceOnEdge(path.NewPath(symbol.ID(203), "err"), presence.Present(), true),
				NewBranchPathEqualityEvidenceOnEdge(
					path.NewPath(symbol.ID(204), "left").Field("value"),
					path.NewPath(symbol.ID(205), "right").Field("value"),
					true,
				),
			),
		},
		ChannelSelects: map[cfg.Point]ChannelSelectSet{
			point: NewChannelSelectSet(
				NewChannelSelect(ChannelSelectConfig{
					SelectID:      ChannelSelectID("select-1"),
					Kind:          ChannelSelectSelect,
					ResultPath:    path.NewPath(symbol.ID(206), "select").Field("result"),
					HasResultPath: true,
					Index:         0,
				}),
				NewChannelSelect(ChannelSelectConfig{
					SelectID:    ChannelSelectID("select-1"),
					Kind:        ChannelSelectCase,
					CasePath:    path.NewPath(symbol.ID(207), "select").Field("case"),
					HasCasePath: true,
					Index:       1,
				}),
			),
		},
	}

	facts := NewFacts(input)
	input.PathStaticMemberWrites[point] = NewPathStaticMemberWrite(path.NewPath(symbol.ID(208), "changed"), callSource)
	input.DynamicIndexWrites[point] = NewDynamicIndexWrite(path.NewPath(symbol.ID(209), "changed"), callSource, callSource, dynamicindex.AdmissionRejected, DynamicIndexReadbackNone)
	input.BranchPathEvidence[point] = NewBranchPathEvidenceSet(NewBranchPathInequalityEvidenceOnEdge(path.NewPath(symbol.ID(210), "changed"), path.NewPath(symbol.ID(211), "changed"), true))
	input.ChannelSelects[point] = NewChannelSelectSet(NewChannelSelect(ChannelSelectConfig{SelectID: ChannelSelectID("changed"), Kind: ChannelSelectReceive}))

	if _, ok := facts.PathStaticMemberWrite(missing); ok {
		t.Fatal("missing static-member write returned ok")
	}
	if _, ok := facts.DynamicIndexWrite(missing); ok {
		t.Fatal("missing dynamic-index write returned ok")
	}
	if got := facts.BranchPathEvidence(missing); len(got) != 0 {
		t.Fatalf("missing branch path evidence = %#v, want empty", got)
	}
	if got := facts.ChannelSelects(missing); len(got) != 0 {
		t.Fatalf("missing channel selects = %#v, want empty", got)
	}
	if facts.HasChannelSelects(missing) {
		t.Fatal("missing channel selects reported present")
	}
	if !facts.HasChannelSelects(point) {
		t.Fatal("point channel selects reported absent")
	}

	staticWrite, ok := facts.PathStaticMemberWrite(point)
	if !ok {
		t.Fatal("static-member write missing")
	}
	assertDirectField(t, staticWrite.TargetPath(), "member")
	staticTarget := staticWrite.TargetPath()
	staticTarget.Segments[0].Name = "mutated"
	staticAgain, _ := facts.PathStaticMemberWrite(point)
	assertDirectField(t, staticAgain.TargetPath(), "member")
	if staticAgain.Source() != source {
		t.Fatalf("static source = %#v, want %#v", staticAgain.Source(), source)
	}

	dynamicWrite, ok := facts.DynamicIndexWrite(point)
	if !ok {
		t.Fatal("dynamic-index write missing")
	}
	assertDirectField(t, dynamicWrite.TablePath(), "dynamic")
	dynamicTable := dynamicWrite.TablePath()
	dynamicTable.Segments[0].Name = "mutated"
	dynamicAgain, _ := facts.DynamicIndexWrite(point)
	assertDirectField(t, dynamicAgain.TablePath(), "dynamic")
	if dynamicAgain.KeySource() != keySource || dynamicAgain.Source() != source ||
		dynamicAgain.Admission() != dynamicindex.AdmissionAdmitted ||
		dynamicAgain.ReadbackIntent() != DynamicIndexReadbackKeyAndValue {
		t.Fatalf("dynamic write = %#v", dynamicAgain)
	}

	evidence := facts.BranchPathEvidence(point)
	if len(evidence) != 2 {
		t.Fatalf("branch path evidence len = %d, want 2", len(evidence))
	}
	if evidence[0].Kind() != BranchPathEvidencePresence {
		t.Fatalf("branch path evidence kind = %v, want presence", evidence[0].Kind())
	}
	assertPathEqual(t, evidence[0].Path(), path.NewPath(symbol.ID(203), "err"))
	if got, ok := evidence[0].Presence(); !ok || !presence.Equal(got, presence.Present()) {
		t.Fatalf("branch path evidence presence = %s/%v, want present/true", got, ok)
	}
	assertDirectField(t, evidence[1].Path(), "value")
	otherPath, ok := evidence[1].OtherPath()
	if !ok {
		t.Fatal("branch path equality evidence other path missing")
	}
	assertDirectField(t, otherPath, "value")
	otherPath.Segments[0].Name = "mutated"
	evidence[0] = NewBranchPathInequalityEvidenceOnEdge(path.NewPath(symbol.ID(212), "mutated"), path.NewPath(symbol.ID(213), "mutated"), true)
	evidenceAgain := facts.BranchPathEvidence(point)
	if evidenceAgain[0].Kind() != BranchPathEvidencePresence {
		t.Fatalf("facts branch path evidence exposed mutable slice, got %v", evidenceAgain[0].Kind())
	}
	otherAgain, _ := evidenceAgain[1].OtherPath()
	assertDirectField(t, otherAgain, "value")

	selects := facts.ChannelSelects(point)
	if len(selects) != 2 {
		t.Fatalf("channel selects len = %d, want 2", len(selects))
	}
	if selects[0].SelectID() != ChannelSelectID("select-1") || selects[0].Kind() != ChannelSelectSelect {
		t.Fatalf("channel select first = %q/%v", selects[0].SelectID(), selects[0].Kind())
	}
	resultPath, ok := selects[0].ResultPath()
	if !ok {
		t.Fatal("channel select result path missing")
	}
	assertDirectField(t, resultPath, "result")
	resultPath.Segments[0].Name = "mutated"
	casePath, ok := selects[1].CasePath()
	if !ok {
		t.Fatal("channel select case path missing")
	}
	assertDirectField(t, casePath, "case")
	casePath.Segments[0].Name = "mutated"
	selects[0] = NewChannelSelect(ChannelSelectConfig{SelectID: ChannelSelectID("mutated"), Kind: ChannelSelectReceive})
	selectsAgain := facts.ChannelSelects(point)
	if selectsAgain[0].SelectID() != ChannelSelectID("select-1") {
		t.Fatalf("facts channel selects exposed mutable slice, got %q", selectsAgain[0].SelectID())
	}
	resultAgain, _ := selectsAgain[0].ResultPath()
	assertDirectField(t, resultAgain, "result")
	caseAgain, _ := selectsAgain[1].CasePath()
	assertDirectField(t, caseAgain, "case")
}
