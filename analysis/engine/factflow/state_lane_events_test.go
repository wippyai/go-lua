package factflow

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
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
		DynamicIndexAdmissionAdmitted,
		DynamicIndexReadbackKeyAndValue,
	)
	dynamicTable.Segments[0].Name = "changed"
	assertDirectField(t, dynamicWrite.TablePath(), "dynamic")
	gotDynamicTable := dynamicWrite.TablePath()
	gotDynamicTable.Segments[0].Name = "changed-again"
	assertDirectField(t, dynamicWrite.TablePath(), "dynamic")
	if dynamicWrite.KeySource() != keySource || dynamicWrite.Source() != source {
		t.Fatalf("dynamic sources = %#v/%#v, want %#v/%#v", dynamicWrite.KeySource(), dynamicWrite.Source(), keySource, source)
	}
	if dynamicWrite.Admission() != DynamicIndexAdmissionAdmitted || dynamicWrite.ReadbackIntent() != DynamicIndexReadbackKeyAndValue {
		t.Fatalf("dynamic intent = %v/%v", dynamicWrite.Admission(), dynamicWrite.ReadbackIntent())
	}

	proofPath := path.NewPath(symbol.ID(103), "err")
	presenceProof := NewBranchPathPresenceProof(proofPath, presence.Present())
	proofPath.Root = "changed"
	assertPathEqual(t, presenceProof.Path(), path.NewPath(symbol.ID(103), "err"))
	if got, ok := presenceProof.Presence(); !ok || !presence.Equal(got, presence.Present()) {
		t.Fatalf("presence proof = %s/%v, want present/true", got, ok)
	}
	if _, ok := presenceProof.OtherPath(); ok {
		t.Fatalf("presence proof unexpectedly has other path")
	}
	gotProofPath := presenceProof.Path()
	gotProofPath.Root = "changed-again"
	assertPathEqual(t, presenceProof.Path(), path.NewPath(symbol.ID(103), "err"))

	leftPath := path.NewPath(symbol.ID(104), "left").Field("value")
	rightPath := path.NewPath(symbol.ID(105), "right").Field("value")
	equalityProof := NewBranchPathEqualityProof(leftPath, rightPath)
	inequalityProof := NewBranchPathInequalityProof(leftPath, rightPath)
	leftPath.Segments[0].Name = "changed"
	rightPath.Segments[0].Name = "changed"
	if equalityProof.Kind() != BranchProofPathEqual || inequalityProof.Kind() != BranchProofPathNotEqual {
		t.Fatalf("proof kinds = %v/%v", equalityProof.Kind(), inequalityProof.Kind())
	}
	assertDirectField(t, equalityProof.Path(), "value")
	otherPath, ok := equalityProof.OtherPath()
	if !ok {
		t.Fatalf("equality proof other path missing")
	}
	assertDirectField(t, otherPath, "value")
	otherPath.Segments[0].Name = "changed-again"
	otherAgain, _ := equalityProof.OtherPath()
	assertDirectField(t, otherAgain, "value")
	proofSet := NewBranchProofSet(presenceProof, equalityProof)
	proofs := proofSet.Proofs()
	proofs[0] = inequalityProof
	if got := proofSet.Proofs(); got[0].Kind() != BranchProofPathPresence {
		t.Fatalf("branch proof set exposed mutable slice, got %v", got[0].Kind())
	}

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
				DynamicIndexAdmissionAdmitted,
				DynamicIndexReadbackKeyAndValue,
			),
		},
		BranchProofs: map[cfg.Point]BranchProofSet{
			point: NewBranchProofSet(
				NewBranchPathPresenceProof(path.NewPath(symbol.ID(203), "err"), presence.Present()),
				NewBranchPathEqualityProof(
					path.NewPath(symbol.ID(204), "left").Field("value"),
					path.NewPath(symbol.ID(205), "right").Field("value"),
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
	input.DynamicIndexWrites[point] = NewDynamicIndexWrite(path.NewPath(symbol.ID(209), "changed"), callSource, callSource, DynamicIndexAdmissionRejected, DynamicIndexReadbackNone)
	input.BranchProofs[point] = NewBranchProofSet(NewBranchPathInequalityProof(path.NewPath(symbol.ID(210), "changed"), path.NewPath(symbol.ID(211), "changed")))
	input.ChannelSelects[point] = NewChannelSelectSet(NewChannelSelect(ChannelSelectConfig{SelectID: ChannelSelectID("changed"), Kind: ChannelSelectReceive}))

	if _, ok := facts.PathStaticMemberWrite(missing); ok {
		t.Fatal("missing static-member write returned ok")
	}
	if _, ok := facts.DynamicIndexWrite(missing); ok {
		t.Fatal("missing dynamic-index write returned ok")
	}
	if got := facts.BranchProofs(missing); len(got) != 0 {
		t.Fatalf("missing branch proofs = %#v, want empty", got)
	}
	if got := facts.ChannelSelects(missing); len(got) != 0 {
		t.Fatalf("missing channel selects = %#v, want empty", got)
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
		dynamicAgain.Admission() != DynamicIndexAdmissionAdmitted ||
		dynamicAgain.ReadbackIntent() != DynamicIndexReadbackKeyAndValue {
		t.Fatalf("dynamic write = %#v", dynamicAgain)
	}

	proofs := facts.BranchProofs(point)
	if len(proofs) != 2 {
		t.Fatalf("branch proofs len = %d, want 2", len(proofs))
	}
	if proofs[0].Kind() != BranchProofPathPresence {
		t.Fatalf("branch proof kind = %v, want presence", proofs[0].Kind())
	}
	assertPathEqual(t, proofs[0].Path(), path.NewPath(symbol.ID(203), "err"))
	if got, ok := proofs[0].Presence(); !ok || !presence.Equal(got, presence.Present()) {
		t.Fatalf("branch proof presence = %s/%v, want present/true", got, ok)
	}
	assertDirectField(t, proofs[1].Path(), "value")
	otherPath, ok := proofs[1].OtherPath()
	if !ok {
		t.Fatal("branch equality other path missing")
	}
	assertDirectField(t, otherPath, "value")
	otherPath.Segments[0].Name = "mutated"
	proofs[0] = NewBranchPathInequalityProof(path.NewPath(symbol.ID(212), "mutated"), path.NewPath(symbol.ID(213), "mutated"))
	proofsAgain := facts.BranchProofs(point)
	if proofsAgain[0].Kind() != BranchProofPathPresence {
		t.Fatalf("facts branch proofs exposed mutable slice, got %v", proofsAgain[0].Kind())
	}
	otherAgain, _ := proofsAgain[1].OtherPath()
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
