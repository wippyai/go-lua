package program

import (
	"bytes"
	"context"
	"sort"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
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

// This parity test intentionally keeps the retiring State-shaped publication
// alive as its oracle. The direct normal-return readers must keep matching it
// until the following slice removes that old path.
func TestFormalNormalReturnReadersMatchStatePublication(t *testing.T) {
	statements := parseRelationProgramInputChunk(t, `
local function require_nonempty(value: string): string
    if value == "" then
        error("empty")
    end
    return value
end
return require_nonempty("ok")
`)
	bindings := bind.BindChunk(statements, bind.Options{Globals: []string{"error"}})
	registry := standard.Registry()
	check := body.Config{
		Registry: registry, Context: context.Background(), Globals: []string{"error"},
		Signatures: signaturelookup.Source{IncludeStdlib: true},
	}
	keys := collectKeys(bindings, rootKey(summary.SummaryKey{}), registry, nil, check.ModuleExports, statements)
	prepared, err := prepareBoundChunkBodies(statements, bindings, check, keys)
	if err != nil {
		t.Fatal(err)
	}
	ctx, factories, err := newRelationProgramExecutionFactories(check.Context, prepared, check)
	if err != nil {
		t.Fatal(err)
	}
	units, err := relationProgramInput(prepared, factories, check.Initial)
	if err != nil {
		t.Fatal(err)
	}
	formal, err := transformer.FreezeRelationProgram(units, prepared.callTopology)
	if err != nil {
		t.Fatal(err)
	}
	rootBody := prepared.root.StableLexicalBodyID()
	if len(prepared.functions) != 1 {
		t.Fatalf("representative body count = %d, want one function", len(prepared.functions))
	}
	var targetBody lexicalidentity.StableLexicalBodyID
	for _, static := range prepared.functions {
		targetBody = static.StableLexicalBodyID()
	}
	if targetBody == (lexicalidentity.StableLexicalBodyID{}) {
		t.Fatal("representative function has no lexical identity")
	}
	view, err := formal.Solve(ctx, rootBody, check.EntryState)
	if err != nil {
		t.Fatal(err)
	}
	var direct transformer.FormalLexicalBodyCoordinates
	for _, lexical := range view.LexicalBodies() {
		if lexical.Body == targetBody {
			direct = lexical
			break
		}
	}
	if direct.Body == (lexicalidentity.StableLexicalBodyID{}) {
		t.Fatal("formal root has no direct observations")
	}
	bodyKeys, err := relationProgramBodyKeys(prepared, prepared.root, keys)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := publishFormalLexicalProgram(
		ctx, view.LexicalBodies(), factories, rootBody, check.EntryState, check.Initial,
		check, bodyKeys, prepared, keys,
	)
	if err != nil {
		t.Fatal(err)
	}
	stateResult := legacy.results[targetBody]
	if stateResult == nil {
		t.Fatal("legacy State publication has no root result")
	}
	entry, entryOK := stateResult.EntryState()
	exit, exitOK := stateResult.ExitState()
	if !entryOK || !exitOK {
		t.Fatal("representative body has no State-shaped entry or exit")
	}
	slots := stateResult.ParameterValueSlots()
	if len(slots) == 0 || len(direct.NormalReturnParameters.Entry) != len(slots) || len(direct.NormalReturnParameters.Exit) != len(slots) {
		t.Fatalf("direct parameter reader widths = entry:%d exit:%d slots:%d", len(direct.NormalReturnParameters.Entry), len(direct.NormalReturnParameters.Exit), len(slots))
	}
	for index, slot := range slots {
		if got, want := direct.NormalReturnParameters.Entry[index], entry.ReadValue(registry, slot); !product.Equal(registry, got, want) {
			t.Fatalf("direct normal-return entry parameter %d = %#v, want State value %#v", index, got, want)
		}
		if got, want := direct.NormalReturnParameters.Exit[index], exit.ReadValue(registry, slot); !product.Equal(registry, got, want) {
			t.Fatalf("direct normal-return exit parameter %d = %#v, want State value %#v", index, got, want)
		}
	}
	if got, want := direct.NormalReturnParameters.HasNormalExit, exitOK; got != want {
		t.Fatalf("direct normal-exit availability = %t, want State-shaped %t", got, want)
	}
	for _, point := range cfg.RPOReadOnly(stateResult.Graph()) {
		want := legacyCanCompleteNormally(registry, stateResult, point, make(map[cfg.Point]bool), make(map[cfg.Point]struct{}))
		got, present := direct.NormalReturnReachability[point]
		if !present || got != want {
			t.Fatalf("direct normal-return reachability at point %d = %t/%t, want State-shaped %t", point, got, present, want)
		}
	}
}

func legacyCanCompleteNormally(
	registry *axis.Registry,
	result *body.Result,
	point cfg.Point,
	memo map[cfg.Point]bool,
	visiting map[cfg.Point]struct{},
) bool {
	if got, present := memo[point]; present {
		return got
	}
	if _, cycle := visiting[point]; cycle {
		return true
	}
	at, present := result.StateAt(point)
	if !present || state.Domain(registry).Equal(at, state.State{}) {
		memo[point] = false
		return false
	}
	if point == result.Graph().Exit() {
		memo[point] = true
		return true
	}
	if result.NoNormalReturn(point) {
		memo[point] = false
		return false
	}
	visiting[point] = struct{}{}
	defer delete(visiting, point)
	for _, successor := range cfg.SuccessorsReadOnly(result.Graph(), point) {
		if legacyCanCompleteNormally(registry, result, successor, memo, visiting) {
			memo[point] = true
			return true
		}
	}
	memo[point] = false
	return false
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
