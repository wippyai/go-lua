package factapply

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/standard"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestFactsNodeTransferAppliesReturnSlotsThroughSourceValues(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	ret := graph.AddNode(cfg.NodeReturn)
	graph.AddEdge(graph.Entry(), ret, false)
	graph.AddEdge(ret, graph.Exit(), false)

	expr := factflow.ExprRef(20)
	exprValue := presentValue(reg)
	sources := sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[factflow.ExprRef]product.Value{
			expr: exprValue,
		},
	})

	got := transfer.Run(transfer.Config{
		Graph:    graph,
		Registry: reg,
		NodeTransfer: NewFactsNodeTransfer(FactsNodeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				Returns: map[cfg.Point]factflow.Return{
					ret: factflow.NewReturn([]factflow.ValueSource{
						{Kind: factflow.ValueSourceExpression, ExprRef: expr, HasExpr: true},
						{Kind: factflow.ValueSourceNil},
					}),
				},
			}),
			Sources: sources,
		}),
	})

	assertValue(t, reg, got[ret], key.ReturnSlot(0), product.Bottom(reg))
	assertValue(t, reg, got[graph.Exit()], key.ReturnSlot(0), exprValue)
	assertValue(t, reg, got[graph.Exit()], key.ReturnSlot(1), absentValue(reg))
}

func TestFactsNodeTransferUnresolvedReturnSourceLeavesSlotUnchanged(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(21)
	slotValue := presentValue(reg)
	in := state.State{}.WriteReturnSlot(reg, 0, slotValue)
	sources := sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{Registry: reg})

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			Returns: map[cfg.Point]factflow.Return{
				point: factflow.NewReturn([]factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(21), HasExpr: true},
				}),
			},
		}),
		Sources: sources,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, in)

	assertValue(t, reg, got, key.ReturnSlot(0), slotValue)
}

func TestFactsNodeTransferReturnCallSourceReadsReturnSlotThroughRead(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	ret := graph.AddNode(cfg.NodeReturn)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, ret, false)
	graph.AddEdge(ret, graph.Exit(), false)

	callValue := presentValue(reg)
	sources := sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{Registry: reg})

	got := transfer.Run(transfer.Config{
		Graph:    graph,
		Registry: reg,
		Initial: func(point cfg.Point) (state.State, bool) {
			if point == call {
				return state.State{}.WriteReturnSlot(reg, 2, callValue), true
			}
			return state.State{}, false
		},
		NodeTransfer: NewFactsNodeTransfer(FactsNodeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				Returns: map[cfg.Point]factflow.Return{
					ret: factflow.NewReturn([]factflow.ValueSource{
						{
							Kind:         factflow.ValueSourceCall,
							CallPoint:    call,
							HasCallPoint: true,
							ResultIndex:  2,
						},
					}),
				},
			}),
			Sources: sources,
		}),
	})

	assertValue(t, reg, got[graph.Exit()], key.ReturnSlot(0), callValue)
}

func TestFactsNodeTransferCallProducerProviderWritesReturnSlots(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(22)
	target := symbol.ID(111)
	in := state.State{}.WriteValue(reg, key.SymbolValue(target), presentValue(reg))
	first := presentValue(reg)
	third := absentValue(reg)

	var providerCalled bool
	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			CallSites: map[cfg.Point]factflow.CallSite{
				point: factflow.NewCallSite(factflow.CallSiteConfig{
					Context:      factflow.CallSiteContextAssignmentSource,
					CalleeSymbol: symbol.ID(201),
				}),
			},
		}),
		CallResults: func(ctx transfer.NodeContext, call factflow.CallProducer, gotIn state.State, read func(cfg.Point) state.State) []CallResult {
			providerCalled = true
			if ctx.Point != point {
				t.Fatalf("provider point = %d, want %d", ctx.Point, point)
			}
			if call.CalleeSymbol() != symbol.ID(201) {
				t.Fatalf("provider call = %#v", call)
			}
			assertStateEqual(t, reg, gotIn, in)
			assertValue(t, reg, read(point), key.SymbolValue(target), presentValue(reg))
			return []CallResult{
				{Index: 0, Value: first},
				{Index: 2, Value: third},
				{Index: -1, Value: product.Top()},
			}
		},
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, in)

	if !providerCalled {
		t.Fatal("call result provider was not called")
	}
	assertValue(t, reg, got, key.ReturnSlot(0), first)
	assertValue(t, reg, got, key.ReturnSlot(1), product.Bottom(reg))
	assertValue(t, reg, got, key.ReturnSlot(2), third)
	assertValue(t, reg, got, key.SymbolValue(target), presentValue(reg))
}

func TestFactsNodeTransferAssignmentCallSourceConsumesProviderReturnSlotThroughRead(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	assign := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, assign, false)
	graph.AddEdge(assign, graph.Exit(), false)

	target := symbol.ID(112)
	callValue := presentValue(reg)
	sources := sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{Registry: reg})

	got := transfer.Run(transfer.Config{
		Graph:    graph,
		Registry: reg,
		NodeTransfer: NewFactsNodeTransfer(FactsNodeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				CallSites: map[cfg.Point]factflow.CallSite{
					call: factflow.NewCallSite(factflow.CallSiteConfig{
						Context: factflow.CallSiteContextAssignmentSource,
					}),
				},
				RootAssignments: map[cfg.Point]factflow.RootAssignment{
					assign: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, target, path.NewPath(target, "local"), factflow.ValueSource{
						Kind:         factflow.ValueSourceCall,
						CallPoint:    call,
						HasCallPoint: true,
						ResultIndex:  0,
					}),
				},
			}),
			Sources: sources,
			CallResults: func(ctx transfer.NodeContext, call factflow.CallProducer, in state.State, read func(cfg.Point) state.State) []CallResult {
				return []CallResult{{Index: 0, Value: callValue}}
			},
		}),
		EdgeTransfer: func(ctx transfer.EdgeContext, out state.State) state.State {
			if ctx.Edge.From == call && ctx.Edge.To == assign {
				return out.WriteReturnSlot(reg, 0, product.Bottom(reg))
			}
			return out
		},
	})

	assertValue(t, reg, got[assign], key.ReturnSlot(0), product.Bottom(reg))
	assertValue(t, reg, got[graph.Exit()], key.SymbolValue(target), callValue)
}

func TestFactsNodeTransferCallResultTargetsDoNotDirectlyWriteTargets(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(23)
	target := symbol.ID(113)
	targetPath := path.NewPath(target, "table").Field("field")
	pathKey := path.PathKey("sym113@1.field")
	symbolValue := presentValue(reg)
	pathValue := presentValue(reg)
	resultValue := absentValue(reg)
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(target), symbolValue).
		WritePathKey(reg, pathKey, pathValue)

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			CallSites: map[cfg.Point]factflow.CallSite{
				point: factflow.NewCallSite(factflow.CallSiteConfig{
					Context: factflow.CallSiteContextAssignmentSource,
					ResultTargets: []factflow.CallResultTarget{
						factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, 0, 0, target, path.NewPath(target, "local")),
						factflow.NewCallResultTarget(factflow.CallResultTargetOrdinaryAssignment, 0, 0, target, targetPath),
					},
				}),
			},
		}),
		CallResults: func(ctx transfer.NodeContext, call factflow.CallProducer, in state.State, read func(cfg.Point) state.State) []CallResult {
			return []CallResult{{Index: 0, Value: resultValue}}
		},
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, in)

	assertValue(t, reg, got, key.ReturnSlot(0), resultValue)
	assertValue(t, reg, got, key.SymbolValue(target), symbolValue)
	assertPathValue(t, reg, got, pathKey, pathValue)
}

func TestFactsNodeTransferMissingCallResultProviderOrNoResultsLeavesStateUnchanged(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(24)
	target := symbol.ID(114)
	in := state.State{}.
		WriteReturnSlot(reg, 0, presentValue(reg)).
		WriteValue(reg, key.SymbolValue(target), absentValue(reg))

	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			point: factflow.NewCallSite(factflow.CallSiteConfig{
				Context: factflow.CallSiteContextAssignmentSource,
			}),
		},
	})
	tests := []struct {
		name     string
		provider CallResultProvider
	}{
		{name: "nil provider"},
		{
			name: "nil results",
			provider: func(ctx transfer.NodeContext, call factflow.CallProducer, in state.State, read func(cfg.Point) state.State) []CallResult {
				return nil
			},
		},
		{
			name: "empty results",
			provider: func(ctx transfer.NodeContext, call factflow.CallProducer, in state.State, read func(cfg.Point) state.State) []CallResult {
				return []CallResult{}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := NewFactsNodeTransfer(FactsNodeTransferConfig{
				Facts:       facts,
				CallResults: tc.provider,
			})(transfer.NodeContext{
				Registry: reg,
				Point:    point,
			}, in)

			assertStateEqual(t, reg, got, in)
		})
	}
}
