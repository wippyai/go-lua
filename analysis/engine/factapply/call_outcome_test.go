package factapply

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
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
		CallOutcome: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
			return callpayload.CallOutcome{
				ParamConditions: []callpayload.CallParamCondition{
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
		CallOutcome: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
			return callpayload.CallOutcome{
				ParamPathRelations: []callpayload.CallParamPathRelation{
					{Kind: callpayload.CallPathRelationEqual, Left: pathdom.NewPlaceholder(0), Right: pathdom.NewPlaceholder(1)},
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

func TestFactsNodeTransferCallOutcomeAppliesStoreRelationEvidence(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(614)
	sourceExpr := factflow.ExprRef(614)
	intoExpr := factflow.ExprRef(615)
	source := symbol.ID(614)
	into := symbol.ID(615)
	sourcePath := pathdom.NewPath(source, "source").Field("stored")
	intoPath := pathdom.NewPath(into, "into").Field("container")
	builder := visibility.NewBuilder()
	builder.Define(point, source, "source")
	builder.Define(point, into, "into")
	resolver := visibility.NewResolver(builder.Build())

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			CallSites: map[cfg.Point]factflow.CallSite{
				point: factflow.NewCallSite(factflow.CallSiteConfig{
					Context: factflow.CallSiteContextStatement,
					ArgumentSources: []factflow.ValueSource{
						{Kind: factflow.ValueSourceExpression, ExprRef: sourceExpr, HasExpr: true},
						{Kind: factflow.ValueSourceExpression, ExprRef: intoExpr, HasExpr: true},
					},
				}),
			},
			ExpressionPaths: map[factflow.ExprRef]pathdom.Path{
				sourceExpr: pathdom.NewPath(source, "source"),
				intoExpr:   pathdom.NewPath(into, "into"),
			},
		}),
		CallOutcome: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
			return callpayload.CallOutcome{
				NormalReturnFacts: callboundary.NormalReturnFacts{
					StoreRelations: []callboundary.StoreRelationFact{{
						Source: pathdom.NewPlaceholder(0).Field("stored"),
						Into:   pathdom.NewPlaceholder(1).Field("container"),
					}},
				},
			}
		},
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	sourceKey, sourceKeyOK := resolver.StateKeyAt(point, sourcePath)
	intoKey, intoKeyOK := resolver.StateKeyAt(point, intoPath)
	if !sourceKeyOK || !intoKeyOK {
		t.Fatalf("visibility failed for store relation paths: %q/%v -> %q/%v", sourceKey, sourceKeyOK, intoKey, intoKeyOK)
	}
	relation := state.StoreRelation{Source: sourceKey, Into: intoKey}
	if !got.HasStoreRelation(relation) {
		t.Fatalf("store relations = %#v, want rebased source/into relation", got.StoreRelationsSnapshot())
	}
}

func TestFactsNodeTransferCallOutcomeAppliesLifecycleFacts(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(616)
	txExpr := factflow.ExprRef(616)
	tx := symbol.ID(616)
	txPath := pathdom.NewPath(tx, "tx")
	builder := visibility.NewBuilder()
	builder.Define(point, tx, "tx")
	resolver := visibility.NewResolver(builder.Build())

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			CallSites: map[cfg.Point]factflow.CallSite{
				point: factflow.NewCallSite(factflow.CallSiteConfig{
					Context: factflow.CallSiteContextStatement,
					ArgumentSources: []factflow.ValueSource{
						{Kind: factflow.ValueSourceExpression, ExprRef: txExpr, HasExpr: true},
					},
				}),
			},
			ExpressionPaths: map[factflow.ExprRef]pathdom.Path{
				txExpr: txPath,
			},
		}),
		CallOutcome: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
			return callpayload.CallOutcome{
				NormalReturnFacts: callboundary.NormalReturnFacts{
					LifecycleFacts: []callboundary.LifecycleFact{
						{
							Target:   pathdom.NewPlaceholder(0),
							Kind:     callboundary.LifecycleAcquire,
							Protocol: typestate.Protocol("transaction"),
							To:       typestate.State("open"),
							Obligation: typestate.Obligation{
								Final: typestate.State("closed"),
							},
						},
						{
							Target:   pathdom.NewPlaceholder(0),
							Kind:     callboundary.LifecycleTransition,
							Protocol: typestate.Protocol("transaction"),
							From:     typestate.State("open"),
							To:       typestate.State("closed"),
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

	targetKey := resolver.KeyAt(point, txPath)
	if targetKey == "" {
		t.Fatal("visibility failed for typestate target")
	}
	if open := got.OpenTypestateObligations(); len(open) != 0 {
		t.Fatalf("open typestate obligations = %#v, want closed transaction after rebased transition", open)
	}
}

func TestFactsNodeTransferCallOutcomeLengthFloorSurvivesInvalidations(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(631)
	items := symbol.ID(631)
	itemsExpr := factflow.ExprRef(631)
	itemsPath := pathdom.NewPath(items, "items")
	placeholder := pathdom.NewPlaceholder(0)
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, items, "items")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()
	itemsKey, itemsKeyOK := resolver.StateKeyAt(point, itemsPath)
	if !itemsKeyOK {
		t.Fatal("StateKeyAt(items) failed")
	}
	childKey := resolver.KeyAt(point, itemsPath.IndexInt(1))

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			CallSites: map[cfg.Point]factflow.CallSite{
				point: factflow.NewCallSite(factflow.CallSiteConfig{
					Context: factflow.CallSiteContextStatement,
					ArgumentSources: []factflow.ValueSource{
						{Kind: factflow.ValueSourceExpression, ExprRef: itemsExpr, HasExpr: true},
					},
				}),
			},
			ExpressionPaths: map[factflow.ExprRef]pathdom.Path{
				itemsExpr: itemsPath,
			},
		}),
		CallOutcome: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
			return callpayload.CallOutcome{
				ParamPathInvalidations: []callpayload.CallParamPathInvalidation{
					{Path: placeholder},
				},
				ParamLengthFloors: []callpayload.CallParamLengthFloor{
					{Path: placeholder, Floor: 1},
				},
				NormalReturnFacts: callboundary.NormalReturnFacts{
					PathInvalidations: []callboundary.PathInvalidationFact{
						{Path: placeholder},
					},
				},
			}
		},
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{}.
		WriteLenFloor(ks, itemsKey, 9).
		WritePathKey(reg, ks, childKey, presentValue(reg)))

	if floor, ok := got.ReadLenFloor(ks, itemsKey); !ok || floor != 10 {
		t.Fatalf("post-call length floor = %d/%v, want prior floor plus delta after invalidations", floor, ok)
	}
	if child := got.ReadPathKey(reg, ks, childKey); !product.Equal(reg, child, product.Bottom(reg)) {
		t.Fatalf("child path after invalidation = %s, want bottom", formatValue(reg, child))
	}
}

func TestFactsNodeTransferCallOutcomeAppliesNormalReturnNumFloors(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(632)
	index := symbol.ID(632)
	indexExpr := factflow.ExprRef(632)
	indexPath := pathdom.NewPath(index, "i")
	memberPath := indexPath.Field("next")
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, index, "i")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			CallSites: map[cfg.Point]factflow.CallSite{
				point: factflow.NewCallSite(factflow.CallSiteConfig{
					Context: factflow.CallSiteContextStatement,
					ArgumentSources: []factflow.ValueSource{
						{Kind: factflow.ValueSourceExpression, ExprRef: indexExpr, HasExpr: true},
					},
				}),
			},
			ExpressionPaths: map[factflow.ExprRef]pathdom.Path{
				indexExpr: indexPath,
			},
		}),
		CallOutcome: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
			return callpayload.CallOutcome{
				NormalReturnFacts: callboundary.NormalReturnFacts{
					NumFloors: []callboundary.NumFloorFact{
						{Path: pathdom.NewPlaceholder(0), Floor: 1},
						{Path: pathdom.NewPlaceholder(0).Field("next"), Floor: 2},
					},
				},
			}
		},
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	rootKey, rootKeyOK := visibility.RootOrVisibleStateKeyAt(resolver, point, indexPath)
	memberKey, memberKeyOK := visibility.RootOrVisibleStateKeyAt(resolver, point, memberPath)
	if !rootKeyOK || !memberKeyOK {
		t.Fatal("RootOrVisibleStateKeyAt failed")
	}
	if floor, ok := got.ReadNumFloor(ks, rootKey); !ok || floor != 1 {
		t.Fatalf("root num floor = %d/%v, want 1 at %s", floor, ok, rootKey)
	}
	if floor, ok := got.ReadNumFloor(ks, memberKey); !ok || floor != 2 {
		t.Fatalf("member num floor = %d/%v, want 2 at %s", floor, ok, memberKey)
	}
}

func TestFactsNodeTransferCallOutcomeAppliesNormalReturnRelConstraints(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(633)
	xs := symbol.ID(633)
	i := symbol.ID(634)
	j := symbol.ID(635)
	xsExpr := factflow.ExprRef(633)
	iExpr := factflow.ExprRef(634)
	jExpr := factflow.ExprRef(635)
	xsPath := pathdom.NewPath(xs, "xs")
	iPath := pathdom.NewPath(i, "i")
	jPath := pathdom.NewPath(j, "j")
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, xs, "xs")
	visibilityBuilder.Define(point, i, "i")
	visibilityBuilder.Define(point, j, "j")
	resolver := visibility.NewResolver(visibilityBuilder.Build())

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			CallSites: map[cfg.Point]factflow.CallSite{
				point: factflow.NewCallSite(factflow.CallSiteConfig{
					Context: factflow.CallSiteContextStatement,
					ArgumentSources: []factflow.ValueSource{
						{Kind: factflow.ValueSourceExpression, ExprRef: xsExpr, HasExpr: true},
						{Kind: factflow.ValueSourceExpression, ExprRef: iExpr, HasExpr: true},
						{Kind: factflow.ValueSourceExpression, ExprRef: jExpr, HasExpr: true},
					},
				}),
			},
			ExpressionPaths: map[factflow.ExprRef]pathdom.Path{
				xsExpr: xsPath,
				iExpr:  iPath,
				jExpr:  jPath,
			},
		}),
		CallOutcome: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
			return callpayload.CallOutcome{
				NormalReturnFacts: callboundary.NormalReturnFacts{
					RelConstraints: []callboundary.RelConstraintFact{{
						CoA: 1,
						A:   callboundary.RelOperand{Path: pathdom.NewPlaceholder(1)},
						CoB: 1,
						B:   callboundary.RelOperand{Path: pathdom.NewPlaceholder(2)},
						C:   callboundary.RelOperand{Path: pathdom.NewPlaceholder(0), IsLength: true},
					}},
				},
			}
		},
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	xsKey, xsOK := visibility.RootOrVisibleStateKeyAt(resolver, point, xsPath)
	iKey, iOK := visibility.RootOrVisibleStateKeyAt(resolver, point, iPath)
	jKey, jOK := visibility.RootOrVisibleStateKeyAt(resolver, point, jPath)
	if !xsOK || !iOK || !jOK {
		t.Fatalf("RootOrVisibleStateKeyAt failed for xs=%v i=%v j=%v", xsOK, iOK, jOK)
	}
	constraints := got.RelConstraints().Constraints
	if len(constraints) != 1 {
		t.Fatalf("relational constraints = %#v, want one rebased relation", constraints)
	}
	constraint := constraints[0]
	if constraint.CoA != 1 || constraint.CoB != 1 || constraint.K != 0 ||
		!((constraint.A == state.RelValueOperand(iKey) && constraint.B == state.RelValueOperand(jKey)) ||
			(constraint.A == state.RelValueOperand(jKey) && constraint.B == state.RelValueOperand(iKey))) ||
		constraint.C != state.RelLengthOperand(xsKey) {
		t.Fatalf("relational constraint = %#v, want i+j-len(xs)<=0 after rebasing", constraint)
	}
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
		CallOutcome: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
			return callpayload.CallOutcome{
				ParamPathRefinements: []callpayload.CallParamPathRefinement{
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
		CallOutcome: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
			return callpayload.CallOutcome{
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
	gotEvent := state.EscapeEvent{
		Target:    testStateKey(t, targetKey),
		Kind:      callboundary.EscapeEventStore,
		Recursive: true,
	}
	if !got.HasEscapeEvent(gotEvent) {
		t.Fatalf("escape event missing: %#v, want rebased store on %s", gotEvent, childPath)
	}
}

func TestFactsNodeTransferCallOutcomeEscapeSendMarksIdentityPlacementAndEffectDelta(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(701)
	target := symbol.ID(701)
	argExpr := factflow.ExprRef(701)
	targetPath := pathdom.NewPath(target, "obj")
	tableID := identity.ID{Kind: "lua.table", Site: "escape-placement", Index: 1}
	tableValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(tableID))
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
			argExpr: targetPath,
		},
	})
	builder := visibility.NewBuilder()
	builder.Define(point, target, "obj")
	resolver := visibility.NewResolver(builder.Build())

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: facts,
		CallOutcome: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
			return callpayload.CallOutcome{
				NormalReturnFacts: callboundary.NormalReturnFacts{
					EscapeEvents: []callboundary.EscapeEventFact{
						{Target: pathdom.NewPlaceholder(0), Kind: callboundary.EscapeEventSend},
					},
				},
			}
		},
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{}.WriteValue(reg, key.SymbolValue(target), tableValue))

	if gotPlacement := got.ReadPlacement(tableID); gotPlacement != placement.SharedHeap {
		t.Fatalf("placement[%v] = %s, want %s", tableID, gotPlacement, placement.SharedHeap)
	}
	targetKey := resolver.KeyAt(point, targetPath)
	assertEscapeEvent(t, got, testStateKey(t, targetKey), callboundary.EscapeEventSend, false)
}

func TestFactsNodeTransferCallOutcomeFrozenTableFactFreezesSingletonIdentity(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(704)
	target := symbol.ID(704)
	argExpr := factflow.ExprRef(704)
	targetPath := pathdom.NewPath(target, "obj")
	tableID := identity.ID{Kind: "lua.table", Site: "freeze-apply", Index: 1}
	tableValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(tableID))
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
			argExpr: targetPath,
		},
	})
	builder := visibility.NewBuilder()
	builder.Define(point, target, "obj")
	resolver := visibility.NewResolver(builder.Build())

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: facts,
		CallOutcome: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
			return callpayload.CallOutcome{
				NormalReturnFacts: callboundary.NormalReturnFacts{
					FrozenTables: []callboundary.FrozenTableFact{
						{Target: pathdom.NewPlaceholder(0)},
					},
				},
			}
		},
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{}.WriteValue(reg, key.SymbolValue(target), tableValue))

	if !got.IsTableFrozen(tableID) {
		t.Fatalf("table %v was not frozen", tableID)
	}
	if !hasFrozenTableEffectDelta(got.EffectDeltasSnapshot().Deltas) {
		t.Fatalf("effect deltas = %#v, want frozen-table marker", got.EffectDeltasSnapshot().Deltas)
	}
}

func TestFactsNodeTransferCallOutcomeFrozenTableFactIsShallow(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(708)
	target := symbol.ID(708)
	argExpr := factflow.ExprRef(708)
	targetPath := pathdom.NewPath(target, "obj")
	childPath := targetPath.Field("child")
	rootID := identity.ID{Kind: "lua.table", Site: "freeze-apply", Index: 4}
	childID := identity.ID{Kind: "lua.table", Site: "freeze-apply", Index: 5}
	rootValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(rootID))
	childValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(childID))
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
			argExpr: targetPath,
		},
	})
	builder := visibility.NewBuilder()
	builder.Define(point, target, "obj")
	resolver := visibility.NewResolver(builder.Build())
	ks := resolver.KeySpace()

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: facts,
		CallOutcome: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
			return callpayload.CallOutcome{
				HeapTableObjects: map[identity.ID]heapidentity.TableObject{
					rootID: heapidentity.NewTableObject(heapidentity.TableObjectConfig{
						Root:          rootValue,
						StaticMembers: map[keyspace.Key]product.Value{fieldStaticKey(t, ks, "child"): childValue},
					}),
					childID: heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: childValue}),
				},
				NormalReturnFacts: callboundary.NormalReturnFacts{
					FrozenTables: []callboundary.FrozenTableFact{
						{Target: pathdom.NewPlaceholder(0)},
					},
				},
			}
		},
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{}.
		WriteValue(reg, key.SymbolValue(target), rootValue).
		WritePathKey(reg, ks, resolver.KeyAt(point, childPath), childValue))

	if !got.IsTableFrozen(rootID) {
		t.Fatalf("root table %v was not frozen", rootID)
	}
	if got.IsTableFrozen(childID) {
		t.Fatalf("child table %v was frozen by shallow root freeze", childID)
	}
}

func TestFactsNodeTransferCallOutcomeFrozenTableFactsFreezeRootAndNestedChild(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(705)
	target := symbol.ID(705)
	argExpr := factflow.ExprRef(705)
	targetPath := pathdom.NewPath(target, "obj")
	childPath := targetPath.Field("child")
	rootID := identity.ID{Kind: "lua.table", Site: "freeze-apply", Index: 2}
	childID := identity.ID{Kind: "lua.table", Site: "freeze-apply", Index: 3}
	rootValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(rootID))
	childValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(childID))
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
			argExpr: targetPath,
		},
	})
	builder := visibility.NewBuilder()
	builder.Define(point, target, "obj")
	resolver := visibility.NewResolver(builder.Build())
	ks := resolver.KeySpace()

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: facts,
		CallOutcome: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
			return callpayload.CallOutcome{
				HeapTableObjects: map[identity.ID]heapidentity.TableObject{
					rootID: heapidentity.NewTableObject(heapidentity.TableObjectConfig{
						Root:          rootValue,
						StaticMembers: map[keyspace.Key]product.Value{fieldStaticKey(t, ks, "child"): childValue},
					}),
					childID: heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: childValue}),
				},
				NormalReturnFacts: callboundary.NormalReturnFacts{
					FrozenTables: []callboundary.FrozenTableFact{
						{Target: pathdom.NewPlaceholder(0)},
						{Target: pathdom.NewPlaceholder(0).Field("child")},
					},
				},
			}
		},
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{}.
		WriteValue(reg, key.SymbolValue(target), rootValue).
		WritePathKey(reg, ks, resolver.KeyAt(point, childPath), childValue))

	if !got.IsTableFrozen(rootID) {
		t.Fatalf("root table %v was not frozen", rootID)
	}
	if !got.IsTableFrozen(childID) {
		t.Fatalf("child table %v was not frozen", childID)
	}
}

func TestFactsNodeTransferCallOutcomeRecursiveEscapeSendMarksStaticMemberIdentityPlacement(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(702)
	target := symbol.ID(702)
	argExpr := factflow.ExprRef(702)
	targetPath := pathdom.NewPath(target, "obj")
	rootID := identity.ID{Kind: "lua.table", Site: "escape-placement", Index: 2}
	childID := identity.ID{Kind: "lua.table", Site: "escape-placement", Index: 3}
	rootValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(rootID))
	childValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(childID))
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
			argExpr: targetPath,
		},
	})
	builder := visibility.NewBuilder()
	builder.Define(point, target, "obj")
	resolver := visibility.NewResolver(builder.Build())
	ks := resolver.KeySpace()
	memberKey := fieldStaticKey(t, ks, "child")

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: facts,
		CallOutcome: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
			return callpayload.CallOutcome{
				NormalReturnFacts: callboundary.NormalReturnFacts{
					EscapeEvents: []callboundary.EscapeEventFact{
						{Target: pathdom.NewPlaceholder(0), Kind: callboundary.EscapeEventSend, Recursive: true},
					},
				},
			}
		},
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{}.
		WriteValue(reg, key.SymbolValue(target), rootValue).
		WriteHeapTableObject(reg, rootID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root:          rootValue,
			StaticMembers: map[keyspace.Key]product.Value{memberKey: childValue},
		})).
		WriteHeapTableObject(reg, childID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: childValue,
		})))

	if gotPlacement := got.ReadPlacement(rootID); gotPlacement != placement.SharedHeap {
		t.Fatalf("placement[%v] = %s, want %s", rootID, gotPlacement, placement.SharedHeap)
	}
	if gotPlacement := got.ReadPlacement(childID); gotPlacement != placement.SharedHeap {
		t.Fatalf("placement[%v] = %s, want %s", childID, gotPlacement, placement.SharedHeap)
	}
}

func TestFactsNodeTransferCallOutcomeRecursiveEscapeStoreMarksStaticMemberIdentityOwnedHeap(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(706)
	target := symbol.ID(706)
	argExpr := factflow.ExprRef(706)
	targetPath := pathdom.NewPath(target, "obj")
	rootID := identity.ID{Kind: "lua.table", Site: "escape-placement", Index: 6}
	childID := identity.ID{Kind: "lua.table", Site: "escape-placement", Index: 7}
	rootValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(rootID))
	childValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(childID))
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
			argExpr: targetPath,
		},
	})
	builder := visibility.NewBuilder()
	builder.Define(point, target, "obj")
	resolver := visibility.NewResolver(builder.Build())
	ks := resolver.KeySpace()

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: facts,
		CallOutcome: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
			return callpayload.CallOutcome{
				NormalReturnFacts: callboundary.NormalReturnFacts{
					EscapeEvents: []callboundary.EscapeEventFact{
						{Target: pathdom.NewPlaceholder(0), Kind: callboundary.EscapeEventStore, Recursive: true},
					},
				},
			}
		},
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{}.
		WriteValue(reg, key.SymbolValue(target), rootValue).
		WriteHeapTableObject(reg, rootID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root:          rootValue,
			StaticMembers: map[keyspace.Key]product.Value{fieldStaticKey(t, ks, "child"): childValue},
		})).
		WriteHeapTableObject(reg, childID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: childValue,
		})))

	if gotPlacement := got.ReadPlacement(rootID); gotPlacement != placement.OwnedHeap {
		t.Fatalf("placement[%v] = %s, want %s", rootID, gotPlacement, placement.OwnedHeap)
	}
	if gotPlacement := got.ReadPlacement(childID); gotPlacement != placement.OwnedHeap {
		t.Fatalf("placement[%v] = %s, want %s", childID, gotPlacement, placement.OwnedHeap)
	}
}

func TestFactsNodeTransferCallOutcomeRecursiveEscapeSendMarksDynamicIndexKeyAndValueIdentities(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(707)
	target := symbol.ID(707)
	argExpr := factflow.ExprRef(707)
	targetPath := pathdom.NewPath(target, "obj")
	rootID := identity.ID{Kind: "lua.table", Site: "escape-placement", Index: 8}
	keyID := identity.ID{Kind: "lua.table", Site: "escape-placement", Index: 9}
	valueID := identity.ID{Kind: "lua.table", Site: "escape-placement", Index: 10}
	rootValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(rootID))
	keyValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(keyID))
	dynamicValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(valueID))
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
			argExpr: targetPath,
		},
	})
	builder := visibility.NewBuilder()
	builder.Define(point, target, "obj")
	resolver := visibility.NewResolver(builder.Build())
	ks := resolver.KeySpace()

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: facts,
		CallOutcome: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
			return callpayload.CallOutcome{
				NormalReturnFacts: callboundary.NormalReturnFacts{
					EscapeEvents: []callboundary.EscapeEventFact{
						{Target: pathdom.NewPlaceholder(0), Kind: callboundary.EscapeEventSend, Recursive: true},
					},
				},
			}
		},
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{}.
		WriteValue(reg, key.SymbolValue(target), rootValue).
		WriteHeapTableObject(reg, rootID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: rootValue,
			DynamicIndexFacts: map[dynamicindex.Key]dynamicindex.Fact{
				{Table: suffixStaticKey(t, ks, fieldSuffix("dynamic").Segments), Site: "dynamic-identity"}: {
					KeyPresence: presence.Present(),
					KeyValue:    keyValue,
					Value:       dynamicValue,
					Admission:   dynamicindex.AdmissionAdmitted,
				},
			},
		})).
		WriteHeapTableObject(reg, keyID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: keyValue,
		})).
		WriteHeapTableObject(reg, valueID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: dynamicValue,
		})))

	if gotPlacement := got.ReadPlacement(rootID); gotPlacement != placement.SharedHeap {
		t.Fatalf("placement[%v] = %s, want %s", rootID, gotPlacement, placement.SharedHeap)
	}
	if gotPlacement := got.ReadPlacement(keyID); gotPlacement != placement.SharedHeap {
		t.Fatalf("placement[%v] = %s, want %s", keyID, gotPlacement, placement.SharedHeap)
	}
	if gotPlacement := got.ReadPlacement(valueID); gotPlacement != placement.SharedHeap {
		t.Fatalf("placement[%v] = %s, want %s", valueID, gotPlacement, placement.SharedHeap)
	}
}

func TestFactsNodeTransferCallOutcomeEscapeRecoversDynamicEntryIdentityWhenPathValueIsTypeOnly(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(708)
	batch := symbol.ID(708)
	argExpr := factflow.ExprRef(708)
	batchPath := pathdom.NewPath(batch, "batch")
	itemsPath := batchPath.Field("items")
	itemPath := itemsPath.IndexStr("route-1")
	batchID := identity.ID{Kind: "lua.table", Site: "escape-placement", Index: 20}
	itemsID := identity.ID{Kind: "lua.table", Site: "escape-placement", Index: 21}
	itemID := identity.ID{Kind: "lua.table", Site: "escape-placement", Index: 22}
	childID := identity.ID{Kind: "lua.table", Site: "escape-placement", Index: 23}
	batchValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(batchID))
	itemsValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(itemsID))
	itemValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(itemID))
	childValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(childID))
	routeKeyType := typ.LiteralString("route-1")
	routeKeyValue := typevalue.WithWitness(reg, typevalue.FromType(reg, routeKeyType), routeKeyType)
	builder := visibility.NewBuilder()
	builder.Define(point, batch, "batch")
	resolver := visibility.NewResolver(builder.Build())
	ks := resolver.KeySpace()
	itemsKey, ok := heapidentity.StaticMemberSuffixKey(ks, fieldSuffix("items").Segments)
	if !ok {
		t.Fatal("missing items suffix key")
	}
	childKey, ok := heapidentity.StaticMemberSuffixKey(ks, fieldSuffix("child").Segments)
	if !ok {
		t.Fatal("missing child suffix key")
	}
	itemPathKey := resolver.KeyAt(point, itemPath)
	if itemPathKey == "" {
		t.Fatal("missing item path key")
	}
	itemsPathKey := resolver.KeyAt(point, itemsPath)
	if itemsPathKey == "" {
		t.Fatal("missing items path key")
	}
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(batch), batchValue).
		WritePathKey(reg, ks, itemsPathKey, presentValue(reg)).
		WritePathKey(reg, ks, itemPathKey, presentValue(reg)).
		WriteHeapTableObject(reg, batchID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root:          batchValue,
			StaticMembers: map[keyspace.Key]product.Value{itemsKey: itemsValue},
		})).
		WriteHeapTableObject(reg, itemsID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: itemsValue,
			DynamicIndexFacts: map[dynamicindex.Key]dynamicindex.Fact{
				{Table: mustStateKey(t, ks, pathdom.PathKey("callee.items")), Site: "callee.write"}: {
					KeyPresence: presence.Present(),
					KeyValue:    routeKeyValue,
					Value:       itemValue,
					Admission:   dynamicindex.AdmissionAdmitted,
				},
			},
		})).
		WriteHeapTableObject(reg, itemID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root:          itemValue,
			StaticMembers: map[keyspace.Key]product.Value{childKey: childValue},
		})).
		WriteHeapTableObject(reg, childID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: childValue,
		}))
	projected, ok := projectPathDynamicIndexValue(reg, resolver, point, in, itemPath)
	if !ok || !product.Equal(reg, projected, itemValue) {
		t.Fatalf("dynamic item projection = %s/%v, want item identity", formatValue(reg, projected), ok)
	}

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			CallSites: map[cfg.Point]factflow.CallSite{
				point: factflow.NewCallSite(factflow.CallSiteConfig{
					Context: factflow.CallSiteContextStatement,
					ArgumentSources: []factflow.ValueSource{
						{Kind: factflow.ValueSourceExpression, ExprRef: argExpr, HasExpr: true},
					},
				}),
			},
			ExpressionPaths: map[factflow.ExprRef]pathdom.Path{
				argExpr: itemPath,
			},
		}),
		CallOutcome: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
			return callpayload.CallOutcome{
				NormalReturnFacts: callboundary.NormalReturnFacts{
					EscapeEvents: []callboundary.EscapeEventFact{
						{Target: pathdom.NewPlaceholder(0), Kind: callboundary.EscapeEventSend, Recursive: true},
					},
				},
			}
		},
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, in)

	if gotPlacement := got.ReadPlacement(itemID); gotPlacement != placement.SharedHeap {
		t.Fatalf("placement[%v] = %s, want %s", itemID, gotPlacement, placement.SharedHeap)
	}
	if gotPlacement := got.ReadPlacement(childID); gotPlacement != placement.SharedHeap {
		t.Fatalf("placement[%v] = %s, want %s", childID, gotPlacement, placement.SharedHeap)
	}
}

func TestFactsNodeTransferCallOutcomeRecursiveEscapeTerminatesOnCyclicHeapPlacement(t *testing.T) {
	tests := []struct {
		name string
		kind callboundary.EscapeEventKind
		want placement.Value
	}{
		{name: "send", kind: callboundary.EscapeEventSend, want: placement.SharedHeap},
		{name: "store", kind: callboundary.EscapeEventStore, want: placement.OwnedHeap},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reg := standard.Registry()
			point := cfg.Point(709)
			target := symbol.ID(709)
			argExpr := factflow.ExprRef(709)
			targetPath := pathdom.NewPath(target, "obj")
			rootID := identity.ID{Kind: "lua.table", Site: "escape-cycle", Index: 1}
			childID := identity.ID{Kind: "lua.table", Site: "escape-cycle", Index: 2}
			rootValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(rootID))
			childValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(childID))
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
					argExpr: targetPath,
				},
			})
			builder := visibility.NewBuilder()
			builder.Define(point, target, "obj")
			resolver := visibility.NewResolver(builder.Build())
			ks := resolver.KeySpace()

			got := NewFactsNodeTransfer(FactsNodeTransferConfig{
				Facts: facts,
				CallOutcome: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
					return callpayload.CallOutcome{
						NormalReturnFacts: callboundary.NormalReturnFacts{
							EscapeEvents: []callboundary.EscapeEventFact{
								{Target: pathdom.NewPlaceholder(0), Kind: tc.kind, Recursive: true},
							},
						},
					}
				},
				Visibility: resolver,
			})(transfer.NodeContext{
				Registry: reg,
				Point:    point,
			}, state.State{}.
				WriteValue(reg, key.SymbolValue(target), rootValue).
				WriteHeapTableObject(reg, rootID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
					Root:          rootValue,
					StaticMembers: map[keyspace.Key]product.Value{fieldStaticKey(t, ks, "child"): childValue},
				})).
				WriteHeapTableObject(reg, childID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
					Root:          childValue,
					StaticMembers: map[keyspace.Key]product.Value{fieldStaticKey(t, ks, "parent"): rootValue},
				})))

			if gotPlacement := got.ReadPlacement(rootID); gotPlacement != tc.want {
				t.Fatalf("placement[%v] = %s, want %s", rootID, gotPlacement, tc.want)
			}
			if gotPlacement := got.ReadPlacement(childID); gotPlacement != tc.want {
				t.Fatalf("placement[%v] = %s, want %s", childID, gotPlacement, tc.want)
			}
		})
	}
}

func TestFactsNodeTransferCallOutcomeEscapeEventPlacementFromBottom(t *testing.T) {
	tests := []struct {
		name string
		kind callboundary.EscapeEventKind
		want placement.Value
	}{
		{name: "borrow", kind: callboundary.EscapeEventBorrow, want: placement.Bottom},
		{name: "retain", kind: callboundary.EscapeEventRetain, want: placement.OwnedHeap},
		{name: "store", kind: callboundary.EscapeEventStore, want: placement.OwnedHeap},
		{name: "send", kind: callboundary.EscapeEventSend, want: placement.SharedHeap},
		{name: "export", kind: callboundary.EscapeEventExport, want: placement.SharedHeap},
		{name: "opaque", kind: callboundary.EscapeEventOpaque, want: placement.SharedHeap},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reg := standard.Registry()
			point := cfg.Point(710)
			target := symbol.ID(710)
			argExpr := factflow.ExprRef(710)
			targetPath := pathdom.NewPath(target, "obj")
			tableID := identity.ID{Kind: "lua.table", Site: "escape-placement", Index: 11}
			tableValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(tableID))
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
					argExpr: targetPath,
				},
			})
			builder := visibility.NewBuilder()
			builder.Define(point, target, "obj")
			resolver := visibility.NewResolver(builder.Build())

			got := NewFactsNodeTransfer(FactsNodeTransferConfig{
				Facts: facts,
				CallOutcome: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
					return callpayload.CallOutcome{
						NormalReturnFacts: callboundary.NormalReturnFacts{
							EscapeEvents: []callboundary.EscapeEventFact{
								{Target: pathdom.NewPlaceholder(0), Kind: tc.kind},
							},
						},
					}
				},
				Visibility: resolver,
			})(transfer.NodeContext{
				Registry: reg,
				Point:    point,
			}, state.State{}.WriteValue(reg, key.SymbolValue(target), tableValue))

			if gotPlacement := got.ReadPlacement(tableID); gotPlacement != tc.want {
				t.Fatalf("placement[%v] = %s, want %s", tableID, gotPlacement, tc.want)
			}
		})
	}
}

func TestFactsNodeTransferCallOutcomeEscapeBorrowDoesNotForcePlacement(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(703)
	target := symbol.ID(703)
	argExpr := factflow.ExprRef(703)
	targetPath := pathdom.NewPath(target, "obj")
	tableID := identity.ID{Kind: "lua.table", Site: "escape-placement", Index: 4}
	tableValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(tableID))
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
			argExpr: targetPath,
		},
	})
	builder := visibility.NewBuilder()
	builder.Define(point, target, "obj")
	resolver := visibility.NewResolver(builder.Build())

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: facts,
		CallOutcome: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
			return callpayload.CallOutcome{
				NormalReturnFacts: callboundary.NormalReturnFacts{
					EscapeEvents: []callboundary.EscapeEventFact{
						{Target: pathdom.NewPlaceholder(0), Kind: callboundary.EscapeEventBorrow},
					},
				},
			}
		},
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{}.WriteValue(reg, key.SymbolValue(target), tableValue))

	if gotPlacement := got.ReadPlacement(tableID); gotPlacement != placement.Bottom {
		t.Fatalf("placement[%v] = %s, want %s", tableID, gotPlacement, placement.Bottom)
	}
}

func TestFactsNodeTransferCallOutcomeEscapeStoreRetainDoesNotWeakenSharedHeap(t *testing.T) {
	tests := []struct {
		name string
		kind callboundary.EscapeEventKind
	}{
		{name: "store", kind: callboundary.EscapeEventStore},
		{name: "retain", kind: callboundary.EscapeEventRetain},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reg := standard.Registry()
			point := cfg.Point(704)
			target := symbol.ID(704)
			argExpr := factflow.ExprRef(704)
			targetPath := pathdom.NewPath(target, "obj")
			tableID := identity.ID{Kind: "lua.table", Site: "escape-placement", Index: 5}
			tableValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(tableID))
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
					argExpr: targetPath,
				},
			})
			builder := visibility.NewBuilder()
			builder.Define(point, target, "obj")
			resolver := visibility.NewResolver(builder.Build())

			got := NewFactsNodeTransfer(FactsNodeTransferConfig{
				Facts: facts,
				CallOutcome: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
					return callpayload.CallOutcome{
						NormalReturnFacts: callboundary.NormalReturnFacts{
							EscapeEvents: []callboundary.EscapeEventFact{
								{Target: pathdom.NewPlaceholder(0), Kind: tc.kind},
							},
						},
					}
				},
				Visibility: resolver,
			})(transfer.NodeContext{
				Registry: reg,
				Point:    point,
			}, state.State{}.
				WriteValue(reg, key.SymbolValue(target), tableValue).
				WritePlacement(tableID, placement.SharedHeap))

			if gotPlacement := got.ReadPlacement(tableID); gotPlacement != placement.SharedHeap {
				t.Fatalf("placement[%v] = %s, want %s", tableID, gotPlacement, placement.SharedHeap)
			}
		})
	}
}

func TestFactsNodeTransferCallOutcomeAppliesHeapTableObjects(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	point := cfg.Point(619)
	tableID := identity.ID{Kind: "table", Site: "call-outcome", Index: 1}
	memberKey := fieldStaticKey(t, ks, "field")
	value := presentValue(reg)

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			CallSites: map[cfg.Point]factflow.CallSite{
				point: factflow.NewCallSite(factflow.CallSiteConfig{Context: factflow.CallSiteContextStatement}),
			},
		}),
		CallOutcome: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
			return callpayload.CallOutcome{
				HeapTableObjects: map[identity.ID]heapidentity.TableObject{
					tableID: heapidentity.NewTableObject(heapidentity.TableObjectConfig{
						Root:          value,
						StaticMembers: map[keyspace.Key]product.Value{memberKey: value},
					}),
				},
			}
		},
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	object := got.ReadHeapTableObject(reg, tableID)
	if !product.Equal(reg, object.Root(), value) {
		t.Fatalf("heap object root = %#v, want %#v", object.Root(), value)
	}
	if member, ok := object.StaticMember(memberKey); !ok || !product.Equal(reg, member, value) {
		t.Fatalf("heap object member = %#v/%v, want %#v", member, ok, value)
	}
}

func TestFactsNodeTransferCallOutcomeAppliesPlacementFacts(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(620)
	tableID := identity.ID{Kind: "table", Site: "call-placement", Index: 1}

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			CallSites: map[cfg.Point]factflow.CallSite{
				point: factflow.NewCallSite(factflow.CallSiteConfig{Context: factflow.CallSiteContextStatement}),
			},
		}),
		CallOutcome: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
			return callpayload.CallOutcome{
				Placements: map[identity.ID]placement.Value{
					tableID: placement.Stack,
				},
			}
		},
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	if gotPlacement := got.ReadPlacement(tableID); gotPlacement != placement.Stack {
		t.Fatalf("placement[%v] = %s, want %s", tableID, gotPlacement, placement.Stack)
	}
}

func assertEscapeEvent(
	t *testing.T,
	got state.State,
	target pathaddr.StateKey,
	kind callboundary.EscapeEventKind,
	recursive bool,
) {
	t.Helper()
	event := state.EscapeEvent{Target: target, Kind: kind, Recursive: recursive}
	if !got.HasEscapeEvent(event) {
		t.Fatalf("escape event missing: %#v", event)
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
			CallOutcome: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
				return callpayload.CallOutcome{
					ReturnConditionRefinements: []callpayload.CallReturnConditionRefinement{
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
			CallOutcome: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
				return callpayload.CallOutcome{
					ReturnConditionRefinements: []callpayload.CallReturnConditionRefinement{
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

func TestFactsEdgeTransferCallOutcomeTraversalCacheResetsForDifferentGraphs(t *testing.T) {
	reg := standard.Registry()
	killGraph, facts, branch, thenPoint, _, _, value := callOutcomeReturnPresenceGraph(reg, true)
	noKillGraph := callOutcomeReturnPresenceSamePointsGraph(false)
	edgeTransfer := NewFactsEdgeTransfer(FactsEdgeTransferConfig{
		Facts:       facts,
		CallOutcome: callOutcomeReturnPresenceProvider(),
	})
	in := state.State{}.WriteValue(reg, key.SymbolValue(value), product.Top())

	killOut := edgeTransfer(transfer.EdgeContext{
		Graph:    killGraph,
		Registry: reg,
		Edge:     cfg.Edge{From: branch, To: thenPoint, Cond: true},
		HasCond:  true,
	}, in)
	noKillOut := edgeTransfer(transfer.EdgeContext{
		Graph:    noKillGraph,
		Registry: reg,
		Edge:     cfg.Edge{From: branch, To: thenPoint, Cond: true},
		HasCond:  true,
	}, in)

	assertValue(t, reg, killOut, key.SymbolValue(value), product.Top())
	assertValue(t, reg, noKillOut, key.SymbolValue(value), absentValue(reg))
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
		reassignErr: factflow.NewRootAssignment(factflow.RootAssignmentOrdinaryRootWrite, err, errPath, factflow.NewNilValueSource(0)),
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
	assertValue(t, reg, flow[graph.Exit()], key.SymbolValue(err), nilSourceValue(reg))
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
		CallOutcome: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
			providerCalls++
			return callOutcomeReturnPresenceProvider()(transfer.NodeContext{}, factflow.CallSiteView{}, state.State{}, nil)
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
			factflow.NewNilValueSource(0),
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

func callOutcomeReturnPresenceSamePointsGraph(kill bool) cfg.Graph {
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	assignValue := graph.AddNode(cfg.NodeAssign)
	assignErr := graph.AddNode(cfg.NodeAssign)
	killPoint := graph.AddNode(cfg.NodeAssign)
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, assignValue, false)
	graph.AddEdge(assignValue, assignErr, false)
	if kill {
		graph.AddEdge(assignErr, killPoint, false)
		graph.AddEdge(killPoint, branch, false)
	} else {
		graph.AddEdge(assignErr, branch, false)
	}
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)
	return graph
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

func callOutcomeReturnPresenceProvider() callpayload.CallOutcomeProvider {
	return func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
		return callpayload.CallOutcome{
			Results: []callpayload.CallResult{
				{Index: 0, Value: product.Top()},
				{Index: 1, Value: product.Top()},
			},
			ReturnPresenceRelations: []callpayload.CallReturnPresenceRelation{
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

func hasFrozenTableEffectDelta(deltas map[effectdelta.Key]effectdelta.Value) bool {
	for key := range deltas {
		if key.Kind == effectdelta.Freeze && callboundary.IsFrozenTableEffectSite(key.Site) {
			return true
		}
	}
	return false
}
