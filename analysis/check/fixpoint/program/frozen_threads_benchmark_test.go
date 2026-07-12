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
	Mode              string            `json:"mode"`
	Wall              time.Duration     `json:"wall"`
	AllocatedBytes    uint64            `json:"allocated_bytes"`
	HeapDeltaBytes    int64             `json:"heap_delta_bytes"`
	BodySolves        int               `json:"body_solves"`
	PointTransfers    int               `json:"point_transfers"`
	PrepassSolves     int               `json:"prepass_solves"`
	SummarySolves     int               `json:"summary_solves"`
	SummaryTransfers  int               `json:"summary_transfers"`
	MaterializeSolves int               `json:"materialize_solves"`
	Composition       []CompositionCost `json:"composition"`
	Diagnostics       int               `json:"diagnostics"`
	DiagnosticsDigest string            `json:"diagnostics_digest"`
	SummaryDigest     string            `json:"summary_digest"`
	RelationProducers int               `json:"relation_producers"`
	RelationOwners    int               `json:"relation_owners"`
	RelationCalls     int               `json:"relation_calls"`
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
	legacyA := runFrozenThreads(t, stmts, bindings, false)
	legacyB := runFrozenThreads(t, stmts, bindings, false)
	activeA := runFrozenThreads(t, stmts, bindings, true)
	activeB := runFrozenThreads(t, stmts, bindings, true)
	for _, run := range []frozenThreadsRun{legacyA, legacyB, activeA, activeB} {
		logFrozenReport(t, run.report)
	}
	reg := standard.Registry()
	legacyRepeat := firstFrozenProductDivergence(reg, legacyA.result, legacyB.result)
	activeRepeat := firstFrozenProductDivergence(reg, activeA.result, activeB.result)
	cross := firstFrozenProductDivergence(reg, legacyA.result, activeA.result)
	t.Logf("FROZEN_THREADS_ORACLE legacy_repeat=%q active_repeat=%q legacy_active=%q", legacyRepeat, activeRepeat, cross)
	if legacyA.report.DiagnosticsDigest != legacyB.report.DiagnosticsDigest || legacyA.report.DiagnosticsDigest != activeA.report.DiagnosticsDigest || activeA.report.DiagnosticsDigest != activeB.report.DiagnosticsDigest {
		t.Fatal("diagnostic oracle is nondeterministic")
	}
	if legacyRepeat != "" || activeRepeat != "" || cross != "" {
		t.Fatalf("frozen oracle diverged: legacy repeat=%q active repeat=%q legacy/active=%q", legacyRepeat, activeRepeat, cross)
	}
}

func BenchmarkFrozenThreadsContract(b *testing.B) {
	stmts, bindings := loadFrozenThreads(b)
	for _, mode := range []struct {
		name   string
		active bool
	}{{"legacy", false}, {"active", true}} {
		b.Run(mode.name, func(b *testing.B) {
			b.ReportAllocs()
			var last frozenThreadsRun
			for range b.N {
				last = runFrozenThreads(b, stmts, bindings, mode.active)
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

func runFrozenThreads(t testing.TB, stmts []ast.Stmt, bindings *bind.Result, active bool) frozenThreadsRun {
	t.Helper()
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	stats := Stats{}
	start := time.Now()
	result, err := RunBoundChunk(stmts, bindings, Config{Check: frozenThreadsCheck(standard.Registry()), Stats: &stats, enableRelationActivation: active})
	wall := time.Since(start)
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatal(err)
	}
	diagnosticBytes, count := frozenDiagnosticBytes(result.RootResult())
	report := frozenThreadsReport{Mode: map[bool]string{false: "legacy", true: "active"}[active], Wall: wall, AllocatedBytes: after.TotalAlloc - before.TotalAlloc, HeapDeltaBytes: int64(after.HeapAlloc) - int64(before.HeapAlloc), BodySolves: stats.Body.BodySolves, PointTransfers: stats.Body.Transfer.Solver.TransferCalls, PrepassSolves: stats.PrepassBodySolves, SummarySolves: stats.SummaryBodySolves, SummaryTransfers: stats.SummaryPointTransfers, MaterializeSolves: stats.MaterializeBodySolves, Composition: stats.CompositionCostCensus(), Diagnostics: count, DiagnosticsDigest: sha256Hex(diagnosticBytes), SummaryDigest: frozenSummaryDigest(standard.Registry(), result), RelationProducers: stats.RelationProducersEligible, RelationOwners: stats.RelationOwnersActive, RelationCalls: stats.RelationCallsHandled}
	return frozenThreadsRun{result, report}
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
