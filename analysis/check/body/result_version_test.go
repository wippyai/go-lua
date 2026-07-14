package body

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/solve"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
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

func TestInputDigestConcurrentSameStaticIsReadOnly(t *testing.T) {
	reg := standard.Registry()
	prepared, err := PrepareChunk(parseChunk(t, "return 1"), Config{Registry: reg})
	if err != nil {
		t.Fatal(err)
	}
	value := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	entry := state.State{}
	for i := 1; i <= 64; i++ {
		entry = entry.WritePathKey(reg, prepared.KeySpace(), pathdom.PathKey("sym1@1.value["+fmt.Sprint(i)+"]"), value)
	}
	config := SolveConfig{EntryState: entry}
	want, err := InputDigestContext(prepared, config)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 50 {
				got, digestErr := InputDigestContext(prepared, config)
				if digestErr != nil {
					errs <- digestErr
					return
				}
				if got != want {
					errs <- fmt.Errorf("concurrent digest = %d, want %d", got, want)
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
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

func TestInputDigestIncludesCompleteHeapTableObjectState(t *testing.T) {
	reg := standard.Registry()
	prepared, err := PrepareChunk(parseChunk(t, "return 1"), Config{Registry: reg})
	if err != nil {
		t.Fatalf("PrepareChunk: %v", err)
	}
	left, right := resultVersionHeapStates(t, reg, prepared.KeySpace())
	domain := state.Domain(reg)
	if domain.Equal(left, right) {
		t.Fatal("heap-table-object variants unexpectedly compare equal")
	}

	t.Run("entry", func(t *testing.T) {
		leftDigest := InputDigest(prepared, SolveConfig{EntryState: left})
		rightDigest := InputDigest(prepared, SolveConfig{EntryState: right})
		if leftDigest == rightDigest {
			t.Fatalf("distinct entry heap states share input digest %d", leftDigest)
		}
	})

	t.Run("initial", func(t *testing.T) {
		initial := func(st state.State) transfer.InitialState {
			return func(point cfg.Point) (state.State, bool) {
				return st, point == prepared.cfg.Graph.Entry()
			}
		}
		leftDigest := InputDigest(prepared, SolveConfig{Initial: initial(left)})
		rightDigest := InputDigest(prepared, SolveConfig{Initial: initial(right)})
		if leftDigest == rightDigest {
			t.Fatalf("distinct initial heap states share input digest %d", leftDigest)
		}
	})
}

func TestRetainedCompatibilityRejectsChangedHeapTableObjectState(t *testing.T) {
	reg := standard.Registry()
	prepared, err := PrepareChunk(parseChunk(t, "return 1"), Config{Registry: reg, Schedule: transfer.ScheduleWTO})
	if err != nil {
		t.Fatalf("PrepareChunk: %v", err)
	}
	left, right := resultVersionHeapStates(t, reg, prepared.KeySpace())
	_, retained, err := SolvePreparedRetained(prepared, SolveConfig{
		EntryState: left,
		Schedule:   transfer.ScheduleWTO,
	}, transfer.RetainedBudget{})
	if err != nil {
		t.Fatalf("SolvePreparedRetained: %v", err)
	}
	defer retained.Release()
	compatible, err := retained.StructurallyCompatible(prepared, SolveConfig{
		EntryState: right,
		Schedule:   transfer.ScheduleWTO,
	})
	if err != nil {
		t.Fatalf("StructurallyCompatible: %v", err)
	}
	if compatible {
		t.Fatal("retained session accepted a changed heap-table-object graph")
	}
}

func TestRetainedInputWitnessIncludesStructuralSolveConfig(t *testing.T) {
	reg := standard.Registry()
	prepared, err := PrepareChunk(parseChunk(t, "return 1"), Config{Registry: reg, Schedule: transfer.ScheduleWTO})
	if err != nil {
		t.Fatalf("PrepareChunk: %v", err)
	}
	entryPoint := prepared.cfg.Graph.Entry()
	baseConfig := SolveConfig{}
	base, _, err := captureRetainedInputWitness(prepared, baseConfig, prepared.solveTypeValues(baseConfig), state.State{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	variants := map[string]SolveConfig{
		"schedule": {Schedule: transfer.ScheduleWTO},
		"widen-at": {WidenAt: func(point cfg.Point) bool { return point == entryPoint }},
		"widen-delay": {WidenDelay: func(point cfg.Point) int {
			if point == entryPoint {
				return 2
			}
			return 0
		}},
		"closed-dynamic": {ClosedDynamicAllValues: []factapply.ClosedDynamicAllValueInvariant{{
			Container: pathdom.NewPath(1, "items"),
			Table:     pathdom.NewPath(2, "index"),
		}}},
	}
	for name, config := range variants {
		t.Run(name, func(t *testing.T) {
			other, _, err := captureRetainedInputWitness(prepared, config, prepared.solveTypeValues(config), state.State{}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if base.equal(reg, other) {
				t.Fatal("structural SolveConfig change preserved exact retained witness")
			}
		})
	}
}

func TestRetainedInputWitnessPreservesShadowedClosedDynamicPaths(t *testing.T) {
	reg := standard.Registry()
	prepared, err := PrepareChunk(parseChunk(t, "return 1"), Config{Registry: reg, Schedule: transfer.ScheduleWTO})
	if err != nil {
		t.Fatal(err)
	}
	first := factapply.ClosedDynamicAllValueInvariant{
		Container: pathdom.NewPath(101, "items"),
		Table:     pathdom.NewPath(201, "index"),
	}
	second := factapply.ClosedDynamicAllValueInvariant{
		Container: pathdom.NewPath(102, "items"),
		Table:     pathdom.NewPath(202, "index"),
	}
	capture := func(invariants ...factapply.ClosedDynamicAllValueInvariant) retainedInputWitness {
		config := SolveConfig{ClosedDynamicAllValues: invariants}
		witness, _, captureErr := captureRetainedInputWitness(
			prepared, config, prepared.solveTypeValues(config), state.State{}, nil,
		)
		if captureErr != nil {
			t.Fatal(captureErr)
		}
		return witness
	}
	left := capture(first, second)
	reordered := capture(second, first)
	if !left.equal(reg, reordered) {
		t.Fatal("closed-dynamic witness depends on input order")
	}
	shadowChanged := second
	shadowChanged.Container = pathdom.NewPath(103, "items")
	if left.equal(reg, capture(first, shadowChanged)) {
		t.Fatal("same-name shadowed container symbols alias in retained witness")
	}
	shadowChanged = second
	shadowChanged.Table = pathdom.NewPath(203, "index")
	if left.equal(reg, capture(first, shadowChanged)) {
		t.Fatal("same-name shadowed table symbols alias in retained witness")
	}
}

func resultVersionHeapStates(t *testing.T, reg *axis.Registry, keys *keyspace.KeySpace) (state.State, state.State) {
	t.Helper()
	id := identity.ID{Kind: "lua.table", Site: "result-version-regression", Index: 1}
	root := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	tableKey, ok := keys.FromStateKey(pathdom.PathKey("sym1@1.table"))
	if !ok {
		t.Fatal("intern heap dynamic-index table key")
	}
	fact := dynamicindex.NewFact(reg, dynamicindex.FactConfig{
		KeyValue:    root,
		HasKeyValue: true,
		Value:       root,
		HasValue:    true,
		Admission:   dynamicindex.AdmissionAdmitted,
	})
	leftObject := heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root:              root,
		PrefixStableShape: true,
	})
	rightObject := heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root: root,
		DynamicIndexFacts: map[dynamicindex.Key]dynamicindex.Fact{
			{Table: tableKey, Site: "nested-write"}: fact,
		},
	})
	return state.State{}.WriteHeapTableObject(reg, id, leftObject),
		state.State{}.WriteHeapTableObject(reg, id, rightObject)
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
