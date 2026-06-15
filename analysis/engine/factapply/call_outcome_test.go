package factapply

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestFactsNodeTransferCallOutcomeAppliesParamCondition(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(601)
	argExpr := factflow.ExprRef(601)
	arg := symbol.ID(601)
	target := symbol.ID(602)
	other := symbol.ID(603)
	argPath := pathdom.NewPath(arg, "arg")
	targetPath := pathdom.NewPath(target, "target")
	otherPath := pathdom.NewPath(other, "other")
	present := presentValue(reg)
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			point: factflow.NewCallSite(factflow.CallSiteConfig{
				Context: factflow.CallSiteContextStatement,
				ArgumentSources: []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression, ExprRef: argExpr, HasExpr: true},
				},
			}),
		},
		ExpressionPaths: map[factflow.ExprRef]pathdom.Path{
			argExpr: argPath,
		},
		ExpressionConditions: map[factflow.ExprRef]factflow.ExpressionCondition{
			argExpr: factflow.NewExpressionCondition(
				[]factflow.PostconditionRefinement{
					factflow.NewPostconditionRefinement(targetPath, factflow.NewValueConstraint(present)),
				},
				nil,
				[]factflow.PostconditionPathRelation{
					factflow.NewPostconditionPathEquality(targetPath, otherPath),
				},
				nil,
			),
		},
	})

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: facts,
		CallOutcome: func(transfer.NodeContext, factflow.CallSite, state.State, func(cfg.Point) state.State) CallOutcome {
			return CallOutcome{
				ParamConditions: []CallParamCondition{
					{ParamIndex: 0, Value: true},
				},
			}
		},
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{}.
		WriteValue(reg, key.SymbolValue(target), product.Top()).
		WriteValue(reg, key.SymbolValue(other), product.Top()))

	assertValue(t, reg, got, key.SymbolValue(target), present)
	assertValue(t, reg, got, key.SymbolValue(other), present)
}

func TestWithSupplementalCallOutcomeKeepsPrimarySlotsFillsMissingSlotsAndMergesSideFacts(t *testing.T) {
	reg := standard.Registry()
	primaryValue := product.Absent(reg)
	supplementalValue := product.Top()
	primary := func(transfer.NodeContext, factflow.CallSite, state.State, func(cfg.Point) state.State) CallOutcome {
		return CallOutcome{
			Results: []CallResult{{Index: 0, Value: primaryValue}},
			NormalReturnFacts: callboundary.NormalReturnFacts{
				PathRefinements: []callboundary.PathValueFact{
					{Path: pathdom.NewPlaceholder(0), Value: primaryValue},
				},
			},
			ParamConditions: []CallParamCondition{
				{ParamIndex: 0, Value: true},
			},
		}
	}
	supplemental := func(transfer.NodeContext, factflow.CallSite, state.State, func(cfg.Point) state.State) CallOutcome {
		return CallOutcome{
			Results: []CallResult{{Index: 0, Value: product.Top()}, {Index: 1, Value: supplementalValue}},
			NormalReturnFacts: callboundary.NormalReturnFacts{
				PathRefinements: []callboundary.PathValueFact{
					{Path: pathdom.NewPlaceholder(1), Value: supplementalValue},
				},
			},
			ParamConditions: []CallParamCondition{
				{ParamIndex: 1, Value: false},
			},
			ReturnPresenceRelations: []CallReturnPresenceRelation{
				{TriggerIndex: 1, TriggerPresence: presence.Present(), TargetIndex: 0, TargetPresence: presence.Absent()},
			},
		}
	}

	got := WithSupplementalCallOutcome(primary, supplemental)(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{}), state.State{}, nil)

	if len(got.Results) != 2 {
		t.Fatalf("got %d results, want 2: %#v", len(got.Results), got.Results)
	}
	if got.Results[0].Index != 0 || !product.Equal(reg, got.Results[0].Value, primaryValue) {
		t.Fatalf("primary slot = %#v, want index 0 primary value", got.Results[0])
	}
	if got.Results[1].Index != 1 || !product.Equal(reg, got.Results[1].Value, supplementalValue) {
		t.Fatalf("supplemental slot = %#v, want index 1 supplemental value", got.Results[1])
	}
	if len(got.ParamConditions) != 2 ||
		got.ParamConditions[0].ParamIndex != 0 || !got.ParamConditions[0].Value ||
		got.ParamConditions[1].ParamIndex != 1 || got.ParamConditions[1].Value {
		t.Fatalf("param conditions = %#v, want primary and supplemental facts", got.ParamConditions)
	}
	if len(got.NormalReturnFacts.PathRefinements) != 2 ||
		!got.NormalReturnFacts.PathRefinements[0].Path.Equal(pathdom.NewPlaceholder(0)) ||
		!got.NormalReturnFacts.PathRefinements[1].Path.Equal(pathdom.NewPlaceholder(1)) {
		t.Fatalf("normal return facts = %#v, want primary and supplemental path refinements", got.NormalReturnFacts)
	}
	if len(got.ReturnPresenceRelations) != 1 ||
		got.ReturnPresenceRelations[0].TriggerIndex != 1 ||
		!presence.Equal(got.ReturnPresenceRelations[0].TriggerPresence, presence.Present()) ||
		got.ReturnPresenceRelations[0].TargetIndex != 0 ||
		!presence.Equal(got.ReturnPresenceRelations[0].TargetPresence, presence.Absent()) {
		t.Fatalf("return presence relations = %#v, want supplemental relation", got.ReturnPresenceRelations)
	}
}

func TestWithSupplementalCallOutcomePreservesPrimaryNonTypeEvidence(t *testing.T) {
	reg := standard.Registry()
	primaryValue := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	supplementalValue := typeValue(reg, typ.String)
	primary := func(transfer.NodeContext, factflow.CallSite, state.State, func(cfg.Point) state.State) CallOutcome {
		return CallOutcome{Results: []CallResult{{Index: 0, Value: primaryValue}}}
	}
	supplemental := func(transfer.NodeContext, factflow.CallSite, state.State, func(cfg.Point) state.State) CallOutcome {
		return CallOutcome{Results: []CallResult{{Index: 0, Value: supplementalValue}}}
	}

	got := WithSupplementalCallOutcome(primary, supplemental)(
		transfer.NodeContext{Registry: reg},
		factflow.NewCallSite(factflow.CallSiteConfig{}),
		state.State{},
		nil,
	)

	if len(got.Results) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got.Results), got.Results)
	}
	if gotPresence := product.PresenceOf(got.Results[0].Value); !presence.Equal(gotPresence, presence.Present()) {
		t.Fatalf("presence = %s, want present", gotPresence)
	}
	gotType, ok := typevalue.TypeOf(reg, got.Results[0].Value)
	if !ok || !typ.TypeEquals(gotType, typ.String) {
		t.Fatalf("type = %v/%v, want string", gotType, ok)
	}
}

func TestFactsNodeTransferCallOutcomeAppliesParamPathRelation(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(611)
	leftExpr := factflow.ExprRef(611)
	rightExpr := factflow.ExprRef(612)
	left := symbol.ID(611)
	right := symbol.ID(612)
	leftPath := pathdom.NewPath(left, "left")
	rightPath := pathdom.NewPath(right, "right")
	present := presentValue(reg)

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			CallSites: map[cfg.Point]factflow.CallSite{
				point: factflow.NewCallSite(factflow.CallSiteConfig{
					Context: factflow.CallSiteContextStatement,
					ArgumentSources: []factflow.ValueSource{
						{Kind: factflow.ValueSourceExpression, ExprRef: leftExpr, HasExpr: true},
						{Kind: factflow.ValueSourceExpression, ExprRef: rightExpr, HasExpr: true},
					},
				}),
			},
			ExpressionPaths: map[factflow.ExprRef]pathdom.Path{
				leftExpr:  leftPath,
				rightExpr: rightPath,
			},
		}),
		CallOutcome: func(transfer.NodeContext, factflow.CallSite, state.State, func(cfg.Point) state.State) CallOutcome {
			return CallOutcome{
				ParamPathRelations: []CallParamPathRelation{
					{Kind: CallPathRelationEqual, Left: pathdom.NewPlaceholder(0), Right: pathdom.NewPlaceholder(1)},
				},
			}
		},
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{}.
		WriteValue(reg, key.SymbolValue(left), present).
		WriteValue(reg, key.SymbolValue(right), product.Top()))

	assertValue(t, reg, got, key.SymbolValue(left), present)
	assertValue(t, reg, got, key.SymbolValue(right), present)
}

func TestFactsNodeTransferCallOutcomeParamPathRefinementUsesArgumentNotReceiver(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(616)
	receiver := symbol.ID(616)
	arg := symbol.ID(617)
	argExpr := factflow.ExprRef(616)
	receiverPath := pathdom.NewPath(receiver, "receiver")
	argPath := pathdom.NewPath(arg, "arg")
	present := presentValue(reg)

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			CallSites: map[cfg.Point]factflow.CallSite{
				point: factflow.NewCallSite(factflow.CallSiteConfig{
					Context:         factflow.CallSiteContextStatement,
					ReceiverPath:    receiverPath,
					HasReceiverPath: true,
					ArgumentSources: []factflow.ValueSource{
						{Kind: factflow.ValueSourceExpression, ExprRef: argExpr, HasExpr: true},
					},
				}),
			},
			ExpressionPaths: map[factflow.ExprRef]pathdom.Path{
				argExpr: argPath,
			},
		}),
		CallOutcome: func(transfer.NodeContext, factflow.CallSite, state.State, func(cfg.Point) state.State) CallOutcome {
			return CallOutcome{
				ParamPathRefinements: []CallParamPathRefinement{
					{Path: pathdom.NewPlaceholder(0), Value: present},
				},
			}
		},
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{}.
		WriteValue(reg, key.SymbolValue(receiver), product.Top()).
		WriteValue(reg, key.SymbolValue(arg), product.Top()))

	assertValue(t, reg, got, key.SymbolValue(arg), present)
	assertValue(t, reg, got, key.SymbolValue(receiver), product.Top())
}

func TestFactsNodeTransferCallOutcomeRebasesNestedEscapeEventToConsumerChildPath(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(618)
	consumer := symbol.ID(618)
	argExpr := factflow.ExprRef(618)
	rootPath := pathdom.NewPath(consumer, "producer")
	childPath := rootPath.Field("child").Field("leaf")
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			point: factflow.NewCallSite(factflow.CallSiteConfig{
				Context: factflow.CallSiteContextStatement,
				ArgumentSources: []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression, ExprRef: argExpr, HasExpr: true},
				},
			}),
		},
		ExpressionPaths: map[factflow.ExprRef]pathdom.Path{
			argExpr: rootPath,
		},
	})
	builder := visibility.NewBuilder()
	builder.Define(point, consumer, "producer")
	resolver := visibility.NewResolver(builder.Build())

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: facts,
		CallOutcome: func(transfer.NodeContext, factflow.CallSite, state.State, func(cfg.Point) state.State) CallOutcome {
			return CallOutcome{
				NormalReturnFacts: callboundary.NormalReturnFacts{
					EscapeEvents: []callboundary.EscapeEventFact{
						{
							Target:    pathdom.NewPlaceholder(0).Field("child").Field("leaf"),
							Kind:      callboundary.EscapeEventStore,
							Recursive: true,
						},
					},
				},
			}
		},
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	targetKey := resolver.KeyAt(point, childPath)
	if targetKey == "" {
		t.Fatalf("no visible key for %s", childPath)
	}
	gotDelta := got.ReadEffectDelta(effectdelta.Key{
		Target: targetKey,
		Site:   callboundary.EscapeEventEffectSite(callboundary.EscapeEventStore, true),
		Kind:   effectdelta.Escape,
	})
	if !effectdelta.Domain(reg).Equal(gotDelta, effectdelta.Top()) {
		t.Fatalf("escape delta = %#v, want rebased store on %s", gotDelta, childPath)
	}
}

func TestFactsEdgeTransferCallOutcomeAppliesReturnConditionRefinement(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)

	argExpr := factflow.ExprRef(621)
	arg := symbol.ID(621)
	argPath := pathdom.NewPath(arg, "arg")
	present := presentValue(reg)
	absent := absentValue(reg)
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			call: factflow.NewCallSite(factflow.CallSiteConfig{
				Context: factflow.CallSiteContextCondition,
				ArgumentSources: []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression, ExprRef: argExpr, HasExpr: true},
				},
			}),
		},
		ExpressionPaths: map[factflow.ExprRef]pathdom.Path{
			argExpr: argPath,
		},
	})

	flow := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: state.State{}.WriteValue(reg, key.SymbolValue(arg), product.Top()),
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: facts,
			CallOutcome: func(transfer.NodeContext, factflow.CallSite, state.State, func(cfg.Point) state.State) CallOutcome {
				return CallOutcome{
					ReturnConditionRefinements: []CallReturnConditionRefinement{
						{ReturnIndex: 0, ReturnValue: true, Target: pathdom.NewPlaceholder(0), Value: present},
						{ReturnIndex: 1, ReturnValue: true, Target: pathdom.NewPlaceholder(0), Value: absent},
					},
				}
			},
		}),
	})

	assertValue(t, reg, flow[thenPoint], key.SymbolValue(arg), present)
	assertValue(t, reg, flow[elsePoint], key.SymbolValue(arg), product.Top())
}

func TestFactsEdgeTransferCallOutcomeReturnConditionUsesArgumentNotReceiver(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)

	receiver := symbol.ID(626)
	arg := symbol.ID(627)
	argExpr := factflow.ExprRef(626)
	receiverPath := pathdom.NewPath(receiver, "receiver")
	argPath := pathdom.NewPath(arg, "arg")
	present := presentValue(reg)
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			call: factflow.NewCallSite(factflow.CallSiteConfig{
				Context:         factflow.CallSiteContextCondition,
				ReceiverPath:    receiverPath,
				HasReceiverPath: true,
				ArgumentSources: []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression, ExprRef: argExpr, HasExpr: true},
				},
			}),
		},
		ExpressionPaths: map[factflow.ExprRef]pathdom.Path{
			argExpr: argPath,
		},
	})

	flow := transfer.Run(transfer.Config{
		Graph:    graph,
		Registry: reg,
		EntryState: state.State{}.
			WriteValue(reg, key.SymbolValue(receiver), product.Top()).
			WriteValue(reg, key.SymbolValue(arg), product.Top()),
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: facts,
			CallOutcome: func(transfer.NodeContext, factflow.CallSite, state.State, func(cfg.Point) state.State) CallOutcome {
				return CallOutcome{
					ReturnConditionRefinements: []CallReturnConditionRefinement{
						{ReturnIndex: 0, ReturnValue: true, Target: pathdom.NewPlaceholder(0), Value: present},
					},
				}
			},
		}),
	})

	assertValue(t, reg, flow[thenPoint], key.SymbolValue(arg), present)
	assertValue(t, reg, flow[thenPoint], key.SymbolValue(receiver), product.Top())
}

func TestFactsEdgeTransferCallOutcomeAppliesReturnPresenceRelation(t *testing.T) {
	reg := standard.Registry()
	graph, facts, branch, thenPoint, elsePoint, valuePath, value := callOutcomeReturnPresenceGraph(reg, false)
	edgeTransfer := NewFactsEdgeTransfer(FactsEdgeTransferConfig{
		Facts:       facts,
		CallOutcome: callOutcomeReturnPresenceProvider(),
	})
	in := state.State{}.WriteValue(reg, key.SymbolValue(value), product.Top())

	trueOut := edgeTransfer(transfer.EdgeContext{
		Graph:    graph,
		Registry: reg,
		Edge:     cfg.Edge{From: branch, To: thenPoint, Cond: true},
		HasCond:  true,
	}, in)
	falseOut := edgeTransfer(transfer.EdgeContext{
		Graph:    graph,
		Registry: reg,
		Edge:     cfg.Edge{From: branch, To: elsePoint, Cond: false},
		HasCond:  true,
	}, in)

	assertValue(t, reg, trueOut, key.SymbolValue(valuePath.Symbol), absentValue(reg))
	assertValue(t, reg, falseOut, key.SymbolValue(valuePath.Symbol), presentValue(reg))
}

func TestFactsEdgeTransferCallOutcomeReturnPresenceMatchesUnversionedBranchPath(t *testing.T) {
	reg := standard.Registry()
	graph, facts, branch, thenPoint, elsePoint, valuePath, value := callOutcomeReturnPresenceGraph(reg, false, true)
	edgeTransfer := NewFactsEdgeTransfer(FactsEdgeTransferConfig{
		Facts:       facts,
		CallOutcome: callOutcomeReturnPresenceProvider(),
	})
	in := state.State{}.WriteValue(reg, key.SymbolValue(value), product.Top())

	trueOut := edgeTransfer(transfer.EdgeContext{
		Graph:    graph,
		Registry: reg,
		Edge:     cfg.Edge{From: branch, To: thenPoint, Cond: true},
		HasCond:  true,
	}, in)
	falseOut := edgeTransfer(transfer.EdgeContext{
		Graph:    graph,
		Registry: reg,
		Edge:     cfg.Edge{From: branch, To: elsePoint, Cond: false},
		HasCond:  true,
	}, in)

	assertValue(t, reg, trueOut, key.SymbolValue(valuePath.Symbol), absentValue(reg))
	assertValue(t, reg, falseOut, key.SymbolValue(valuePath.Symbol), presentValue(reg))
}

func TestFactsEdgeTransferCallOutcomeReturnPresenceStopsAtReassignment(t *testing.T) {
	reg := standard.Registry()
	graph, facts, branch, thenPoint, _, _, value := callOutcomeReturnPresenceGraph(reg, true)
	edgeTransfer := NewFactsEdgeTransfer(FactsEdgeTransferConfig{
		Facts:       facts,
		CallOutcome: callOutcomeReturnPresenceProvider(),
	})
	in := state.State{}.WriteValue(reg, key.SymbolValue(value), product.Top())

	got := edgeTransfer(transfer.EdgeContext{
		Graph:    graph,
		Registry: reg,
		Edge:     cfg.Edge{From: branch, To: thenPoint, Cond: true},
		HasCond:  true,
	}, in)

	assertValue(t, reg, got, key.SymbolValue(value), product.Top())
}

func TestFactsNodeTransferCallOutcomeReturnPresencePersistsForLaterPathRefinement(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	assignValue := graph.AddNode(cfg.NodeAssign)
	assignErr := graph.AddNode(cfg.NodeAssign)
	refineErr := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, assignValue, false)
	graph.AddEdge(assignValue, assignErr, false)
	graph.AddEdge(assignErr, refineErr, false)
	graph.AddEdge(refineErr, graph.Exit(), false)

	value := symbol.ID(641)
	err := symbol.ID(642)
	valuePath := pathdom.NewPath(value, "value")
	errPath := pathdom.NewPath(err, "err")
	facts := callOutcomePersistentPresenceFacts(reg, call, assignValue, assignErr, refineErr, value, err, valuePath, errPath, false)
	resolver := callOutcomePersistentPresenceResolver(graph, assignValue, assignErr, refineErr, 0, value, err)

	flow := transfer.Run(transfer.Config{
		Graph:    graph,
		Registry: reg,
		NodeTransfer: NewFactsNodeTransfer(FactsNodeTransferConfig{
			Facts:       facts,
			Sources:     sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{Registry: reg}),
			CallOutcome: callOutcomeReturnPresenceProvider(),
			Visibility:  resolver,
		}),
	})

	assertValue(t, reg, flow[graph.Exit()], key.SymbolValue(value), presentValue(reg))
	assertValue(t, reg, flow[graph.Exit()], key.SymbolValue(err), absentValue(reg))
}

func TestFactsNodeTransferCallOutcomeReturnPresenceInvalidatesOnResultPathWrite(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	assignValue := graph.AddNode(cfg.NodeAssign)
	assignErr := graph.AddNode(cfg.NodeAssign)
	reassignErr := graph.AddNode(cfg.NodeAssign)
	refineErr := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, assignValue, false)
	graph.AddEdge(assignValue, assignErr, false)
	graph.AddEdge(assignErr, reassignErr, false)
	graph.AddEdge(reassignErr, refineErr, false)
	graph.AddEdge(refineErr, graph.Exit(), false)

	value := symbol.ID(646)
	err := symbol.ID(647)
	valuePath := pathdom.NewPath(value, "value")
	errPath := pathdom.NewPath(err, "err")
	facts := callOutcomePersistentPresenceFacts(reg, call, assignValue, assignErr, refineErr, value, err, valuePath, errPath, false)
	rootAssignments := map[cfg.Point]factflow.RootAssignment{
		assignValue: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, value, valuePath, factflow.ValueSource{
			Kind:         factflow.ValueSourceCall,
			TargetIndex:  0,
			ResultIndex:  0,
			CallPoint:    call,
			HasCallPoint: true,
		}),
		assignErr: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, err, errPath, factflow.ValueSource{
			Kind:         factflow.ValueSourceCall,
			TargetIndex:  1,
			ResultIndex:  1,
			CallPoint:    call,
			HasCallPoint: true,
		}),
		reassignErr: factflow.NewRootAssignment(factflow.RootAssignmentOrdinaryRootWrite, err, errPath, factflow.ValueSource{
			Kind: factflow.ValueSourceNil,
		}),
	}
	facts = factflow.NewFacts(factflow.FactsInput{
		CallSites:                callOutcomePersistentPresenceCallSites(call, value, err, valuePath, errPath),
		RootAssignments:          rootAssignments,
		PostconditionRefinements: callOutcomePersistentPresenceRefinements(reg, refineErr, errPath),
	})
	resolver := callOutcomePersistentPresenceResolver(graph, assignValue, assignErr, refineErr, reassignErr, value, err)

	flow := transfer.Run(transfer.Config{
		Graph:    graph,
		Registry: reg,
		NodeTransfer: NewFactsNodeTransfer(FactsNodeTransferConfig{
			Facts:       facts,
			Sources:     sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{Registry: reg}),
			CallOutcome: callOutcomeReturnPresenceProvider(),
			Visibility:  resolver,
		}),
	})

	assertValue(t, reg, flow[graph.Exit()], key.SymbolValue(value), product.Top())
	assertValue(t, reg, flow[graph.Exit()], key.SymbolValue(err), absentValue(reg))
}

func TestFactsNodeTransferCallOutcomeReturnPresenceSkipsIrrelevantAssignment(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	assignValue := graph.AddNode(cfg.NodeAssign)
	assignErr := graph.AddNode(cfg.NodeAssign)
	assignOther := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, assignValue, false)
	graph.AddEdge(assignValue, assignErr, false)
	graph.AddEdge(assignErr, assignOther, false)
	graph.AddEdge(assignOther, graph.Exit(), false)

	value := symbol.ID(651)
	err := symbol.ID(652)
	other := symbol.ID(653)
	valuePath := pathdom.NewPath(value, "value")
	errPath := pathdom.NewPath(err, "err")
	otherPath := pathdom.NewPath(other, "other")
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: callOutcomePersistentPresenceCallSites(call, value, err, valuePath, errPath),
		RootAssignments: map[cfg.Point]factflow.RootAssignment{
			assignValue: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, value, valuePath, factflow.ValueSource{
				Kind:         factflow.ValueSourceCall,
				TargetIndex:  0,
				ResultIndex:  0,
				CallPoint:    call,
				HasCallPoint: true,
			}),
			assignErr: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, err, errPath, factflow.ValueSource{
				Kind:         factflow.ValueSourceCall,
				TargetIndex:  1,
				ResultIndex:  1,
				CallPoint:    call,
				HasCallPoint: true,
			}),
			assignOther: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, other, otherPath, factflow.ValueSource{
				Kind: factflow.ValueSourceNil,
			}),
		},
	})

	providerCalls := 0
	transferFn := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts:   facts,
		Sources: sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{Registry: reg}),
		CallOutcome: func(transfer.NodeContext, factflow.CallSite, state.State, func(cfg.Point) state.State) CallOutcome {
			providerCalls++
			return callOutcomeReturnPresenceProvider()(transfer.NodeContext{}, factflow.CallSite{}, state.State{}, nil)
		},
	})
	transferFn(transfer.NodeContext{
		Graph:    graph,
		Registry: reg,
		Point:    assignOther,
		Node:     graph.Node(assignOther),
	}, state.State{})

	if providerCalls != 0 {
		t.Fatalf("call outcome provider called %d times for unrelated assignment, want 0", providerCalls)
	}
}

func callOutcomeReturnPresenceGraph(
	reg *axis.Registry,
	kill bool,
	versionedTargets ...bool,
) (cfg.Graph, factflow.Facts, cfg.Point, cfg.Point, cfg.Point, pathdom.Path, symbol.ID) {
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	assignValue := graph.AddNode(cfg.NodeAssign)
	assignErr := graph.AddNode(cfg.NodeAssign)
	branchPred := assignErr
	if kill {
		branchPred = graph.AddNode(cfg.NodeAssign)
	}
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, assignValue, false)
	graph.AddEdge(assignValue, assignErr, false)
	if kill {
		graph.AddEdge(assignErr, branchPred, false)
	}
	graph.AddEdge(branchPred, branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)

	value := symbol.ID(631)
	err := symbol.ID(632)
	valuePath := pathdom.NewPath(value, "value")
	errPath := pathdom.NewPath(err, "err")
	branchErrPath := errPath
	if len(versionedTargets) != 0 && versionedTargets[0] {
		valuePath.Version = 1
		errPath.Version = 1
	}
	rootAssignments := map[cfg.Point]factflow.RootAssignment{
		assignValue: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, value, valuePath, factflow.ValueSource{
			Kind:         factflow.ValueSourceCall,
			TargetIndex:  0,
			ResultIndex:  0,
			CallPoint:    call,
			HasCallPoint: true,
		}),
		assignErr: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, err, errPath, factflow.ValueSource{
			Kind:         factflow.ValueSourceCall,
			TargetIndex:  1,
			ResultIndex:  1,
			CallPoint:    call,
			HasCallPoint: true,
		}),
	}
	if kill {
		rootAssignments[branchPred] = factflow.NewRootAssignment(
			factflow.RootAssignmentOrdinaryRootWrite,
			err,
			errPath,
			factflow.ValueSource{Kind: factflow.ValueSourceNil},
		)
	}
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			call: factflow.NewCallSite(factflow.CallSiteConfig{
				Context: factflow.CallSiteContextAssignmentSource,
				ResultTargets: []factflow.CallResultTarget{
					factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, 0, 0, value, valuePath),
					factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, 1, 1, err, errPath),
				},
			}),
		},
		RootAssignments: rootAssignments,
		BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
			branch: factflow.NewBranchRefinementSet(
				factflow.NewBranchRefinement(
					branchErrPath,
					factflow.NewValueConstraint(product.NewWithPresence(reg, product.ShapeTop, presence.Present())), true,
					factflow.NewValueConstraint(product.NewWithPresence(reg, product.ShapeTop, presence.Absent())), true,
				),
			),
		},
	})
	return graph, facts, branch, thenPoint, elsePoint, valuePath, value
}

func callOutcomePersistentPresenceFacts(
	reg *axis.Registry,
	call cfg.Point,
	assignValue cfg.Point,
	assignErr cfg.Point,
	refineErr cfg.Point,
	value symbol.ID,
	err symbol.ID,
	valuePath pathdom.Path,
	errPath pathdom.Path,
	_ bool,
) factflow.Facts {
	return factflow.NewFacts(factflow.FactsInput{
		CallSites: callOutcomePersistentPresenceCallSites(call, value, err, valuePath, errPath),
		RootAssignments: map[cfg.Point]factflow.RootAssignment{
			assignValue: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, value, valuePath, factflow.ValueSource{
				Kind:         factflow.ValueSourceCall,
				TargetIndex:  0,
				ResultIndex:  0,
				CallPoint:    call,
				HasCallPoint: true,
			}),
			assignErr: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, err, errPath, factflow.ValueSource{
				Kind:         factflow.ValueSourceCall,
				TargetIndex:  1,
				ResultIndex:  1,
				CallPoint:    call,
				HasCallPoint: true,
			}),
		},
		PostconditionRefinements: callOutcomePersistentPresenceRefinements(reg, refineErr, errPath),
	})
}

func callOutcomePersistentPresenceCallSites(
	call cfg.Point,
	value symbol.ID,
	err symbol.ID,
	valuePath pathdom.Path,
	errPath pathdom.Path,
) map[cfg.Point]factflow.CallSite {
	return map[cfg.Point]factflow.CallSite{
		call: factflow.NewCallSite(factflow.CallSiteConfig{
			Context: factflow.CallSiteContextAssignmentSource,
			ResultTargets: []factflow.CallResultTarget{
				factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, 0, 0, value, valuePath),
				factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, 1, 1, err, errPath),
			},
		}),
	}
}

func callOutcomePersistentPresenceRefinements(
	reg *axis.Registry,
	refineErr cfg.Point,
	errPath pathdom.Path,
) map[cfg.Point]factflow.PostconditionRefinementSet {
	return map[cfg.Point]factflow.PostconditionRefinementSet{
		refineErr: factflow.NewPostconditionRefinementSet(
			factflow.NewPostconditionRefinement(errPath, factflow.NewValueConstraint(absentValue(reg))),
		),
	}
}

func callOutcomePersistentPresenceResolver(
	graph cfg.Graph,
	assignValue cfg.Point,
	assignErr cfg.Point,
	refineErr cfg.Point,
	reassignErr cfg.Point,
	value symbol.ID,
	err symbol.ID,
) *visibility.Resolver {
	builder := visibility.NewBuilder()
	valueVersion := builder.Define(assignValue, value, "value")
	errVersion := builder.Define(assignErr, err, "err")
	for _, point := range graph.RPO() {
		if point != graph.Entry() && point != assignValue {
			builder.SetVisible(point, value, valueVersion)
		}
		if point != graph.Entry() && point != assignErr {
			builder.SetVisible(point, err, errVersion)
		}
	}
	if reassignErr != 0 {
		builder.SetVisible(reassignErr, err, errVersion)
		builder.SetVisible(refineErr, err, errVersion)
		builder.SetVisible(graph.Exit(), err, errVersion)
	}
	return visibility.NewResolver(builder.Build())
}

func callOutcomeReturnPresenceProvider() CallOutcomeProvider {
	return func(transfer.NodeContext, factflow.CallSite, state.State, func(cfg.Point) state.State) CallOutcome {
		return CallOutcome{
			Results: []CallResult{
				{Index: 0, Value: product.Top()},
				{Index: 1, Value: product.Top()},
			},
			ReturnPresenceRelations: []CallReturnPresenceRelation{
				{
					TriggerIndex:    1,
					TriggerPresence: presence.Present(),
					TargetIndex:     0,
					TargetPresence:  presence.Absent(),
				},
				{
					TriggerIndex:    1,
					TriggerPresence: presence.Absent(),
					TargetIndex:     0,
					TargetPresence:  presence.Present(),
				},
			},
		}
	}
}
