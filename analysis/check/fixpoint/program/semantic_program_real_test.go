package program

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	summaryprojection "github.com/wippyai/go-lua/analysis/check/fixpoint/program/internal/projectsummary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/module/importlookup"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestSemanticProgramValidateGraphConcreteDifferential(t *testing.T) {
	src, err := os.ReadFile("../../../../testdata/fixtures/regression/deadlock-compiler-lua/main.lua")
	if err != nil {
		t.Fatal(err)
	}
	stmts := parseChunk(t, string(src))
	reg := standard.Registry()
	uuid := manifest.New("uuid")
	uuid.SetExport(typetable.NewRecord().Field("v7", typ.Func().Returns(typ.String).Build()).Build())
	check := body.Config{
		Registry: reg, TypeValues: typevalue.NewCache(), Globals: []string{"uuid"}, Schedule: transfer.ScheduleWTO,
		Signatures:    signaturelookup.Source{IncludeStdlib: true, Manifests: []*manifest.Manifest{uuid}},
		ModuleExports: importlookup.Source{Manifests: []*manifest.Manifest{uuid}},
	}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: body.Globals(check)})
	audits := 0
	semanticDigests := make(map[summary.Digest]int)
	semanticFields := make(map[string]int)
	normalReturnFields := make(map[string]int)
	stats := &Stats{}
	config := Config{Check: check, Stats: stats}
	config.semanticProgramAudit = func(prepared *body.Static, oracleConfig body.Config, oracle *body.Result) error {
		fn := oracle.Function()
		if fn == nil || fn.Line() != 743 {
			return nil
		}
		audits++
		projected := summaryprojection.FromResult(oracle)
		semanticDigests[summary.NormalizedPayloadDigest(reg, projected)]++
		value := reflect.ValueOf(projected)
		typeOf := value.Type()
		for field := 0; field < value.NumField(); field++ {
			if !value.Field(field).IsZero() {
				semanticFields[typeOf.Field(field).Name]++
			}
		}
		normalReturn := reflect.ValueOf(projected.NormalReturnFacts)
		normalReturnType := normalReturn.Type()
		for field := 0; field < normalReturn.NumField(); field++ {
			if !normalReturn.Field(field).IsZero() {
				normalReturnFields[normalReturnType.Field(field).Name]++
			}
		}
		stats := &body.Stats{}
		concreteConfig := oracleConfig
		concreteConfig.Stats = stats
		comparisons := 0
		concreteConfig.CompareWTO = func(report transfer.WTOComparison) {
			comparisons++
			if report.Fallback || report.StateDifferences != 0 || report.FIFOBelowWTO != 0 || report.WTOBelowFIFO != 0 || report.Incomparable != 0 || len(report.LaneDifferences) != 0 {
				t.Errorf("solve %d concrete point differential: %#v", audits, report)
			}
		}
		concrete, solveErr := body.SolvePrepared(prepared, concreteConfig.SolveConfig())
		if solveErr != nil {
			return solveErr
		}
		if comparisons != 1 || stats.Transfer.DenseCompleted != 1 || stats.Transfer.DenseFallbacks != 0 {
			t.Errorf("solve %d concrete coverage comparisons=%d stats=%#v", audits, comparisons, stats.Transfer)
		}
		comparePreparedResults(t, reg, oracle, concrete, audits)
		return nil
	}
	result, err := RunBoundChunk(stmts, bindings, config)
	if err != nil {
		t.Fatalf("RunBoundChunk: %v", err)
	}
	if audits != 6 {
		t.Fatalf("compiler.validate_graph audited solves=%d, want 6 after dependency-first query scheduling", audits)
	}
	validateKey, ok := result.FunctionKey(mustFunctionSymbolAtLine(t, bindings, 743))
	if !ok {
		t.Fatal("compiler.validate_graph summary key missing")
	}
	validateSummarySolves := 0
	validateSummaryVariants := 0
	for _, attribution := range stats.BodySolveAttribution() {
		if attribution.Function.Ref == validateKey.Ref && attribution.Phase == SolvePhaseSummary {
			validateSummaryVariants++
			validateSummarySolves += attribution.BodySolves
			if attribution.BodySolves != 1 {
				t.Fatalf("compiler.validate_graph summary key %v body solves=%d, want one initial acyclic solve", attribution.Function, attribution.BodySolves)
			}
		}
	}
	// validate_graph has one base equation and one context-specialized equation.
	// Both now run exactly once: the old caller-first order revisited the base
	// equation twice after its two lexical callee summaries grew. Removing the
	// remaining distinct variant requires transformer/context abstraction, not
	// scheduling: merging unequal entry equations here would change semantics.
	if validateSummarySolves != 2 {
		t.Fatalf("compiler.validate_graph summary body solves=%d, want one solve for each of two exact variants", validateSummarySolves)
	}
	if validateSummaryVariants != 2 {
		t.Fatalf("compiler.validate_graph summary variants=%d, want 2", validateSummaryVariants)
	}
	t.Logf("compiler.validate_graph semantic summary classes=%d multiplicities=%v", len(semanticDigests), semanticDigests)
	t.Logf("compiler.validate_graph non-empty Summary fields=%v", semanticFields)
	t.Logf("compiler.validate_graph non-empty NormalReturnFacts fields=%v", normalReturnFields)
}

func mustFunctionSymbolAtLine(t *testing.T, bindings *bind.Result, line int) symbol.ID {
	t.Helper()
	fn := functionAtLine(t, bindings, line)
	id, ok := bindings.FunctionSymbol(fn)
	if !ok {
		t.Fatalf("function symbol at line %d missing", line)
	}
	return id
}

func comparePreparedResults(t *testing.T, reg *axis.Registry, oracle, concrete *body.Result, solve int) {
	t.Helper()
	graph := oracle.Graph()
	if graph.Size() != concrete.Graph().Size() {
		t.Fatalf("solve %d graph size differs", solve)
	}
	for point := cfg.Point(0); int(point) < graph.Size(); point++ {
		want, wantOK := oracle.StateAt(point)
		got, gotOK := concrete.StateAt(point)
		if wantOK != gotOK {
			t.Fatalf("solve %d point %d presence differs", solve, point)
		}
		if wantOK {
			// Path-evidence keys are local to each independently prepared Result.
			// Compare semantic paths, not incidental keyspace ordinals.
			got = got.RekeyPathEvidence(concrete.KeySpace(), oracle.KeySpace())
			for _, lane := range state.DefaultLanes() {
				domain := state.DomainWithLanes(reg, []state.LaneID{lane})
				if !domain.Equal(want, got) {
					t.Fatalf("solve %d point %d lane %s differs", solve, point, lane)
				}
			}
		}
		wantBoundary, wantBoundaryOK := oracle.StateAtBoundary(point)
		gotBoundary, gotBoundaryOK := concrete.StateAtBoundary(point)
		if gotBoundaryOK {
			gotBoundary = gotBoundary.RekeyPathEvidence(concrete.KeySpace(), oracle.KeySpace())
		}
		if wantBoundaryOK != gotBoundaryOK || (wantBoundaryOK && !state.Domain(reg).Equal(wantBoundary, gotBoundary)) {
			t.Fatalf("solve %d boundary observation %d differs", solve, point)
		}
		wantOutcome, wantOutcomeOK := oracle.CallOutcomeAt(point)
		gotOutcome, gotOutcomeOK := concrete.CallOutcomeAt(point)
		if wantOutcomeOK != gotOutcomeOK || (wantOutcomeOK && !reflect.DeepEqual(wantOutcome, gotOutcome)) {
			t.Fatalf("solve %d lowered CallOutcome %d differs", solve, point)
		}
	}
	wantExit, wantExitOK := oracle.ExitState()
	gotExit, gotExitOK := concrete.ExitState()
	if gotExitOK {
		gotExit = gotExit.RekeyPathEvidence(concrete.KeySpace(), oracle.KeySpace())
	}
	if wantExitOK != gotExitOK || (wantExitOK && !state.Domain(reg).Equal(wantExit, gotExit)) {
		t.Fatalf("solve %d exit differs", solve)
	}
	wantSummary := summary.Normalize(reg, summaryprojection.FromResult(oracle))
	gotSummary := summary.Normalize(reg, summaryprojection.FromResult(concrete))
	if !summary.Equal(reg, wantSummary, gotSummary) {
		t.Fatalf("solve %d normalized Summary differs", solve)
	}
	wantDiagnostics, err := json.Marshal(diagnostics.Produce(oracle))
	if err != nil {
		t.Fatal(err)
	}
	gotDiagnostics, err := json.Marshal(diagnostics.Produce(concrete))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(wantDiagnostics, gotDiagnostics) {
		t.Fatalf("solve %d diagnostic bytes differ", solve)
	}
}
