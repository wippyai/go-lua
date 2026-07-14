package program

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/ref"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

type handoffFinalizeResource struct{ releases int }

func (r *handoffFinalizeResource) Release() { r.releases++ }

func TestMaterializationHandoffFinalizesTakenResourceExactlyOnce(t *testing.T) {
	reg := standard.Registry()
	prepared := retainedHandoffPrepared(t, reg, "local value = 1\nreturn value")
	reader := summary.NewSnapshot(reg)
	build := func(summary.Reader) body.Config {
		return body.Config{Registry: reg, Schedule: transfer.ScheduleWTO}
	}
	config := build(reader)
	result, err := body.SolvePrepared(prepared, config.SolveConfig())
	if err != nil {
		t.Fatalf("SolvePrepared: %v", err)
	}
	bodyDigest, err := prepared.IdentityDigestContext(config.Context)
	if err != nil {
		t.Fatalf("IdentityDigestContext: %v", err)
	}
	inputDigest, err := body.InputDigestContext(prepared, config.SolveConfig())
	if err != nil {
		t.Fatalf("InputDigestContext: %v", err)
	}

	const profile = "handoff-finalize"
	const resolution = uint64(871)
	ownerKey := dependencyTestKey(91871)
	run := newRetainedSummaryApplicationRun(reg, true, profile)
	owner := run.newOwner(ownerKey)
	resource := &handoffFinalizeResource{}
	attempt := owner.begin(retainedSummaryApplicationKey{
		body: bodyDigest, input: inputDigest, profile: profile, resolution: resolution,
	}, reader)
	if !attempt.publishResult(nil, pointSummaryDependencies{}, resource, summary.Summary{}, result) {
		t.Fatal("result publication rejected")
	}

	got, solved, err := solveMaterializedPreparedAttributed(
		newMaterializedSolveCache(reg, run), prepared, ownerKey, 19, resolution, materializedSolveEntryState{},
		reader, build, nil, nil,
	)
	if err != nil {
		t.Fatalf("materialization error = %v", err)
	}
	if solved {
		t.Fatal("taken handoff reported a clean body solve")
	}
	if got == nil {
		t.Fatal("successful handoff returned nil result")
	}
	if got.Graph() == nil {
		t.Fatal("successful handoff released the installed result")
	}
	if resource.releases != 1 || owner.published.resource != nil || owner.published.result != nil {
		t.Fatalf("post-handoff releases/resource/result = %d/%v/%v, want 1/nil/nil", resource.releases, owner.published.resource, owner.published.result)
	}
	run.Release()
	if resource.releases != 1 {
		t.Fatalf("run Release released finalized resource %d times, want exactly 1", resource.releases)
	}
	if got != nil {
		got.ReleaseTransient()
	}
}

func TestMaterializationHandoffRebindPanicReleasesRetainedSession(t *testing.T) {
	reg := standard.Registry()
	prepared := retainedHandoffPrepared(t, reg, retainedLargeHandoffSource("return f(total)"))
	dep := summary.DefaultSummaryKey(ref.FromSymbol(symbol.ID(91880)))
	ownerKey := summary.DefaultSummaryKey(ref.FromSymbol(symbol.ID(91881)))
	readers := []summary.Snapshot{
		retainedSummaryTestSnapshot(reg, typevalue.FromType(reg, typ.String), dep),
		retainedSummaryTestSnapshot(reg, typevalue.FromType(reg, typ.Number), dep),
	}
	const profile = "handoff-session-finalize"
	const resolution = uint64(881)
	run := newRetainedSummaryApplicationRun(reg, true, profile)
	owner := run.newOwner(ownerKey)
	cache := NewSummarySolveCache(reg)
	build := retainedProductionConfig(reg, dep, &body.Stats{})
	for _, reader := range readers {
		if _, err := cache.solveRetainedAttributed(
			prepared, profile, resolution, reader, build, owner,
			nil, nil, nil, nil, nil, nil, nil,
		); err != nil {
			t.Fatalf("summary solve: %v", err)
		}
	}
	publication := owner.published
	session, ok := publication.resource.(*body.RetainedPreparedSession)
	if !ok || !session.Retained() || publication.result == nil {
		t.Fatal("fixture did not publish a retained result/session")
	}
	transferred := publication.result

	panicToken := &struct{}{}
	panicBuild := func(reader summary.Reader) body.Config {
		config := build(reader)
		config.SignatureArgumentTypeFactory = func(body.CallOutcomeContext) body.SignatureArgumentTypeFunc {
			panic(panicToken)
		}
		return config
	}
	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_, _, _ = solveMaterializedPreparedAttributed(
			newMaterializedSolveCache(reg, run), prepared, ownerKey, 29, resolution,
			materializedSolveEntryState{}, readers[1], panicBuild, nil, nil,
		)
	}()
	if recovered != panicToken {
		t.Fatalf("rebind panic = %v, want injected token", recovered)
	}
	if session.Retained() || publication.resource != nil || publication.result != nil {
		t.Fatalf("failed handoff retained session/resource/result = %v/%v/%v, want false/nil/nil", session.Retained(), publication.resource, publication.result)
	}
	if transferred.Graph() != nil {
		t.Fatal("rebind panic left the taken Result transient flow live")
	}
	run.Release()
	if session.Retained() {
		t.Fatal("run Release revived or retained the finalized session")
	}
}
