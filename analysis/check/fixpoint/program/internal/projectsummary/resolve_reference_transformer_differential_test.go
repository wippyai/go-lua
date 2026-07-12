package projectsummary

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestResolveReferenceRelationMatchesCanonicalProductionPaths(t *testing.T) {
	reg := standard.Registry()
	prepared := prepareResolveReferenceRelationFixture(t)
	plan := prepared.OperationPlan()
	params := plan.BoundaryParams()
	if len(params) != 2 {
		t.Fatalf("resolve_reference boundary params = %v, want self/name", params)
	}
	shape := transformer.Shape{Params: 2}
	relation := transformer.NewPlanCompiler().Compile(reg, prepared.Graph(), plan, shape)
	if reason := relation.ContextualReason(); reason != "" {
		t.Fatalf("resolve_reference relation compiled contextually: %s", reason)
	}

	base := state.State{}
	tests := []struct {
		name string
		key  product.Value
	}{{name: "nil name", key: typevalue.Nil(reg)}}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			values := []product.Value{product.Top(), tc.key}
			cursor, err := transformer.NewBindingCursor(shape, values, []pathdom.Path{
				pathdom.NewPlaceholder(0), pathdom.NewPlaceholder(1),
			})
			if err != nil {
				t.Fatal(err)
			}
			stats := &body.Stats{}
			got, exact := relation.SpecializeWithContext(cursor, nil, transformer.SpecializationContext{})
			if !exact || stats.BodySolves != 0 {
				t.Fatalf("relation specialization exact/solves = %v/%d, want true/0", exact, stats.BodySolves)
			}
			entry := base
			for i, param := range params {
				entry = entry.WriteValue(reg, statekey.SymbolValue(param), values[i])
			}
			concrete, err := body.SolvePrepared(prepared, body.SolveConfig{EntryState: entry, Stats: stats})
			if err != nil {
				t.Fatal(err)
			}
			want := summary.Normalize(reg, FromResult(concrete))
			if !summary.Equal(reg, got, want) || summary.NormalizedPayloadDigest(reg, got) != summary.NormalizedPayloadDigest(reg, want) {
				t.Fatalf("symbolic/canonical Summary differs\n got=%#v\nwant=%#v", got, want)
			}
			if stats.BodySolves != 1 {
				t.Fatalf("canonical oracle solves = %d, want one", stats.BodySolves)
			}
			assertResolveReferenceProductionComposition(t, prepared, base, values, got, want)
		})
	}
}

func assertResolveReferenceProductionComposition(t *testing.T, prepared *body.Static, base state.State, values []product.Value, got, want summary.Summary) {
	t.Helper()
	reg := standard.Registry()
	caller := parseBranchTransformerFunction(t, `function caller(self, name)
		return resolve_reference(self, name)
	end`)
	callerPrepared, err := body.PrepareFunction(caller, body.Config{Registry: reg, Globals: []string{"resolve_reference"}})
	if err != nil {
		t.Fatal(err)
	}
	callerEntry := base
	for i, param := range callerPrepared.OperationPlan().BoundaryParams() {
		callerEntry = callerEntry.WriteValue(reg, statekey.SymbolValue(param), values[i])
	}
	gotResult := solveDynamicIndexCaller(t, callerPrepared, callerEntry, got)
	wantResult := solveDynamicIndexCaller(t, callerPrepared, callerEntry, want)
	point := dynamicIndexTransformerCallPoint(t, gotResult)
	wantPoint := dynamicIndexTransformerCallPoint(t, wantResult)
	gotOutcome, gotOK := gotResult.CallOutcomeAt(point)
	wantOutcome, wantOK := wantResult.CallOutcomeAt(wantPoint)
	if gotOK != wantOK || !reflect.DeepEqual(gotOutcome, wantOutcome) {
		t.Fatalf("prepared CallOutcome differs\n got=%#v\nwant=%#v", gotOutcome, wantOutcome)
	}
	gotState, gotOK := gotResult.StateAtBoundary(point)
	wantState, wantOK := wantResult.StateAtBoundary(wantPoint)
	if gotOK != wantOK {
		t.Fatalf("post-call state presence = %v/%v", gotOK, wantOK)
	}
	lanes := state.DefaultLaneCatalog().LaneSet().IDs()
	if len(lanes) != 17 {
		t.Fatalf("state lane catalog = %d, want 17", len(lanes))
	}
	for _, lane := range lanes {
		domain, err := state.TryDomainWithLanes(reg, []state.LaneID{lane})
		if err != nil {
			t.Fatal(err)
		}
		if gotOK && !domain.Equal(gotState, wantState) {
			t.Fatalf("caller state differs on lane %q", lane)
		}
	}
	gotDiagnostics, _ := json.Marshal(diagnostics.Produce(gotResult))
	wantDiagnostics, _ := json.Marshal(diagnostics.Produce(wantResult))
	if !reflect.DeepEqual(gotDiagnostics, wantDiagnostics) {
		t.Fatalf("diagnostic JSON differs\n got=%s\nwant=%s", gotDiagnostics, wantDiagnostics)
	}
}

func prepareResolveReferenceRelationFixture(t testing.TB) *body.Static {
	t.Helper()
	src, err := os.ReadFile("../../../../../../testdata/fixtures/regression/deadlock-compiler-lua/main.lua")
	if err != nil {
		t.Fatal(err)
	}
	stmts, err := parse.ParseString(string(src), "deadlock-compiler-lua/main.lua")
	if err != nil {
		t.Fatal(err)
	}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"uuid"}})
	for _, origin := range bindings.FunctionOrigins() {
		if origin.Func.Line() == 289 {
			prepared, err := body.PrepareBoundFunction(origin.Func, bindings, body.Config{
				Registry: standard.Registry(), Globals: []string{"uuid"},
				Signatures: signaturelookup.Source{IncludeStdlib: true},
			})
			if err != nil {
				t.Fatal(err)
			}
			return prepared
		}
	}
	t.Fatal("FlowGraph.resolve_reference at line 289 is missing")
	return nil
}
