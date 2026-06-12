package callresult

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/ref"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/effect/signature"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/factflow/apply"
	"github.com/wippyai/go-lua/analysis/engine/factflow/source"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

type signatureMap map[string]signature.Function

func (m signatureMap) Lookup(name string) (signature.Function, bool) {
	sig, ok := m[name]
	return sig, ok
}

func TestByCalleeSymbolProviderReadsSummaryReturns(t *testing.T) {
	reg := product.DefaultRegistry()
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

func TestSignatureProviderMaterializesDeclaredReturns(t *testing.T) {
	reg := product.DefaultRegistry()
	provider := SignatureProvider(signatureMap{
		"f": {Type: typ.Func().Returns(typ.Number, typ.String).Build()},
	}, StaticName("f"))

	got := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallProducer(factflow.CallProducerConfig{
		CalleeSymbol: symbol.ID(17),
	}), state.State{}, nil)

	if len(got) != 2 {
		t.Fatalf("got %d results, want 2: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.Number))
	assertRuntimeKind(t, reg, got[1].Value, runtimekind.Singleton(runtimekind.String))
}

func TestFallbackKeepsPrimarySlotsAndFillsMissingSignatureSlots(t *testing.T) {
	reg := product.DefaultRegistry()
	primaryValue := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.Boolean))
	primary := func(transfer.NodeContext, factflow.CallProducer, state.State, func(cfg.Point) state.State) []apply.CallResult {
		return []apply.CallResult{{Index: 0, Value: primaryValue}}
	}
	signatures := SignatureProvider(signatureMap{
		"f": {Type: typ.Func().Returns(typ.Number, typ.String).Build()},
	}, StaticName("f"))

	got := Fallback(primary, signatures)(transfer.NodeContext{Registry: reg}, factflow.NewCallProducer(factflow.CallProducerConfig{}), state.State{}, nil)

	if len(got) != 2 {
		t.Fatalf("got %d results, want 2: %#v", len(got), got)
	}
	if got[0].Index != 0 || !product.Equal(reg, got[0].Value, primaryValue) {
		t.Fatalf("primary slot = %#v, want index 0 primary value", got[0])
	}
	if got[1].Index != 1 {
		t.Fatalf("fallback slot index = %d, want 1", got[1].Index)
	}
	assertRuntimeKind(t, reg, got[1].Value, runtimekind.Singleton(runtimekind.String))
}

func TestProviderMissingAndEmptyReturnsYieldNoResults(t *testing.T) {
	reg := product.DefaultRegistry()
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
		provider apply.CallResultProvider
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

func TestByCalleeSymbolKeyMapsAreCloned(t *testing.T) {
	reg := product.DefaultRegistry()
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
		NodeTransfer: apply.NewFactsNodeTransfer(apply.FactsNodeTransferConfig{
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
			Sources: source.NewSourceValues(source.SourceValuesConfig{Registry: reg}),
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
		"github.com/wippyai/go-lua/analysis/check/fixpoint/summary":        true,
		"github.com/wippyai/go-lua/analysis/domain/effect/signature":       true,
		"github.com/wippyai/go-lua/analysis/domain/value/axis":             true,
		"github.com/wippyai/go-lua/analysis/domain/value/axis/presence":    true,
		"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind": true,
		"github.com/wippyai/go-lua/analysis/domain/value/product":          true,
		"github.com/wippyai/go-lua/analysis/engine/factflow":               true,
		"github.com/wippyai/go-lua/analysis/engine/factflow/apply":         true,
		"github.com/wippyai/go-lua/analysis/engine/state":                  true,
		"github.com/wippyai/go-lua/analysis/engine/transfer":               true,
		"github.com/wippyai/go-lua/analysis/ir/cfg":                        true,
		"github.com/wippyai/go-lua/analysis/symbol":                        true,
		"github.com/wippyai/go-lua/analysis/type/kind":                     true,
		"github.com/wippyai/go-lua/analysis/type/typ":                      true,
		"strings": true,
	}
	for _, dep := range strings.Fields(string(out)) {
		if !allowed[dep] {
			t.Fatalf("unexpected production import %q", dep)
		}
	}

	forbidden := []string{"/__old", "/adapter", "/query", "/compiler", "/analysis/lua", "/cfgbuild", "/semantics", "/diagnostic", "/diagnostics", "/store", "/session"}
	for _, dep := range strings.Fields(string(out)) {
		for _, forbiddenPart := range forbidden {
			if strings.Contains(dep, forbiddenPart) {
				t.Fatalf("forbidden production import %q matched %q", dep, forbiddenPart)
			}
		}
	}
}

func assertCallResults(t *testing.T, reg *axis.Registry, got []apply.CallResult, want []product.Value) {
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

func assertRuntimeKind(t *testing.T, reg *axis.Registry, got product.Value, want runtimekind.Value) {
	t.Helper()
	if kind := product.Get(reg, got, runtimekind.Key); !runtimekind.Equal(kind, want) {
		t.Fatalf("runtimekind = %s, want %s", kind, want)
	}
}
