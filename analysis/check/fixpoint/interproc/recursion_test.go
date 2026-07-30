package interproc

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
)

func TestRecursionCoordinatorClosesDirectRecursionAsOneAtomicSCC(t *testing.T) {
	table := NewProjectedTable()
	key := recursiveKey(t, "direct")
	evaluator := recursiveEvaluator{
		discover: func(_ context.Context, got InstanceKey) ([]InstanceKey, error) {
			if !got.equal(key) {
				return nil, errors.New("unexpected key")
			}
			return []InstanceKey{key}, nil
		},
		evaluate: func(_ context.Context, got InstanceKey, values RecursiveValues) (ClosedOutcome, error) {
			if _, err := values.Read(got); err != nil {
				return ClosedOutcome{}, err
			}
			return recursiveOutcome(t, "one"), nil
		},
	}
	coordinator := NewRecursionCoordinator(table, 8)
	outcome, err := coordinator.Resolve(context.Background(), key, evaluator, binaryOutcomeLattice{})
	if err != nil {
		t.Fatal(err)
	}
	if string(outcome.CanonicalBytes()) != "one" {
		t.Fatalf("outcome = %q, want one", outcome.CanonicalBytes())
	}
	metrics := coordinator.Metrics()
	if metrics.Groups != 1 || metrics.DirectSCCs != 1 || metrics.MutualSCCs != 0 || metrics.AtomicCommits != 1 {
		t.Fatalf("recursion metrics = %+v, want one direct atomic SCC", metrics)
	}
	if got := table.Metrics(); got.Cells != 1 || got.Executions != 1 || got.Failures != 0 {
		t.Fatalf("table metrics = %+v, want one closed cell", got)
	}
}

func TestRecursionCoordinatorSolvesMutualRecursionInDeterministicKeyOrder(t *testing.T) {
	table := NewProjectedTable()
	left, right := recursiveKey(t, "left"), recursiveKey(t, "right")
	var mu sync.Mutex
	var order []InstanceKey
	evaluator := recursiveEvaluator{
		discover: func(_ context.Context, key InstanceKey) ([]InstanceKey, error) {
			switch {
			case key.equal(left):
				return []InstanceKey{right}, nil
			case key.equal(right):
				return []InstanceKey{left}, nil
			default:
				return nil, errors.New("unknown instance")
			}
		},
		evaluate: func(_ context.Context, key InstanceKey, values RecursiveValues) (ClosedOutcome, error) {
			other := left
			if key.equal(left) {
				other = right
			}
			if _, err := values.Read(other); err != nil {
				return ClosedOutcome{}, err
			}
			mu.Lock()
			order = append(order, key)
			mu.Unlock()
			return recursiveOutcome(t, "one"), nil
		},
	}
	coordinator := NewRecursionCoordinator(table, 8)
	if _, err := coordinator.Resolve(context.Background(), right, evaluator, binaryOutcomeLattice{}); err != nil {
		t.Fatal(err)
	}
	if len(order) != 4 { // two synchronous lattice passes for two members
		t.Fatalf("evaluation count = %d, want 4", len(order))
	}
	ordered := []InstanceKey{left, right}
	sort.Slice(ordered, func(i, j int) bool { return instanceLess(ordered[i], ordered[j]) })
	for pass := 0; pass < 2; pass++ {
		for index, want := range ordered {
			if !order[pass*2+index].equal(want) {
				t.Fatalf("pass %d index %d evaluated a nondeterministic SCC owner order", pass, index)
			}
		}
	}
	metrics := coordinator.Metrics()
	if metrics.Groups != 1 || metrics.MutualSCCs != 1 || metrics.AtomicCommits != 1 {
		t.Fatalf("recursion metrics = %+v, want one mutual atomic SCC", metrics)
	}
	if got := table.Metrics(); got.Cells != 2 || got.Executions != 2 {
		t.Fatalf("table metrics = %+v, want two atomically closed cells", got)
	}
}

func TestRecursionCoordinatorDoesNotPublishPartialMutualSCC(t *testing.T) {
	table := NewProjectedTable()
	left, right := recursiveKey(t, "partial-left"), recursiveKey(t, "partial-right")
	evaluator := recursiveEvaluator{
		discover: func(_ context.Context, key InstanceKey) ([]InstanceKey, error) {
			if key.equal(left) {
				return []InstanceKey{right}, nil
			}
			return []InstanceKey{left}, nil
		},
		evaluate: func(_ context.Context, key InstanceKey, _ RecursiveValues) (ClosedOutcome, error) {
			if key.equal(right) {
				return ClosedOutcome{}, errors.New("body failure")
			}
			return recursiveOutcome(t, "one"), nil
		},
	}
	coordinator := NewRecursionCoordinator(table, 8)
	if _, err := coordinator.Resolve(context.Background(), left, evaluator, binaryOutcomeLattice{}); err == nil {
		t.Fatal("Resolve succeeded after an SCC member failed")
	}
	if got := table.Metrics(); got.Cells != 0 || got.Failures != 2 {
		t.Fatalf("partial recursive SCC published cells: %+v", got)
	}
	if got := coordinator.Metrics(); got.AtomicCommits != 0 || got.Failures != 1 {
		t.Fatalf("coordinator metrics = %+v, want failed uncommitted group", got)
	}
}

func TestRecursionCoordinatorRejectsUnboundedExactDiscovery(t *testing.T) {
	table := NewProjectedTable()
	root := recursiveKey(t, "root")
	coordinator := NewRecursionCoordinator(table, 1)
	evaluator := recursiveEvaluator{
		discover: func(_ context.Context, key InstanceKey) ([]InstanceKey, error) {
			if key.equal(root) {
				return []InstanceKey{recursiveKey(t, "new-projection")}, nil
			}
			return nil, nil
		},
		evaluate: func(_ context.Context, _ InstanceKey, _ RecursiveValues) (ClosedOutcome, error) {
			return recursiveOutcome(t, "one"), nil
		},
	}
	_, err := coordinator.Resolve(context.Background(), root, evaluator, binaryOutcomeLattice{})
	var limit *DiscoveryLimitError
	if !errors.As(err, &limit) {
		t.Fatalf("error = %v, want discovery limit", err)
	}
	if got := table.Metrics(); got.Cells != 0 {
		t.Fatalf("discovery-limit failure retained exact cells: %+v", got)
	}
}

type recursiveEvaluator struct {
	discover func(context.Context, InstanceKey) ([]InstanceKey, error)
	evaluate func(context.Context, InstanceKey, RecursiveValues) (ClosedOutcome, error)
}

func (e recursiveEvaluator) Discover(ctx context.Context, key InstanceKey) ([]InstanceKey, error) {
	return e.discover(ctx, key)
}
func (e recursiveEvaluator) Evaluate(ctx context.Context, key InstanceKey, values RecursiveValues) (ClosedOutcome, error) {
	return e.evaluate(ctx, key, values)
}

type binaryOutcomeLattice struct{}

func (binaryOutcomeLattice) Bottom(InstanceKey) (ClosedOutcome, error) {
	return recursiveOutcome(nil, "zero"), nil
}
func (binaryOutcomeLattice) Height() uint64 { return 1 }
func (binaryOutcomeLattice) Join(_ InstanceKey, previous, candidate ClosedOutcome) (ClosedOutcome, bool, error) {
	if string(previous.CanonicalBytes()) == string(candidate.CanonicalBytes()) {
		return previous, false, nil
	}
	if string(previous.CanonicalBytes()) != "zero" || string(candidate.CanonicalBytes()) != "one" {
		return ClosedOutcome{}, false, errors.New("non-monotone binary lattice transition")
	}
	return candidate, true, nil
}

func recursiveKey(t *testing.T, value string) InstanceKey {
	t.Helper()
	artifact := demandedBodyArtifactFixture(t)
	return mustRecursiveKey(t, artifact, tableEntry(t, value, "true", "diagnostic", "ignored"))
}

func mustRecursiveKey(t *testing.T, artifact DemandedBodyArtifact, entry EntryBinding) InstanceKey {
	t.Helper()
	key, err := NewInstanceKey(artifact, entry)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func recursiveOutcome(t *testing.T, value string) ClosedOutcome {
	if t != nil {
		t.Helper()
	}
	outcome, err := NewClosedOutcome([]byte(value))
	if err != nil {
		if t != nil {
			t.Fatal(err)
		}
		panic(err)
	}
	return outcome
}
