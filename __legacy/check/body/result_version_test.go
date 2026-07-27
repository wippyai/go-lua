package body

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/solve"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

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
