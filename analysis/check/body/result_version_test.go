package body

import (
	"context"
	"errors"
	"testing"
	"time"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/solve"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestCanceledObservationSealReturnsPromptly(t *testing.T) {
	result, err := CheckChunk(parseChunk(t, "local value = 1"), Config{Registry: standard.Registry()})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	started := time.Now()
	err = result.sealObservationsContext(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("sealObservationsContext error = %v, want context cancellation", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("canceled observation seal took %s, want prompt return", elapsed)
	}
}

func TestResultVersionCanonicalizesManifestOperationalEffectsAndTypestate(t *testing.T) {
	left := resultVersionForManifest(t, resultVersionManifest(false))
	right := resultVersionForManifest(t, resultVersionManifest(true))
	if left != right {
		t.Fatalf("semantically equivalent manifests produced result versions %d and %d", left, right)
	}
}

func TestResultVersionIncludesScheduleSemantics(t *testing.T) {
	stmts := parseChunk(t, "local value = 1")
	fifo, err := CheckChunk(stmts, Config{Registry: standard.Registry()})
	if err != nil {
		t.Fatalf("FIFO CheckChunk: %v", err)
	}
	wto, err := CheckChunk(stmts, Config{Registry: standard.Registry(), Schedule: transfer.ScheduleWTO})
	if err != nil {
		t.Fatalf("WTO CheckChunk: %v", err)
	}
	if fifo.ResultVersion() == wto.ResultVersion() {
		t.Fatalf("FIFO and WTO result versions both %d", fifo.ResultVersion())
	}
}

func TestCachedResultVersionPrefixStillObservesCancellation(t *testing.T) {
	prepared, err := PrepareChunk(parseChunk(t, "local value = 1"), Config{Registry: standard.Registry()})
	if err != nil {
		t.Fatalf("PrepareChunk: %v", err)
	}
	if _, err := InputDigestContext(prepared, SolveConfig{Context: context.Background()}); err != nil {
		t.Fatalf("prime InputDigestContext: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := InputDigestContext(prepared, SolveConfig{Context: ctx}); !errors.Is(err, context.Canceled) || !errors.Is(err, solve.ErrCanceled) {
		t.Fatalf("cached InputDigestContext error = %v, want solve and context cancellation", err)
	}
}

func TestStableProductHashIncludesSemanticAxes(t *testing.T) {
	reg := standard.Registry()
	base := typevalue.NewCache().FromTypeWithWitness(reg, typ.NewArray(typ.String))
	left := identityvalue.WithExact(reg, base, identity.LuaFunction(1))
	right := identityvalue.WithExact(reg, base, identity.LuaFunction(2))
	if product.Equal(reg, left, right) {
		t.Fatal("identity variants unexpectedly compare equal")
	}

	w := newBodyDigestWriter(&Static{registry: reg})
	if leftHash, rightHash := w.stableProductHash(left), w.stableProductHash(right); leftHash == rightHash {
		t.Fatalf("identity variants share stable product hash %d", leftHash)
	}
}

func resultVersionForManifest(t *testing.T, m *manifest.Manifest) uint64 {
	t.Helper()
	result, err := CheckChunk(parseChunk(t, "local value = 1"), Config{
		Registry: standard.Registry(),
		Signatures: signaturelookup.Source{
			Manifests: []*manifest.Manifest{m},
		},
	})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	return result.ResultVersion()
}

func resultVersionManifest(reverse bool) *manifest.Manifest {
	effects := []signature.PathPresenceRefinement{
		{Path: pathdom.NewPlaceholder(0).Field("ready"), Presence: presence.Present()},
		{Path: pathdom.NewPlaceholder(0).Field("failed"), Presence: presence.Absent()},
	}
	states := []typestate.State{"active", "finished"}
	transitions := []typestate.TransitionDecl{
		{From: "active", To: "finished"},
		{From: "finished", To: "active"},
	}
	if reverse {
		effects[0], effects[1] = effects[1], effects[0]
		states[0], states[1] = states[1], states[0]
		transitions[0], transitions[1] = transitions[1], transitions[0]
	}
	m := manifest.New("result-version-canonical")
	m.TypestateProtocols["transaction"] = typestate.Definition{
		Protocol:    "transaction",
		States:      states,
		FinalStates: []typestate.State{"finished"},
		Transitions: transitions,
	}
	m.DefineFunctionSignature("f", signature.Function{
		Type: typ.Func().Param("value", typ.Any).Build(),
		OperationalEffects: &signature.OperationalEffects{
			NormalReturnPresenceRefinements: effects,
		},
	})
	return m
}
