package cfgbuild_test

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestBuildChunkCallFactsMatchSemanticExtractor(t *testing.T) {
	stmts, err := parse.ParseString(`
local a, b = make(), pack()
sink(inner())
if can_access() and guard() then
    log(a)
end
for i = start_at(), stop_at(), step_by() do
    tick(i)
end
for k, v in iter(), state() do
    use(k, v)
end
return done(), tail()
`, "calls_parity.lua")
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	if built == nil {
		t.Fatal("BuildChunk returned nil")
	}
	extracted, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	for _, point := range built.Graph.RPO() {
		semFact, semOK := extracted.Call(point)
		cfgFact, cfgOK := built.Calls.Get(point)
		if semOK != cfgOK {
			t.Fatalf("call fact presence at point %d: cfgbuild=%v semantics=%v", point, cfgOK, semOK)
		}
		if !semOK {
			continue
		}
		assertCallFactParity(t, point, cfgFact, semFact)
	}
}

func assertCallFactParity(t *testing.T, point cfg.Point, got cfgbuild.Call, want semantics.CallFact) {
	t.Helper()
	if got.Call != want.Call {
		t.Fatalf("point %d call pointer differs: cfgbuild=%p semantics=%p", point, got.Call, want.Call)
	}
	if got.SourceStmt != want.SourceStmt || got.Stmt != want.Stmt {
		t.Fatalf("point %d source statement differs: cfgbuild=(%T,%T) semantics=(%T,%T)", point, got.SourceStmt, got.Stmt, want.SourceStmt, want.Stmt)
	}
	if uint8(got.Context) != uint8(want.Context) ||
		got.ExprIndex != want.ExprIndex ||
		got.ConditionNegated != want.ConditionNegated ||
		got.Final != want.Final ||
		got.Expanded != want.Expanded ||
		got.Adjusted != want.Adjusted ||
		got.OpenTail != want.OpenTail {
		t.Fatalf("point %d shape differs:\ncfgbuild=%s\nsemantics=%s", point, callShape(got), semanticCallShape(want))
	}
	if got.Func != want.Func || got.Receiver != want.Receiver || got.Method != want.Method {
		t.Fatalf("point %d callee differs: cfgbuild=(%p,%p,%q) semantics=(%p,%p,%q)", point, got.Func, got.Receiver, got.Method, want.Func, want.Receiver, want.Method)
	}
	if got.HasCalleePath != want.HasCalleePath || !got.CalleePath.Equal(want.CalleePath) ||
		got.HasReceiverPath != want.HasReceiverPath || !got.ReceiverPath.Equal(want.ReceiverPath) ||
		got.HasMethodPath != want.HasMethodPath || !got.MethodPath.Equal(want.MethodPath) {
		t.Fatalf("point %d paths differ:\ncfgbuild=%s\nsemantics=%s", point, callShape(got), semanticCallShape(want))
	}
	if got.HasReceiverSource != want.HasReceiverSource || got.ReceiverSource.Kind != want.ReceiverSource.Kind {
		t.Fatalf("point %d receiver source differs: cfgbuild=%#v semantics=%#v", point, got.ReceiverSource, want.ReceiverSource)
	}
	if got.HasCalleeSymbol != want.HasCalleeSymbol || got.CalleeSymbol != want.CalleeSymbol {
		t.Fatalf("point %d callee symbol differs: cfgbuild=(%v,%d) semantics=(%v,%d)", point, got.HasCalleeSymbol, got.CalleeSymbol, want.HasCalleeSymbol, want.CalleeSymbol)
	}
	if len(got.ArgumentSources) != len(want.ArgumentSources) || len(got.ArgumentSpans) != len(want.ArgumentSpans) || len(got.ArgumentLabels) != len(want.ArgumentLabels) {
		t.Fatalf("point %d argument metadata lengths differ: cfgbuild=(%d,%d,%d) semantics=(%d,%d,%d)", point, len(got.ArgumentSources), len(got.ArgumentSpans), len(got.ArgumentLabels), len(want.ArgumentSources), len(want.ArgumentSpans), len(want.ArgumentLabels))
	}
	assertResultTargetParity(t, point, got.ResultTargets, want.ResultTargets)
}

func assertResultTargetParity(t *testing.T, point cfg.Point, got []cfgbuild.CallResultTarget, want []semantics.CallResultTarget) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("point %d result target count = %d, want %d: %#v vs %#v", point, len(got), len(want), got, want)
	}
	for i := range want {
		if uint8(got[i].Kind) != uint8(want[i].Kind) ||
			got[i].Index != want[i].Index ||
			got[i].ResultIndex != want[i].ResultIndex ||
			got[i].Name != want[i].Name ||
			got[i].OpenTail != want[i].OpenTail ||
			got[i].HasSymbol != want[i].HasSymbol ||
			got[i].Symbol != want[i].Symbol ||
			got[i].HasPath != want[i].HasPath ||
			!got[i].Path.Equal(want[i].Path) {
			t.Fatalf("point %d result target %d differs:\ncfgbuild=%#v\nsemantics=%#v", point, i, got[i], want[i])
		}
	}
}

func callShape(f cfgbuild.Call) string {
	return fmt.Sprintf("context=%d expr=%d neg=%v final=%v expanded=%v adjusted=%v open=%v calleePath=(%v,%v) receiverPath=(%v,%v) methodPath=(%v,%v)",
		f.Context, f.ExprIndex, f.ConditionNegated, f.Final, f.Expanded, f.Adjusted, f.OpenTail,
		f.HasCalleePath, f.CalleePath, f.HasReceiverPath, f.ReceiverPath, f.HasMethodPath, f.MethodPath)
}

func semanticCallShape(f semantics.CallFact) string {
	return fmt.Sprintf("context=%d expr=%d neg=%v final=%v expanded=%v adjusted=%v open=%v calleePath=(%v,%v) receiverPath=(%v,%v) methodPath=(%v,%v)",
		f.Context, f.ExprIndex, f.ConditionNegated, f.Final, f.Expanded, f.Adjusted, f.OpenTail,
		f.HasCalleePath, f.CalleePath, f.HasReceiverPath, f.ReceiverPath, f.HasMethodPath, f.MethodPath)
}
