package query

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/ref"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/solve"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestRunSingleBodyReturnsExactKeySnapshot(t *testing.T) {
	reg := standard.Registry()
	key := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 1})

	snap, err := Run(Config{
		Registry: reg,
		Functions: []Function{{
			Key: key,
			Body: func(Context) (summary.Summary, error) {
				return oneReturn(product.Top()), nil
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	got, ok := snap.Read(key)
	if !ok {
		t.Fatalf("Read(key) missing")
	}
	if len(got.Returns) != 1 || !product.Equal(reg, got.Returns[0], product.Top()) {
		t.Fatalf("Read(key) = %#v, want one top return", got)
	}
}

func TestSnapshotExactReadsDoNotFallbackByFuncRef(t *testing.T) {
	reg := standard.Registry()
	fn := ref.FuncRef{Kind: ref.KindSymbol, ID: 2}
	exact := summary.SummaryKey{Ref: fn, Entry: summary.EntryKey{Values: 1}}

	snap, err := Run(Config{
		Registry: reg,
		Functions: []Function{{
			Key: exact,
			Body: func(Context) (summary.Summary, error) {
				return oneReturn(product.Top()), nil
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if got, ok := snap.Read(summary.DefaultSummaryKey(fn)); ok {
		t.Fatalf("Read(default same ref) = %#v, want missing exact key", got)
	}
}

func TestBodyReadCreatesDependencyAndObservesUpdatedValue(t *testing.T) {
	reg, err := standard.RegistryWithAxes(queryTestSpec().Erase())
	if err != nil {
		t.Fatalf("RegistryWithAxes() error = %v", err)
	}
	source := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 3})
	dependent := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 4})
	sourceVisits := 0
	stats := Stats{}

	snap, err := Run(Config{
		Registry: reg,
		Stats:    &stats,
		Functions: []Function{
			{
				Key: source,
				Body: func(ctx Context) (summary.Summary, error) {
					sourceVisits++
					_, _ = ctx.Summaries.Read(source)
					if sourceVisits == 1 {
						return oneReturn(testValue(reg, queryTestLow)), nil
					}
					return oneReturn(testValue(reg, queryTestHigh)), nil
				},
			},
			{
				Key: dependent,
				Body: func(ctx Context) (summary.Summary, error) {
					got, ok := ctx.Summaries.Read(source)
					if !ok {
						t.Fatalf("source read missing")
					}
					return got, nil
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	got, ok := snap.Read(dependent)
	if !ok {
		t.Fatalf("Read(dependent) missing")
	}
	if len(got.Returns) != 1 {
		t.Fatalf("dependent returns = %d, want 1", len(got.Returns))
	}
	if value := product.Get(reg, got.Returns[0], queryTestKey); value != queryTestHigh {
		t.Fatalf("dependent observed axis %v, want high", value)
	}
	if stats.BodyInvocations <= 2 {
		t.Fatalf("BodyInvocations = %d, want requeued body invocations", stats.BodyInvocations)
	}
	if stats.Solver.TransferCalls != stats.BodyInvocations {
		t.Fatalf("Solver.TransferCalls = %d, BodyInvocations = %d", stats.Solver.TransferCalls, stats.BodyInvocations)
	}
}

func TestMissingReturnSlotsAreBottomInJoinAndWiden(t *testing.T) {
	reg := standard.Registry()
	joinKey := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 5})
	widenKey := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 6})
	joinVisits := 0
	widenVisits := 0

	snap, err := Run(Config{
		Registry: reg,
		Functions: []Function{
			{
				Key: joinKey,
				Body: func(ctx Context) (summary.Summary, error) {
					joinVisits++
					_, _ = ctx.Summaries.Read(joinKey)
					if joinVisits == 1 {
						return oneReturn(product.Absent(reg)), nil
					}
					return summary.Summary{Returns: []product.Value{product.Absent(reg), product.Top()}}, nil
				},
			},
			{
				Key: widenKey,
				Body: func(ctx Context) (summary.Summary, error) {
					widenVisits++
					_, _ = ctx.Summaries.Read(widenKey)
					if widenVisits == 1 {
						return oneReturn(product.Absent(reg)), nil
					}
					return summary.Summary{Returns: []product.Value{product.Absent(reg), product.Top()}}, nil
				},
			},
		},
		WidenAt: func(key summary.SummaryKey) bool {
			return key == widenKey
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	for _, key := range []summary.SummaryKey{joinKey, widenKey} {
		got, ok := snap.Read(key)
		if !ok {
			t.Fatalf("Read(%v) missing", key)
		}
		if len(got.Returns) != 2 || !product.Equal(reg, got.Returns[1], product.Top()) {
			t.Fatalf("Read(%v) = %#v, want missing second slot joined/widened to top", key, got)
		}
	}
}

func TestSeedInitializesExactKeyAndDoesNotFallbackByFuncRef(t *testing.T) {
	reg := standard.Registry()
	fn := ref.FuncRef{Kind: ref.KindSymbol, ID: 7}
	seededKey := summary.SummaryKey{Ref: fn, Entry: summary.EntryKey{Values: 1}}
	configuredKey := summary.DefaultSummaryKey(fn)
	seed := summary.NewSnapshot(reg, summary.EntrySummary{
		Key:     seededKey,
		Summary: oneReturn(product.Top()),
	})
	visits := 0

	snap, err := Run(Config{
		Registry: reg,
		Seed:     seed,
		Functions: []Function{{
			Key: configuredKey,
			Body: func(ctx Context) (summary.Summary, error) {
				visits++
				if got, ok := ctx.Summaries.Read(configuredKey); visits == 1 && ok && len(got.Returns) != 0 {
					t.Fatalf("configured key unexpectedly read seeded same-ref summary: %#v", got)
				}
				got, ok := ctx.Summaries.Read(seededKey)
				if !ok {
					t.Fatalf("seeded exact key missing")
				}
				return got, nil
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	got, ok := snap.Read(configuredKey)
	if !ok {
		t.Fatalf("Read(configuredKey) missing")
	}
	if len(got.Returns) != 1 || !product.Equal(reg, got.Returns[0], product.Top()) {
		t.Fatalf("Read(configuredKey) = %#v, want seeded top return", got)
	}
	if got, ok := snap.Read(seededKey); ok {
		t.Fatalf("snapshot included unconfigured seed key: %#v", got)
	}
}

func TestSeedInitializesConfiguredExactKey(t *testing.T) {
	reg := standard.Registry()
	key := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 8})
	seed := summary.NewSnapshot(reg, summary.EntrySummary{
		Key:     key,
		Summary: oneReturn(product.Top()),
	})

	snap, err := Run(Config{
		Registry: reg,
		Seed:     seed,
		Functions: []Function{{
			Key: key,
			Body: func(ctx Context) (summary.Summary, error) {
				got, _ := ctx.Summaries.Read(key)
				return got, nil
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	got, ok := snap.Read(key)
	if !ok {
		t.Fatalf("Read(key) missing")
	}
	if len(got.Returns) != 1 || !product.Equal(reg, got.Returns[0], product.Top()) {
		t.Fatalf("Read(key) = %#v, want seeded top return", got)
	}
}

func TestWidenHooksAreForwardedForSummaryKeys(t *testing.T) {
	reg := standard.Registry()
	key := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 9})
	var widenAtKeys []summary.SummaryKey
	var widenDelayKeys []summary.SummaryKey
	visits := 0

	_, err := Run(Config{
		Registry: reg,
		Functions: []Function{{
			Key: key,
			Body: func(ctx Context) (summary.Summary, error) {
				visits++
				_, _ = ctx.Summaries.Read(key)
				if visits == 1 {
					return oneReturn(product.Absent(reg)), nil
				}
				return oneReturn(product.Top()), nil
			},
		}},
		WidenAt: func(got summary.SummaryKey) bool {
			widenAtKeys = append(widenAtKeys, got)
			return got == key
		},
		WidenDelay: func(got summary.SummaryKey) int {
			widenDelayKeys = append(widenDelayKeys, got)
			return 1
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !containsKey(widenAtKeys, key) {
		t.Fatalf("WidenAt was not called with key; calls=%v", widenAtKeys)
	}
	if !containsKey(widenDelayKeys, key) {
		t.Fatalf("WidenDelay was not called with key; calls=%v", widenDelayKeys)
	}
}

func TestPinnedSummaryIsVisibleButNeverScheduledOrWidened(t *testing.T) {
	reg := standard.Registry()
	pinnedKey := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 101})
	functionKey := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 102})
	input := []summary.EntrySummary{{Key: pinnedKey, Summary: oneReturn(product.Top())}}
	stats := Stats{}
	var widened []summary.SummaryKey

	driver, err := New(Config{
		Registry: reg,
		Pinned:   input,
		Stats:    &stats,
		Functions: []Function{{
			Key: functionKey,
			Body: func(ctx Context) (summary.Summary, error) {
				got, ok := ctx.Summaries.Read(pinnedKey)
				if !ok {
					t.Fatalf("Read(pinnedKey) missing")
				}
				return got, nil
			},
		}},
		WidenAt: func(key summary.SummaryKey) bool {
			widened = append(widened, key)
			return true
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	// New owns a normalized copy; caller mutation cannot alter the pinned input.
	input[0].Summary.Returns = nil

	snap, err := driver.Run()
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	for _, key := range []summary.SummaryKey{pinnedKey, functionKey} {
		got, ok := snap.Read(key)
		if !ok || len(got.Returns) != 1 || !product.Equal(reg, got.Returns[0], product.Top()) {
			t.Fatalf("Read(%v) = %#v, %v; want one top return", key, got, ok)
		}
	}
	if stats.BodyInvocations != 1 {
		t.Fatalf("BodyInvocations = %d, want only the configured equation body", stats.BodyInvocations)
	}
	if containsKey(widened, pinnedKey) {
		t.Fatalf("WidenAt called for pinned key; calls=%v", widened)
	}
}

func TestActiveReaderPinnedReadBypassesSolverRead(t *testing.T) {
	reg := standard.Registry()
	key := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 107})
	reads := 0
	r := activeReader{
		reg:    reg,
		known:  map[summary.SummaryKey]struct{}{},
		pinned: map[summary.SummaryKey]summary.Summary{key: oneReturn(product.Top())},
		read: func(summary.SummaryKey) summary.Summary {
			reads++
			return summary.Summary{}
		},
	}
	got, ok := r.Read(key)
	if !ok || len(got.Returns) != 1 || !product.Equal(reg, got.Returns[0], product.Top()) {
		t.Fatalf("Read(pinned) = %#v, %v; want one top return", got, ok)
	}
	if reads != 0 {
		t.Fatalf("solver reads = %d, want 0 for pinned input", reads)
	}
}

func TestPinnedSnapshotOrderIsDeterministic(t *testing.T) {
	reg := standard.Registry()
	low := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 103})
	high := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 104})
	entry := func(key summary.SummaryKey) summary.EntrySummary {
		return summary.EntrySummary{Key: key, Summary: oneReturn(product.Top())}
	}

	first, err := Run(Config{Registry: reg, Pinned: []summary.EntrySummary{entry(high), entry(low)}})
	if err != nil {
		t.Fatalf("Run(first) error = %v", err)
	}
	second, err := Run(Config{Registry: reg, Pinned: []summary.EntrySummary{entry(low), entry(high)}})
	if err != nil {
		t.Fatalf("Run(second) error = %v", err)
	}
	firstEntries, secondEntries := first.Entries(), second.Entries()
	if len(firstEntries) != 2 || len(secondEntries) != 2 {
		t.Fatalf("entry lengths = %d, %d; want 2", len(firstEntries), len(secondEntries))
	}
	for i := range firstEntries {
		if firstEntries[i].Key != secondEntries[i].Key || !summary.Equal(reg, firstEntries[i].Summary, secondEntries[i].Summary) {
			t.Fatalf("entry %d differs across pinned input order", i)
		}
	}
	if firstEntries[0].Key != low || firstEntries[1].Key != high {
		t.Fatalf("entry order = %v, %v; want key order", firstEntries[0].Key, firstEntries[1].Key)
	}
}

func TestCanceledRunDoesNotEvaluateOrPublishPinnedInputs(t *testing.T) {
	reg := standard.Registry()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	pinnedKey := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 105})
	functionKey := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 106})
	invocations := 0

	snap, err := Run(Config{
		Context:  ctx,
		Registry: reg,
		Pinned:   []summary.EntrySummary{{Key: pinnedKey, Summary: oneReturn(product.Top())}},
		Functions: []Function{{Key: functionKey, Body: func(Context) (summary.Summary, error) {
			invocations++
			return oneReturn(product.Top()), nil
		}}},
	})
	if !errors.Is(err, solve.ErrCanceled) || !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want solve and context cancellation", err)
	}
	if invocations != 0 {
		t.Fatalf("body invocations = %d, want 0", invocations)
	}
	if len(snap.Entries()) != 0 {
		t.Fatalf("canceled snapshot entries = %v, want none", snap.Entries())
	}
}

func TestNewValidationErrors(t *testing.T) {
	reg := standard.Registry()
	key := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 10})

	if _, err := New(Config{}); !errors.Is(err, ErrRegistryRequired) {
		t.Fatalf("New(no registry) error = %v, want ErrRegistryRequired", err)
	}
	if _, err := New(Config{
		Registry: reg,
		Functions: []Function{
			{Key: key, Body: func(Context) (summary.Summary, error) { return summary.Summary{}, nil }},
			{Key: key, Body: func(Context) (summary.Summary, error) { return summary.Summary{}, nil }},
		},
	}); !errors.Is(err, ErrDuplicateFunction) {
		t.Fatalf("New(duplicate) error = %v, want ErrDuplicateFunction", err)
	}
	if _, err := New(Config{
		Registry:  reg,
		Functions: []Function{{Key: key}},
	}); !errors.Is(err, ErrNilBody) {
		t.Fatalf("New(nil body) error = %v, want ErrNilBody", err)
	}
	if _, err := New(Config{
		Registry: reg,
		Pinned: []summary.EntrySummary{
			{Key: key},
			{Key: key},
		},
	}); !errors.Is(err, ErrDuplicatePinned) {
		t.Fatalf("New(duplicate pinned) error = %v, want ErrDuplicatePinned", err)
	}
	if _, err := New(Config{
		Registry: reg,
		Functions: []Function{{
			Key:  key,
			Body: func(Context) (summary.Summary, error) { return summary.Summary{}, nil },
		}},
		Pinned: []summary.EntrySummary{{Key: key}},
	}); !errors.Is(err, ErrPinnedFunctionConflict) {
		t.Fatalf("New(pinned/function conflict) error = %v, want ErrPinnedFunctionConflict", err)
	}
}

func TestProductionDependenciesAvoidForbiddenPackages(t *testing.T) {
	out, err := exec.Command("go", "list", "-f", "{{range .Imports}}{{.}}\n{{end}}", ".").Output()
	if err != nil {
		t.Fatalf("go list imports . error = %v", err)
	}
	forbidden := []string{
		"/__old",
		"/compiler",
		"/analysis/lua",
		"/cfgbuild",
		"/semantics",
		"/diagnostic",
		"/store",
		"/session",
	}
	for _, dep := range strings.Fields(string(out)) {
		for _, forbiddenPart := range forbidden {
			if strings.Contains(dep, forbiddenPart) {
				t.Fatalf("forbidden dependency %q matched %q", dep, forbiddenPart)
			}
		}
	}
}

func oneReturn(v product.Value) summary.Summary {
	return summary.Summary{Returns: []product.Value{v}}
}

func containsKey(keys []summary.SummaryKey, want summary.SummaryKey) bool {
	for _, got := range keys {
		if got == want {
			return true
		}
	}
	return false
}

type queryTestAxis uint8

const (
	queryTestBottom queryTestAxis = iota
	queryTestLow
	queryTestHigh
	queryTestTop
)

var queryTestKey = axis.NewKey[queryTestAxis]("test.query.axis")

func queryTestSpec() axis.Spec[queryTestAxis] {
	return axis.Spec[queryTestAxis]{
		Key:    queryTestKey,
		Bottom: func() queryTestAxis { return queryTestBottom },
		Top:    func() queryTestAxis { return queryTestTop },
		Equal:  func(a, b queryTestAxis) bool { return a == b },
		LessOrEq: func(a, b queryTestAxis) bool {
			return a <= b
		},
		Join: func(a, b queryTestAxis) queryTestAxis {
			if a > b {
				return a
			}
			return b
		},
		Meet: func(a, b queryTestAxis) queryTestAxis {
			if a < b {
				return a
			}
			return b
		},
		Widen: func(prev, next queryTestAxis) queryTestAxis {
			if prev > next {
				return prev
			}
			return next
		},
		Hash:      func(v queryTestAxis) uint64 { return uint64(v) },
		Boundary:  axis.PortableIdentity,
		Retention: axis.ImmutableRetention[queryTestAxis](),
		Canonical: axis.PendingCanonical[queryTestAxis]("test-only axis"),
	}
}

func testValue(reg *axis.Registry, value queryTestAxis) product.Value {
	return product.Set(reg, product.Top(), queryTestKey, value)
}
