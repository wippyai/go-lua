package callresult

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/ref"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	"github.com/wippyai/go-lua/analysis/domain/effect/signature"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
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
	provider := SignatureProvider(SignatureProviderConfig{
		Signatures: signatureMap{
			"f": {Type: typ.Func().Returns(typ.Number, typ.String).Build()},
		},
		NameFor: StaticName("f"),
	})

	got := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallProducer(factflow.CallProducerConfig{
		CalleeSymbol: symbol.ID(17),
	}), state.State{}, nil)

	if len(got) != 2 {
		t.Fatalf("got %d results, want 2: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.Number))
	assertRuntimeKind(t, reg, got[1].Value, runtimekind.Singleton(runtimekind.String))
}

func TestSignatureProviderSameAsReturnsArgumentValue(t *testing.T) {
	reg := product.DefaultRegistry()
	point := cfg.Point(4)
	argRef := factflow.ExprRef(7)
	argValue := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.String))
	provider := SignatureProvider(SignatureProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type:   typ.Func().Param("value", typ.Any).Returns(typ.Number).Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.SameAs{Source: effect.ParamRef{Index: 0}}}),
			},
		},
		NameFor: StaticName("f"),
		Facts: signatureProviderFacts(point, []factflow.ValueSource{{
			Kind:    factflow.ValueSourceExpression,
			ExprRef: argRef,
			HasExpr: true,
		}}),
		Sources: source.NewSourceValues(source.SourceValuesConfig{
			Registry: reg,
			ExpressionValues: map[factflow.ExprRef]product.Value{
				argRef: argValue,
			},
		}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallProducer(factflow.CallProducerConfig{}), state.State{}, nil)

	assertCallResults(t, reg, got, []product.Value{argValue})
}

func TestSignatureProviderSameAsResolvesNegativeParamRef(t *testing.T) {
	reg := product.DefaultRegistry()
	point := cfg.Point(5)
	firstRef := factflow.ExprRef(8)
	lastRef := factflow.ExprRef(9)
	firstValue := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.Number))
	lastValue := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.Boolean))
	provider := SignatureProvider(SignatureProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type:   typ.Func().Param("first", typ.Any).Param("last", typ.Any).Returns(typ.String).Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.SameAs{Source: effect.ParamRef{Index: -1}}}),
			},
		},
		NameFor: StaticName("f"),
		Facts: signatureProviderFacts(point, []factflow.ValueSource{
			{Kind: factflow.ValueSourceExpression, ExprRef: firstRef, HasExpr: true},
			{Kind: factflow.ValueSourceExpression, ExprRef: lastRef, HasExpr: true},
		}),
		Sources: source.NewSourceValues(source.SourceValuesConfig{
			Registry: reg,
			ExpressionValues: map[factflow.ExprRef]product.Value{
				firstRef: firstValue,
				lastRef:  lastValue,
			},
		}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallProducer(factflow.CallProducerConfig{}), state.State{}, nil)

	assertCallResults(t, reg, got, []product.Value{lastValue})
}

func TestSignatureProviderSameAsFallsBackToDeclaredReturnTypeWhenArgumentUnresolved(t *testing.T) {
	reg := product.DefaultRegistry()
	point := cfg.Point(6)
	provider := SignatureProvider(SignatureProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type:   typ.Func().Returns(typ.Number).Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.SameAs{Source: effect.ParamRef{Index: 1}}}),
			},
		},
		NameFor: StaticName("f"),
		Facts: signatureProviderFacts(point, []factflow.ValueSource{
			{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(10), HasExpr: true},
		}),
		Sources: source.NewSourceValues(source.SourceValuesConfig{Registry: reg}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallProducer(factflow.CallProducerConfig{}), state.State{}, nil)

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.Number))
}

func TestSignatureProviderElementOfArrayReturnsElementRuntimeKind(t *testing.T) {
	reg := product.DefaultRegistry()
	point := cfg.Point(8)
	provider := SignatureProvider(SignatureProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type:   typ.Func().Param("items", typ.NewArray(typ.String)).Returns(typ.Any).Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.ElementOf{Source: effect.ParamRef{Index: 0}}}),
			},
		},
		NameFor: StaticName("f"),
		Facts:   signatureProviderFacts(point, []factflow.ValueSource{{Kind: factflow.ValueSourceExpression}}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallProducer(factflow.CallProducerConfig{}), state.State{}, nil)

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.String))
}

func TestSignatureProviderElementOfMapReturnsValueRuntimeKind(t *testing.T) {
	reg := product.DefaultRegistry()
	point := cfg.Point(9)
	provider := SignatureProvider(SignatureProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type:   typ.Func().Param("items", typ.NewMap(typ.String, typ.Number)).Returns(typ.Any).Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.ElementOf{Source: effect.ParamRef{Index: 0}}}),
			},
		},
		NameFor: StaticName("f"),
		Facts:   signatureProviderFacts(point, []factflow.ValueSource{{Kind: factflow.ValueSourceExpression}}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallProducer(factflow.CallProducerConfig{}), state.State{}, nil)

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.Number))
}

func TestSignatureProviderElementOfTupleReturnsElementUnionRuntimeKind(t *testing.T) {
	reg := product.DefaultRegistry()
	point := cfg.Point(10)
	provider := SignatureProvider(SignatureProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type:   typ.Func().Param("items", typ.NewTuple(typ.String, typ.Number)).Returns(typ.Any).Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.ElementOf{Source: effect.ParamRef{Index: 0}}}),
			},
		},
		NameFor: StaticName("f"),
		Facts:   signatureProviderFacts(point, []factflow.ValueSource{{Kind: factflow.ValueSourceExpression}}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallProducer(factflow.CallProducerConfig{}), state.State{}, nil)

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Join(
		runtimekind.Singleton(runtimekind.String),
		runtimekind.Singleton(runtimekind.Number),
	))
}

func TestSignatureProviderOptionalElementOfArrayKeepsMaybePresence(t *testing.T) {
	reg := product.DefaultRegistry()
	point := cfg.Point(11)
	provider := SignatureProvider(SignatureProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type:   typ.Func().Param("items", typ.NewArray(typ.String)).Returns(typ.Any).Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.OptionalElementOf{Source: effect.ParamRef{Index: 0}}}),
			},
		},
		NameFor: StaticName("f"),
		Facts:   signatureProviderFacts(point, []factflow.ValueSource{{Kind: factflow.ValueSourceExpression}}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallProducer(factflow.CallProducerConfig{}), state.State{}, nil)

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.String))
	if gotPresence := product.PresenceOf(got[0].Value); !presence.Equal(gotPresence, presence.Top()) {
		t.Fatalf("presence = %s, want maybe/top", gotPresence)
	}
}

func TestSignatureProviderElementOfFallsBackToDeclaredReturnTypeWhenParamRefUnresolved(t *testing.T) {
	reg := product.DefaultRegistry()
	point := cfg.Point(12)
	provider := SignatureProvider(SignatureProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type:   typ.Func().Param("items", typ.NewArray(typ.String)).Returns(typ.Number).Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.ElementOf{Source: effect.ParamRef{Index: 1}}}),
			},
		},
		NameFor: StaticName("f"),
		Facts:   signatureProviderFacts(point, []factflow.ValueSource{{Kind: factflow.ValueSourceExpression}}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallProducer(factflow.CallProducerConfig{}), state.State{}, nil)

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.Number))
}

func TestSignatureProviderCallbackReturnProjectsFirstReturnRuntimeKind(t *testing.T) {
	reg := product.DefaultRegistry()
	point := cfg.Point(13)
	provider := SignatureProvider(SignatureProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type: typ.Func().
					Param("callback", typ.Func().Returns(typ.Integer).Build()).
					Returns(typ.Any).
					Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.CallbackReturn{CallbackParam: effect.ParamRef{Index: 0}}}),
			},
		},
		NameFor: StaticName("f"),
		Facts:   signatureProviderFacts(point, []factflow.ValueSource{{Kind: factflow.ValueSourceExpression}}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallProducer(factflow.CallProducerConfig{}), state.State{}, nil)

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.Number))
}

func TestSignatureProviderCallbackReturnResolvesNegativeParamRef(t *testing.T) {
	reg := product.DefaultRegistry()
	point := cfg.Point(14)
	provider := SignatureProvider(SignatureProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type: typ.Func().
					Param("value", typ.String).
					Param("callback", typ.Func().Returns(typ.Boolean).Build()).
					Returns(typ.Any).
					Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.CallbackReturn{CallbackParam: effect.ParamRef{Index: -1}}}),
			},
		},
		NameFor: StaticName("f"),
		Facts: signatureProviderFacts(point, []factflow.ValueSource{
			{Kind: factflow.ValueSourceExpression},
			{Kind: factflow.ValueSourceExpression},
		}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallProducer(factflow.CallProducerConfig{}), state.State{}, nil)

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.Boolean))
}

func TestSignatureProviderArrayOfCallbackReturnProjectsTableRuntimeKind(t *testing.T) {
	reg := product.DefaultRegistry()
	point := cfg.Point(15)
	provider := SignatureProvider(SignatureProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type: typ.Func().
					Param("callback", typ.Func().Returns(typ.String).Build()).
					Returns(typ.Any).
					Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.ArrayOfCallbackReturn{CallbackParam: effect.ParamRef{Index: 0}}}),
			},
		},
		NameFor: StaticName("f"),
		Facts:   signatureProviderFacts(point, []factflow.ValueSource{{Kind: factflow.ValueSourceExpression}}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallProducer(factflow.CallProducerConfig{}), state.State{}, nil)

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.Table))
}

func TestSignatureProviderCallbackReturnFallsBackToDeclaredReturnType(t *testing.T) {
	reg := product.DefaultRegistry()

	tests := []struct {
		name      string
		point     cfg.Point
		paramType typ.Type
		ref       effect.ParamRef
		args      []factflow.ValueSource
		want      runtimekind.Value
	}{
		{
			name:      "non-callable callback parameter",
			point:     cfg.Point(16),
			paramType: typ.String,
			ref:       effect.ParamRef{Index: 0},
			args:      []factflow.ValueSource{{Kind: factflow.ValueSourceExpression}},
			want:      runtimekind.Singleton(runtimekind.Boolean),
		},
		{
			name:      "out-of-range callback parameter",
			point:     cfg.Point(17),
			paramType: typ.Func().Returns(typ.Number).Build(),
			ref:       effect.ParamRef{Index: 1},
			args:      []factflow.ValueSource{{Kind: factflow.ValueSourceExpression}},
			want:      runtimekind.Singleton(runtimekind.Boolean),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			provider := SignatureProvider(SignatureProviderConfig{
				Signatures: signatureMap{
					"f": {
						Type: typ.Func().
							Param("callback", tc.paramType).
							Returns(typ.Boolean).
							Build(),
						Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.CallbackReturn{CallbackParam: tc.ref}}),
					},
				},
				NameFor: StaticName("f"),
				Facts:   signatureProviderFacts(tc.point, tc.args),
			})

			got := provider(transfer.NodeContext{Registry: reg, Point: tc.point}, factflow.NewCallProducer(factflow.CallProducerConfig{}), state.State{}, nil)

			if len(got) != 1 {
				t.Fatalf("got %d results, want 1: %#v", len(got), got)
			}
			assertRuntimeKind(t, reg, got[0].Value, tc.want)
		})
	}
}

func TestFallbackKeepsPrimarySlotsAndFillsMissingSignatureSlots(t *testing.T) {
	reg := product.DefaultRegistry()
	primaryValue := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.Boolean))
	primary := func(transfer.NodeContext, factflow.CallProducer, state.State, func(cfg.Point) state.State) []apply.CallResult {
		return []apply.CallResult{{Index: 0, Value: primaryValue}}
	}
	signatures := SignatureProvider(SignatureProviderConfig{
		Signatures: signatureMap{
			"f": {Type: typ.Func().Returns(typ.Number, typ.String).Build()},
		},
		NameFor: StaticName("f"),
	})

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

func TestFallbackKeepsPrimarySlotOverSignatureSameAs(t *testing.T) {
	reg := product.DefaultRegistry()
	point := cfg.Point(7)
	argRef := factflow.ExprRef(11)
	primaryValue := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.Boolean))
	argValue := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.String))
	primary := func(transfer.NodeContext, factflow.CallProducer, state.State, func(cfg.Point) state.State) []apply.CallResult {
		return []apply.CallResult{{Index: 0, Value: primaryValue}}
	}
	signatures := SignatureProvider(SignatureProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type:   typ.Func().Returns(typ.Number).Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.SameAs{Source: effect.ParamRef{Index: 0}}}),
			},
		},
		NameFor: StaticName("f"),
		Facts: signatureProviderFacts(point, []factflow.ValueSource{{
			Kind:    factflow.ValueSourceExpression,
			ExprRef: argRef,
			HasExpr: true,
		}}),
		Sources: source.NewSourceValues(source.SourceValuesConfig{
			Registry: reg,
			ExpressionValues: map[factflow.ExprRef]product.Value{
				argRef: argValue,
			},
		}),
	})

	got := Fallback(primary, signatures)(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallProducer(factflow.CallProducerConfig{}), state.State{}, nil)

	assertCallResults(t, reg, got, []product.Value{primaryValue})
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
		"github.com/wippyai/go-lua/analysis/domain/effect":                 true,
		"github.com/wippyai/go-lua/analysis/domain/effect/returns":         true,
		"github.com/wippyai/go-lua/analysis/domain/effect/signature":       true,
		"github.com/wippyai/go-lua/analysis/domain/value/axis":             true,
		"github.com/wippyai/go-lua/analysis/domain/value/axis/presence":    true,
		"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind": true,
		"github.com/wippyai/go-lua/analysis/domain/value/product":          true,
		"github.com/wippyai/go-lua/analysis/engine/factflow":               true,
		"github.com/wippyai/go-lua/analysis/engine/factflow/apply":         true,
		"github.com/wippyai/go-lua/analysis/engine/factflow/source":        true,
		"github.com/wippyai/go-lua/analysis/engine/state":                  true,
		"github.com/wippyai/go-lua/analysis/engine/transfer":               true,
		"github.com/wippyai/go-lua/analysis/ir/cfg":                        true,
		"github.com/wippyai/go-lua/analysis/lua/typeaccess":                true,
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
		if dep == "github.com/wippyai/go-lua/analysis/lua/typeaccess" {
			continue
		}
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

func signatureProviderFacts(point cfg.Point, args []factflow.ValueSource) factflow.Facts {
	return factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			point: factflow.NewCallSite(factflow.CallSiteConfig{ArgumentSources: args}),
		},
	})
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
