package persistentcalls

import (
	"context"
	"errors"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/symbol"
	latticelaws "github.com/wippyai/go-lua/analysis/test/laws/lattice"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

const testSlot = key.Value(1)

func TestCanonicalCellExtendsOnlyMonotoneEntryGrowth(t *testing.T) {
	reg := standard.Registry()
	domain := state.Domain(reg)
	engine, err := New(reg, domain, []Definition[state.State]{{
		ID: "identity",
		Factory: func(entry state.State) Workspace[state.State] {
			return &identityWorkspace{domain: domain, entry: entry}
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	stringEntry := stateWithValue(reg, typevalue.LiteralString(reg, "x"))
	numberEntry := stateWithValue(reg, typevalue.LiteralInt(reg, 1))
	if err := engine.AddEntry("identity", stringEntry); err != nil {
		t.Fatal(err)
	}
	first, firstStats, err := engine.Solve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if firstStats.WorkspaceBuilds["identity"] != 1 || firstStats.EntryExtensions["identity"] != 0 {
		t.Fatalf("first stats=%#v", firstStats)
	}
	if err := engine.AddEntry("identity", numberEntry); err != nil {
		t.Fatal(err)
	}
	second, secondStats, err := engine.Solve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if secondStats.WorkspaceBuilds["identity"] != 1 || secondStats.EntryExtensions["identity"] == 0 {
		t.Fatalf("entry growth rebuilt instead of extending: %#v", secondStats)
	}
	want := domain.Join(stringEntry, numberEntry)
	if !domain.Equal(second["identity"].Value, want) || second["identity"].Revision <= first["identity"].Revision {
		t.Fatal("monotone entry growth was not published")
	}
}

func TestCalleeRevisionInvalidatesWorkspaceFromCanonicalEntry(t *testing.T) {
	reg := standard.Registry()
	domain := state.Domain(reg)
	definitions := []Definition[state.State]{
		{
			ID:      "a-caller",
			Callees: []FunctionID{"z-callee"},
			Factory: func(entry state.State) Workspace[state.State] {
				return &cachedCalleeWorkspace{entry: entry, callee: "z-callee"}
			},
		},
		{
			ID: "z-callee",
			Factory: func(entry state.State) Workspace[state.State] {
				return &identityWorkspace{domain: domain, entry: entry}
			},
		},
	}
	engine, err := New(reg, domain, definitions)
	if err != nil {
		t.Fatal(err)
	}
	entry := stateWithValue(reg, typevalue.LiteralString(reg, "argument"))
	if err := engine.AddEntry("a-caller", entry); err != nil {
		t.Fatal(err)
	}
	got, stats, err := engine.Solve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !domain.Equal(got["a-caller"].Value, entry) || !domain.Equal(got["z-callee"].Value, entry) {
		t.Fatal("caller retained the callee's initial bottom summary")
	}
	if stats.WorkspaceBuilds["a-caller"] < 2 {
		t.Fatalf("callee refinement did not reset caller workspace: %#v", stats.WorkspaceBuilds)
	}
}

func TestOldHybridReuseDemonstratesStaleDependencyHole(t *testing.T) {
	reg := standard.Registry()
	domain := state.Domain(reg)
	entry := stateWithValue(reg, typevalue.LiteralString(reg, "argument"))
	workspace := &cachedCalleeWorkspace{entry: entry, callee: "callee"}
	bottomReader := staticReader{"callee": {Value: domain.Bottom(), Revision: 0}}
	first, err := workspace.Solve(context.Background(), bottomReader)
	if err != nil {
		t.Fatal(err)
	}
	refinedReader := staticReader{"callee": {Value: entry, Revision: 1}}
	stale, err := workspace.Solve(context.Background(), refinedReader)
	if err != nil {
		t.Fatal(err)
	}
	if !domain.Equal(first.Summary, stale.Summary) || domain.Equal(stale.Summary, entry) {
		t.Fatal("test workspace did not reproduce stale hybrid reuse")
	}
	fresh, err := (&cachedCalleeWorkspace{entry: entry, callee: "callee"}).Solve(context.Background(), refinedReader)
	if err != nil || !domain.Equal(fresh.Summary, entry) {
		t.Fatal("canonical reset did not consume refined dependency")
	}
}

func TestCanceledTransactionPublishesNothingAndDropsWorkspace(t *testing.T) {
	reg := standard.Registry()
	domain := state.Domain(reg)
	fail := false
	engine, err := New(reg, domain, []Definition[state.State]{{
		ID: "body",
		Factory: func(entry state.State) Workspace[state.State] {
			return &controlledWorkspace{entry: entry, fail: &fail}
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	firstEntry := stateWithValue(reg, typevalue.LiteralString(reg, "first"))
	_ = engine.AddEntry("body", firstEntry)
	first, _, err := engine.Solve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	fail = true
	_ = engine.AddEntry("body", stateWithValue(reg, typevalue.LiteralInt(reg, 2)))
	failed, _, err := engine.Solve(context.Background())
	if !errors.Is(err, errControlled) {
		t.Fatalf("solve error=%v", err)
	}
	if !domain.Equal(failed["body"].Value, first["body"].Value) || failed["body"].Revision != first["body"].Revision {
		t.Fatal("failed transaction partially published")
	}
	if engine.cells["body"].workspace != nil {
		t.Fatal("failed transaction retained a partially mutated workspace")
	}
}

func TestInterproceduralCycleUsesWTOAndAllDefaultStateLanes(t *testing.T) {
	reg := standard.Registry()
	domain := state.Domain(reg)
	makeDefinition := func(id, callee FunctionID) Definition[state.State] {
		return Definition[state.State]{
			ID:      id,
			Callees: []FunctionID{callee},
			Factory: func(entry state.State) Workspace[state.State] {
				return &forwardWorkspace{entry: entry, callee: callee, domain: domain}
			},
		}
	}
	engine, err := New(reg, domain, []Definition[state.State]{makeDefinition("left", "right"), makeDefinition("right", "left")})
	if err != nil {
		t.Fatal(err)
	}
	entry := stateWithValue(reg, typevalue.LiteralString(reg, "cycle"))
	_ = engine.AddEntry("left", entry)
	got, stats, err := engine.Solve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !domain.Equal(got["left"].Value, entry) || !domain.Equal(got["right"].Value, entry) {
		t.Fatal("WTO cycle did not propagate the entry")
	}
	if stats.WTO.TransferCalls == 0 || !recursiveCell("left", engine.influences) || !recursiveCell("right", engine.influences) {
		t.Fatalf("cycle was not scheduled as recursive WTO cells: %#v", stats.WTO)
	}
	if len(state.DefaultLanes()) == 0 {
		t.Fatal("production State catalog unexpectedly empty")
	}
	if !engine.domain.Equal(engine.domain.Top(), domain.Top()) {
		t.Fatal("engine did not retain the default State domain")
	}
}

func TestSummaryDomainIsGenericOverProductionShapedPayload(t *testing.T) {
	reg := standard.Registry()
	domain := lattice.Lattice[summaryMask]{
		Bottom:   func() summaryMask { return 0 },
		Top:      func() summaryMask { return 0xff },
		Equal:    func(a, b summaryMask) bool { return a == b },
		LessOrEq: func(a, b summaryMask) bool { return a&b == a },
		Join:     func(a, b summaryMask) summaryMask { return a | b },
		Widen:    func(a, b summaryMask) summaryMask { return a | b },
	}
	engine, err := New(reg, domain, []Definition[summaryMask]{{
		ID: "production-shaped-summary",
		Factory: func(state.State) Workspace[summaryMask] {
			return maskWorkspace{summary: summaryReturns | summaryObligations | summaryFacts | summaryEffects | summaryContexts}
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := engine.Solve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := summaryReturns | summaryObligations | summaryFacts | summaryEffects | summaryContexts
	if got["production-shaped-summary"].Value != want {
		t.Fatalf("generic summary=%08b want=%08b", got["production-shaped-summary"].Value, want)
	}
}

func TestInterproceduralApproximationLatticeLaws(t *testing.T) {
	reg := standard.Registry()
	states := state.Domain(reg)
	stringState := stateWithValue(reg, typevalue.LiteralString(reg, "s"))
	numberState := stateWithValue(reg, typevalue.LiteralInt(reg, 1))
	a := approximation[state.State]{
		valid:       true,
		summary:     stringState,
		callEntries: map[FunctionID]state.State{"callee": stringState},
	}
	b := approximation[state.State]{
		valid:       true,
		summary:     numberState,
		callEntries: map[FunctionID]state.State{"callee": numberState},
	}
	domain := approximationDomain(states, states)
	latticelaws.LawSuite[approximation[state.State]]{
		Name:          "poc.persistentcalls.approximation",
		Domain:        domain,
		Sample:        []approximation[state.State]{{}, domain.Top(), a, b, domain.Join(a, b)},
		WideningBound: 8,
	}.Run(t)
}

func TestMergedContextsAreNotGenerallyPrecisionEquivalent(t *testing.T) {
	reg := standard.Registry()
	domain := state.Domain(reg)
	stringEntry := stateWithValue(reg, typevalue.LiteralString(reg, "s"))
	numberEntry := stateWithValue(reg, typevalue.LiteralInt(reg, 1))
	stringResult := stateWithValue(reg, typevalue.LiteralBool(reg, true))
	numberResult := stateWithValue(reg, typevalue.LiteralBool(reg, false))
	factory := func(entry state.State) Workspace[state.State] {
		return &contextSensitiveWorkspace{
			reg:          reg,
			domain:       domain,
			entry:        entry,
			stringEntry:  stringEntry,
			numberEntry:  numberEntry,
			stringResult: stringResult,
			numberResult: numberResult,
		}
	}
	solve := func(entries ...state.State) state.State {
		engine, err := New(reg, domain, []Definition[state.State]{{ID: "f", Factory: factory}})
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if err := engine.AddEntry("f", entry); err != nil {
				t.Fatal(err)
			}
		}
		got, _, err := engine.Solve(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		return got["f"].Value
	}
	separateString := solve(stringEntry)
	separateNumber := solve(numberEntry)
	merged := solve(stringEntry, numberEntry)
	if !domain.Equal(separateString, stringResult) || !domain.Equal(separateNumber, numberResult) {
		t.Fatal("separate context baseline is wrong")
	}
	if domain.Equal(merged, separateString) || domain.Equal(merged, separateNumber) {
		t.Fatal("merged lexical cell unexpectedly retained caller-specific precision")
	}
	// The generic mechanism is sound, but byte-identical migration requires a
	// bounded context partition (or a proven distributive body) for this case.
}

func stateWithValue(reg *axis.Registry, value product.Value) state.State {
	domain := state.Domain(reg)
	return domain.Bottom().WriteValue(reg, key.SymbolValue(symbol.ID(testSlot)), value)
}

type identityWorkspace struct {
	domain lattice.Lattice[state.State]
	entry  state.State
}

func (w *identityWorkspace) ExtendEntry(entry state.State) { w.entry = entry }
func (w *identityWorkspace) Solve(context.Context, SummaryReader[state.State]) (BodyResult[state.State], error) {
	return BodyResult[state.State]{Summary: w.entry}, nil
}

type cachedCalleeWorkspace struct {
	entry  state.State
	callee FunctionID
	cached *state.State
}

func (w *cachedCalleeWorkspace) ExtendEntry(entry state.State) { w.entry = entry }
func (w *cachedCalleeWorkspace) Solve(_ context.Context, reader SummaryReader[state.State]) (BodyResult[state.State], error) {
	if w.cached == nil {
		summary, _ := reader.Read(w.callee)
		cached := summary.Value
		w.cached = &cached
	}
	return BodyResult[state.State]{Summary: *w.cached, CallEntries: map[FunctionID]state.State{w.callee: w.entry}}, nil
}

type forwardWorkspace struct {
	entry  state.State
	callee FunctionID
	domain lattice.Lattice[state.State]
}

func (w *forwardWorkspace) ExtendEntry(entry state.State) { w.entry = entry }
func (w *forwardWorkspace) Solve(_ context.Context, reader SummaryReader[state.State]) (BodyResult[state.State], error) {
	summary, _ := reader.Read(w.callee)
	return BodyResult[state.State]{
		Summary:     w.domain.Join(w.entry, summary.Value),
		CallEntries: map[FunctionID]state.State{w.callee: w.entry},
	}, nil
}

var errControlled = errors.New("controlled solve failure")

type controlledWorkspace struct {
	entry state.State
	fail  *bool
}

func (w *controlledWorkspace) ExtendEntry(entry state.State) { w.entry = entry }
func (w *controlledWorkspace) Solve(context.Context, SummaryReader[state.State]) (BodyResult[state.State], error) {
	if *w.fail {
		return BodyResult[state.State]{}, errControlled
	}
	return BodyResult[state.State]{Summary: w.entry}, nil
}

type staticReader map[FunctionID]Summary[state.State]

func (r staticReader) Read(id FunctionID) (Summary[state.State], bool) {
	summary, ok := r[id]
	return summary, ok
}

type summaryMask uint8

const (
	summaryReturns summaryMask = 1 << iota
	summaryObligations
	summaryFacts
	summaryEffects
	summaryContexts
)

type maskWorkspace struct{ summary summaryMask }

func (maskWorkspace) ExtendEntry(state.State) {}
func (w maskWorkspace) Solve(context.Context, SummaryReader[summaryMask]) (BodyResult[summaryMask], error) {
	return BodyResult[summaryMask]{Summary: w.summary}, nil
}

type contextSensitiveWorkspace struct {
	reg          *axis.Registry
	domain       lattice.Lattice[state.State]
	entry        state.State
	stringEntry  state.State
	numberEntry  state.State
	stringResult state.State
	numberResult state.State
}

func (w *contextSensitiveWorkspace) ExtendEntry(entry state.State) { w.entry = entry }
func (w *contextSensitiveWorkspace) Solve(context.Context, SummaryReader[state.State]) (BodyResult[state.State], error) {
	var result state.State
	switch {
	case w.domain.Equal(w.entry, w.stringEntry):
		result = w.stringResult
	case w.domain.Equal(w.entry, w.numberEntry):
		result = w.numberResult
	default:
		result = w.domain.Top()
	}
	return BodyResult[state.State]{Summary: result}, nil
}
