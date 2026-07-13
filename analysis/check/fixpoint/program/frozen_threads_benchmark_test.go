package program

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

const frozenThreadsPath = "/tmp/frozen-threads-contract.lua"
const frozenThreadsEnv = "GO_LUA_FROZEN_THREADS"

type frozenThreadsReport struct {
	Mode                   string            `json:"mode"`
	Wall                   time.Duration     `json:"wall"`
	AllocatedBytes         uint64            `json:"allocated_bytes"`
	HeapDeltaBytes         int64             `json:"heap_delta_bytes"`
	BodySolves             int               `json:"body_solves"`
	PointTransfers         int               `json:"point_transfers"`
	PrepassSolves          int               `json:"prepass_solves"`
	SummarySolves          int               `json:"summary_solves"`
	SummaryTransfers       int               `json:"summary_transfers"`
	MaterializeSolves      int               `json:"materialize_solves"`
	Composition            []CompositionCost `json:"composition"`
	Diagnostics            int               `json:"diagnostics"`
	DiagnosticsDigest      string            `json:"diagnostics_digest"`
	SummaryDigest          string            `json:"summary_digest"`
	RelationProducers      int               `json:"relation_producers"`
	RelationOwners         int               `json:"relation_owners"`
	RelationCalls          int               `json:"relation_calls"`
	PlannerScanned         int               `json:"planner_scanned"`
	PlannerPrefiltered     int               `json:"planner_prefiltered"`
	PlannerCompiled        int               `json:"planner_compiled"`
	PlannerActivated       int               `json:"planner_activated"`
	ContextsSpecialized    int               `json:"contexts_specialized"`
	EquationsOmitted       int               `json:"equations_omitted"`
	MaterializationsReused int               `json:"materializations_reused"`
	TargetBodySolves       int               `json:"target_body_solves"`
	TargetTransfers        int               `json:"target_transfers"`
}

type frozenThreadsRun struct {
	result Result
	report frozenThreadsReport
}

// TestFrozenThreadsContractDifferential is an explicit, optional real-file
// oracle. It reads the frozen source in place and never copies or modifies it.
// Set GO_LUA_FROZEN_THREADS=1 to opt in.
func TestFrozenThreadsContractDifferential(t *testing.T) {
	stmts, bindings := loadFrozenThreads(t)
	legacyA := runFrozenThreads(t, stmts, bindings, "legacy")
	legacyB := runFrozenThreads(t, stmts, bindings, "legacy")
	activeA := runFrozenThreads(t, stmts, bindings, "active")
	activeB := runFrozenThreads(t, stmts, bindings, "active")
	strictA := runFrozenThreads(t, stmts, bindings, "strict")
	strictB := runFrozenThreads(t, stmts, bindings, "strict")
	for _, run := range []frozenThreadsRun{legacyA, legacyB, activeA, activeB, strictA, strictB} {
		logFrozenReport(t, run.report)
	}
	reg := standard.Registry()
	legacyRepeat := firstFrozenProductDivergence(reg, legacyA.result, legacyB.result)
	activeRepeat := firstFrozenProductDivergence(reg, activeA.result, activeB.result)
	cross := firstFrozenProductDivergence(reg, legacyA.result, activeA.result)
	strictRepeat := firstFrozenProductDivergence(reg, strictA.result, strictB.result)
	strictCross := firstFrozenProductDivergence(reg, legacyA.result, strictA.result)
	t.Logf("FROZEN_THREADS_ORACLE legacy_repeat=%q active_repeat=%q strict_repeat=%q legacy_active=%q legacy_strict=%q", legacyRepeat, activeRepeat, strictRepeat, cross, strictCross)
	if legacyA.report.DiagnosticsDigest != legacyB.report.DiagnosticsDigest || legacyA.report.DiagnosticsDigest != activeA.report.DiagnosticsDigest ||
		activeA.report.DiagnosticsDigest != activeB.report.DiagnosticsDigest || legacyA.report.DiagnosticsDigest != strictA.report.DiagnosticsDigest ||
		strictA.report.DiagnosticsDigest != strictB.report.DiagnosticsDigest {
		t.Fatal("diagnostic oracle is nondeterministic")
	}
	if legacyRepeat != "" || activeRepeat != "" || strictRepeat != "" || cross != "" || strictCross != "" {
		t.Fatalf("frozen oracle diverged: legacy repeat=%q active repeat=%q strict repeat=%q legacy/active=%q legacy/strict=%q", legacyRepeat, activeRepeat, strictRepeat, cross, strictCross)
	}
	for _, run := range []frozenThreadsRun{legacyA, legacyB} {
		if run.report.TargetBodySolves != 10 || run.report.TargetTransfers != 70 {
			t.Fatalf("legacy is_str work = %d/%d, want 10/70", run.report.TargetBodySolves, run.report.TargetTransfers)
		}
	}
	for _, run := range []frozenThreadsRun{strictA, strictB} {
		if run.report.TargetBodySolves != 6 || run.report.TargetTransfers != 42 || run.report.ContextsSpecialized != 4 || run.report.EquationsOmitted != 4 || run.report.MaterializationsReused != 4 {
			t.Fatalf("strict is_str work=%d/%d contexts/omitted/reused=%d/%d/%d, want 6/42 and 4/4/4", run.report.TargetBodySolves, run.report.TargetTransfers, run.report.ContextsSpecialized, run.report.EquationsOmitted, run.report.MaterializationsReused)
		}
	}
}

func BenchmarkFrozenThreadsContract(b *testing.B) {
	stmts, bindings := loadFrozenThreads(b)
	for _, mode := range []struct {
		name string
	}{{"legacy"}, {"active"}, {"strict"}} {
		b.Run(mode.name, func(b *testing.B) {
			b.ReportAllocs()
			var last frozenThreadsRun
			for range b.N {
				last = runFrozenThreads(b, stmts, bindings, mode.name)
			}
			b.ReportMetric(float64(last.report.BodySolves), "body-solves/op")
			b.ReportMetric(float64(last.report.PointTransfers), "transfers/op")
		})
	}
}

func loadFrozenThreads(t testing.TB) ([]ast.Stmt, *bind.Result) {
	t.Helper()
	if os.Getenv(frozenThreadsEnv) != "1" {
		t.Skipf("set %s=1 to run %s", frozenThreadsEnv, frozenThreadsPath)
	}
	source, err := os.ReadFile(frozenThreadsPath)
	if os.IsNotExist(err) {
		t.Skipf("frozen contract absent: %s", frozenThreadsPath)
	}
	if err != nil {
		t.Fatal(err)
	}
	stmts, err := parse.ParseString(string(source), frozenThreadsPath)
	if err != nil {
		t.Fatal(err)
	}
	check := frozenThreadsCheck(standard.Registry())
	return stmts, bind.BindChunk(stmts, bind.Options{Globals: body.Globals(check)})
}

func frozenThreadsCheck(reg *axis.Registry) body.Config {
	return body.Config{Registry: reg, TypeValues: typevalue.NewCache(), Schedule: transfer.ScheduleWTO, Signatures: signaturelookup.Source{IncludeStdlib: true}}
}

func runFrozenThreads(t testing.TB, stmts []ast.Stmt, bindings *bind.Result, mode string) frozenThreadsRun {
	t.Helper()
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	stats := Stats{}
	start := time.Now()
	config := Config{Check: frozenThreadsCheck(standard.Registry()), Stats: &stats}
	switch mode {
	case "legacy":
		config.forceLegacyRelations = true
	case "active":
		config.enableRelationActivation = true
	case "strict":
		config.enableStrictRelationPhaseCollapse = true
	default:
		t.Fatalf("unknown frozen threads mode %q", mode)
	}
	result, err := RunBoundChunk(stmts, bindings, config)
	wall := time.Since(start)
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatal(err)
	}
	diagnosticBytes, count := frozenDiagnosticBytes(result.RootResult())
	targetSolves, targetTransfers := frozenBodyWork(stats.BodySolveAttribution(), 16010901263544322178)
	report := frozenThreadsReport{Mode: mode, Wall: wall, AllocatedBytes: after.TotalAlloc - before.TotalAlloc, HeapDeltaBytes: int64(after.HeapAlloc) - int64(before.HeapAlloc), BodySolves: stats.Body.BodySolves, PointTransfers: stats.Body.Transfer.Solver.TransferCalls, PrepassSolves: stats.PrepassBodySolves, SummarySolves: stats.SummaryBodySolves, SummaryTransfers: stats.SummaryPointTransfers, MaterializeSolves: stats.MaterializeBodySolves, Composition: stats.CompositionCostCensus(), Diagnostics: count, DiagnosticsDigest: sha256Hex(diagnosticBytes), SummaryDigest: frozenSummaryDigest(standard.Registry(), result), RelationProducers: stats.RelationProducersEligible, RelationOwners: stats.RelationOwnersActive, RelationCalls: stats.RelationCallsHandled, PlannerScanned: stats.RelationPlannerOwnersScanned, PlannerPrefiltered: stats.RelationPlannerOwnersPrefiltered, PlannerCompiled: stats.RelationPlannerOwnersCompiled, PlannerActivated: stats.RelationPlannerOwnersActivated, ContextsSpecialized: stats.RelationContextsSpecialized, EquationsOmitted: stats.RelationSummaryEquationsOmitted, MaterializationsReused: stats.RelationMaterializationsReused, TargetBodySolves: targetSolves, TargetTransfers: targetTransfers}
	return frozenThreadsRun{result, report}
}

func frozenBodyWork(entries []BodySolveAttribution, bodyID uint64) (solves, transfers int) {
	for _, entry := range entries {
		if entry.BodyID == bodyID {
			solves += entry.BodySolves
			transfers += entry.PointTransfers
		}
	}
	return solves, transfers
}

func logFrozenReport(t testing.TB, report frozenThreadsReport) {
	t.Helper()
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("FROZEN_THREADS %s", encoded)
}

func frozenDiagnosticBytes(root *body.Result) ([]byte, int) {
	var all []any
	var visit func(*body.Result)
	visit = func(result *body.Result) {
		if result == nil {
			return
		}
		for _, diagnostic := range diagnostics.Produce(result) {
			all = append(all, diagnostic)
		}
		for _, child := range result.FunctionResults() {
			visit(child)
		}
	}
	visit(root)
	encoded, _ := json.Marshal(all)
	return encoded, len(all)
}
func sha256Hex(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}
func frozenSummaryDigest(reg *axis.Registry, result Result) string {
	hash := sha256.New()
	var encoded [8]byte
	for _, entry := range result.Snapshot().Entries() {
		fmt.Fprintf(hash, "%#v\x00", entry.Key)
		binary.BigEndian.PutUint64(encoded[:], uint64(summary.NormalizedPayloadDigest(reg, entry.Summary)))
		hash.Write(encoded[:])
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func firstFrozenProductDivergence(reg *axis.Registry, legacy, active Result) string {
	want, got := legacy.Snapshot().Entries(), active.Snapshot().Entries()
	if len(want) != len(got) {
		return fmt.Sprintf("summary/count legacy=%d active=%d", len(want), len(got))
	}
	for i := range want {
		if want[i].Key != got[i].Key {
			return fmt.Sprintf("summary/%d/key", i)
		}
		if !summary.Equal(reg, want[i].Summary, got[i].Summary) {
			lane := firstFrozenSummaryLane(reg, want[i].Summary, got[i].Summary)
			detail := ""
			if lane == "HeapTableObjects" {
				detail = fmt.Sprintf(" heap-ids=%v/%v", frozenHeapIDs(want[i].Summary), frozenHeapIDs(got[i].Summary))
			}
			return fmt.Sprintf("summary/%d/%#v/lane/%s digest=%d/%d%s", i, want[i].Key, lane, summary.NormalizedPayloadDigest(reg, want[i].Summary), summary.NormalizedPayloadDigest(reg, got[i].Summary), detail)
		}
	}
	return firstFrozenBodyDivergence(reg, legacy.RootResult(), active.RootResult(), "root")
}
func frozenHeapIDs(value summary.Summary) []string {
	out := make([]string, 0, len(value.HeapTableObjects))
	for id := range value.HeapTableObjects {
		out = append(out, id.String())
	}
	slices.Sort(out)
	return out
}

func firstFrozenSummaryLane(reg *axis.Registry, left, right summary.Summary) string {
	left = summary.Normalize(reg, left)
	right = summary.Normalize(reg, right)
	leftValue, rightValue := reflect.ValueOf(left), reflect.ValueOf(right)
	typ := leftValue.Type()
	for index := 0; index < typ.NumField(); index++ {
		field := typ.Field(index)
		if field.Name == "HeapKeySpace" {
			continue
		}
		leftOnly, rightOnly := summary.Summary{}, summary.Summary{}
		reflect.ValueOf(&leftOnly).Elem().Field(index).Set(leftValue.Field(index))
		reflect.ValueOf(&rightOnly).Elem().Field(index).Set(rightValue.Field(index))
		if !summary.Equal(reg, leftOnly, rightOnly) {
			if field.Name == "NormalReturnFacts" {
				if nested := firstFrozenNormalReturnLane(reg, leftOnly, rightOnly); nested != "" {
					return field.Name + "." + nested
				}
			}
			return field.Name
		}
	}
	return "unknown"
}
func firstFrozenNormalReturnLane(reg *axis.Registry, left, right summary.Summary) string {
	leftValue, rightValue := reflect.ValueOf(left.NormalReturnFacts), reflect.ValueOf(right.NormalReturnFacts)
	typ := leftValue.Type()
	for index := 0; index < typ.NumField(); index++ {
		leftOnly, rightOnly := summary.Summary{}, summary.Summary{}
		reflect.ValueOf(&leftOnly.NormalReturnFacts).Elem().Field(index).Set(leftValue.Field(index))
		reflect.ValueOf(&rightOnly.NormalReturnFacts).Elem().Field(index).Set(rightValue.Field(index))
		if !summary.Equal(reg, leftOnly, rightOnly) {
			return typ.Field(index).Name
		}
	}
	return ""
}
func firstFrozenBodyDivergence(reg *axis.Registry, legacy, active *body.Result, path string) string {
	if (legacy == nil) != (active == nil) {
		return path + "/presence"
	}
	if legacy == nil {
		return ""
	}
	if legacy.Graph().Size() != active.Graph().Size() {
		return path + "/graph-size"
	}
	for point := cfg.Point(0); int(point) < legacy.Graph().Size(); point++ {
		left, lok := legacy.StateAt(point)
		right, rok := active.StateAt(point)
		if lok != rok {
			return fmt.Sprintf("%s/point/%d/presence", path, point)
		}
		if lok {
			right = right.RekeyPathEvidence(active.KeySpace(), legacy.KeySpace())
			for _, lane := range state.DefaultLanes() {
				domain := state.DomainWithLanes(reg, []state.LaneID{lane})
				if !domain.Equal(left, right) {
					return fmt.Sprintf("%s/point/%d/lane/%s", path, point, lane)
				}
			}
		}
		leftBoundary, lbo := legacy.StateAtBoundary(point)
		rightBoundary, rbo := active.StateAtBoundary(point)
		if rbo {
			rightBoundary = rightBoundary.RekeyPathEvidence(active.KeySpace(), legacy.KeySpace())
		}
		if lbo != rbo || (lbo && !state.Domain(reg).Equal(leftBoundary, rightBoundary)) {
			return fmt.Sprintf("%s/point/%d/boundary", path, point)
		}
	}
	leftExit, leo := legacy.ExitState()
	rightExit, reo := active.ExitState()
	if reo {
		rightExit = rightExit.RekeyPathEvidence(active.KeySpace(), legacy.KeySpace())
	}
	if leo != reo || (leo && !state.Domain(reg).Equal(leftExit, rightExit)) {
		return path + "/exit"
	}
	leftDiagnostics, _ := json.Marshal(diagnostics.Produce(legacy))
	rightDiagnostics, _ := json.Marshal(diagnostics.Produce(active))
	if !slices.Equal(leftDiagnostics, rightDiagnostics) {
		return path + "/diagnostics"
	}
	leftChildren, rightChildren := legacy.FunctionResults(), active.FunctionResults()
	if len(leftChildren) != len(rightChildren) {
		return path + "/children/count"
	}
	for i := range leftChildren {
		if divergence := firstFrozenBodyDivergence(reg, leftChildren[i], rightChildren[i], fmt.Sprintf("%s/child/%d", path, i)); divergence != "" {
			return divergence
		}
	}
	return ""
}
