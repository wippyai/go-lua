package program

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	summaryprojection "github.com/wippyai/go-lua/analysis/check/fixpoint/program/internal/projectsummary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/ref"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/solve"
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

func TestRetainedSummaryHandoffRejectsRevisedProviderWithoutPublishingStaleResult(t *testing.T) {
	reg := standard.Registry()
	prepared := retainedHandoffPrepared(t, reg, retainedLargeHandoffSource("return f(total)\n"))
	dep := summary.DefaultSummaryKey(ref.FromSymbol(symbol.ID(99801)))
	ownerKey := summary.DefaultSummaryKey(ref.FromSymbol(symbol.ID(99802)))
	const resolution = uint64(1201)
	values := []product.Value{
		typevalue.FromType(reg, typ.String),
		typevalue.FromType(reg, typ.Number),
		typevalue.FromType(reg, typ.Boolean),
	}
	readers := make([]summary.Snapshot, len(values))
	for i, value := range values {
		readers[i] = retainedSummaryTestSnapshot(reg, value, dep)
	}

	run := newRetainedSummaryApplicationRun(reg, true, "stale-provider-fence")
	defer run.Release()
	owner := run.newOwner(ownerKey)
	summaryCache := NewSummarySolveCache(reg)
	for _, reader := range readers[:2] {
		if _, err := summaryCache.solveRetainedAttributed(
			prepared, "stale-provider-fence", resolution, reader,
			retainedProductionConfig(reg, dep, &body.Stats{}), owner,
			nil, nil, nil, nil, nil, nil, nil,
		); err != nil {
			t.Fatalf("summary solve: %v", err)
		}
	}
	publication := owner.published
	if publication == nil || publication.result == nil || !publication.retained {
		t.Fatal("setup did not publish a retained final summary generation")
	}
	retainedResult := publication.result

	materialCache := newMaterializedSolveCache(reg, run)
	materialSolves := 0
	got, solved, err := solveMaterializedPreparedAttributed(
		materialCache, prepared, ownerKey, 21, resolution, materializedSolveEntryState{}, readers[2],
		retainedProductionConfig(reg, dep, &body.Stats{}), &materialSolves, nil,
	)
	if err != nil {
		t.Fatalf("revised-provider materialization: %v", err)
	}
	if !solved || materialSolves != 1 {
		t.Fatalf("revised provider materialization solved=%v bodies=%d, want one clean solve", solved, materialSolves)
	}
	if owner.published != publication || owner.published.result != retainedResult {
		t.Fatal("rejected stale handoff mutated the retained publication")
	}

	want := retainedCleanSolve(t, prepared, retainedProductionConfig(reg, dep, &body.Stats{}), readers[2])
	compareResultTrees(t, reg, want, got, "revised-provider-clean-fallback")
	projected := summaryprojection.FromResult(got)
	if len(projected.Returns) != 1 || !product.Equal(reg, projected.Returns[0], values[2]) {
		t.Fatal("materialization returned the stale retained provider result")
	}
}

func TestRetainedSummaryHandoffCancellationTransfersAndPublishesNothing(t *testing.T) {
	reg := standard.Registry()
	prepared := retainedHandoffPrepared(t, reg, retainedLargeHandoffSource("return f(total)\n"))
	dep := summary.DefaultSummaryKey(ref.FromSymbol(symbol.ID(99803)))
	ownerKey := summary.DefaultSummaryKey(ref.FromSymbol(symbol.ID(99804)))
	const resolution = uint64(1202)
	readers := []summary.Snapshot{
		retainedSummaryTestSnapshot(reg, typevalue.FromType(reg, typ.String), dep),
		retainedSummaryTestSnapshot(reg, typevalue.FromType(reg, typ.Number), dep),
	}
	run := newRetainedSummaryApplicationRun(reg, true, "canceled-handoff")
	defer run.Release()
	owner := run.newOwner(ownerKey)
	summaryCache := NewSummarySolveCache(reg)
	for _, reader := range readers {
		if _, err := summaryCache.solveRetainedAttributed(
			prepared, "canceled-handoff", resolution, reader,
			retainedProductionConfig(reg, dep, &body.Stats{}), owner,
			nil, nil, nil, nil, nil, nil, nil,
		); err != nil {
			t.Fatalf("summary solve: %v", err)
		}
	}
	publication := owner.published
	retainedResult := publication.result
	session, ok := publication.resource.(*body.RetainedPreparedSession)
	if !ok || !session.Retained() {
		t.Fatal("setup did not retain a live generation")
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	build := retainedProductionConfig(reg, dep, &body.Stats{})
	canceledBuild := func(reader summary.Reader) body.Config {
		config := build(reader)
		config.Context = canceled
		return config
	}
	materialCache := newMaterializedSolveCache(reg, run)
	materialSolves := 0
	result, solved, err := solveMaterializedPreparedAttributed(
		materialCache, prepared, ownerKey, 22, resolution, materializedSolveEntryState{}, readers[1],
		canceledBuild, &materialSolves, nil,
	)
	if !errors.Is(err, context.Canceled) || !errors.Is(err, solve.ErrCanceled) {
		t.Fatalf("canceled handoff error=%v, want context and solve cancellation", err)
	}
	if result != nil || solved || materialSolves != 0 || len(materialCache.entries) != 0 {
		t.Fatalf("canceled handoff result=%v solved=%v bodies=%d cache=%d", result != nil, solved, materialSolves, len(materialCache.entries))
	}
	if owner.published != publication || owner.published.result != retainedResult || !session.Retained() {
		t.Fatal("canceled handoff transferred, replaced, or released the prior publication")
	}
}

func TestRetainedSummaryHandoffFreezesStructuralCallbacksOnce(t *testing.T) {
	reg := standard.Registry()
	prepared := retainedHandoffPrepared(t, reg, retainedLargeHandoffSource("return f(total)\n"))
	dep := summary.DefaultSummaryKey(ref.FromSymbol(symbol.ID(99805)))
	ownerKey := summary.DefaultSummaryKey(ref.FromSymbol(symbol.ID(99806)))
	const resolution = uint64(1203)
	readers := []summary.Snapshot{
		retainedSummaryTestSnapshot(reg, typevalue.FromType(reg, typ.String), dep),
		retainedSummaryTestSnapshot(reg, typevalue.FromType(reg, typ.Number), dep),
	}

	structural := func(reader summary.Reader, initial, widenAt, widenDelay map[cfg.Point]int) body.Config {
		config := retainedProductionConfig(reg, dep, &body.Stats{})(reader)
		config.Initial = func(point cfg.Point) (state.State, bool) {
			initial[point]++
			return state.State{}, false
		}
		config.WidenAt = func(point cfg.Point) bool {
			widenAt[point]++
			return point == prepared.Graph().RPO()[0]
		}
		config.WidenDelay = func(point cfg.Point) int {
			widenDelay[point]++
			return 2
		}
		return config
	}
	stableBuild := func(reader summary.Reader) body.Config {
		return structural(reader, map[cfg.Point]int{}, map[cfg.Point]int{}, map[cfg.Point]int{})
	}
	run := newRetainedSummaryApplicationRun(reg, true, "callback-freeze")
	defer run.Release()
	owner := run.newOwner(ownerKey)
	summaryCache := NewSummarySolveCache(reg)
	for _, reader := range readers {
		if _, err := summaryCache.solveRetainedAttributed(
			prepared, "callback-freeze", resolution, reader, stableBuild, owner,
			nil, nil, nil, nil, nil, nil, nil,
		); err != nil {
			t.Fatalf("summary solve: %v", err)
		}
	}
	if owner.published == nil || !owner.published.retained {
		t.Fatal("setup did not publish a retained generation")
	}

	initial, widenAt, widenDelay := map[cfg.Point]int{}, map[cfg.Point]int{}, map[cfg.Point]int{}
	materialBuild := func(reader summary.Reader) body.Config {
		return structural(reader, initial, widenAt, widenDelay)
	}
	result, solved, err := solveMaterializedPreparedAttributed(
		newMaterializedSolveCache(reg, run), prepared, ownerKey, 23, resolution,
		materializedSolveEntryState{}, readers[1], materialBuild, nil, nil,
	)
	if err != nil || result == nil || solved {
		t.Fatalf("handoff result=%v solved=%v err=%v", result != nil, solved, err)
	}
	assertRetainedHandoffCallbackCallsOnce(t, prepared, "initial", initial)
	assertRetainedHandoffCallbackCallsOnce(t, prepared, "widen-at", widenAt)
	assertRetainedHandoffCallbackCallsOnce(t, prepared, "widen-delay", widenDelay)
}

func TestRetainedSummaryHandoffPreservesAllLanesHeapDiagnosticsAndManifests(t *testing.T) {
	reg := standard.Registry()
	contract := manifest.New("retained-handoff-contract")
	contract.SetExport(typetable.NewRecord().Field("id", typ.Func().Returns(typ.String).Build()).Build())
	staticConfig := body.Config{
		Registry: reg, Globals: []string{"f"}, Schedule: transfer.ScheduleWTO,
		Signatures:    signaturelookup.Source{IncludeStdlib: true, Manifests: []*manifest.Manifest{contract}},
		ModuleExports: importlookup.Source{Manifests: []*manifest.Manifest{contract}},
	}
	prepared := retainedHandoffPreparedConfig(t, staticConfig, retainedLargeHandoffSource(`
local contract = require("retained-handoff-contract")
local id = contract.id()
local box = { stable = 1 }
local key = "dynamic"
box[key] = f(total)
local wrong: string = 1
return box
`))
	dep := summary.DefaultSummaryKey(ref.FromSymbol(symbol.ID(99807)))
	ownerKey := summary.DefaultSummaryKey(ref.FromSymbol(symbol.ID(99808)))
	const resolution = uint64(1204)
	readers := []summary.Snapshot{
		retainedSummaryTestSnapshot(reg, typevalue.FromType(reg, typ.String), dep),
		retainedSummaryTestSnapshot(reg, typevalue.FromType(reg, typ.Number), dep),
	}
	base := retainedProductionConfig(reg, dep, &body.Stats{})
	build := func(reader summary.Reader) body.Config {
		config := base(reader)
		config.Signatures = staticConfig.Signatures
		config.ModuleExports = staticConfig.ModuleExports
		return config
	}

	run := newRetainedSummaryApplicationRun(reg, true, "complete-parity")
	defer run.Release()
	owner := run.newOwner(ownerKey)
	summaryCache := NewSummarySolveCache(reg)
	for _, reader := range readers {
		if _, err := summaryCache.solveRetainedAttributed(
			prepared, "complete-parity", resolution, reader, build, owner,
			nil, nil, nil, nil, nil, nil, nil,
		); err != nil {
			t.Fatalf("summary solve: %v", err)
		}
	}
	got, solved, err := solveMaterializedPreparedAttributed(
		newMaterializedSolveCache(reg, run), prepared, ownerKey, 24, resolution,
		materializedSolveEntryState{}, readers[1], build, nil, nil,
	)
	if err != nil || got == nil || solved {
		t.Fatalf("handoff result=%v solved=%v err=%v", got != nil, solved, err)
	}
	want := retainedCleanSolve(t, prepared, build, readers[1])
	if len(want.SignatureManifests()) == 0 {
		t.Fatal("fixture did not exercise manifest preservation")
	}
	if strictDiagnosticCount(want) == 0 {
		t.Fatal("fixture did not exercise diagnostic byte preservation")
	}
	wantSummary := summaryprojection.FromResult(want)
	if len(wantSummary.HeapTableObjects) == 0 && len(wantSummary.FreshHeapAllocations) == 0 {
		t.Fatal("fixture did not exercise heap identity")
	}
	if got := len(state.DefaultLanes()); got != 17 {
		t.Fatalf("default lane census=%d, want 17", got)
	}
	compareResultTrees(t, reg, want, got, "complete-retained-handoff")
}

func retainedHandoffPrepared(t *testing.T, reg *axis.Registry, source string) *body.Static {
	t.Helper()
	return retainedHandoffPreparedConfig(t, body.Config{Registry: reg, Globals: []string{"f"}, Schedule: transfer.ScheduleWTO}, source)
}

func retainedHandoffPreparedConfig(t *testing.T, config body.Config, source string) *body.Static {
	t.Helper()
	stmts := parseChunk(t, source)
	bindings := bind.BindChunk(stmts, bind.Options{Globals: body.Globals(config)})
	prepared, err := body.PrepareBoundChunk(stmts, bindings, config)
	if err != nil {
		t.Fatalf("PrepareBoundChunk: %v", err)
	}
	return prepared
}

func retainedCleanSolve(t *testing.T, prepared *body.Static, build func(summary.Reader) body.Config, reader summary.Reader) *body.Result {
	t.Helper()
	tracked := &trackingSummaryReader{reg: build(reader).Registry, base: reader}
	config := build(tracked)
	config.SummaryInputDigests = func() []uint64 { return trackedSummaryReadDigests(config.Registry, tracked.deps) }
	result, err := body.SolvePrepared(prepared, config.SolveConfig())
	if err != nil {
		t.Fatalf("clean SolvePrepared: %v", err)
	}
	return result
}

func assertRetainedHandoffCallbackCallsOnce(t *testing.T, prepared *body.Static, name string, calls map[cfg.Point]int) {
	t.Helper()
	for _, point := range prepared.Graph().RPO() {
		if calls[point] != 1 {
			t.Fatalf("%s callback calls at point %d=%d, want 1", name, point, calls[point])
		}
	}
	if len(calls) != len(prepared.Graph().RPO()) {
		t.Fatalf("%s callback observed %d points, want %d", name, len(calls), len(prepared.Graph().RPO()))
	}
}

func retainedLargeHandoffSource(tail string) string {
	return "local total = 0\n" + strings.Repeat("total = total + 1\n", 300) + tail
}
