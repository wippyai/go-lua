package composite

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	lualower "github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/schema"
)

const callStageTransportSource = `
local function identity(value) return value end
local result = identity(1)
return result
`

// callStageTransportDeclarations reads the sealed issuance directory as
// declarations: which mounted factor axis each rule key writes, and which axes
// the rules issued at one call stage produce.
type callStageTransportDeclarations struct {
	axisOf   map[schema.Key]schema.Key
	mounted  map[schema.Key]struct{}
	byStage  map[programartifact.RuleStage]map[schema.Key]struct{}
	transfer programartifact.IssuanceDirectory
}

func readCallStageTransportDeclarations(t *testing.T) callStageTransportDeclarations {
	t.Helper()
	directory, ok := ArtifactIssuanceDirectory()
	if !ok {
		t.Fatal("issuance directory did not project from the sealed table")
	}
	read := callStageTransportDeclarations{
		axisOf:   make(map[schema.Key]schema.Key),
		mounted:  make(map[schema.Key]struct{}),
		byStage:  make(map[programartifact.RuleStage]map[schema.Key]struct{}),
		transfer: directory,
	}
	for _, placement := range directory {
		if !placement.Transport {
			continue
		}
		read.axisOf[placement.Key] = placement.Writes
		read.mounted[placement.Writes] = struct{}{}
		axes := read.byStage[placement.Stage]
		if axes == nil {
			axes = make(map[schema.Key]struct{})
			read.byStage[placement.Stage] = axes
		}
		axes[placement.Writes] = struct{}{}
	}
	if len(read.mounted) == 0 {
		t.Fatal("the sealed directory declares no mounted factor axis")
	}
	return read
}

// axesOf projects one transported key list onto the declared factor axes and
// refuses a list that names one axis twice or names a key the directory never
// declared as a transport writer.
func (read callStageTransportDeclarations) axesOf(t *testing.T, label string, edge programartifact.LocalTransfer) map[schema.Key]struct{} {
	t.Helper()
	axes := make(map[schema.Key]struct{}, edge.WritesCount())
	for index := 0; index < edge.WritesCount(); index++ {
		key, keyOK := edge.WritesAt(index)
		if !keyOK {
			t.Fatalf("%s transport key %d is unavailable", label, index)
		}
		axis, declared := read.axisOf[key]
		if !declared {
			t.Fatalf("%s transports %q, which no mounted rule declares as a factor writer", label, key)
		}
		representative, representativeOK := read.transfer.TransportKey(axis)
		if !representativeOK || representative != key {
			t.Fatalf("%s names %q for axis %q; the declared transport key is %q", label, key, axis, representative)
		}
		if _, duplicate := axes[axis]; duplicate {
			t.Fatalf("%s transports axis %q twice", label, axis)
		}
		axes[axis] = struct{}{}
	}
	return axes
}

func assertSameAxes(t *testing.T, label string, got, want map[schema.Key]struct{}) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s carries %v, want %v", label, got, want)
	}
	for axis := range want {
		if _, present := got[axis]; !present {
			t.Fatalf("%s carries %v, want %v", label, got, want)
		}
	}
}

// TestCallStageTransportNamesDeclaredFactorAxes is the G4 law. The call-stage
// transport plan is stated in the sealed directory's own vocabulary: the axis
// the effect stage writes reaches the summary stage straight from the base,
// every other mounted axis enters at dispatch, and the axis the dispatch stage
// writes is carried forward to both of its successors. Every transported key
// is the directory's declared transport key for the axis it names, so no
// authored key list survives in the compiler.
func TestCallStageTransportNamesDeclaredFactorAxes(t *testing.T) {
	if _, failure := Table(); failure.Available() {
		t.Fatalf("declaration table rejected: contributor=%d law=%d", failure.Contributor, failure.Law)
	}
	read := readCallStageTransportDeclarations(t)
	published, err := lualower.Lower(lualower.Source{Name: "call-stage-transport.lua", Text: []byte(callStageTransportSource)})
	if err != nil {
		t.Fatal(err)
	}
	compilation, compilationOK := Global()
	if !compilationOK {
		t.Fatal("the program schema receipt is unavailable")
	}
	artifact, failure := CompileArtifactDetailed(published, compilation)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile the call fixture: %s", failure.Error())
	}
	var dispatch, effect identity.ContentID
	for index := 0; index < artifact.RulePlacementCount(); index++ {
		row, rowOK := artifact.RulePlacementAt(index)
		point, pointOK := row.PointAt(0)
		if !rowOK || !pointOK {
			t.Fatalf("placement %d is unavailable", index)
		}
		switch row.Stage() {
		case programartifact.RuleStageCallDispatch:
			dispatch = point
		case programartifact.RuleStageCallEffect:
			effect = point
		}
	}
	if !dispatch.Available() || !effect.Available() {
		t.Fatal("the call fixture placed no call-dispatch and call-effect stage")
	}
	edges := make(map[[2]identity.ContentID]programartifact.LocalTransfer, artifact.LocalTransferCount())
	var base, summary identity.ContentID
	for index := 0; index < artifact.LocalTransferCount(); index++ {
		edge, edgeOK := artifact.LocalTransferAt(index)
		if !edgeOK {
			t.Fatalf("local transfer %d is unavailable", index)
		}
		edges[[2]identity.ContentID{edge.From(), edge.To()}] = edge
		if edge.To() == dispatch {
			base = edge.From()
		}
		if edge.From() == dispatch && edge.To() != effect {
			summary = edge.To()
		}
	}
	if !base.Available() || !summary.Available() {
		t.Fatal("the call stage triple has no base transport and no summary stage")
	}
	entry, entryOK := edges[[2]identity.ContentID{base, dispatch}]
	bypass, bypassOK := edges[[2]identity.ContentID{base, summary}]
	forwardSummary, forwardSummaryOK := edges[[2]identity.ContentID{dispatch, summary}]
	full, fullOK := edges[[2]identity.ContentID{base, effect}]
	forwardEffect, forwardEffectOK := edges[[2]identity.ContentID{dispatch, effect}]
	if !entryOK || !bypassOK || !forwardSummaryOK || !fullOK || !forwardEffectOK {
		t.Fatal("the call stage triple is not spliced by the five declared transports")
	}
	if !full.FullEnvironment() {
		t.Fatal("the base to call-effect transport is not a full environment transport")
	}
	effectAxes := read.byStage[programartifact.RuleStageCallEffect]
	dispatchAxes := read.byStage[programartifact.RuleStageCallDispatch]
	if len(effectAxes) == 0 || len(dispatchAxes) == 0 {
		t.Fatal("the sealed directory issues no rule at a call stage")
	}
	entryAxes := make(map[schema.Key]struct{}, len(read.mounted))
	for axis := range read.mounted {
		if _, produced := effectAxes[axis]; !produced {
			entryAxes[axis] = struct{}{}
		}
	}
	assertSameAxes(t, "base to call-dispatch", read.axesOf(t, "base to call-dispatch", entry), entryAxes)
	assertSameAxes(t, "base to call-summary", read.axesOf(t, "base to call-summary", bypass), effectAxes)
	assertSameAxes(t, "call-dispatch to call-summary", read.axesOf(t, "call-dispatch to call-summary", forwardSummary), dispatchAxes)
	assertSameAxes(t, "call-dispatch to call-effect", read.axesOf(t, "call-dispatch to call-effect", forwardEffect), dispatchAxes)
}
