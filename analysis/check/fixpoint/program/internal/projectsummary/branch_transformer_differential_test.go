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
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestBranchTransformerMatchesCanonicalConcreteProjection(t *testing.T) {
	reg := standard.Registry()
	fn := parseBranchTransformerFunction(t, `function f(flag: boolean): integer
		if flag then return 1 else return 2 end
	end`)
	prepared, err := body.PrepareFunction(fn, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("PrepareFunction: %v", err)
	}
	plan := prepared.OperationPlan()
	params := plan.BoundaryParams()
	if len(params) != 1 {
		t.Fatalf("boundary params = %v, want one", params)
	}
	shape := transformer.Shape{Params: 1}
	relation := transformer.NewPlanCompiler().Compile(reg, prepared.Graph(), plan, shape)
	if reason := relation.ContextualReason(); reason != "" {
		t.Fatalf("branch relation compiled contextually: %s", reason)
	}
	caller := parseBranchTransformerFunction(t, `function caller(flag: boolean): integer
		return f(flag)
	end`)
	callerPrepared, err := body.PrepareFunction(caller, body.Config{Registry: reg, Globals: []string{"f"}})
	if err != nil {
		t.Fatalf("PrepareFunction caller: %v", err)
	}
	callerParams := callerPrepared.OperationPlan().BoundaryParams()
	if len(callerParams) != 1 {
		t.Fatalf("caller boundary params = %v, want one", callerParams)
	}

	for _, input := range []product.Value{
		typevalue.LiteralBool(reg, true),
		typevalue.LiteralBool(reg, false),
	} {
		cursor, cursorErr := transformer.NewBindingCursor(shape, []product.Value{input}, nil)
		if cursorErr != nil {
			t.Fatal(cursorErr)
		}
		got, exact := relation.Specialize(cursor, nil, nil)
		if !exact {
			t.Fatalf("branch relation did not specialize for %#v", input)
		}
		concrete, solveErr := body.SolvePrepared(prepared, body.SolveConfig{
			EntryState: state.State{}.WriteValue(reg, key.SymbolValue(params[0]), input),
		})
		if solveErr != nil {
			t.Fatalf("SolvePrepared: %v", solveErr)
		}
		want := summary.Normalize(reg, FromResult(concrete))
		if !summary.Equal(reg, got, want) || summary.NormalizedPayloadDigest(reg, got) != summary.NormalizedPayloadDigest(reg, want) {
			var returnAxes []string
			if len(got.Returns) == 1 && len(want.Returns) == 1 {
				returnAxes = product.DifferenceAxes(reg, got.Returns[0], want.Returns[0])
			}
			t.Fatalf("symbolic/canonical concrete Summary differs for %#v (return axes %v)\n got=%#v\nwant=%#v", input, returnAxes, got, want)
		}
		assertBranchProductionComposition(t, reg, callerPrepared, callerParams[0], input, got, want)
	}
}

func assertBranchProductionComposition(t *testing.T, reg *axis.Registry, prepared *body.Static, param symbol.ID, input product.Value, gotSummary, wantSummary summary.Summary) {
	t.Helper()
	entry := state.State{}.WriteValue(reg, key.SymbolValue(param), input)
	solve := func(sum summary.Summary) *body.Result {
		result, err := body.SolvePrepared(prepared, body.SolveConfig{
			EntryState: entry,
			CallOutcomeFactory: func(ctx body.CallOutcomeContext) callpayload.CallOutcomeProvider {
				transaction := callresult.NewPreparedSummaryTransaction(callresult.ProviderConfig{
					ProtectedCall: ctx.ProtectedCall,
					CalleeValue:   callresult.CalleeValueFunc(ctx.CalleeValue), ReceiverCallable: callresult.ReceiverCallableFunc(ctx.ReceiverCallable),
					Facts: ctx.Facts, Sources: ctx.Sources,
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
	got, want := solve(gotSummary), solve(wantSummary)
	point := branchTransformerCallPoint(t, got)
	wantPoint := branchTransformerCallPoint(t, want)
	if point != wantPoint {
		t.Fatalf("call points = %d/%d", point, wantPoint)
	}
	gotOutcome, gotOK := got.CallOutcomeAt(point)
	wantOutcome, wantOK := want.CallOutcomeAt(point)
	if gotOK != wantOK || !reflect.DeepEqual(gotOutcome, wantOutcome) {
		t.Fatalf("production CallOutcome differs\n got=%#v\nwant=%#v", gotOutcome, wantOutcome)
	}
	gotState, gotOK := got.StateAtBoundary(point)
	wantState, wantOK := want.StateAtBoundary(point)
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
	gotDiagnostics, err := json.Marshal(diagnostics.Produce(got))
	if err != nil {
		t.Fatal(err)
	}
	wantDiagnostics, err := json.Marshal(diagnostics.Produce(want))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotDiagnostics, wantDiagnostics) {
		t.Fatalf("production caller diagnostic JSON differs\n got=%s\nwant=%s", gotDiagnostics, wantDiagnostics)
	}
}

func branchTransformerCallPoint(t *testing.T, result *body.Result) cfg.Point {
	t.Helper()
	for point := cfg.Point(0); int(point) < result.Graph().Size(); point++ {
		if _, ok := result.CallSiteView(point); ok {
			return point
		}
	}
	t.Fatal("caller call point missing")
	return 0
}

func parseBranchTransformerFunction(t *testing.T, source string) *ast.FunctionExpr {
	t.Helper()
	stmts, err := parse.ParseString(source, "branch_transformer_differential.lua")
	if err != nil {
		t.Fatal(err)
	}
	if len(stmts) != 1 {
		t.Fatalf("statements = %d, want one function", len(stmts))
	}
	def, ok := stmts[0].(*ast.FuncDefStmt)
	if !ok || def.Func == nil {
		t.Fatalf("statement = %T, want function definition", stmts[0])
	}
	return def.Func
}
