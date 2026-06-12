package factapply

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/standard"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
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

func callOutcomeReturnPresenceGraph(
	reg *axis.Registry,
	kill bool,
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
					errPath,
					factflow.NewValueConstraint(product.NewWithPresence(reg, product.ShapeTop, presence.Present())), true,
					factflow.NewValueConstraint(product.NewWithPresence(reg, product.ShapeTop, presence.Absent())), true,
				),
			),
		},
	})
	return graph, facts, branch, thenPoint, elsePoint, valuePath, value
}

func callOutcomeReturnPresenceProvider() CallOutcomeProvider {
	return func(transfer.NodeContext, factflow.CallSite, state.State, func(cfg.Point) state.State) CallOutcome {
		return CallOutcome{
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
