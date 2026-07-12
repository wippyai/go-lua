package program

import (
	"os"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/module/importlookup"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestResolveReferenceUnmatchedCallsDoNotSeedContextSolves(t *testing.T) {
	src, err := os.ReadFile("../../../../testdata/fixtures/regression/deadlock-compiler-lua/main.lua")
	if err != nil {
		t.Fatal(err)
	}
	stmts, err := parse.ParseString(string(src), "deadlock-compiler-lua/main.lua")
	if err != nil {
		t.Fatal(err)
	}
	reg := standard.Registry()
	uuid := manifest.New("uuid")
	uuid.SetExport(typetable.NewRecord().Field("v7", typ.Func().Returns(typ.String).Build()).Build())
	check := body.Config{
		Registry: reg, TypeValues: typevalue.NewCache(), Globals: []string{"uuid"}, Schedule: transfer.ScheduleWTO,
		Signatures:    signaturelookup.Source{IncludeStdlib: true, Manifests: []*manifest.Manifest{uuid}},
		ModuleExports: importlookup.Source{Manifests: []*manifest.Manifest{uuid}},
	}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: body.Globals(check)})
	resolveFn, validateFn := functionAtLine(t, bindings, 289), functionAtLine(t, bindings, 743)
	stats := &Stats{}
	var finalValidate *body.Result
	config := Config{Check: check, Stats: stats}
	config.semanticProgramAudit = func(_ *body.Static, _ body.Config, solved *body.Result) error {
		if fn := solved.Function(); fn != nil && fn.Line() == 743 {
			finalValidate = solved
		}
		return nil
	}
	result, err := RunBoundChunk(stmts, bindings, config)
	if err != nil {
		t.Fatal(err)
	}
	resolveSymbol, _ := bindings.FunctionSymbol(resolveFn)
	resolveKey, _ := result.FunctionKey(resolveSymbol)
	validateSymbol, _ := bindings.FunctionSymbol(validateFn)
	validateKey, _ := result.FunctionKey(validateSymbol)
	resolveSolves, validateSolves := map[SolvePhase]int{}, map[SolvePhase]int{}
	resolveContextSolves := 0
	for _, entry := range stats.BodySolveAttribution() {
		switch entry.Function.Ref {
		case resolveKey.Ref:
			resolveSolves[entry.Phase] += entry.BodySolves
			if entry.Context {
				resolveContextSolves += entry.BodySolves
			}
		case validateKey.Ref:
			validateSolves[entry.Phase] += entry.BodySolves
		}
	}
	if finalValidate == nil {
		t.Fatal("final validate_graph result missing")
	}
	unmatched := 0
	for point := cfg.Point(0); int(point) < finalValidate.Graph().Size(); point++ {
		site, ok := finalValidate.CallSiteView(point)
		if !ok || !resolveReferenceUnmatchedSite(site) {
			continue
		}
		out, ok := finalValidate.CallOutcomeAt(point)
		if !ok || out.SuspensionKnown || out.PostReturnAuthority {
			t.Fatalf("line %d outcome unexpectedly matched a Summary: %#v", site.CallSpan().StartLine, out)
		}
		runtime, runtimeOK := finalValidate.CallCalleeValueAtBoundary(point, site)
		_, runtimeIdentity := product.Get(reg, runtime, identity.Key).ID()
		if !runtimeOK || runtimeIdentity {
			t.Fatalf("line %d runtime callee identity = %v/%v, want resolved callable without identity", site.CallSpan().StartLine, runtimeOK, runtimeIdentity)
		}
		unmatched++
	}
	if unmatched != 4 {
		t.Fatalf("unmatched resolve_reference sites = %d, want 4", unmatched)
	}
	if resolveContextSolves != 0 {
		t.Fatalf("resolve_reference context body solves = %d, want zero for runtime-unmatched edges (all solves=%v)", resolveContextSolves, resolveSolves)
	}
	t.Logf("resolve_reference unmatched sites=4 context solves=0, base solves=%v; validate_graph solves=%v", resolveSolves, validateSolves)
}

func resolveReferenceUnmatchedSite(site factflow.CallSiteView) bool {
	line := site.CallSpan().StartLine
	return (line == 767 || line == 787 || line == 834 || line == 856) && site.CalleePathRef().String() == "graph.resolve_reference"
}
