package db

import "testing"

func TestInputRevisionBumps(t *testing.T) {
	db := New()
	input := NewInput[string, int](db)

	input.Set("a", 1)

	rev1 := input.revision("a")
	if rev1 == 0 {
		t.Fatal("expected non-zero revision after Set")
	}

	input.Set("a", 2)

	rev2 := input.revision("a")
	if rev2 <= rev1 {
		t.Fatalf("expected revision to increase, got %d then %d", rev1, rev2)
	}
}

func TestQueryCachesUntilInputChanges(t *testing.T) {
	db := New()
	ctx := NewQueryContext(db)
	input := NewInput[string, int](db)
	input.Set("x", 1)

	calls := 0
	q := NewQuery("double", func(ctx *QueryContext, key string) int {
		calls++
		v, _ := input.Get(ctx, key)

		return v * 2
	}, func(a, b int) bool { return a == b })

	if got := q.Get(ctx, "x"); got != 2 {
		t.Fatalf("got %d, want 2", got)
	}

	if got := q.Get(ctx, "x"); got != 2 {
		t.Fatalf("got %d, want 2", got)
	}

	if calls != 1 {
		t.Fatalf("expected 1 compute call, got %d", calls)
	}

	input.Set("x", 2)

	if got := q.Get(ctx, "x"); got != 4 {
		t.Fatalf("got %d, want 4", got)
	}

	if calls != 2 {
		t.Fatalf("expected 2 compute calls after input change, got %d", calls)
	}
}

func TestQueryTracksQueryDependencies(t *testing.T) {
	db := New()
	ctx := NewQueryContext(db)
	input := NewInput[string, int](db)
	input.Set("n", 3)

	baseCalls := 0
	base := NewQuery("base", func(ctx *QueryContext, key string) int {
		baseCalls++
		v, _ := input.Get(ctx, key)

		return v
	}, func(a, b int) bool { return a == b })

	derivedCalls := 0
	derived := NewQuery("derived", func(ctx *QueryContext, key string) int {
		derivedCalls++
		return base.Get(ctx, key) + 1
	}, func(a, b int) bool { return a == b })

	if got := derived.Get(ctx, "n"); got != 4 {
		t.Fatalf("got %d, want 4", got)
	}

	if got := derived.Get(ctx, "n"); got != 4 {
		t.Fatalf("got %d, want 4", got)
	}

	if baseCalls != 1 || derivedCalls != 1 {
		t.Fatalf("expected 1 call each, got base=%d derived=%d", baseCalls, derivedCalls)
	}

	input.Set("n", 5)

	if got := derived.Get(ctx, "n"); got != 6 {
		t.Fatalf("got %d, want 6", got)
	}

	if baseCalls != 2 || derivedCalls != 2 {
		t.Fatalf("expected recompute after input change, got base=%d derived=%d", baseCalls, derivedCalls)
	}
}

func TestQueryStoresLatestValueWhenDependencyRevisionIsUnchanged(t *testing.T) {
	type projected struct {
		dependency int
		payload    int
	}

	db := New()
	ctx := NewQueryContext(db)
	input := NewInput[string, int](db)
	input.Set("n", 2)

	calls := 0
	query := NewQuery("projected", func(ctx *QueryContext, key string) projected {
		calls++
		v, _ := input.Get(ctx, key)
		return projected{dependency: v % 2, payload: v}
	}, func(a, b projected) bool {
		return a.dependency == b.dependency
	})

	if got := query.Get(ctx, "n"); got.payload != 2 {
		t.Fatalf("initial payload = %d, want 2", got.payload)
	}

	input.Set("n", 4)
	if got := query.Get(ctx, "n"); got.payload != 4 {
		t.Fatalf("recomputed payload = %d, want 4", got.payload)
	}
	if calls != 2 {
		t.Fatalf("expected recompute after input change, got %d calls", calls)
	}
	if got := query.Get(ctx, "n"); got.payload != 4 {
		t.Fatalf("cached payload = %d, want latest equal-dependency value 4", got.payload)
	}
	if calls != 2 {
		t.Fatalf("expected cache hit after latest value stored, got %d calls", calls)
	}
}

func TestRevisionQueryInvalidatesOnDBRevision(t *testing.T) {
	db := New()
	ctx := NewQueryContext(db)
	input := NewInput[string, int](db)
	input.Set("n", 2)

	calls := 0
	query := NewRevisionQuery("revision", func(ctx *QueryContext, key string) int {
		calls++
		v, _ := input.Get(ctx, key)
		return v * 3
	}, func(a, b int) bool { return a == b })

	if got := query.Get(ctx, "n"); got != 6 {
		t.Fatalf("got %d, want 6", got)
	}
	if got := query.Get(ctx, "n"); got != 6 {
		t.Fatalf("got %d, want 6", got)
	}
	if calls != 1 {
		t.Fatalf("expected revision query cache hit, got %d calls", calls)
	}

	input.Set("n", 4)
	if got := query.Get(ctx, "n"); got != 12 {
		t.Fatalf("got %d, want 12", got)
	}
	if calls != 2 {
		t.Fatalf("expected coarse invalidation after revision bump, got %d calls", calls)
	}
}

func TestRevisionQueryNilContextComputesDirectly(t *testing.T) {
	calls := 0
	query := NewRevisionQuery("revision-nil-context", func(_ *QueryContext, _ string) int {
		calls++
		return calls
	}, func(a, b int) bool { return a == b })

	if got := query.Get(nil, "n"); got != 1 {
		t.Fatalf("got %d, want 1", got)
	}
	if got := query.Get(nil, "n"); got != 2 {
		t.Fatalf("got %d, want 2", got)
	}
}

func TestRevisionQueryParentDependencyTracksProjectedValue(t *testing.T) {
	db := New()
	ctx := NewQueryContext(db)
	input := NewInput[string, int](db)
	input.Set("n", 2)

	baseCalls := 0
	base := NewRevisionQuery("revision-base", func(ctx *QueryContext, key string) int {
		baseCalls++
		v, _ := input.Get(ctx, key)
		return v % 2
	}, func(a, b int) bool { return a == b })

	derivedCalls := 0
	derived := NewQuery("derived-from-revision", func(ctx *QueryContext, key string) int {
		derivedCalls++
		return base.Get(ctx, key) + 10
	}, func(a, b int) bool { return a == b })

	if got := derived.Get(ctx, "n"); got != 10 {
		t.Fatalf("got %d, want 10", got)
	}
	if got := derived.Get(ctx, "n"); got != 10 {
		t.Fatalf("got %d, want 10", got)
	}
	if baseCalls != 1 || derivedCalls != 1 {
		t.Fatalf("expected cached calls, got base=%d derived=%d", baseCalls, derivedCalls)
	}

	input.Set("n", 4)
	if got := derived.Get(ctx, "n"); got != 10 {
		t.Fatalf("got %d, want 10", got)
	}
	if baseCalls != 2 {
		t.Fatalf("expected revision query revalidation, got base=%d", baseCalls)
	}
	if derivedCalls != 1 {
		t.Fatalf("expected parent cache to stay valid when projected value is unchanged, got derived=%d", derivedCalls)
	}

	input.Set("n", 5)
	if got := derived.Get(ctx, "n"); got != 11 {
		t.Fatalf("got %d, want 11", got)
	}
	if baseCalls != 3 || derivedCalls != 2 {
		t.Fatalf("expected parent recompute when projected value changes, got base=%d derived=%d", baseCalls, derivedCalls)
	}
}

func TestQueryCycleUsesWiden(t *testing.T) {
	db := New()
	ctx := NewQueryContext(db)

	var q *Query[int, int]
	q = NewQueryWithWiden("cycle", func(ctx *QueryContext, key int) int {
		return q.Get(ctx, key) + 1
	}, func(a, b int) bool { return a == b }, func(prev, _ int) int {
		return prev
	})

	if got := q.Get(ctx, 1); got != 1 {
		t.Fatalf("expected widened result 1, got %d", got)
	}
}

func TestQueryCycleUsesKeyAwareSeed(t *testing.T) {
	db := New()
	ctx := NewQueryContext(db)

	var q *Query[int, int]
	q = NewQueryWithSeedAndWiden("key-seeded-cycle", func(ctx *QueryContext, key int) int {
		return q.Get(ctx, key) + 1
	}, func(a, b int) bool { return a == b }, func(_ *QueryContext, key int) int {
		return key * 10
	}, func(prev, _ int) int {
		return prev
	})

	if got := q.Get(ctx, 2); got != 21 {
		t.Fatalf("expected key-aware seed result 21, got %d", got)
	}
}

func TestQueryRevalidationHandlesCyclicQueryDependency(t *testing.T) {
	db := New()
	ctx := NewQueryContext(db)
	input := NewInput[string, int](db)
	input.Set("n", 1)

	var q *Query[string, int]
	q = NewQueryWithWiden("self-revalidating", func(ctx *QueryContext, key string) int {
		limit, _ := input.Get(ctx, key)
		current := q.Get(ctx, key)
		if current < limit {
			return current + 1
		}
		return current
	}, func(a, b int) bool { return a == b }, func(prev, next int) int {
		if next > prev {
			return next
		}
		return prev
	})

	if got := q.Get(ctx, "n"); got != 1 {
		t.Fatalf("initial cyclic query result = %d, want 1", got)
	}

	input.Set("n", 2)
	if got := q.Get(ctx, "n"); got != 2 {
		t.Fatalf("revalidated cyclic query result = %d, want 2", got)
	}
}
