package program

import (
	"bytes"
	"context"
	"sort"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestFormalLexicalPublicationOwnsOneResultAndSummaryPerBodyWithTwoCallers(t *testing.T) {
	stmts := parseRelationProgramInputChunk(t, `
local function identity(value: string): string return value end
local function left(): string return identity("left") end
local function right(): string return identity("right") end
return left(), right()
`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	check := body.Config{Registry: standard.Registry(), Context: context.Background()}
	keys := collectKeys(bindings, rootKey(summary.SummaryKey{}), check.Registry, nil, check.ModuleExports, stmts)
	prepared, err := prepareBoundChunkBodies(stmts, bindings, check, keys)
	if err != nil {
		t.Fatal(err)
	}
	ctx, factories, err := newRelationProgramExecutionFactories(check.Context, prepared, check)
	if err != nil {
		t.Fatal(err)
	}
	rootBody := prepared.root.StableLexicalBodyID()
	coordinates := make([]transformer.FormalLexicalBodyCoordinates, 0, len(factories))
	for bodyID, factory := range factories {
		graph := factory.Graph()
		reachable := state.Reachable(factory.Domain().Bottom())
		lexical := transformer.FormalLexicalBodyCoordinates{
			Body:                bodyID,
			PointInputs:         make(map[cfg.Point]state.State, graph.Size()),
			PlannedNodeOutputs:  make(map[cfg.Point]state.State, graph.Size()),
			PointReachable:      make(map[cfg.Point]bool, graph.Size()),
			NodeOutputReachable: make(map[cfg.Point]bool, graph.Size()),
			EdgeNormal:          make(map[cfg.Edge]bool),
			CallOutcomes:        make(map[cfg.Point]callpayload.CallOutcome),
		}
		facts := relationProgramStaticByBody(prepared, bodyID).OperationPlan().Facts()
		for _, point := range cfg.RPOReadOnly(graph) {
			lexical.PointInputs[point], lexical.PlannedNodeOutputs[point] = reachable, reachable
			lexical.PointReachable[point], lexical.NodeOutputReachable[point] = true, true
			if _, call := facts.CallSiteView(point); call {
				lexical.CallOutcomes[point] = callpayload.CallOutcome{}
			}
			if graph.IsBranch(point) {
				for _, successor := range cfg.SuccessorsReadOnly(graph, point) {
					lexical.EdgeNormal[cfg.Edge{From: point, To: successor}] = true
				}
			}
		}
		coordinates = append(coordinates, lexical)
	}
	sort.Slice(coordinates, func(i, j int) bool { return bytes.Compare(coordinates[i].Body[:], coordinates[j].Body[:]) < 0 })
	if len(coordinates) < 4 {
		t.Fatalf("lexical bodies = %d, want root plus three functions", len(coordinates))
	}
	rootIndex := -1
	for index := range coordinates {
		if coordinates[index].Body == rootBody {
			rootIndex = index
			break
		}
	}
	if rootIndex < 0 {
		t.Fatal("root coordinate is absent")
	}
	targetIndex := (rootIndex + 1) % len(coordinates)
	secondCaller := (targetIndex + 1) % len(coordinates)
	if secondCaller == rootIndex {
		secondCaller = (secondCaller + 1) % len(coordinates)
	}
	target := coordinates[targetIndex].Body
	coordinates[rootIndex].Calls = []transformer.FormalLexicalCallDependency{{
		Point: factories[rootBody].Graph().Entry(), Occurrence: 1, Target: target,
	}}
	coordinates[secondCaller].Calls = []transformer.FormalLexicalCallDependency{{
		Point: factories[coordinates[secondCaller].Body].Graph().Entry(), Occurrence: 1, Target: target,
	}}
	bodyKeys, err := relationProgramBodyKeys(prepared, prepared.root, keys)
	if err != nil {
		t.Fatal(err)
	}
	published, err := publishFormalLexicalProgram(
		ctx, coordinates, factories, rootBody, check.EntryState, check.Initial,
		check, bodyKeys, prepared, keys,
	)
	if err != nil {
		t.Fatal(err)
	}
	if published.root == nil || len(published.results) != len(factories) || len(published.snapshot.EntriesOwnedNormalized()) != len(factories) {
		t.Fatalf("formal lexical publication = root:%t results:%d summaries:%d, want exactly %d bodies",
			published.root != nil, len(published.results), len(published.snapshot.EntriesOwnedNormalized()), len(factories))
	}
	for bodyID, result := range published.results {
		if result == nil {
			t.Fatalf("formal lexical result %s = %#v", bodyID, result)
		}
	}
	firstRootVersion := published.root.ResultVersion()
	coordinates[targetIndex].NormalReturnParameters.HasNormalExit = true
	formalRepublished, err := publishFormalLexicalProgram(
		ctx, coordinates, factories, rootBody, check.EntryState, check.Initial,
		check, bodyKeys, prepared, keys,
	)
	if err != nil {
		t.Fatal(err)
	}
	if formalRepublished.root.ResultVersion() == firstRootVersion {
		t.Fatal("formal normal-return observation did not alter caller lexical lineage")
	}
	firstRootVersion = formalRepublished.root.ResultVersion()
	coordinates[targetIndex].DiagnosticOutput = callpayload.DiagnosticOutput{SuspensionKnown: true}
	republished, err := publishFormalLexicalProgram(
		ctx, coordinates, factories, rootBody, check.EntryState, check.Initial,
		check, bodyKeys, prepared, keys,
	)
	if err != nil {
		t.Fatal(err)
	}
	if republished.root.ResultVersion() == firstRootVersion {
		t.Fatal("callee coordinate change did not alter caller lexical lineage")
	}
}

func relationProgramStaticByBody(prepared preparedBodies, bodyID lexicalidentity.StableLexicalBodyID) *body.Static {
	if prepared.root != nil && prepared.root.StableLexicalBodyID() == bodyID {
		return prepared.root
	}
	for _, static := range prepared.functions {
		if static != nil && static.StableLexicalBodyID() == bodyID {
			return static
		}
	}
	return nil
}
