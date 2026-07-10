package factapply

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
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
						factflow.NewNilValueSource(1),
					}),
				},
			}),
			Sources: sources,
		}),
	})

	assertValue(t, reg, got[ret], key.ReturnSlot(0), product.Bottom(reg))
	assertValue(t, reg, got[graph.Exit()], key.ReturnSlot(0), exprValue)
	assertValue(t, reg, got[graph.Exit()], key.ReturnSlot(1), nilSourceValue(reg))
}

func TestFactsNodeTransferReturnPathSourcePreservesStateIdentity(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	ret := graph.AddNode(cfg.NodeReturn)
	graph.AddEdge(graph.Entry(), ret, false)
	graph.AddEdge(ret, graph.Exit(), false)

	expr := factflow.ExprRef(22)
	batch := symbol.ID(700)
	batchID := identity.ID{Kind: "test.table", Site: "return", Index: 1}
	batchValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(batchID))
	sources := sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{Registry: reg})

	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: state.State{}.WriteValue(reg, key.SymbolValue(batch), batchValue),
		NodeTransfer: NewFactsNodeTransfer(FactsNodeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				Returns: map[cfg.Point]factflow.Return{
					ret: factflow.NewReturn([]factflow.ValueSource{
						{Kind: factflow.ValueSourceExpression, ExprRef: expr, HasExpr: true},
					}),
				},
				ExpressionPaths: map[factflow.ExprRef]path.Path{
					expr: {Symbol: batch},
				},
			}),
			Sources: sources,
		}),
	})

	assertValue(t, reg, got[graph.Exit()], key.ReturnSlot(0), batchValue)
}

func TestFactsNodeTransferReturnPathSourceUsesExpressionDeclaredContract(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	ret := graph.AddNode(cfg.NodeReturn)
	graph.AddEdge(graph.Entry(), ret, false)
	graph.AddEdge(ret, graph.Exit(), false)

	inner := factflow.ExprRef(31)
	outer := factflow.ExprRef(32)
	value := symbol.ID(710)
	declared := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)
	declared = product.Set(reg, declared, assertion.Key, assertion.Of(assertion.TypeClaim, assertion.RuntimeClaim))
	sources := sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[factflow.ExprRef]product.Value{
			inner: product.Top(),
		},
	})

	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: state.State{}.WriteValue(reg, key.SymbolValue(value), product.Top()),
		NodeTransfer: NewFactsNodeTransfer(FactsNodeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				Returns: map[cfg.Point]factflow.Return{
					ret: factflow.NewReturn([]factflow.ValueSource{
						{Kind: factflow.ValueSourceExpression, ExprRef: outer, HasExpr: true},
					}),
				},
				ExpressionPaths: map[factflow.ExprRef]path.Path{
					outer: {Symbol: value},
				},
				ExpressionRefinements: map[factflow.ExprRef]factflow.ExpressionRefinement{
					outer: factflow.NewExpressionDeclaredContract(
						factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: inner, HasExpr: true},
						declared,
					),
				},
			}),
			Sources: sources,
		}),
	})

	slot := got[graph.Exit()].ReadReturnSlot(reg, 0)
	if gotType, ok := typevalue.TypeOf(reg, slot); !ok || !typ.TypeEquals(gotType, typ.String) {
		t.Fatalf("return slot type = %v/%v, want string", gotType, ok)
	}
}

func TestFactsNodeTransferReturnDeclaredPathSourceBlocksHeapContainerReprojection(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	ret := graph.AddNode(cfg.NodeReturn)
	graph.AddEdge(graph.Entry(), ret, false)
	graph.AddEdge(ret, graph.Exit(), false)

	inner := factflow.ExprRef(121)
	outer := factflow.ExprRef(122)
	value := symbol.ID(711)
	tableID := identity.ID{Kind: "test.table", Site: "return-declared", Index: 1}
	declaredType := typ.NewArray(typ.Any)
	declared := typevalue.WithWitness(reg, typevalue.FromType(reg, declaredType), declaredType)
	recordType := typetable.NewRecord().Field("id", typ.String).Build()
	tableValue := product.Set(reg, typevalue.FromType(reg, declaredType), identity.Key, identity.Singleton(tableID))
	sources := sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[factflow.ExprRef]product.Value{
			inner: tableValue,
		},
	})
	heap := heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root: tableValue,
		DynamicIndexFacts: map[dynamicindex.Key]dynamicindex.Fact{
			{Table: keyspace.Key{Kind: keyspace.KindNamed, Root: 1}, Site: "insert"}: dynamicindex.NewFact(reg, dynamicindex.FactConfig{
				KeyValue:    typevalue.WithWitness(reg, typevalue.FromType(reg, typ.Integer), typ.Integer),
				HasKeyValue: true,
				Value:       typevalue.WithWitness(reg, typevalue.FromType(reg, recordType), recordType),
				HasValue:    true,
				Admission:   dynamicindex.AdmissionAdmitted,
			}),
		},
	})

	got := transfer.Run(transfer.Config{
		Graph:    graph,
		Registry: reg,
		EntryState: state.State{}.
			WriteValue(reg, key.SymbolValue(value), tableValue).
			WriteHeapTableObject(reg, tableID, heap),
		NodeTransfer: NewFactsNodeTransfer(FactsNodeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				Returns: map[cfg.Point]factflow.Return{
					ret: factflow.NewReturn([]factflow.ValueSource{
						{Kind: factflow.ValueSourceExpression, ExprRef: outer, HasExpr: true},
					}),
				},
				ExpressionPaths: map[factflow.ExprRef]path.Path{
					outer: {Symbol: value},
				},
				ExpressionRefinements: map[factflow.ExprRef]factflow.ExpressionRefinement{
					outer: factflow.NewExpressionDeclaredContract(
						factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: inner, HasExpr: true},
						declared,
					),
				},
			}),
			Sources: sources,
		}),
	})

	slot := got[graph.Exit()].ReadReturnSlot(reg, 0)
	gotType, ok := typevalue.TypeOf(reg, slot)
	if !ok || !typ.TypeEquals(gotType, declaredType) {
		t.Fatalf("return slot type = %v/%v, want declared %v instead of heap-projected %v[]", gotType, ok, declaredType, recordType)
	}
}

func TestFactsNodeTransferUnresolvedReturnSourceWritesTopInsteadOfStaleSlot(t *testing.T) {
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

	assertValue(t, reg, got, key.ReturnSlot(0), product.Top())
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

func TestFactsNodeTransferCallOutcomeProviderWritesReturnSlots(t *testing.T) {
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
		CallOutcome: func(ctx transfer.NodeContext, site factflow.CallSiteView, gotIn state.State, read func(cfg.Point) state.State) callpayload.CallOutcome {
			providerCalled = true
			if ctx.Point != point {
				t.Fatalf("provider point = %d, want %d", ctx.Point, point)
			}
			if site.CalleeSymbol() != symbol.ID(201) {
				t.Fatalf("provider site = %#v", site)
			}
			assertStateEqual(t, reg, gotIn, in)
			assertValue(t, reg, read(point), key.SymbolValue(target), presentValue(reg))
			return callpayload.CallOutcome{
				Results: []callpayload.CallResult{
					{Index: 0, Value: first},
					{Index: 2, Value: third},
					{Index: -1, Value: product.Top()},
				},
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

func TestFactsNodeTransferReadMemoizesDistinctCallResultsWithinInvocation(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	firstCall := graph.AddNode(cfg.NodeCall)
	secondCall := graph.AddNode(cfg.NodeCall)
	consumer := graph.AddNode(cfg.NodeCall)
	firstValue := presentValue(reg)
	secondValue := absentValue(reg)
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			firstCall:  factflow.NewCallSite(factflow.CallSiteConfig{Context: factflow.CallSiteContextAssignmentSource}),
			secondCall: factflow.NewCallSite(factflow.CallSiteConfig{Context: factflow.CallSiteContextAssignmentSource}),
			consumer:   factflow.NewCallSite(factflow.CallSiteConfig{Context: factflow.CallSiteContextAssignmentSource}),
		},
	})
	callCounts := map[cfg.Point]int{}

	nodeTransfer := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: facts,
		CallOutcome: func(ctx transfer.NodeContext, site factflow.CallSiteView, in state.State, read func(cfg.Point) state.State) callpayload.CallOutcome {
			callCounts[ctx.Point]++
			switch ctx.Point {
			case firstCall:
				return callpayload.CallOutcome{Results: []callpayload.CallResult{{Index: 0, Value: firstValue}}}
			case secondCall:
				return callpayload.CallOutcome{Results: []callpayload.CallResult{{Index: 0, Value: secondValue}}}
			case consumer:
				assertValue(t, reg, read(firstCall), key.ReturnSlot(0), firstValue)
				assertValue(t, reg, read(secondCall), key.ReturnSlot(0), secondValue)
				assertValue(t, reg, read(firstCall), key.ReturnSlot(0), firstValue)
			}
			return callpayload.CallOutcome{}
		},
	})
	nodeTransfer(transfer.NodeContext{Graph: graph, Registry: reg, Point: consumer, Node: graph.Node(consumer)}, state.State{})

	if callCounts[firstCall] != 1 || callCounts[secondCall] != 1 {
		t.Fatalf("provider calls = first:%d second:%d, want one materialization each", callCounts[firstCall], callCounts[secondCall])
	}
}

func TestFactsNodeTransferReadCycleReturnsActiveBaseState(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	firstCall := graph.AddNode(cfg.NodeCall)
	secondCall := graph.AddNode(cfg.NodeCall)
	baseValue := product.Top()
	firstValue := presentValue(reg)
	secondValue := absentValue(reg)
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			firstCall:  factflow.NewCallSite(factflow.CallSiteConfig{Context: factflow.CallSiteContextAssignmentSource}),
			secondCall: factflow.NewCallSite(factflow.CallSiteConfig{Context: factflow.CallSiteContextAssignmentSource}),
		},
	})
	callCounts := map[cfg.Point]int{}

	nodeTransfer := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: facts,
		CallOutcome: func(ctx transfer.NodeContext, site factflow.CallSiteView, in state.State, read func(cfg.Point) state.State) callpayload.CallOutcome {
			callCounts[ctx.Point]++
			switch ctx.Point {
			case firstCall:
				assertValue(t, reg, read(secondCall), key.ReturnSlot(0), secondValue)
				return callpayload.CallOutcome{Results: []callpayload.CallResult{{Index: 0, Value: firstValue}}}
			case secondCall:
				assertValue(t, reg, read(firstCall), key.ReturnSlot(0), baseValue)
				return callpayload.CallOutcome{Results: []callpayload.CallResult{{Index: 0, Value: secondValue}}}
			default:
				return callpayload.CallOutcome{}
			}
		},
	})
	in := state.State{}.WriteReturnSlot(reg, 0, baseValue)
	got := nodeTransfer(transfer.NodeContext{Graph: graph, Registry: reg, Point: firstCall, Node: graph.Node(firstCall)}, in)

	assertValue(t, reg, got, key.ReturnSlot(0), firstValue)
	if callCounts[firstCall] != 1 || callCounts[secondCall] != 1 {
		t.Fatalf("provider calls = first:%d second:%d, want one materialization each", callCounts[firstCall], callCounts[secondCall])
	}
}

func TestFactsNodeTransferSamePointCallSourceReadsMaterializedOut(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	point := graph.AddNode(cfg.NodeCall)
	target := symbol.ID(1120)
	callValue := presentValue(reg)
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			point: factflow.NewCallSite(factflow.CallSiteConfig{Context: factflow.CallSiteContextAssignmentSource}),
		},
		RootAssignments: map[cfg.Point]factflow.RootAssignment{
			point: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, target, path.NewPath(target, "samePoint"), factflow.ValueSource{
				Kind:         factflow.ValueSourceCall,
				CallPoint:    point,
				HasCallPoint: true,
				ResultIndex:  0,
			}),
		},
	})

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts:   facts,
		Sources: sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{Registry: reg}),
		CallOutcome: func(ctx transfer.NodeContext, site factflow.CallSiteView, in state.State, read func(cfg.Point) state.State) callpayload.CallOutcome {
			return callpayload.CallOutcome{Results: []callpayload.CallResult{{Index: 0, Value: callValue}}}
		},
	})(transfer.NodeContext{Graph: graph, Registry: reg, Point: point, Node: graph.Node(point)}, state.State{})

	assertValue(t, reg, got, key.SymbolValue(target), callValue)
}

func TestFactsNodeTransferSamePointCallSourceReadsFixedResultFact(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	point := graph.AddNode(cfg.NodeCall)
	target := symbol.ID(1121)
	callValue := typevalue.FromType(reg, typ.String)
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			point: factflow.NewCallSite(factflow.CallSiteConfig{
				Context: factflow.CallSiteContextAssignmentSource,
				ResultTargets: []factflow.CallResultTarget{
					factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, 0, 0, target, path.NewPath(target, "samePoint")),
				},
			}),
		},
		CallResultValues: map[cfg.Point]factflow.CallResultValueSet{
			point: factflow.NewCallResultValueSet(factflow.NewCallResultValue(0, callValue)),
		},
		RootAssignments: map[cfg.Point]factflow.RootAssignment{
			point: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, target, path.NewPath(target, "samePoint"), factflow.ValueSource{
				Kind:         factflow.ValueSourceCall,
				CallPoint:    point,
				HasCallPoint: true,
				ResultIndex:  0,
			}),
		},
	})

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts:   facts,
		Sources: sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{Registry: reg}),
		CallOutcome: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
			return callpayload.CallOutcome{}
		},
	})(transfer.NodeContext{Graph: graph, Registry: reg, Point: point, Node: graph.Node(point)}, state.State{})

	assertValue(t, reg, got, key.ReturnSlot(0), callValue)
	assertValue(t, reg, got, key.SymbolValue(target), callValue)
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
			CallOutcome: func(ctx transfer.NodeContext, site factflow.CallSiteView, in state.State, read func(cfg.Point) state.State) callpayload.CallOutcome {
				return callpayload.CallOutcome{Results: []callpayload.CallResult{{Index: 0, Value: callValue}}}
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

func TestFactsNodeTransferAssignmentCallSourceConsumesFixedResultFactThroughRead(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	assign := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, assign, false)
	graph.AddEdge(assign, graph.Exit(), false)

	target := symbol.ID(1122)
	callValue := typevalue.FromType(reg, typ.String)
	sources := sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{Registry: reg})

	got := transfer.Run(transfer.Config{
		Graph:    graph,
		Registry: reg,
		NodeTransfer: NewFactsNodeTransfer(FactsNodeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				CallSites: map[cfg.Point]factflow.CallSite{
					call: factflow.NewCallSite(factflow.CallSiteConfig{
						Context: factflow.CallSiteContextAssignmentSource,
						ResultTargets: []factflow.CallResultTarget{
							factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, 0, 0, target, path.NewPath(target, "local")),
						},
					}),
				},
				CallResultValues: map[cfg.Point]factflow.CallResultValueSet{
					call: factflow.NewCallResultValueSet(factflow.NewCallResultValue(0, callValue)),
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
			CallOutcome: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
				return callpayload.CallOutcome{}
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
	ks := keyspace.New()
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(target), symbolValue).
		WritePathKey(reg, ks, pathKey, pathValue)

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
		CallOutcome: func(ctx transfer.NodeContext, site factflow.CallSiteView, in state.State, read func(cfg.Point) state.State) callpayload.CallOutcome {
			return callpayload.CallOutcome{Results: []callpayload.CallResult{{Index: 0, Value: resultValue}}}
		},
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, in)

	assertValue(t, reg, got, key.ReturnSlot(0), resultValue)
	assertValue(t, reg, got, key.SymbolValue(target), symbolValue)
	assertPathValue(t, reg, ks, got, pathKey, pathValue)
}

func TestFactsNodeTransferCallProducerClearsStaleReturnSlotsWithoutOutcome(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(25)
	target := symbol.ID(115)
	stale := presentValue(reg)
	in := state.State{}.
		WriteReturnSlot(reg, 0, stale).
		WriteValue(reg, key.SymbolValue(target), absentValue(reg))

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			CallSites: map[cfg.Point]factflow.CallSite{
				point: factflow.NewCallSite(factflow.CallSiteConfig{
					Context: factflow.CallSiteContextAssignmentSource,
					ResultTargets: []factflow.CallResultTarget{
						factflow.NewCallResultTarget(
							factflow.CallResultTargetLocalAssignment,
							0,
							0,
							target,
							path.NewPath(target, "out"),
						),
					},
				}),
			},
		}),
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, in)

	assertValue(t, reg, got, key.ReturnSlot(0), product.Bottom(reg))
	assertValue(t, reg, got, key.SymbolValue(target), absentValue(reg))
}

func TestFactsNodeTransferExpressionProducerClearsSlotZero(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(26)
	stale := presentValue(reg)
	in := state.State{}.WriteReturnSlot(reg, 0, stale)

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			CallSites: map[cfg.Point]factflow.CallSite{
				point: factflow.NewCallSite(factflow.CallSiteConfig{
					Context: factflow.CallSiteContextExpressionProducer,
					ResultTargets: []factflow.CallResultTarget{
						factflow.NewCallResultTarget(
							factflow.CallResultTargetExpression,
							factflow.NoValueSourceIndex,
							0,
							0,
							path.Path{},
						),
					},
				}),
			},
		}),
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, in)

	assertValue(t, reg, got, key.ReturnSlot(0), product.Bottom(reg))
}

func TestFactsNodeTransferMissingCallOutcomeProviderOrNoResultsLeavesStateUnchanged(t *testing.T) {
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
		provider callpayload.CallOutcomeProvider
	}{
		{name: "nil provider"},
		{
			name: "nil results",
			provider: func(ctx transfer.NodeContext, site factflow.CallSiteView, in state.State, read func(cfg.Point) state.State) callpayload.CallOutcome {
				return callpayload.CallOutcome{}
			},
		},
		{
			name: "empty results",
			provider: func(ctx transfer.NodeContext, site factflow.CallSiteView, in state.State, read func(cfg.Point) state.State) callpayload.CallOutcome {
				return callpayload.CallOutcome{Results: []callpayload.CallResult{}}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := NewFactsNodeTransfer(FactsNodeTransferConfig{
				Facts:       facts,
				CallOutcome: tc.provider,
			})(transfer.NodeContext{
				Registry: reg,
				Point:    point,
			}, in)

			assertStateEqual(t, reg, got, in)
		})
	}
}

func BenchmarkFactsNodeTransferNoMaterializationFacts(b *testing.B) {
	reg := standard.Registry()
	point := cfg.Point(27)
	target := symbol.ID(127)
	value := presentValue(reg)
	in := state.State{}.WriteValue(reg, key.SymbolValue(target), value)
	ctx := transfer.NodeContext{Registry: reg, Point: point}
	nodeTransfer := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{}),
	})

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		got := nodeTransfer(ctx, in)
		if !product.Equal(reg, got.ReadValue(reg, key.SymbolValue(target)), value) {
			b.Fatalf("value changed on iteration %d", i)
		}
	}
}
