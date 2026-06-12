package callresult

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/ref"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/standard"
	factapply "github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestByCalleeSymbolProviderReadsSummaryReturns(t *testing.T) {
	reg := standard.Registry()
	callee := symbol.ID(17)
	key := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 18})
	first := product.Top()
	second := product.Absent(reg)
	provider := Provider(summary.NewSnapshot(reg, summary.EntrySummary{
		Key:     key,
		Summary: summary.Summary{Returns: []product.Value{first, second}},
	}), ByCalleeSymbol(map[symbol.ID]summary.SummaryKey{callee: key}))

	got := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallProducer(factflow.CallProducerConfig{
		CalleeSymbol: callee,
	}), state.State{}, nil)

	assertCallResults(t, reg, got, []product.Value{first, second})
}

func TestOutcomeProviderMapsSummaryReturnsAndNormalReturnFacts(t *testing.T) {
	reg := standard.Registry()
	callee := symbol.ID(37)
	key := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 38})
	ret := product.Absent(reg)
	present := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	absent := product.Absent(reg)
	provider := OutcomeProvider(summary.NewSnapshot(reg, summary.EntrySummary{
		Key: key,
		Summary: summary.Summary{
			Returns: []product.Value{ret},
			NormalReturnFacts: summary.NormalReturnFacts{
				PathRefinements: []summary.PathValueFact{
					{Path: path.NewPlaceholder(0).Field("field"), Value: absent},
				},
				PathStaticMembers: []summary.PathStaticMemberFact{
					{Path: path.NewPlaceholder(0).Field("static"), Value: present},
				},
				DynamicIndexFacts: []summary.DynamicIndexFact{
					{
						Table:       path.NewPlaceholder(0).Field("items"),
						Site:        "summary.dynamic",
						KeyPresence: presence.Present(),
						KeyValue:    present,
						Value:       absent,
						Admission:   summary.DynamicIndexAdmissionAdmitted,
					},
				},
				BranchProofs: []summary.BranchProof{
					{
						Kind:  summary.BranchProofPathEqual,
						Path:  path.NewPlaceholder(0).Field("left"),
						Other: path.NewPlaceholder(1).Field("right"),
					},
				},
				ChannelSelects: []summary.ChannelSelectFact{
					{
						Select: "summary.select",
						Kind:   summary.ChannelSelectFactReceive,
						Result: path.NewPlaceholder(0).Field("result"),
						Case:   path.NewPlaceholder(1).Field("case"),
						Index:  2,
					},
				},
				EffectDeltas: []summary.EffectDelta{
					{
						Target: path.NewPlaceholder(0).Field("items"),
						Site:   "summary.effect",
						Kind:   summary.EffectDeltaMutation,
						Before: present,
						After:  absent,
						Change: summary.EffectDeltaChangeChanged,
					},
				},
			},
		},
	}), ByCalleeSymbol(map[symbol.ID]summary.SummaryKey{callee: key}))

	got := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		CalleeSymbol: callee,
	}), state.State{}, nil)

	assertCallResults(t, reg, got.Results, []product.Value{ret})
	if len(got.PathRefinements) != 1 ||
		!got.PathRefinements[0].Path.Equal(path.NewPlaceholder(0).Field("field")) ||
		!product.Equal(reg, got.PathRefinements[0].Value, absent) {
		t.Fatalf("path refinements = %#v, want mapped summary path refinement", got.PathRefinements)
	}
	if len(got.PathStaticMembers) != 1 ||
		!got.PathStaticMembers[0].Path.Equal(path.NewPlaceholder(0).Field("static")) ||
		!product.Equal(reg, got.PathStaticMembers[0].Value, present) {
		t.Fatalf("path static members = %#v, want mapped summary static member", got.PathStaticMembers)
	}
	if len(got.DynamicIndexFacts) != 1 ||
		!got.DynamicIndexFacts[0].Table.Equal(path.NewPlaceholder(0).Field("items")) ||
		got.DynamicIndexFacts[0].Site != "summary.dynamic" ||
		!presence.Equal(got.DynamicIndexFacts[0].KeyPresence, presence.Present()) ||
		!product.Equal(reg, got.DynamicIndexFacts[0].KeyValue, present) ||
		!product.Equal(reg, got.DynamicIndexFacts[0].Value, absent) ||
		got.DynamicIndexFacts[0].Admission != factapply.CallDynamicIndexAdmissionAdmitted {
		t.Fatalf("dynamic-index facts = %#v, want mapped summary dynamic fact", got.DynamicIndexFacts)
	}
	if len(got.BranchProofs) != 1 ||
		got.BranchProofs[0].Kind != factapply.CallBranchProofPathEqual ||
		!got.BranchProofs[0].Path.Equal(path.NewPlaceholder(0).Field("left")) ||
		!got.BranchProofs[0].Other.Equal(path.NewPlaceholder(1).Field("right")) {
		t.Fatalf("branch proofs = %#v, want mapped summary branch proof", got.BranchProofs)
	}
	if len(got.ChannelSelects) != 1 ||
		got.ChannelSelects[0].Kind != factapply.CallChannelSelectFactReceive ||
		got.ChannelSelects[0].Select != "summary.select" ||
		!got.ChannelSelects[0].Result.Equal(path.NewPlaceholder(0).Field("result")) ||
		!got.ChannelSelects[0].Case.Equal(path.NewPlaceholder(1).Field("case")) ||
		got.ChannelSelects[0].Index != 2 {
		t.Fatalf("channel selects = %#v, want mapped summary channel select", got.ChannelSelects)
	}
	if len(got.EffectDeltas) != 1 ||
		got.EffectDeltas[0].Kind != factapply.CallEffectDeltaMutation ||
		got.EffectDeltas[0].Site != "summary.effect" ||
		!got.EffectDeltas[0].Target.Equal(path.NewPlaceholder(0).Field("items")) ||
		!product.Equal(reg, got.EffectDeltas[0].Before, present) ||
		!product.Equal(reg, got.EffectDeltas[0].After, absent) ||
		got.EffectDeltas[0].Change != factapply.CallEffectDeltaChangeChanged {
		t.Fatalf("effect deltas = %#v, want mapped summary effect delta", got.EffectDeltas)
	}
}

func TestProviderMissingAndEmptyReturnsYieldNoResults(t *testing.T) {
	reg := standard.Registry()
	callee := symbol.ID(17)
	key := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 18})
	missingKey := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 19})
	snap := summary.NewSnapshot(reg, summary.EntrySummary{
		Key:     key,
		Summary: summary.Summary{Returns: []product.Value{product.Top()}},
	})
	call := factflow.NewCallProducer(factflow.CallProducerConfig{CalleeSymbol: callee})
	ctx := transfer.NodeContext{Registry: reg}

	tests := []struct {
		name     string
		provider factapply.CallResultProvider
	}{
		{
			name:     "nil reader",
			provider: Provider(nil, ByCalleeSymbol(map[symbol.ID]summary.SummaryKey{callee: key})),
		},
		{
			name:     "nil key func",
			provider: Provider(snap, nil),
		},
		{
			name:     "missing key",
			provider: Provider(snap, ByCalleeSymbol(map[symbol.ID]summary.SummaryKey{})),
		},
		{
			name:     "missing summary",
			provider: Provider(snap, ByCalleeSymbol(map[symbol.ID]summary.SummaryKey{callee: missingKey})),
		},
		{
			name:     "empty returns",
			provider: Provider(summary.NewSnapshot(reg, summary.EntrySummary{Key: key, Summary: summary.Summary{Returns: nil}}), ByCalleeSymbol(map[symbol.ID]summary.SummaryKey{callee: key})),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.provider(ctx, call, state.State{}, nil); len(got) != 0 {
				t.Fatalf("provider returned %d results, want none", len(got))
			}
		})
	}
}

func TestFallbackKeepsPrimarySlotsAndFillsMissingSlots(t *testing.T) {
	reg := standard.Registry()
	primaryValue := product.Absent(reg)
	fallbackValue := product.Top()
	primary := func(transfer.NodeContext, factflow.CallProducer, state.State, func(cfg.Point) state.State) []factapply.CallResult {
		return []factapply.CallResult{{Index: 0, Value: primaryValue}}
	}
	fallback := func(transfer.NodeContext, factflow.CallProducer, state.State, func(cfg.Point) state.State) []factapply.CallResult {
		return []factapply.CallResult{{Index: 0, Value: product.Top()}, {Index: 1, Value: fallbackValue}}
	}

	got := Fallback(primary, fallback)(transfer.NodeContext{Registry: reg}, factflow.NewCallProducer(factflow.CallProducerConfig{}), state.State{}, nil)

	if len(got) != 2 {
		t.Fatalf("got %d results, want 2: %#v", len(got), got)
	}
	if got[0].Index != 0 || !product.Equal(reg, got[0].Value, primaryValue) {
		t.Fatalf("primary slot = %#v, want index 0 primary value", got[0])
	}
	if got[1].Index != 1 || !product.Equal(reg, got[1].Value, fallbackValue) {
		t.Fatalf("fallback slot = %#v, want index 1 fallback value", got[1])
	}
}

func TestByCalleeSymbolKeyMapsAreCloned(t *testing.T) {
	reg := standard.Registry()
	callee := symbol.ID(21)
	symbolKey := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 23})
	symbolValue := product.Absent(reg)
	snap := summary.NewSnapshot(reg,
		summary.EntrySummary{Key: symbolKey, Summary: summary.Summary{Returns: []product.Value{symbolValue}}},
	)

	symbolMap := map[symbol.ID]summary.SummaryKey{callee: symbolKey}
	symbolProvider := Provider(snap, ByCalleeSymbol(symbolMap))
	symbolMap[callee] = summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 25})

	assertCallResults(t, reg, symbolProvider(transfer.NodeContext{Registry: reg}, factflow.NewCallProducer(factflow.CallProducerConfig{CalleeSymbol: callee}), state.State{}, nil), []product.Value{symbolValue})
}

func TestByCalleeIdentityPrefersSymbolAndFallsBackToPath(t *testing.T) {
	symbolKey := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 31})
	pathKey := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 32})
	calleeSymbol := symbol.ID(33)
	calleePath := path.NewPath(symbol.ID(34), "module").Field("call")
	keyFor := ByCalleeIdentity(
		map[symbol.ID]summary.SummaryKey{calleeSymbol: symbolKey},
		map[path.PathKey]summary.SummaryKey{calleePath.Key(): pathKey},
	)

	got, ok := keyFor(transfer.NodeContext{}, factflow.NewCallProducer(factflow.CallProducerConfig{
		CalleeSymbol: calleeSymbol,
		CalleePath:   calleePath,
	}))
	if !ok || got != symbolKey {
		t.Fatalf("symbol key = %v, %v; want %v, true", got, ok, symbolKey)
	}

	got, ok = keyFor(transfer.NodeContext{}, factflow.NewCallProducer(factflow.CallProducerConfig{
		CalleePath: calleePath,
	}))
	if !ok || got != pathKey {
		t.Fatalf("path key = %v, %v; want %v, true", got, ok, pathKey)
	}
}

func TestProviderIntegratesWithFactflowCallRead(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	assign := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, assign, false)
	graph.AddEdge(assign, graph.Exit(), false)

	calleeKey := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 26})
	callValue := product.Top()
	existingTargetValue := product.Absent(reg)
	target := symbol.ID(27)

	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: state.State{}.WriteValue(reg, key.SymbolValue(target), existingTargetValue),
		NodeTransfer: factapply.NewFactsNodeTransfer(factapply.FactsNodeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				CallSites: map[cfg.Point]factflow.CallSite{
					call: factflow.NewCallSite(factflow.CallSiteConfig{
						Context:      factflow.CallSiteContextAssignmentSource,
						CalleeSymbol: symbol.ID(28),
						ResultTargets: []factflow.CallResultTarget{
							factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, 0, 0, target, path.NewPath(target, "x")),
						},
					}),
				},
				RootAssignments: map[cfg.Point]factflow.RootAssignment{
					assign: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, target, path.NewPath(target, "x"), factflow.ValueSource{
						Kind:         factflow.ValueSourceCall,
						CallPoint:    call,
						HasCallPoint: true,
						ResultIndex:  0,
					}),
				},
			}),
			Sources: sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{Registry: reg}),
			CallResults: Provider(summary.NewSnapshot(reg, summary.EntrySummary{
				Key:     calleeKey,
				Summary: summary.Summary{Returns: []product.Value{callValue}},
			}), ByCalleeSymbol(map[symbol.ID]summary.SummaryKey{symbol.ID(28): calleeKey})),
		}),
	})

	assertValue(t, reg, got[call], key.SymbolValue(target), existingTargetValue)
	assertValue(t, reg, got[graph.Exit()], key.SymbolValue(target), callValue)
}

func TestProductionImportsAreBounded(t *testing.T) {
	out, err := exec.Command("go", "list", "-f", "{{range .Imports}}{{.}}\n{{end}}", ".").Output()
	if err != nil {
		t.Fatalf("go list imports . error = %v", err)
	}
	allowed := map[string]bool{
		"github.com/wippyai/go-lua/analysis/check/fixpoint/summary": true,
		"github.com/wippyai/go-lua/analysis/domain/path":            true,
		"github.com/wippyai/go-lua/analysis/engine/factapply":       true,
		"github.com/wippyai/go-lua/analysis/engine/factflow":        true,
		"github.com/wippyai/go-lua/analysis/engine/state":           true,
		"github.com/wippyai/go-lua/analysis/engine/transfer":        true,
		"github.com/wippyai/go-lua/analysis/ir/cfg":                 true,
		"github.com/wippyai/go-lua/analysis/symbol":                 true,
	}
	for _, imp := range strings.Fields(string(out)) {
		if !allowed[imp] {
			t.Fatalf("unexpected production import %q", imp)
		}
	}
}

func TestFactapplyDoesNotImportSummary(t *testing.T) {
	out, err := exec.Command("go", "list", "-f", "{{range .Imports}}{{.}}\n{{end}}", "../../../engine/factapply").Output()
	if err != nil {
		t.Fatalf("go list imports factapply error = %v", err)
	}
	for _, imp := range strings.Fields(string(out)) {
		if imp == "github.com/wippyai/go-lua/analysis/check/fixpoint/summary" {
			t.Fatalf("factapply imports summary")
		}
	}
}

func assertCallResults(t *testing.T, reg *axis.Registry, got []factapply.CallResult, want []product.Value) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d results, want %d: %#v", len(got), len(want), got)
	}
	for i, value := range want {
		if got[i].Index != i {
			t.Fatalf("result %d index = %d, want %d", i, got[i].Index, i)
		}
		if !product.Equal(reg, got[i].Value, value) {
			t.Fatalf("result %d value = %#v, want %#v", i, got[i].Value, value)
		}
	}
}

func assertValue(t *testing.T, reg *axis.Registry, st state.State, slot key.Value, want product.Value) {
	t.Helper()
	got := st.ReadValue(reg, slot)
	if !product.Equal(reg, got, want) {
		t.Fatalf("state[%v] = %#v, want %#v", slot, got, want)
	}
}
