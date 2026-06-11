package callresult

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/canonical/ref"
	"github.com/wippyai/go-lua/analysis/check/canonical/summary"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestByPointProviderReadsSummaryReturns(t *testing.T) {
	reg := product.DefaultRegistry()
	point := cfg.Point(7)
	key := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindCFG, ID: 11})
	first := product.Top()
	second := product.Absent(reg)
	provider := Provider(summary.NewSnapshot(reg, summary.EntrySummary{
		Key:     key,
		Summary: summary.Summary{Returns: []product.Value{first, second}},
	}), ByPoint(map[cfg.Point]summary.SummaryKey{point: key}))

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallProducer(factflow.CallProducerConfig{}), state.State{}, nil)

	assertCallResults(t, reg, got, []product.Value{first, second})
}

func TestByCalleeSymbolProviderReadsSummaryReturns(t *testing.T) {
	reg := product.DefaultRegistry()
	callee := symbol.ID(17)
	key := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 18})
	want := product.Top()
	provider := Provider(summary.NewSnapshot(reg, summary.EntrySummary{
		Key:     key,
		Summary: summary.Summary{Returns: []product.Value{want}},
	}), ByCalleeSymbol(map[symbol.ID]summary.SummaryKey{callee: key}))

	got := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallProducer(factflow.CallProducerConfig{
		CalleeSymbol: callee,
	}), state.State{}, nil)

	assertCallResults(t, reg, got, []product.Value{want})
}

func TestProviderMissingInputsReturnNoResults(t *testing.T) {
	reg := product.DefaultRegistry()
	point := cfg.Point(3)
	key := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindCFG, ID: 19})
	snap := summary.NewSnapshot(reg, summary.EntrySummary{
		Key:     key,
		Summary: summary.Summary{Returns: []product.Value{product.Top()}},
	})
	call := factflow.NewCallProducer(factflow.CallProducerConfig{})
	ctx := transfer.NodeContext{Registry: reg, Point: point}

	tests := []struct {
		name     string
		provider factflow.CallResultProvider
	}{
		{
			name:     "nil reader",
			provider: Provider(nil, ByPoint(map[cfg.Point]summary.SummaryKey{point: key})),
		},
		{
			name:     "nil key func",
			provider: Provider(snap, nil),
		},
		{
			name:     "missing key",
			provider: Provider(snap, ByPoint(map[cfg.Point]summary.SummaryKey{})),
		},
		{
			name:     "missing summary",
			provider: Provider(snap, ByPoint(map[cfg.Point]summary.SummaryKey{point: summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindCFG, ID: 20})})),
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

func TestKeyMapsAreCloned(t *testing.T) {
	reg := product.DefaultRegistry()
	point := cfg.Point(5)
	callee := symbol.ID(21)
	pointKey := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindCFG, ID: 22})
	symbolKey := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 23})
	pointValue := product.Top()
	symbolValue := product.Absent(reg)
	snap := summary.NewSnapshot(reg,
		summary.EntrySummary{Key: pointKey, Summary: summary.Summary{Returns: []product.Value{pointValue}}},
		summary.EntrySummary{Key: symbolKey, Summary: summary.Summary{Returns: []product.Value{symbolValue}}},
	)

	pointMap := map[cfg.Point]summary.SummaryKey{point: pointKey}
	pointProvider := Provider(snap, ByPoint(pointMap))
	pointMap[point] = summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindCFG, ID: 24})

	symbolMap := map[symbol.ID]summary.SummaryKey{callee: symbolKey}
	symbolProvider := Provider(snap, ByCalleeSymbol(symbolMap))
	symbolMap[callee] = summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 25})

	assertCallResults(t, reg, pointProvider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallProducer(factflow.CallProducerConfig{}), state.State{}, nil), []product.Value{pointValue})
	assertCallResults(t, reg, symbolProvider(transfer.NodeContext{Registry: reg}, factflow.NewCallProducer(factflow.CallProducerConfig{CalleeSymbol: callee}), state.State{}, nil), []product.Value{symbolValue})
}

func TestProviderIntegratesWithFactflowCallRead(t *testing.T) {
	reg := product.DefaultRegistry()
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
		NodeTransfer: factflow.NewFactsNodeTransfer(factflow.FactsNodeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				Calls: map[cfg.Point]factflow.CallProducer{
					call: factflow.NewCallProducer(factflow.CallProducerConfig{
						Context:      factflow.CallProducerContextAssignment,
						CalleeSymbol: symbol.ID(28),
						ResultTargets: []factflow.CallResultTarget{
							factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, 0, target, path.NewPath(target, "x")),
						},
					}),
				},
				LocalAssignments: map[cfg.Point]factflow.RootAssignment{
					assign: factflow.NewRootAssignment(target, path.NewPath(target, "x"), factflow.ValueSource{
						Kind:         factflow.ValueSourceCall,
						CallPoint:    call,
						HasCallPoint: true,
						ResultIndex:  0,
					}),
				},
			}),
			Sources: factflow.NewSourceValues(factflow.SourceValuesConfig{Registry: reg}),
			CallResults: Provider(summary.NewSnapshot(reg, summary.EntrySummary{
				Key:     calleeKey,
				Summary: summary.Summary{Returns: []product.Value{callValue}},
			}), ByPoint(map[cfg.Point]summary.SummaryKey{call: calleeKey})),
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
		"github.com/wippyai/go-lua/analysis/check/canonical/summary": true,
		"github.com/wippyai/go-lua/analysis/engine/factflow":         true,
		"github.com/wippyai/go-lua/analysis/engine/state":            true,
		"github.com/wippyai/go-lua/analysis/engine/transfer":         true,
		"github.com/wippyai/go-lua/analysis/ir/cfg":                  true,
		"github.com/wippyai/go-lua/analysis/symbol":                  true,
	}
	for _, dep := range strings.Fields(string(out)) {
		if !allowed[dep] {
			t.Fatalf("unexpected production import %q", dep)
		}
	}

	forbidden := []string{
		"/__old",
		"/adapter",
		"/query",
		"/compiler",
		"/analysis/lua",
		"/cfgbuild",
		"/semantics",
		"/diagnostic",
		"/diagnostics",
		"/store",
		"/session",
	}
	for _, dep := range strings.Fields(string(out)) {
		for _, forbiddenPart := range forbidden {
			if strings.Contains(dep, forbiddenPart) {
				t.Fatalf("forbidden production import %q matched %q", dep, forbiddenPart)
			}
		}
	}
}

func assertCallResults(t *testing.T, reg *axis.Registry, got []factflow.CallResult, want []product.Value) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d results, want %d", len(got), len(want))
	}
	for i, value := range want {
		if got[i].Index != i {
			t.Fatalf("got result[%d].Index = %d, want %d", i, got[i].Index, i)
		}
		if !product.Equal(reg, got[i].Value, value) {
			t.Fatalf("got result[%d].Value = %v, want %v", i, got[i].Value, value)
		}
	}
}

func assertValue(t *testing.T, reg *axis.Registry, st state.State, slot key.Value, want product.Value) {
	t.Helper()
	if got := st.ReadValue(reg, slot); !product.Equal(reg, got, want) {
		t.Fatalf("state[%s] = %v, want %v", slot, got, want)
	}
}
