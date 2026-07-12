package program

import (
	"os"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/program/internal/callresult"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/module/importlookup"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

type findRootNodesApplication struct {
	prepared *body.Static
	config   body.Config
	oracle   *body.Result
}

type findRootNodesPOCFixture struct {
	relation            transformer.Relation
	base                summary.Summary
	baseKey             summary.SummaryKey
	validateKey         summary.SummaryKey
	applications        []findRootNodesApplication
	result              Result
	stats               *Stats
	compileKey          summary.SummaryKey
	compileApplications []findRootNodesApplication
}

func TestFindRootNodesStructuredTransformerMatchesRealValidateGraphContexts(t *testing.T) {
	fixture := newFindRootNodesPOCFixture(t)
	if got := len(fixture.applications); got != 6 {
		t.Fatalf("validate_graph applications = %d, want 6 after dependency-first query scheduling", got)
	}
	if len(state.DefaultLanes()) != 17 {
		t.Fatalf("default State lanes = %d, want 17", len(state.DefaultLanes()))
	}

	// The audit observes intermediate fixed-point applications as dependencies
	// grow. The final application is the converged, materialized validate_graph
	// context whose two lexical calls are the production acceptance surface.
	applications := 0
	for solve, application := range fixture.applications[len(fixture.applications)-1:] {
		cursor, err := transformer.NewBindingCursor(fixture.relation.Shape(), nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		instantiated, ok := fixture.relation.Specialize(cursor, nil, nil)
		if !ok || !summary.Equal(application.config.Registry, instantiated, fixture.base) {
			t.Fatalf("solve %d relation specialization differs from normalized base Summary", solve+1)
		}
		if instantiated.HeapKeySpace != fixture.base.HeapKeySpace || instantiated.MaySuspend != fixture.base.MaySuspend {
			t.Fatalf("solve %d relation lost heap-keyspace or MaySuspend boundary metadata", solve+1)
		}

		adapted, err := solveValidateGraphWithFindRootNodesSummary(application, fixture.validateKey, fixture.baseKey, instantiated)
		if err != nil {
			t.Fatalf("solve %d adapted validate_graph: %v", solve+1, err)
		}
		points := findRootNodesCallPoints(t, application.oracle)
		if len(points) != 2 {
			t.Fatalf("solve %d find_root_nodes call points = %v, want two", solve+1, points)
		}
		for _, point := range points {
			applications++
			wantOutcome, wantOK := application.oracle.CallOutcomeAt(point)
			gotOutcome, gotOK := adapted.CallOutcomeAt(point)
			if wantOK != gotOK || !reflect.DeepEqual(wantOutcome, gotOutcome) {
				t.Fatalf("solve %d line %d CallOutcome differs\nwant=%#v\n got=%#v", solve+1, callLine(application.oracle, point), wantOutcome, gotOutcome)
			}
			want, wantOK := application.oracle.StateAtBoundary(point)
			got, gotOK := adapted.StateAtBoundary(point)
			if wantOK != gotOK {
				t.Fatalf("solve %d line %d post-call State presence differs", solve+1, callLine(application.oracle, point))
			}
			if !wantOK {
				continue
			}
			for _, lane := range state.DefaultLanes() {
				domain := state.DomainWithLanes(application.config.Registry, []state.LaneID{lane})
				if !domain.Equal(want, got) {
					t.Fatalf("solve %d line %d post-call State lane %s differs", solve+1, callLine(application.oracle, point), lane)
				}
			}
		}
	}
	if applications != 2 {
		t.Fatalf("transformer applications = %d, want 2", applications)
	}
	// The adapted path below contains Relation.Specialize and the production
	// Summary provider only. It has no program/body solver reference, so a
	// callee body solve after build is structurally impossible.
	t.Logf("find_root_nodes relation built once; exact at both validate_graph call sites without a callee-solver path (heap objects=%d fresh allocations=%d MaySuspend=%v)", len(fixture.base.HeapTableObjects), len(fixture.base.FreshHeapAllocations), fixture.base.MaySuspend)
}

func newFindRootNodesPOCFixture(tb testing.TB) findRootNodesPOCFixture {
	tb.Helper()
	src, err := os.ReadFile("../../../../testdata/fixtures/regression/deadlock-compiler-lua/main.lua")
	if err != nil {
		tb.Fatal(err)
	}
	stmts, err := parse.ParseString(string(src), "deadlock-compiler-lua/main.lua")
	if err != nil {
		tb.Fatal(err)
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
	findFn, validateFn := functionAtLine(tb, bindings, 690), functionAtLine(tb, bindings, 743)
	var applications []findRootNodesApplication
	var compileApplications []findRootNodesApplication
	stats := &Stats{}
	config := Config{Check: check, Stats: stats}
	config.semanticProgramAudit = func(prepared *body.Static, solveConfig body.Config, solved *body.Result) error {
		if fn := solved.Function(); fn != nil && fn.Line() == 743 {
			applications = append(applications, findRootNodesApplication{prepared: prepared, config: solveConfig, oracle: solved})
		} else if fn != nil && fn.Line() == 1304 {
			compileApplications = append(compileApplications, findRootNodesApplication{prepared: prepared, config: solveConfig, oracle: solved})
		}
		return nil
	}
	result, err := RunBoundChunk(stmts, bindings, config)
	if err != nil {
		tb.Fatal(err)
	}
	findSymbol, ok := bindings.FunctionSymbol(findFn)
	if !ok {
		tb.Fatal("find_root_nodes function symbol missing")
	}
	baseKey, ok := result.FunctionKey(findSymbol)
	if !ok {
		tb.Fatal("find_root_nodes base Summary key missing")
	}
	base, ok := result.Snapshot().Read(baseKey)
	if !ok {
		tb.Fatal("find_root_nodes base Summary missing")
	}
	base = summary.Normalize(reg, base)
	for _, allocation := range base.FreshHeapAllocations {
		if allocation.Placement == placement.Bottom || allocation.Placement == placement.Unknown {
			tb.Fatalf("find_root_nodes claimed fresh allocation %v without exact placement provenance: %v", allocation.ID, allocation.Placement)
		}
	}
	validateSymbol, ok := bindings.FunctionSymbol(validateFn)
	if !ok {
		tb.Fatal("validate_graph function symbol missing")
	}
	validateKey, ok := result.FunctionKey(validateSymbol)
	if !ok {
		tb.Fatal("validate_graph Summary key missing")
	}
	compileSymbol, ok := bindings.FunctionSymbol(functionAtLine(tb, bindings, 1304))
	if !ok {
		tb.Fatal("compiler.compile function symbol missing")
	}
	compileKey, ok := result.FunctionKey(compileSymbol)
	if !ok {
		tb.Fatal("compiler.compile Summary key missing")
	}
	if len(compileApplications) == 0 {
		tb.Fatal("compiler.compile audit applications missing")
	}
	relation := buildFindRootNodesRelation(tb, reg, base)
	return findRootNodesPOCFixture{
		relation: relation, base: base, baseKey: baseKey, validateKey: validateKey, applications: applications,
		result: result, stats: stats, compileKey: compileKey, compileApplications: compileApplications,
	}
}

func buildFindRootNodesRelation(tb testing.TB, reg *axis.Registry, base summary.Summary) transformer.Relation {
	tb.Helper()
	plan := operationplan.New(cfg.New(), factflow.FactsInput{})
	certificate, err := transformer.CertifyPlan(plan, transformer.DefaultSemanticCapabilityRegistry())
	if err != nil {
		tb.Fatal(err)
	}
	caps := transformer.DefaultOutputCapabilityRegistry()
	for _, kind := range summary.PresentFactKinds(base) {
		for _, lane := range state.DefaultLanes() {
			if err := caps.SetSummary(kind, lane, transformer.CapabilitySupported); err != nil {
				tb.Fatal(err)
			}
		}
	}
	builder := transformer.NewBuilder(reg, transformer.Shape{}, caps, plan)
	relation, err := builder.Build(certificate, []transformer.Row{{Guard: builder.Arena().True(), Output: base}})
	if err != nil {
		tb.Fatal(err)
	}
	return relation
}

func functionAtLine(tb testing.TB, bindings *bind.Result, line int) *ast.FunctionExpr {
	tb.Helper()
	for _, origin := range bindings.FunctionOrigins() {
		if origin.Func != nil && origin.Func.Line() == line {
			return origin.Func
		}
	}
	tb.Fatalf("function at line %d missing", line)
	return nil
}

func solveValidateGraphWithFindRootNodesSummary(application findRootNodesApplication, owner, key summary.SummaryKey, sum summary.Summary) (*body.Result, error) {
	baseFactory := application.config.CallOutcomeFactory
	solveConfig := application.config.SolveConfig()
	solveConfig.CallOutcomeFactory = func(ctx body.CallOutcomeContext) callpayload.CallOutcomeProvider {
		var original callpayload.CallOutcomeProvider
		if baseFactory != nil {
			original = baseFactory(ctx)
		}
		index := callresult.NewSummaryIndexBase(callresult.SummaryIndexBaseConfig{}).WithOwnerFunctionExpressionKeys(owner, nil)
		adapted := callresult.OutcomeProvider(callresult.ProviderConfig{
			Summaries:     summary.NewSnapshot(application.config.Registry, summary.EntrySummary{Key: key, Summary: sum}),
			ProtectedCall: ctx.ProtectedCall,
			KeyFor:        func(_ transfer.NodeContext, _ factflow.CallSiteView) (summary.SummaryKey, bool) { return key, true },
			CalleeValue:   callresult.CalleeValueFunc(ctx.CalleeValue), ReceiverCallable: callresult.ReceiverCallableFunc(ctx.ReceiverCallable),
			Facts: ctx.Facts, Index: index, Sources: ctx.Sources,
			ReturnPresenceRelations: callresult.ReturnPresenceRelationsForPathFunc(ctx.ReturnPresenceRelationsPath),
			KeySpace:                ctx.KeySpace, TypeValues: ctx.TypeValues,
		})
		return func(node transfer.NodeContext, site factflow.CallSiteView, in state.State, read func(cfg.Point) state.State) callpayload.CallOutcome {
			line := site.CallSpan().StartLine
			if (line == 773 || line == 843) && site.CalleePathRef().String() == "compiler.find_root_nodes" {
				return adapted(node, site, in, read)
			}
			if original != nil {
				return original(node, site, in, read)
			}
			return callpayload.CallOutcome{}
		}
	}
	return body.SolvePrepared(application.prepared, solveConfig)
}

func findRootNodesCallPoints(tb testing.TB, result *body.Result) []cfg.Point {
	tb.Helper()
	var out []cfg.Point
	for point := cfg.Point(0); int(point) < result.Graph().Size(); point++ {
		site, ok := result.CallSiteView(point)
		if !ok {
			continue
		}
		line := site.CallSpan().StartLine
		if (line == 773 || line == 843) && site.CalleePathRef().String() == "compiler.find_root_nodes" {
			out = append(out, point)
		}
	}
	return out
}

func callLine(result *body.Result, point cfg.Point) int {
	site, _ := result.CallSiteView(point)
	return site.CallSpan().StartLine
}

func BenchmarkFindRootNodesStructuredTransformer(b *testing.B) {
	fixture := newFindRootNodesPOCFixture(b)
	reg := fixture.applications[0].config.Registry
	b.Run("build", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = buildFindRootNodesRelation(b, reg, fixture.base)
		}
	})
	b.Run("two_relation_specializations", func(b *testing.B) {
		cursor, err := transformer.NewBindingCursor(fixture.relation.Shape(), nil, nil)
		if err != nil {
			b.Fatal(err)
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, ok := fixture.relation.Specialize(cursor, nil, nil); !ok {
				b.Fatal("first specialization failed")
			}
			if _, ok := fixture.relation.Specialize(cursor, nil, nil); !ok {
				b.Fatal("second specialization failed")
			}
		}
	})
}
