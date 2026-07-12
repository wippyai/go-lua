package projectsummary

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/program/internal/callresult"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestDynamicIndexTransformerMatchesRealConcreteBoundaryAndProductionCall(t *testing.T) {
	reg := standard.Registry()
	fn := parseBranchTransformerFunction(t, `function f(t: {[string]: number}, k: string, v: number)
		t[k] = v
	end`)
	prepared, err := body.PrepareFunction(fn, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("PrepareFunction: %v", err)
	}
	plan := prepared.OperationPlan()
	params := plan.BoundaryParams()
	if len(params) != 3 {
		t.Fatalf("boundary params = %v, want t/k/v", params)
	}
	shape := transformer.Shape{Params: 3}
	relation := transformer.NewPlanCompiler().Compile(reg, prepared.Graph(), plan, shape)
	if reason := relation.ContextualReason(); reason != "" {
		t.Fatalf("dynamic-index relation compiled contextually: %s", reason)
	}

	tableType := typ.NewMap(typ.String, typ.Number)
	tableValue := typevalue.WithWitness(reg, typevalue.FromType(reg, tableType), tableType)
	keyValue := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)
	valueValue := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.Number), typ.Number)
	values := []product.Value{tableValue, keyValue, valueValue}
	cursor, err := transformer.NewBindingCursor(shape, values, []pathdom.Path{
		pathdom.NewPlaceholder(0), pathdom.NewPlaceholder(1), pathdom.NewPlaceholder(2),
	})
	if err != nil {
		t.Fatal(err)
	}
	got, exact := relation.SpecializeWithEffects(cursor, nil, transformer.SpecializationContext{}, ResolvedEffectSummaryResolver(reg))
	if !exact {
		t.Fatal("dynamic-index relation failed canonical effect specialization")
	}

	entry := state.State{}
	for i, param := range params {
		entry = entry.WriteValue(reg, key.SymbolValue(param), values[i])
	}
	bodySolves := 0
	concrete, err := body.SolvePrepared(prepared, body.SolveConfig{EntryState: entry})
	if err != nil {
		t.Fatalf("SolvePrepared: %v", err)
	}
	bodySolves++
	want := summary.Normalize(reg, FromResult(concrete))
	gotEffects := summary.Normalize(reg, summary.Summary{NormalReturnFacts: got.NormalReturnFacts})
	wantEffects := summary.Normalize(reg, summary.Summary{NormalReturnFacts: want.NormalReturnFacts})
	if !summary.Equal(reg, gotEffects, wantEffects) {
		t.Fatalf("symbolic/concrete dynamic effect Summary differs\n got=%#v\nwant=%#v", gotEffects.NormalReturnFacts, wantEffects.NormalReturnFacts)
	}
	if _, ok := relation.SpecializeWithEffects(cursor, nil, transformer.SpecializationContext{}, ResolvedEffectSummaryResolver(reg)); !ok || bodySolves != 1 {
		t.Fatalf("frozen specialization re-entered body solve: exact=%v solves=%d", ok, bodySolves)
	}

	caller := parseBranchTransformerFunction(t, `function caller(t: {[string]: number}, k: string, v: number)
		f(t, k, v)
	end`)
	callerPrepared, err := body.PrepareFunction(caller, body.Config{Registry: reg, Globals: []string{"f"}})
	if err != nil {
		t.Fatalf("PrepareFunction caller: %v", err)
	}
	callerParams := callerPrepared.OperationPlan().BoundaryParams()
	if len(callerParams) != 3 {
		t.Fatalf("caller params = %v, want t/k/v", callerParams)
	}
	callerEntry := state.State{}
	for i, param := range callerParams {
		callerEntry = callerEntry.WriteValue(reg, key.SymbolValue(param), values[i])
	}
	gotResult := solveDynamicIndexCaller(t, callerPrepared, callerEntry, got)
	wantResult := solveDynamicIndexCaller(t, callerPrepared, callerEntry, want)
	point := dynamicIndexTransformerCallPoint(t, gotResult)
	wantPoint := dynamicIndexTransformerCallPoint(t, wantResult)
	if point != wantPoint {
		t.Fatalf("call points = %d/%d", point, wantPoint)
	}
	gotOutcome, gotOK := gotResult.CallOutcomeAt(point)
	wantOutcome, wantOK := wantResult.CallOutcomeAt(wantPoint)
	if gotOK != wantOK || !reflect.DeepEqual(gotOutcome, wantOutcome) {
		t.Fatalf("production CallOutcome differs\n got=%#v\nwant=%#v", gotOutcome, wantOutcome)
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
			t.Fatalf("production caller state differs on lane %q", lane)
		}
	}
	gotDiagnostics, err := json.Marshal(diagnostics.Produce(gotResult))
	if err != nil {
		t.Fatal(err)
	}
	wantDiagnostics, err := json.Marshal(diagnostics.Produce(wantResult))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotDiagnostics, wantDiagnostics) {
		t.Fatalf("production diagnostics differ\n got=%s\nwant=%s", gotDiagnostics, wantDiagnostics)
	}
}

func solveDynamicIndexCaller(t testing.TB, prepared *body.Static, entry state.State, sum summary.Summary) *body.Result {
	t.Helper()
	result, err := body.SolvePrepared(prepared, body.SolveConfig{
		EntryState: entry,
		CallOutcomeFactory: func(ctx body.CallOutcomeContext) callpayload.CallOutcomeProvider {
			transaction := callresult.NewPreparedSummaryTransaction(callresult.ProviderConfig{
				ProtectedCall:    ctx.ProtectedCall,
				CalleeValue:      callresult.CalleeValueFunc(ctx.CalleeValue),
				ReceiverCallable: callresult.ReceiverCallableFunc(ctx.ReceiverCallable),
				Facts:            ctx.Facts, Sources: ctx.Sources,
				ReturnPresenceRelations: callresult.ReturnPresenceRelationsForPathFunc(ctx.ReturnPresenceRelationsPath),
				KeySpace:                ctx.KeySpace, TypeValues: ctx.TypeValues,
			})
			return func(node transfer.NodeContext, site factflow.CallSiteView, in state.State, read func(cfg.Point) state.State) callpayload.CallOutcome {
				return transaction.Apply(node, site, in, read, sum, nil, false)
			}
		},
	})
	if err != nil {
		t.Fatalf("SolvePrepared caller: %v", err)
	}
	return result
}

func dynamicIndexTransformerCallPoint(t testing.TB, result *body.Result) cfg.Point {
	t.Helper()
	for point := cfg.Point(0); int(point) < result.Graph().Size(); point++ {
		if _, ok := result.CallSiteView(point); ok {
			return point
		}
	}
	t.Fatal("caller call point missing")
	return 0
}
