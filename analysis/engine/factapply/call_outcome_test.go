package factapply

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
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
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/channelselect"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
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

func TestFactsNodeTransferNormalReturnNotEqualProofNarrowsChannelSelect(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(613)
	result := symbol.ID(613)
	inbox := symbol.ID(614)
	timeout := symbol.ID(615)
	resultPath := pathdom.NewPath(result, "result")
	inboxPath := pathdom.NewPath(inbox, "inbox_ch")
	timeoutPath := pathdom.NewPath(timeout, "timeout")
	selectID := factflow.ChannelSelectID("normal-return-neq-select")
	messagePayload := typetable.NewRecord().Field("kind", typ.LiteralString("message")).Build()
	timerPayload := typetable.NewRecord().Field("kind", typ.LiteralString("timer")).Build()
	eventsSet := channelSelectEvents(reg, selectID, resultPath, inboxPath, timeoutPath, messagePayload, timerPayload)
	resultValue, ok := testChannelSelectResultValue(reg, selectID, eventsSet.Events())
	if !ok {
		t.Fatal("failed to build channel select result value")
	}

	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, result, "result")
	visibilityBuilder.Define(point, inbox, "inbox_ch")
	visibilityBuilder.Define(point, timeout, "timeout")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	resultKey, ok := resolver.StateKeyAt(point, resultPath)
	if !ok {
		t.Fatal("missing result state key")
	}
	inboxKey, ok := resolver.StateKeyAt(point, inboxPath)
	if !ok {
		t.Fatal("missing inbox state key")
	}
	timeoutKey, ok := resolver.StateKeyAt(point, timeoutPath)
	if !ok {
		t.Fatal("missing timeout state key")
	}
	resultChannelExpr := factflow.ExprRef(613)
	timeoutExpr := factflow.ExprRef(614)

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			CallSites: map[cfg.Point]factflow.CallSite{
				point: factflow.NewCallSite(factflow.CallSiteConfig{
					Context: factflow.CallSiteContextStatement,
					ArgumentSources: []factflow.ValueSource{
						{Kind: factflow.ValueSourceExpression, ExprRef: resultChannelExpr, HasExpr: true},
						{Kind: factflow.ValueSourceExpression, ExprRef: timeoutExpr, HasExpr: true},
					},
				}),
			},
			ExpressionPaths: map[factflow.ExprRef]pathdom.Path{
				resultChannelExpr: resultPath.Field(channelselect.ResultChannelField),
				timeoutExpr:       timeoutPath,
			},
		}),
		CallOutcome: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
			return callpayload.CallOutcome{
				NormalReturnFacts: callboundary.NormalReturnFacts{
					BranchProofs: []callboundary.BranchProof{{
						Kind:  pathevidence.BranchProofPathNotEqual,
						Path:  pathdom.NewPlaceholder(0),
						Other: pathdom.NewPlaceholder(1),
					}},
				},
			}
		},
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{}.
		WriteValue(reg, key.SymbolValue(result), resultValue).
		AddChannelSelectFact(channelselectfact.Fact{
			Select:     channelselectfact.ID(selectID),
			Kind:       channelselectfact.FactReceive,
			Result:     resultKey,
			Case:       inboxKey,
			Index:      0,
			Payload:    typeValue(reg, messagePayload),
			HasPayload: true,
		}).
		AddChannelSelectFact(channelselectfact.Fact{
			Select:     channelselectfact.ID(selectID),
			Kind:       channelselectfact.FactReceive,
			Result:     resultKey,
			Case:       timeoutKey,
			Index:      1,
			Payload:    typeValue(reg, timerPayload),
			HasPayload: true,
		}))

	narrowed := got.ReadValue(reg, key.SymbolValue(result))
	assertChannelSelectCasePayload(t, reg, narrowed, channelselectfact.ID(selectID), 0, messagePayload)
	assertNoChannelSelectCasePayload(t, reg, narrowed, channelselectfact.ID(selectID), 1)
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

func TestFactsNodeTransferCallParamExposureOverridesContextualPathRefinement(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(615)
	argExpr := factflow.ExprRef(615)
	arg := symbol.ID(615)
	argPath := pathdom.NewPath(arg, "narrow")
	narrowType := typetable.NewRecord().Field("x", typ.Number).Build()
	wideType := typetable.NewRecord().Field("x", typeexpr.Union(typ.Number, typ.String)).Build()
	narrowValue := typevalue.WithWitness(reg, typevalue.FromType(reg, narrowType), narrowType)
	wideValue := typevalue.WithWitness(reg, typevalue.FromType(reg, wideType), wideType)
	staleNumber := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.Number), typ.Number)
	builder := visibility.NewBuilder()
	builder.Define(point, arg, "narrow")
	resolver := visibility.NewResolver(builder.Build())

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
				argExpr: argPath,
			},
		}),
		CallOutcome: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
			return callpayload.CallOutcome{
				ParamExposures: []callpayload.CallParamExposure{{
					Source:   pathdom.NewPlaceholder(0),
					Contract: wideValue,
					Kind:     factflow.CovariantExposureRecord,
				}},
				NormalReturnFacts: callboundary.NormalReturnFacts{
					PathRefinements: []callboundary.PathValueFact{{
						Path:  pathdom.NewPlaceholder(0).Field("x"),
						Value: staleNumber,
					}},
				},
			}
		},
		Visibility:     resolver,
		CovariantWiden: testCovariantRecordWiden,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{}.WriteValue(reg, key.SymbolValue(arg), narrowValue))

	gotType, ok := typevalue.TypeOf(reg, got.ReadValue(reg, key.SymbolValue(arg)))
	if !ok || !typ.TypeEquals(gotType, wideType) {
		t.Fatalf("root witness = %v/%v, want widened %v", gotType, ok, wideType)
	}
	fieldKey, ok := resolver.StateKeyAt(point, argPath.Field("x"))
	if !ok {
		t.Fatal("field state key failed")
	}
	if fieldValue := got.ReadPathKey(reg, resolver.KeySpace(), fieldKey.PathKey()); !product.Equal(reg, fieldValue, product.Bottom(reg)) {
		t.Fatalf("field path evidence = %#v, want bottom after exposure invalidates stale contextual refinement", fieldValue)
	}
}

func testCovariantRecordWiden(sourceWitness, contract typ.Type, segments []segment.Segment) (typ.Type, [][]segment.Segment, bool) {
	if len(segments) != 0 {
		return nil, nil, false
	}
	sourceRecord, sourceOK := sourceWitness.(*typ.Record)
	contractRecord, contractOK := contract.(*typ.Record)
	if !sourceOK || !contractOK {
		return nil, nil, false
	}
	sourceField := sourceRecord.GetField("x")
	contractField := contractRecord.GetField("x")
	if sourceField == nil || contractField == nil || typ.TypeEquals(sourceField.Type, contractField.Type) {
		return nil, nil, false
	}
	return contract, [][]segment.Segment{{{Kind: segment.SegmentField, Name: "x"}}}, true
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

func TestFactsNodeTransferCallOutcomeAppliesReturnSlotDynamicKeyMemberships(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(632)
	source := symbol.ID(632)
	result := symbol.ID(633)
	sourceExpr := factflow.ExprRef(632)
	callExpr := factflow.ExprRef(633)
	sourcePath := pathdom.NewPath(source, "suites")
	resultPath := pathdom.NewPath(result, "suite_names")
	callSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: callExpr, HasExpr: true}
	site := dynamicindex.Site("summary.returned.keys")
	value := presentValue(reg)
	builder := visibility.NewBuilder()
	builder.Define(point, source, "suites")
	builder.Define(point, result, "suite_names")
	resolver := visibility.NewResolver(builder.Build())

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			CallSites: map[cfg.Point]factflow.CallSite{
				point: factflow.NewCallSite(factflow.CallSiteConfig{
					Context: factflow.CallSiteContextAssignmentSource,
					ArgumentSources: []factflow.ValueSource{
						{Kind: factflow.ValueSourceExpression, ExprRef: sourceExpr, HasExpr: true},
					},
					ResultTargets: []factflow.CallResultTarget{
						factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, 0, 0, result, resultPath),
					},
				}),
			},
			ExpressionPaths: map[factflow.ExprRef]pathdom.Path{
				sourceExpr: sourcePath,
				callExpr:   resultPath,
			},
			RootAssignments: map[cfg.Point]factflow.RootAssignment{
				point: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, result, resultPath, callSource),
			},
		}),
		Sources: &recordingSourceValues{values: map[factflow.ValueSource]product.Value{
			callSource: value,
		}},
		CallOutcome: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
			return callpayload.CallOutcome{
				NormalReturnFacts: callboundary.NormalReturnFacts{
					PathInvalidations: []callboundary.PathInvalidationFact{{
						Path: pathdom.Path{Root: "ret[0]"},
					}},
					PathStaticMembers: []callboundary.PathStaticMemberFact{{
						Path:  pathdom.Path{Root: "ret[0]"}.Field("reader"),
						Value: value,
					}},
					DynamicIndexFacts: []callboundary.DynamicIndexFact{{
						Table: pathdom.Path{Root: "ret[0]"},
						Site:  site,
						Value: dynamicindex.Fact{
							KeyPresence: presence.Present(),
							KeyValue:    value,
							Value:       value,
							Admission:   dynamicindex.AdmissionAdmitted,
						},
					}},
					DynamicValueKeys: []callboundary.DynamicValueKeyMembershipFact{{
						Container: pathdom.Path{Root: "ret[0]"},
						Site:      site,
						Table:     pathdom.NewPlaceholder(0),
					}},
				},
			}
		},
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	resultStateKey, resultOK := resolver.StateKeyAt(point, resultPath)
	sourceStateKey, sourceOK := resolver.StateKeyAt(point, sourcePath)
	if !resultOK || !sourceOK {
		t.Fatalf("visibility failed for result/source: %v/%v", resultOK, sourceOK)
	}
	resultKey, resultKeyOK := resolver.KeySpace().InternStateKey(resultStateKey)
	if !resultKeyOK {
		t.Fatalf("InternStateKey(%q) failed", resultStateKey)
	}
	if _, ok := got.DynamicIndexFactsSnapshot().Facts[dynamicindex.Key{Table: resultKey, Site: site}]; !ok {
		t.Fatalf("dynamic-index facts = %#v, want returned container fact", got.DynamicIndexFactsSnapshot())
	}
	staticKey := resolver.KeyAt(point, resultPath.Field("reader"))
	if gotValue, ok := got.ReadPathStaticMember(resolver.KeySpace(), staticKey); !ok || !product.Equal(reg, gotValue, value) {
		t.Fatalf("returned static member = %s/%v, want %s/true", formatValue(reg, gotValue), ok, formatValue(reg, value))
	}
	tables := got.DynamicIndexValueKeyMembershipTables(resultKey, site)
	if len(tables) != 1 || tables[0] != sourceStateKey {
		t.Fatalf("dynamic value key memberships = %#v, want returned values as keys of %q", tables, sourceStateKey)
	}
}

func TestApplyCallOutcomeReturnSlotFactsDropsDescendantsBelowMaybeAbsentResult(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(6321)
	result := symbol.ID(6321)
	callExpr := factflow.ExprRef(6321)
	resultPath := pathdom.NewPath(result, "res")
	callSource := factflow.ValueSource{Kind: factflow.ValueSourceCall, ExprRef: callExpr, HasExpr: true, HasCallPoint: true, CallPoint: point, ResultIndex: 0}
	present := presentValue(reg)
	builder := visibility.NewBuilder()
	builder.Define(point, result, "res")
	resolver := visibility.NewResolver(builder.Build())
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			point: factflow.NewCallSite(factflow.CallSiteConfig{
				Context: factflow.CallSiteContextAssignmentSource,
				ResultTargets: []factflow.CallResultTarget{
					factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, 0, 0, result, resultPath),
				},
			}),
		},
		RootAssignments: map[cfg.Point]factflow.RootAssignment{
			point: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, result, resultPath, callSource),
		},
	})
	outcome := callpayload.CallOutcome{
		Results: []callpayload.CallResult{{
			Index: 0,
			Value: product.WithPresence(reg, present, presence.Maybe()),
		}},
		NormalReturnFacts: callboundary.NormalReturnFacts{
			PathStaticMembers: []callboundary.PathStaticMemberFact{{
				Path:  pathdom.Path{Root: "ret[0]"}.Field("answer"),
				Value: present,
			}},
		},
	}

	got := applyCallOutcomeReturnSlotFactsAfterRootAssignment(
		transfer.NodeContext{Registry: reg, Point: point},
		facts,
		func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
			return outcome
		},
		resolver,
		testLuaPathTypeProjector,
		nil,
		typevalue.NewCache(),
		func(cfg.Point) state.State { return state.State{} },
		state.State{},
		state.State{},
		resultPath,
		callSource,
	)

	staticKey := resolver.KeyAt(point, resultPath.Field("answer"))
	if gotValue, ok := got.ReadPathStaticMember(resolver.KeySpace(), staticKey); ok {
		t.Fatalf("return-slot descendant static member = %s, want absent because result root may be nil", formatValue(reg, gotValue))
	}
}

func TestFactsNodeTransferCallOutcomeAppliesReturnSlotPathLanes(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(633)
	result := symbol.ID(634)
	callExpr := factflow.ExprRef(633)
	resultPath := pathdom.NewPath(result, "returned")
	returnSlot := pathdom.Path{Root: "ret[0]"}
	callSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: callExpr, HasExpr: true}
	present := presentValue(reg)
	absent := absentValue(reg)
	tableID := identity.ID{Kind: "lua.table", Site: "return-slot-lanes", Index: 1}
	tableValue := product.Set(reg, present, identity.Key, identity.Singleton(tableID))
	effectValue := effectdelta.Value{
		Before: present,
		After:  absent,
		Change: effectdelta.ChangeChanged,
	}
	builder := visibility.NewBuilder()
	builder.Define(point, result, "returned")
	resolver := visibility.NewResolver(builder.Build())

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			CallSites: map[cfg.Point]factflow.CallSite{
				point: factflow.NewCallSite(factflow.CallSiteConfig{
					Context: factflow.CallSiteContextAssignmentSource,
					ResultTargets: []factflow.CallResultTarget{
						factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, 0, 0, result, resultPath),
					},
				}),
			},
			ExpressionPaths: map[factflow.ExprRef]pathdom.Path{
				callExpr: resultPath,
			},
			RootAssignments: map[cfg.Point]factflow.RootAssignment{
				point: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, result, resultPath, callSource),
			},
		}),
		Sources: &recordingSourceValues{values: map[factflow.ValueSource]product.Value{
			callSource: tableValue,
		}},
		CallOutcome: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
			return callpayload.CallOutcome{
				NormalReturnFacts: callboundary.NormalReturnFacts{
					BranchProofs: []callboundary.BranchProof{{
						Kind:     pathevidence.BranchProofPathPresence,
						Path:     returnSlot.Field("branch"),
						Presence: presence.Present(),
					}},
					ChannelSelects: []callboundary.ChannelSelectFact{{
						Select: channelselectfact.ID("returned.select"),
						Kind:   channelselectfact.FactReceive,
						Result: returnSlot.Field("selected"),
						Case:   returnSlot.Field("case"),
						Index:  2,
					}},
					FrozenTables: []callboundary.FrozenTableFact{{
						Target: returnSlot,
					}},
					EffectDeltas: []callboundary.EffectDelta{{
						Target: returnSlot.Field("effected"),
						Site:   "returned.effect",
						Kind:   effectdelta.Mutation,
						Value:  effectValue,
					}},
					NumFloors: []callboundary.NumFloorFact{{
						Path:  returnSlot.Field("floor"),
						Floor: 5,
					}},
					RelConstraints: []callboundary.RelConstraintFact{{
						CoA: 1,
						A:   callboundary.RelOperand{Path: returnSlot.Field("i")},
						CoB: 1,
						B:   callboundary.RelOperand{Path: returnSlot.Field("j")},
						C:   callboundary.RelOperand{Path: returnSlot.Field("items"), IsLength: true},
					}},
					StoreRelations: []callboundary.StoreRelationFact{{
						Source: returnSlot.Field("stored"),
						Into:   returnSlot.Field("container"),
					}},
					EscapeEvents: []callboundary.EscapeEventFact{{
						Target:    returnSlot,
						Kind:      callboundary.EscapeEventSend,
						Recursive: true,
					}},
				},
			}
		},
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	ks := resolver.KeySpace()
	branchProof := pathevidence.BranchProof{
		Kind:     pathevidence.BranchProofPathPresence,
		Path:     mustStateKey(t, ks, resolver.KeyAt(point, resultPath.Field("branch"))),
		Presence: presence.Present(),
	}
	if !got.HasBranchProof(branchProof) {
		t.Fatalf("return-slot branch proof missing: %#v", branchProof)
	}
	selectFact := channelselectfact.Fact{
		Select: channelselectfact.ID("returned.select"),
		Kind:   channelselectfact.FactReceive,
		Result: testStateKey(t, resolver.KeyAt(point, resultPath.Field("selected"))),
		Case:   testStateKey(t, resolver.KeyAt(point, resultPath.Field("case"))),
		Index:  2,
	}
	if !got.HasChannelSelectFact(selectFact) {
		t.Fatalf("return-slot channel-select fact missing: %#v", selectFact)
	}
	effectKey := effectdelta.Key{
		Target: mustStateKey(t, ks, resolver.KeyAt(point, resultPath.Field("effected"))),
		Site:   "returned.effect",
		Kind:   effectdelta.Mutation,
	}
	gotEffect := got.ReadEffectDelta(effectKey)
	if !effectdelta.Domain(reg).Equal(gotEffect, effectValue) {
		t.Fatalf("return-slot effect delta = %#v, want %#v", gotEffect, effectValue)
	}
	floorKey, floorKeyOK := visibility.RootOrVisibleStateKeyAt(resolver, point, resultPath.Field("floor"))
	if !floorKeyOK {
		t.Fatalf("visibility failed for return-slot num floor")
	}
	if floor, ok := got.ReadNumFloor(ks, floorKey); !ok || floor != 5 {
		t.Fatalf("return-slot num floor = %d/%v, want 5 at %s", floor, ok, floorKey)
	}
	iKey, iOK := visibility.RootOrVisibleStateKeyAt(resolver, point, resultPath.Field("i"))
	jKey, jOK := visibility.RootOrVisibleStateKeyAt(resolver, point, resultPath.Field("j"))
	itemsKey, itemsOK := visibility.RootOrVisibleStateKeyAt(resolver, point, resultPath.Field("items"))
	if !iOK || !jOK || !itemsOK {
		t.Fatalf("visibility failed for return-slot relation: i=%v j=%v items=%v", iOK, jOK, itemsOK)
	}
	constraints := got.RelConstraints().Constraints
	if len(constraints) != 1 {
		t.Fatalf("return-slot relational constraints = %#v, want one", constraints)
	}
	constraint := constraints[0]
	isSymmetricPair := (constraint.A == state.RelValueOperand(iKey) && constraint.B == state.RelValueOperand(jKey)) ||
		(constraint.A == state.RelValueOperand(jKey) && constraint.B == state.RelValueOperand(iKey))
	if constraint.CoA != 1 || constraint.CoB != 1 || constraint.K != 0 ||
		!isSymmetricPair ||
		constraint.C != state.RelLengthOperand(itemsKey) {
		t.Fatalf("return-slot relational constraint = %#v, want i+j-len(items)<=0", constraint)
	}
	sourceStateKey, sourceOK := resolver.StateKeyAt(point, resultPath.Field("stored"))
	intoStateKey, intoOK := resolver.StateKeyAt(point, resultPath.Field("container"))
	if !sourceOK || !intoOK {
		t.Fatalf("visibility failed for return-slot store relation: %v/%v", sourceOK, intoOK)
	}
	relation := state.StoreRelation{Source: sourceStateKey, Into: intoStateKey}
	if !got.HasStoreRelation(relation) {
		t.Fatalf("return-slot store relations = %#v, want %#v", got.StoreRelationsSnapshot(), relation)
	}
	if !got.IsTableFrozen(tableID) {
		t.Fatalf("return-slot table %v was not frozen", tableID)
	}
	if !hasFrozenTableEffectDelta(got.EffectDeltasSnapshot().Deltas) {
		t.Fatalf("return-slot effect deltas = %#v, want frozen-table marker", got.EffectDeltasSnapshot().Deltas)
	}
	assertEscapeEvent(t, got, testStateKey(t, resolver.KeyAt(point, resultPath)), callboundary.EscapeEventSend, true)
	if gotPlacement := got.ReadPlacement(tableID); gotPlacement != placement.SharedHeap {
		t.Fatalf("return-slot placement[%v] = %s, want %s", tableID, gotPlacement, placement.SharedHeap)
	}
}

func TestApplyCallOutcomeRebasedReturnSlotRootInvalidationPreservesReceiverShape(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(636)
	self := symbol.ID(636)
	selfPath := pathdom.NewPath(self, "self")
	metadataPath := selfPath.Field("_metadata")
	dataTargetsPath := selfPath.Field("data_targets")
	returnSlot := pathdom.Path{Root: "ret[0]"}
	metadataKey := pathdom.PathKey("sym636@1._metadata")
	dataTargetsType := typ.NewArray(typ.String)
	selfType := typetable.NewRecord().
		Field("_metadata", typetable.BuiltinTopMarker()).
		Field("data_targets", dataTargetsType).
		Build()
	selfValue := typevalue.WithWitness(reg, typevalue.FromType(reg, selfType), selfType)
	staleMetadata := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)

	builder := visibility.NewBuilder()
	builder.Define(point, self, "self")
	resolver := visibility.NewResolver(builder.Build())
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			point: factflow.NewCallSite(factflow.CallSiteConfig{
				Context: factflow.CallSiteContextAssignmentSource,
				ResultTargets: []factflow.CallResultTarget{
					factflow.NewCallResultTarget(factflow.CallResultTargetOrdinaryAssignment, 0, 0, self, metadataPath),
				},
			}),
		},
	})
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(self), selfValue).
		WritePathKey(reg, resolver.KeySpace(), metadataKey, staleMetadata)
	site, ok := facts.CallSiteView(point)
	if !ok {
		t.Fatal("missing call site")
	}

	got := applyCallOutcomeFacts(
		transfer.NodeContext{Registry: reg, Point: point},
		facts,
		resolver,
		testLuaPathTypeProjector,
		nil,
		typevalue.NewCache(),
		in,
		site,
		callpayload.CallOutcome{
			NormalReturnFacts: callboundary.NormalReturnFacts{
				PathInvalidations: []callboundary.PathInvalidationFact{{Path: returnSlot}},
			},
		},
	)

	if metadata := got.ReadPathKey(reg, resolver.KeySpace(), metadataKey); !product.Equal(reg, metadata, product.Bottom(reg)) {
		t.Fatalf("rebased return-slot root invalidation left stale metadata = %s, want bottom", formatValue(reg, metadata))
	}
	gotSelf := got.ReadValue(reg, key.SymbolValue(self))
	gotSelfType, ok := typevalue.TypeOf(reg, gotSelf)
	if !ok {
		t.Fatalf("self type missing after rebased invalidation: %s", formatValue(reg, gotSelf))
	}
	gotDataTargets, ok := testLuaPathTypeProjector(gotSelfType, dataTargetsPath)
	if !ok || !typ.TypeEquals(gotDataTargets, dataTargetsType) {
		t.Fatalf("self.data_targets type = %v/%v after rebased invalidation, want %v", gotDataTargets, ok, dataTargetsType)
	}
}

func TestFactsNodeTransferCallOutcomeDynamicIndexMutationKeepsRootStructuralWitness(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(633)
	items := symbol.ID(633)
	itemsExpr := factflow.ExprRef(633)
	itemsPath := pathdom.NewPath(items, "items")
	placeholder := pathdom.NewPlaceholder(0)
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, items, "items")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()
	childKey := resolver.KeyAt(point, itemsPath.IndexInt(1))
	arrayType := typ.NewArray(typ.Any)
	rootValue := typevalue.WithWitness(reg, typevalue.FromType(reg, arrayType), arrayType)

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
				NormalReturnFacts: callboundary.NormalReturnFacts{
					PathInvalidations: []callboundary.PathInvalidationFact{
						{Path: placeholder},
					},
					DynamicIndexFacts: []callboundary.DynamicIndexFact{
						{
							Table: placeholder,
							Site:  "callee.dynamic",
							Value: dynamicindex.Fact{
								KeyPresence: presence.Present(),
								KeyValue:    typevalue.WithWitness(reg, typevalue.FromType(reg, typ.Integer), typ.Integer),
								Value:       product.Top(),
								Admission:   dynamicindex.AdmissionUnknown,
							},
						},
					},
				},
			}
		},
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{}.
		WriteValue(reg, key.SymbolValue(items), rootValue).
		WritePathKey(reg, ks, childKey, presentValue(reg)))

	gotType, ok := typevalue.TypeOf(reg, got.ReadValue(reg, key.SymbolValue(items)))
	if !ok || !typ.TypeEquals(gotType, arrayType) {
		t.Fatalf("root witness = %v/%v, want preserved %v after dynamic-index call mutation", gotType, ok, arrayType)
	}
	if child := got.ReadPathKey(reg, ks, childKey); !product.Equal(reg, child, product.Bottom(reg)) {
		t.Fatalf("child path after dynamic-index call mutation = %s, want bottom", formatValue(reg, child))
	}
}

func TestFactsNodeTransferCallOutcomeParamInvalidationThenRefinementKeepsRootStructuralWitness(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(634)
	items := symbol.ID(634)
	itemsExpr := factflow.ExprRef(634)
	itemsPath := pathdom.NewPath(items, "items")
	placeholder := pathdom.NewPlaceholder(0)
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, items, "items")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()
	childKey := resolver.KeyAt(point, itemsPath.IndexInt(1))
	arrayType := typ.NewArray(typ.Any)
	arrayValue := typevalue.WithWitness(reg, typevalue.FromType(reg, arrayType), arrayType)

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
				ParamPathRefinements: []callpayload.CallParamPathRefinement{
					{Path: placeholder, Value: arrayValue},
				},
			}
		},
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{}.
		WriteValue(reg, key.SymbolValue(items), presentValue(reg)).
		WritePathKey(reg, ks, childKey, presentValue(reg)))

	gotType, ok := typevalue.TypeOf(reg, got.ReadValue(reg, key.SymbolValue(items)))
	if !ok || !typ.TypeEquals(gotType, arrayType) {
		t.Fatalf("root witness = %v/%v, want preserved %v after param invalidation+refinement", gotType, ok, arrayType)
	}
	if child := got.ReadPathKey(reg, ks, childKey); !product.Equal(reg, child, product.Bottom(reg)) {
		t.Fatalf("child path after param invalidation+refinement = %s, want bottom", formatValue(reg, child))
	}
}

func TestFactsNodeTransferCallOutcomeParamInvalidationThenWriteReplacesExactPathValue(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(6341)
	suite := symbol.ID(6341)
	itemsExpr := factflow.ExprRef(6341)
	suitePath := pathdom.NewPath(suite, "suite")
	itemsPath := suitePath.Field("items")
	placeholder := pathdom.NewPlaceholder(0)
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, suite, "suite")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()
	itemsKey := resolver.KeyAt(point, itemsPath)
	childKey := resolver.KeyAt(point, itemsPath.IndexInt(1))
	arrayType := typ.NewArray(typ.String)
	arrayValue := typevalue.WithWitness(reg, typevalue.FromType(reg, arrayType), arrayType)
	emptyRecordType := typetable.NewRecord().Build()
	emptyRecordValue := typevalue.WithWitness(reg, typevalue.FromType(reg, emptyRecordType), emptyRecordType)

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
					{Path: placeholder, PreserveStructuralWitness: true},
				},
				ParamPathWrites: []callpayload.CallParamPathWrite{
					{Path: placeholder, Value: arrayValue},
				},
			}
		},
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{}.
		WritePathKey(reg, ks, itemsKey, emptyRecordValue).
		WritePathKey(reg, ks, childKey, presentValue(reg)))

	gotType, ok := typevalue.TypeOf(reg, got.ReadPathKey(reg, ks, itemsKey))
	if !ok || !typ.TypeEquals(gotType, arrayType) {
		t.Fatalf("path witness = %v/%v, want write to replace stale %v", gotType, ok, arrayType)
	}
	if child := got.ReadPathKey(reg, ks, childKey); !product.Equal(reg, child, product.Bottom(reg)) {
		t.Fatalf("child path after param invalidation+write = %s, want bottom", formatValue(reg, child))
	}
}

func TestFactsNodeTransferStructuralPreservingParamInvalidationKeepsRootWitness(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(635)
	items := symbol.ID(635)
	itemsExpr := factflow.ExprRef(635)
	itemsPath := pathdom.NewPath(items, "items")
	placeholder := pathdom.NewPlaceholder(0)
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, items, "items")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()
	childKey := resolver.KeyAt(point, itemsPath.IndexInt(1))
	arrayType := typ.NewArray(typ.Any)
	arrayValue := typevalue.WithWitness(reg, typevalue.FromType(reg, arrayType), arrayType)

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
					{Path: placeholder, PreserveStructuralWitness: true},
				},
			}
		},
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{}.
		WriteValue(reg, key.SymbolValue(items), arrayValue).
		WritePathKey(reg, ks, childKey, presentValue(reg)))

	gotType, ok := typevalue.TypeOf(reg, got.ReadValue(reg, key.SymbolValue(items)))
	if !ok || !typ.TypeEquals(gotType, arrayType) {
		t.Fatalf("root witness = %v/%v, want preserved %v after structural-preserving invalidation", gotType, ok, arrayType)
	}
	if child := got.ReadPathKey(reg, ks, childKey); !product.Equal(reg, child, product.Bottom(reg)) {
		t.Fatalf("child path after structural-preserving invalidation = %s, want bottom", formatValue(reg, child))
	}
}

func TestFactsNodeTransferTableMutatorKeepsTargetStaticMemberWitness(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(636)
	suite := symbol.ID(636)
	entry := symbol.ID(637)
	tableExpr := factflow.ExprRef(636)
	valueExpr := factflow.ExprRef(637)
	suitePath := pathdom.NewPath(suite, "suite")
	testsPath := suitePath.Field("tests")
	entryPath := pathdom.NewPath(entry, "entry")
	placeholder := pathdom.NewPlaceholder(0)
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, suite, "suite")
	visibilityBuilder.Define(point, entry, "entry")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()
	testsKey := resolver.KeyAt(point, testsPath)
	if gotKey := factPathKeyAt(resolver, point, testsPath); gotKey != testsKey {
		t.Fatalf("factPathKeyAt(%s) = %s, want %s", testsPath.String(), gotKey, testsKey)
	}
	entryType := typetable.NewRecord().Field("id", typ.String).Build()
	testsType := typ.NewArray(entryType)
	testsValue := typevalue.WithWitness(reg, typevalue.FromType(reg, testsType), testsType)
	entryValue := typevalue.WithWitness(reg, typevalue.FromType(reg, entryType), entryType)

	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			point: factflow.NewCallSite(factflow.CallSiteConfig{
				Context: factflow.CallSiteContextStatement,
				ArgumentSources: []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression, ExprRef: tableExpr, HasExpr: true},
					{Kind: factflow.ValueSourceExpression, ExprRef: valueExpr, HasExpr: true},
				},
			}),
		},
		ExpressionPaths: map[factflow.ExprRef]pathdom.Path{
			tableExpr: testsPath,
			valueExpr: entryPath,
		},
	})
	paramOnlyOutcome := callpayload.CallOutcome{
		ParamPathInvalidations: []callpayload.CallParamPathInvalidation{
			{Path: placeholder, PreserveStructuralWitness: true},
		},
	}
	normalOnlyOutcome := callpayload.CallOutcome{
		NormalReturnFacts: callboundary.NormalReturnFacts{
			PathInvalidations: []callboundary.PathInvalidationFact{
				{Path: placeholder, PreserveStructuralWitness: true},
			},
		},
	}
	initial := state.State{}.
		WriteValue(reg, key.SymbolValue(suite), typevalue.WithWitness(reg, typevalue.FromType(reg, typetable.NewRecord().Field("name", typ.String).Build()), typetable.NewRecord().Field("name", typ.String).Build())).
		WriteValue(reg, key.SymbolValue(entry), entryValue).
		WritePathStaticMember(ks, testsKey, testsValue)
	run := func(outcome callpayload.CallOutcome) state.State {
		return NewFactsNodeTransfer(FactsNodeTransferConfig{
			Facts: facts,
			CallOutcome: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
				return outcome
			},
			Visibility: resolver,
		})(transfer.NodeContext{
			Registry: reg,
			Point:    point,
		}, initial)
	}
	if static, ok := run(paramOnlyOutcome).ReadPathStaticMember(ks, testsKey); !ok {
		t.Fatalf("missing %s static member after structural-preserving param invalidation", testsKey)
	} else if staticType, ok := typevalue.TypeOf(reg, static); !ok || !typ.TypeEquals(staticType, testsType) {
		t.Fatalf("static member type after param invalidation = %v/%v, want %v", staticType, ok, testsType)
	}
	if static, ok := run(normalOnlyOutcome).ReadPathStaticMember(ks, testsKey); !ok {
		t.Fatalf("missing %s static member after structural-preserving normal invalidation", testsKey)
	} else if staticType, ok := typevalue.TypeOf(reg, static); !ok || !typ.TypeEquals(staticType, testsType) {
		t.Fatalf("static member type after normal invalidation = %v/%v, want %v", staticType, ok, testsType)
	}
	withDynamic := normalOnlyOutcome
	withDynamic.ParamPathInvalidations = paramOnlyOutcome.ParamPathInvalidations
	withDynamic.NormalReturnFacts.DynamicIndexFacts = []callboundary.DynamicIndexFact{
		{
			Table:     placeholder,
			Site:      dynamicindex.Site("table.insert"),
			ValuePath: pathdom.NewPlaceholder(1),
			Value: dynamicindex.Fact{
				KeyPresence: presence.Present(),
				Value:       entryValue,
				Admission:   dynamicindex.AdmissionAdmitted,
			},
		},
	}

	got := run(withDynamic)

	static, ok := got.ReadPathStaticMember(ks, testsKey)
	if !ok {
		t.Fatalf("missing %s static member after structural-preserving table mutator", testsKey)
	}
	staticType, ok := typevalue.TypeOf(reg, static)
	if !ok || !typ.TypeEquals(staticType, testsType) {
		t.Fatalf("static member type = %v/%v, want %v", staticType, ok, testsType)
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

func TestFactsNodeTransferCallOutcomeSeparatesRootShapeFactsFromVisibleValueFacts(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(634)
	target := symbol.ID(634)
	targetExpr := factflow.ExprRef(634)
	targetPath := pathdom.NewPath(target, "item")
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, target, "item")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()
	present := presentValue(reg)

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			CallSites: map[cfg.Point]factflow.CallSite{
				point: factflow.NewCallSite(factflow.CallSiteConfig{
					Context: factflow.CallSiteContextStatement,
					ArgumentSources: []factflow.ValueSource{
						{Kind: factflow.ValueSourceExpression, ExprRef: targetExpr, HasExpr: true},
					},
				}),
			},
			ExpressionPaths: map[factflow.ExprRef]pathdom.Path{
				targetExpr: targetPath,
			},
		}),
		CallOutcome: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
			return callpayload.CallOutcome{
				NormalReturnFacts: callboundary.NormalReturnFacts{
					NumFloors: []callboundary.NumFloorFact{
						{Path: pathdom.NewPlaceholder(0), Floor: 3},
					},
					PathStaticMembers: []callboundary.PathStaticMemberFact{
						{Path: pathdom.NewPlaceholder(0), Value: present},
					},
				},
			}
		},
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	structuralRootKey := testStateKey(t, targetPath.Key())
	visibleRootPathKey := resolver.KeyAt(point, targetPath)
	if visibleRootPathKey == "" || visibleRootPathKey == targetPath.Key() {
		t.Fatalf("visible root key = %q, want distinct versioned key from structural %q", visibleRootPathKey, targetPath.Key())
	}
	visibleRootKey := testStateKey(t, visibleRootPathKey)
	if floor, ok := got.ReadNumFloor(ks, structuralRootKey); !ok || floor != 3 {
		t.Fatalf("structural root num floor = %d/%v, want 3/true at %s", floor, ok, structuralRootKey)
	}
	if floor, ok := got.ReadNumFloor(ks, visibleRootKey); ok || floor != 0 {
		t.Fatalf("visible root num floor = %d/%v, want absent because root shape floors use structural keys", floor, ok)
	}
	if value, ok := got.ReadPathStaticMember(ks, visibleRootPathKey); !ok || !product.Equal(reg, value, present) {
		t.Fatalf("visible root static member = %s/%v, want precise visible value fact", formatValue(reg, value), ok)
	}
	if value, ok := got.ReadPathStaticMember(ks, targetPath.Key()); ok {
		t.Fatalf("structural root static member = %s/%v, want absent because value facts use visible keys", formatValue(reg, value), ok)
	}
}

func TestFactsNodeTransferCallOutcomePersistentPathWriteUpdatesRoot(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(635)
	captured := symbol.ID(635)
	value := presentValue(reg)

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			CallSites: map[cfg.Point]factflow.CallSite{
				point: factflow.NewCallSite(factflow.CallSiteConfig{
					Context: factflow.CallSiteContextStatement,
				}),
			},
		}),
		CallOutcome: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
			return callpayload.CallOutcome{
				NormalReturnFacts: callboundary.NormalReturnFacts{
					PersistentPathWrites: []callboundary.PathValueFact{
						{Path: pathdom.NewPath(captured, "captured"), Value: value},
					},
				},
			}
		},
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	if gotValue := got.ReadValue(reg, key.SymbolValue(captured)); !product.Equal(reg, gotValue, value) {
		t.Fatalf("captured root value = %s, want %s", formatValue(reg, gotValue), formatValue(reg, value))
	}
}

func TestFactsNodeTransferCallOutcomePersistentRootWriteWinsOverSameOutcomeInvalidation(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(636)
	captured := symbol.ID(636)
	capturedPath := pathdom.NewPath(captured, "captured")
	value := typevalue.FromType(reg, typ.Func().Build())

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			CallSites: map[cfg.Point]factflow.CallSite{
				point: factflow.NewCallSite(factflow.CallSiteConfig{
					Context: factflow.CallSiteContextStatement,
				}),
			},
		}),
		CallOutcome: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
			return callpayload.CallOutcome{
				NormalReturnFacts: callboundary.NormalReturnFacts{
					PathInvalidations: []callboundary.PathInvalidationFact{
						{Path: capturedPath},
					},
					PersistentPathWrites: []callboundary.PathValueFact{
						{Path: capturedPath, Value: value},
					},
				},
			}
		},
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	if gotValue := got.ReadValue(reg, key.SymbolValue(captured)); !product.Equal(reg, gotValue, value) {
		t.Fatalf("captured root value = %s, want persistent write %s after same-outcome invalidation", formatValue(reg, gotValue), formatValue(reg, value))
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
	isSymmetricPair := (constraint.A == state.RelValueOperand(iKey) && constraint.B == state.RelValueOperand(jKey)) ||
		(constraint.A == state.RelValueOperand(jKey) && constraint.B == state.RelValueOperand(iKey))
	if constraint.CoA != 1 || constraint.CoB != 1 || constraint.K != 0 ||
		!isSymmetricPair ||
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

func TestFactsNodeTransferCallOutcomeParamPathRefinementUsesWIRPathArgument(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(6161)
	arg := symbol.ID(6161)
	argPath := pathdom.NewPath(arg, "arg")
	present := presentValue(reg)
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, arg, "arg")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	source, ok := factflow.NewPathValueSource(argPath.Key(), 0, 0, 0, factflow.ValueSourceShape{})
	if !ok {
		t.Fatalf("NewPathValueSource(%q) failed", argPath.Key())
	}

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			CallSites: map[cfg.Point]factflow.CallSite{
				point: factflow.NewCallSite(factflow.CallSiteConfig{
					Context:         factflow.CallSiteContextStatement,
					ArgumentSources: []factflow.ValueSource{source},
				}),
			},
		}),
		CallOutcome: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
			return callpayload.CallOutcome{
				ParamPathRefinements: []callpayload.CallParamPathRefinement{
					{Path: pathdom.NewPlaceholder(0), Value: present},
				},
			}
		},
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{}.WriteValue(reg, key.SymbolValue(arg), product.Top()))

	assertValue(t, reg, got, key.SymbolValue(arg), present)
}

func TestFactsNodeTransferCallOutcomeParamPathRefinementAppliesToMemberArgument(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(617)
	obj := symbol.ID(617)
	argExpr := factflow.ExprRef(617)
	objPath := pathdom.NewPath(obj, "obj")
	memberPath := objPath.Field("data")
	present := presentValue(reg)
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, obj, "obj")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	memberKey := resolver.KeyAt(point, memberPath)

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
				argExpr: memberPath,
			},
		}),
		CallOutcome: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
			return callpayload.CallOutcome{
				ParamPathRefinements: []callpayload.CallParamPathRefinement{
					{Path: pathdom.NewPlaceholder(0), Value: present},
				},
			}
		},
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	gotMember := got.ReadPathKey(reg, resolver.KeySpace(), memberKey)
	if !presence.Equal(product.PresenceOf(gotMember), presence.Present()) {
		t.Fatalf("member argument presence = %s in %s, want present", product.PresenceOf(gotMember), formatValue(reg, gotMember))
	}
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

func TestFactsNodeTransferCallOutcomeShallowEscapeSendDoesNotPromoteStaticMemberIdentity(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(711)
	target := symbol.ID(711)
	argExpr := factflow.ExprRef(711)
	targetPath := pathdom.NewPath(target, "obj")
	rootID := identity.ID{Kind: "lua.table", Site: "escape-placement", Index: 24}
	childID := identity.ID{Kind: "lua.table", Site: "escape-placement", Index: 25}
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
						{Target: pathdom.NewPlaceholder(0), Kind: callboundary.EscapeEventSend},
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

	if gotPlacement := got.ReadPlacement(rootID); gotPlacement != placement.SharedHeap {
		t.Fatalf("placement[%v] = %s, want %s", rootID, gotPlacement, placement.SharedHeap)
	}
	if gotPlacement := got.ReadPlacement(childID); gotPlacement != placement.Bottom {
		t.Fatalf("placement[%v] = %s, want %s for non-recursive escape", childID, gotPlacement, placement.Bottom)
	}
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

func TestFactsNodeTransferCallOutcomeFrozenTableFactUsesWIRPathArgumentMarker(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(7041)
	target := symbol.ID(7041)
	targetPath := pathdom.NewPath(target, "obj")
	tableID := identity.ID{Kind: "lua.table", Site: "freeze-wir-path", Index: 1}
	tableValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(tableID))
	source, ok := factflow.NewPathValueSource(targetPath.Key(), 0, 0, 0, factflow.ValueSourceShape{})
	if !ok {
		t.Fatalf("NewPathValueSource(%q) failed", targetPath.Key())
	}
	builder := visibility.NewBuilder()
	builder.Define(point, target, "obj")
	resolver := visibility.NewResolver(builder.Build())

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			CallSites: map[cfg.Point]factflow.CallSite{
				point: factflow.NewCallSite(factflow.CallSiteConfig{
					Context:         factflow.CallSiteContextStatement,
					ArgumentSources: []factflow.ValueSource{source},
				}),
			},
		}),
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
	itemsStateKey, ok := factKeyspaceKeyAt(resolver, point, itemsPath)
	if !ok {
		t.Fatal("missing items state key")
	}
	liveItemType := typetable.NewRecord().
		Field("id", typ.String).
		Field("child", typetable.NewRecord().Field("meta", typetable.NewRecord().Build()).Build()).
		Build()
	liveItemValue := typevalue.WithWitness(reg, typevalue.FromType(reg, liveItemType), liveItemType)
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(batch), batchValue).
		WritePathKey(reg, ks, itemsPathKey, presentValue(reg)).
		WritePathKey(reg, ks, itemPathKey, presentValue(reg)).
		WriteDynamicIndexFact(reg, dynamicindex.Key{Table: itemsStateKey, Site: "live.type-only.write"}, dynamicindex.Fact{
			KeyPresence: presence.Present(),
			KeyValue:    routeKeyValue,
			Value:       liveItemValue,
			Admission:   dynamicindex.AdmissionAdmitted,
		}).
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
	projectedItem, ok := projectPathDynamicIndexValue(reg, resolver, point, in, itemPath)
	if !ok {
		t.Fatalf("dynamic item projection missing with compatible live and heap dynamic facts")
	}
	if gotID, ok := product.Get(reg, projectedItem, identity.Key).ID(); !ok || gotID != itemID {
		t.Fatalf("dynamic item projection identity = %v/%v, want %v", gotID, ok, itemID)
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

func TestProjectPathDynamicIndexValueUsesHeapTemplateWhenLivePathProvesPresent(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(70801)
	batch := symbol.ID(70801)
	batchPath := pathdom.NewPath(batch, "batch")
	itemsPath := batchPath.Field("items")
	itemPath := itemsPath.IndexStr("route-1")
	batchID := identity.ID{Kind: "lua.table", Site: "dynamic-template", Index: 1}
	itemsID := identity.ID{Kind: "lua.table", Site: "dynamic-template", Index: 2}
	itemID := identity.ID{Kind: "lua.table", Site: "dynamic-template", Index: 3}
	batchValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(batchID))
	itemsValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(itemsID))
	itemValue := product.Set(reg, typeValue(reg, typetable.NewRecord().Field("id", typ.String).Build()), identity.Key, identity.Singleton(itemID))
	routeKeyValue := typeValue(reg, typ.LiteralString("route-1"))
	templateKeyValue := typeValue(reg, typ.String)
	liveItemType := typetable.NewRecord().Field("id", typ.String).Build()
	liveItemValue := typevalue.WithWitness(reg, typevalue.FromType(reg, liveItemType), liveItemType)

	builder := visibility.NewBuilder()
	builder.Define(point, batch, "batch")
	resolver := visibility.NewResolver(builder.Build())
	ks := resolver.KeySpace()
	itemsMemberKey, ok := heapidentity.StaticMemberSuffixKey(ks, fieldSuffix("items").Segments)
	if !ok {
		t.Fatal("missing items suffix key")
	}
	itemsStateKey, ok := factKeyspaceKeyAt(resolver, point, itemsPath)
	if !ok {
		t.Fatal("missing items state key")
	}
	itemLocalKey, ok := ks.FromPathKey(resolver.KeyAt(point, itemPath))
	if !ok {
		t.Fatal("missing item local key")
	}
	itemCanonicalKey, ok := ks.FieldCanonical(itemLocalKey)
	if !ok {
		t.Fatal("missing item field canonical key")
	}
	projected := state.State{}.
		WriteValue(reg, key.SymbolValue(batch), batchValue).
		WriteLocalPathKey(reg, itemCanonicalKey, product.NewWithPresence(reg, product.ShapeTop, presence.Present())).
		WriteDynamicIndexFact(reg, dynamicindex.Key{Table: itemsStateKey, Site: "live-exact"}, dynamicindex.Fact{
			KeyPresence: presence.Present(),
			KeyValue:    routeKeyValue,
			Value:       liveItemValue,
			Admission:   dynamicindex.AdmissionAdmitted,
		}).
		WriteHeapTableObject(reg, batchID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root:          batchValue,
			StaticMembers: map[keyspace.Key]product.Value{itemsMemberKey: itemsValue},
		})).
		WriteHeapTableObject(reg, itemsID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: itemsValue,
			DynamicIndexFacts: map[dynamicindex.Key]dynamicindex.Fact{
				{Table: mustStateKey(t, ks, pathdom.PathKey("template.items")), Site: "template"}: {
					KeyPresence: presence.Present(),
					KeyValue:    templateKeyValue,
					Value:       itemValue,
					Admission:   dynamicindex.AdmissionAdmitted,
				},
			},
		})).
		WriteHeapTableObject(reg, itemID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: itemValue}))

	value, ok := projectPathDynamicIndexValue(reg, resolver, point, projected, itemPath)
	if !ok {
		t.Fatalf("projectPathDynamicIndexValue missing for live exact path plus heap template")
	}
	if gotID, ok := product.Get(reg, value, identity.Key).ID(); !ok || gotID != itemID {
		t.Fatalf("projected identity = %v/%v, want %v in %s", gotID, ok, itemID, formatValue(reg, value))
	}
}

func TestFactsNodeTransferCallOutcomeEscapeUsesHeapTemplateWhenLivePathProvesPresent(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(70802)
	batch := symbol.ID(70802)
	argExpr := factflow.ExprRef(70802)
	batchPath := pathdom.NewPath(batch, "batch")
	itemsPath := batchPath.Field("items")
	itemPath := itemsPath.IndexStr("route-1")
	batchID := identity.ID{Kind: "lua.table", Site: "dynamic-template-escape", Index: 1}
	itemsID := identity.ID{Kind: "lua.table", Site: "dynamic-template-escape", Index: 2}
	itemID := identity.ID{Kind: "lua.table", Site: "dynamic-template-escape", Index: 3}
	childID := identity.ID{Kind: "lua.table", Site: "dynamic-template-escape", Index: 4}
	batchValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(batchID))
	itemsValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(itemsID))
	itemValue := product.Set(reg, typeValue(reg, typetable.NewRecord().Field("id", typ.String).Build()), identity.Key, identity.Singleton(itemID))
	childValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(childID))
	routeKeyValue := typeValue(reg, typ.LiteralString("route-1"))
	templateKeyValue := typeValue(reg, typ.String)
	liveItemType := typetable.NewRecord().Field("id", typ.String).Build()
	liveItemValue := typevalue.WithWitness(reg, typevalue.FromType(reg, liveItemType), liveItemType)

	builder := visibility.NewBuilder()
	builder.Define(point, batch, "batch")
	resolver := visibility.NewResolver(builder.Build())
	ks := resolver.KeySpace()
	itemsMemberKey, ok := heapidentity.StaticMemberSuffixKey(ks, fieldSuffix("items").Segments)
	if !ok {
		t.Fatal("missing items suffix key")
	}
	childMemberKey, ok := heapidentity.StaticMemberSuffixKey(ks, fieldSuffix("child").Segments)
	if !ok {
		t.Fatal("missing child suffix key")
	}
	itemsStateKey, ok := factKeyspaceKeyAt(resolver, point, itemsPath)
	if !ok {
		t.Fatal("missing items state key")
	}
	itemLocalKey, ok := ks.FromPathKey(resolver.KeyAt(point, itemPath))
	if !ok {
		t.Fatal("missing item local key")
	}
	itemCanonicalKey, ok := ks.FieldCanonical(itemLocalKey)
	if !ok {
		t.Fatal("missing item field canonical key")
	}
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(batch), batchValue).
		WriteLocalPathKey(reg, itemCanonicalKey, product.NewWithPresence(reg, product.ShapeTop, presence.Present())).
		WriteDynamicIndexFact(reg, dynamicindex.Key{Table: itemsStateKey, Site: "live-exact"}, dynamicindex.Fact{
			KeyPresence: presence.Present(),
			KeyValue:    routeKeyValue,
			Value:       liveItemValue,
			Admission:   dynamicindex.AdmissionAdmitted,
		}).
		WriteHeapTableObject(reg, batchID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root:          batchValue,
			StaticMembers: map[keyspace.Key]product.Value{itemsMemberKey: itemsValue},
		})).
		WriteHeapTableObject(reg, itemsID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: itemsValue,
			DynamicIndexFacts: map[dynamicindex.Key]dynamicindex.Fact{
				{Table: mustStateKey(t, ks, pathdom.PathKey("template.items")), Site: "template"}: {
					KeyPresence: presence.Present(),
					KeyValue:    templateKeyValue,
					Value:       itemValue,
					Admission:   dynamicindex.AdmissionAdmitted,
				},
			},
		})).
		WriteHeapTableObject(reg, itemID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root:          itemValue,
			StaticMembers: map[keyspace.Key]product.Value{childMemberKey: childValue},
		})).
		WriteHeapTableObject(reg, childID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: childValue}))

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
	})(transfer.NodeContext{Registry: reg, Point: point}, in)

	if gotPlacement := got.ReadPlacement(itemID); gotPlacement != placement.SharedHeap {
		t.Fatalf("placement[%v] = %s, want %s", itemID, gotPlacement, placement.SharedHeap)
	}
	if gotPlacement := got.ReadPlacement(childID); gotPlacement != placement.SharedHeap {
		t.Fatalf("placement[%v] = %s, want %s", childID, gotPlacement, placement.SharedHeap)
	}
}

func TestFactsNodeTransferCallOutcomeEscapePromotesPossibleDynamicValueWithoutPresenceProof(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(7081)
	items := symbol.ID(7081)
	argExpr := factflow.ExprRef(7081)
	itemsPath := pathdom.NewPath(items, "items")
	itemID := identity.ID{Kind: "lua.table", Site: "escape-placement", Index: 31}
	itemsValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(identity.ID{Kind: "lua.table", Site: "escape-placement", Index: 30}))
	itemValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(itemID))
	builder := visibility.NewBuilder()
	builder.Define(point, items, "items")
	resolver := visibility.NewResolver(builder.Build())
	ks := resolver.KeySpace()
	itemsKey, ok := factKeyspaceKeyAt(resolver, point, itemsPath)
	if !ok {
		t.Fatal("missing items state key")
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
				argExpr: itemsPath,
			},
		}),
		CallOutcome: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
			return callpayload.CallOutcome{
				NormalReturnFacts: callboundary.NormalReturnFacts{
					EscapeEvents: []callboundary.EscapeEventFact{
						{Target: pathdom.NewPlaceholder(0).IndexStr("route-1"), Kind: callboundary.EscapeEventSend, Recursive: true},
					},
				},
			}
		},
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{}.
		WriteValue(reg, key.SymbolValue(items), itemsValue).
		WriteDynamicIndexFact(reg, dynamicindex.Key{Table: itemsKey, Site: "maybe-dynamic"}, dynamicindex.Fact{
			KeyPresence: presence.Present(),
			KeyValue:    typeValue(reg, typ.String),
			Value:       itemValue,
			Admission:   dynamicindex.AdmissionAdmitted,
		}).
		WriteHeapTableObject(reg, itemID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: itemValue,
		})).
		WritePathKey(reg, ks, resolver.KeyAt(point, itemsPath), presentValue(reg)))

	if gotPlacement := got.ReadPlacement(itemID); gotPlacement != placement.SharedHeap {
		t.Fatalf("placement[%v] = %s, want %s", itemID, gotPlacement, placement.SharedHeap)
	}
}

func TestFactsNodeTransferCallOutcomeEscapeBindsSparseThirdArgumentDynamicPath(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(70815)
	items := symbol.ID(70815)
	argExpr := factflow.ExprRef(70815)
	itemsPath := pathdom.NewPath(items, "items")
	itemPath := itemsPath.IndexStr("route-1")
	itemID := identity.ID{Kind: "lua.table", Site: "escape-placement-sparse", Index: 31}
	itemsValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(identity.ID{Kind: "lua.table", Site: "escape-placement-sparse", Index: 30}))
	itemValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(itemID))
	builder := visibility.NewBuilder()
	builder.Define(point, items, "items")
	resolver := visibility.NewResolver(builder.Build())
	itemsKey, ok := factKeyspaceKeyAt(resolver, point, itemsPath)
	if !ok {
		t.Fatal("missing items state key")
	}
	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			CallSites: map[cfg.Point]factflow.CallSite{
				point: factflow.NewCallSite(factflow.CallSiteConfig{
					Context: factflow.CallSiteContextStatement,
					ArgumentSources: []factflow.ValueSource{
						{Kind: factflow.ValueSourceUnknown},
						{Kind: factflow.ValueSourceUnknown},
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
						{Target: pathdom.NewPlaceholder(2), Kind: callboundary.EscapeEventSend, Recursive: true},
					},
				},
			}
		},
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{}.
		WriteValue(reg, key.SymbolValue(items), itemsValue).
		WriteDynamicIndexFact(reg, dynamicindex.Key{Table: itemsKey, Site: "maybe-dynamic"}, dynamicindex.Fact{
			KeyPresence: presence.Present(),
			KeyValue:    typeValue(reg, typ.String),
			Value:       itemValue,
			Admission:   dynamicindex.AdmissionAdmitted,
		}).
		WriteHeapTableObject(reg, itemID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: itemValue,
		})))

	if gotPlacement := got.ReadPlacement(itemID); gotPlacement != placement.SharedHeap {
		t.Fatalf("placement[%v] = %s, want %s", itemID, gotPlacement, placement.SharedHeap)
	}
}

func TestFactsNodeTransferCallOutcomeEscapePromotesPossibleHeapDynamicValueThroughStaticParent(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(7082)
	batch := symbol.ID(7082)
	argExpr := factflow.ExprRef(7082)
	batchPath := pathdom.NewPath(batch, "batch")
	itemPath := batchPath.Field("items").IndexStr("route-1")
	batchID := identity.ID{Kind: "lua.table", Site: "escape-placement", Index: 40}
	itemsID := identity.ID{Kind: "lua.table", Site: "escape-placement", Index: 41}
	itemID := identity.ID{Kind: "lua.table", Site: "escape-placement", Index: 42}
	batchValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(batchID))
	itemsValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(itemsID))
	itemValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(itemID))
	builder := visibility.NewBuilder()
	builder.Define(point, batch, "batch")
	resolver := visibility.NewResolver(builder.Build())
	ks := resolver.KeySpace()
	itemsMemberKey, ok := heapidentity.StaticMemberSuffixKey(ks, fieldSuffix("items").Segments)
	if !ok {
		t.Fatal("missing items suffix key")
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
	}, state.State{}.
		WriteValue(reg, key.SymbolValue(batch), batchValue).
		WriteHeapTableObject(reg, batchID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root:          batchValue,
			StaticMembers: map[keyspace.Key]product.Value{itemsMemberKey: itemsValue},
		})).
		WriteHeapTableObject(reg, itemsID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: itemsValue,
			DynamicIndexFacts: map[dynamicindex.Key]dynamicindex.Fact{
				{Table: mustStateKey(t, ks, pathdom.PathKey("template.items")), Site: "maybe-dynamic"}: {
					KeyPresence: presence.Present(),
					KeyValue:    typeValue(reg, typ.String),
					Value:       itemValue,
					Admission:   dynamicindex.AdmissionAdmitted,
				},
			},
		})).
		WriteHeapTableObject(reg, itemID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: itemValue,
		})))

	if gotPlacement := got.ReadPlacement(itemID); gotPlacement != placement.SharedHeap {
		t.Fatalf("placement[%v] = %s, want %s", itemID, gotPlacement, placement.SharedHeap)
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

func TestFactsEdgeTransferCallOutcomeSkipsProviderWhenBranchCannotUseResultRelation(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	assignValue := graph.AddNode(cfg.NodeAssign)
	assignErr := graph.AddNode(cfg.NodeAssign)
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, assignValue, false)
	graph.AddEdge(assignValue, assignErr, false)
	graph.AddEdge(assignErr, branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)

	value := symbol.ID(691)
	err := symbol.ID(692)
	valuePath := pathdom.NewPath(value, "value")
	errPath := pathdom.NewPath(err, "err")
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
	})
	calls := 0
	edgeTransfer := NewFactsEdgeTransfer(FactsEdgeTransferConfig{
		Facts: facts,
		CallOutcome: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
			calls++
			return callOutcomeReturnPresenceProvider()(transfer.NodeContext{}, factflow.CallSiteView{}, state.State{}, nil)
		},
	})
	in := state.State{}.WriteValue(reg, key.SymbolValue(value), product.Top())

	got := edgeTransfer(transfer.EdgeContext{
		Graph:    graph,
		Registry: reg,
		Edge:     cfg.Edge{From: branch, To: thenPoint, Cond: true},
		HasCond:  true,
	}, in)

	if calls != 0 {
		t.Fatalf("call outcome provider calls = %d, want 0 for branch with no result-target refinement", calls)
	}
	assertStateEqual(t, reg, got, in)
}

func TestFactsEdgeTransferCallOutcomeReturnPresenceUsesFalsyAbsentStringGuard(t *testing.T) {
	reg := standard.Registry()
	graph, facts, branch, thenPoint, elsePoint, valuePath, value := callOutcomeReturnPresenceGraph(reg, false, false, true)
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

func TestFactsEdgeTransferCallOutcomeReturnPresenceKeepsFalsyAbsentErrorEdgeReachable(t *testing.T) {
	reg := standard.Registry()
	graph, facts, branch, _, elsePoint, valuePath, value := callOutcomeReturnPresenceGraph(reg, false, false, true)
	err := symbol.ID(632)
	edgeTransfer := NewFactsEdgeTransfer(FactsEdgeTransferConfig{
		Facts:       facts,
		CallOutcome: callOutcomeReturnPresenceProvider(),
	})
	valueType := typetable.NewRecord().Field("release", typ.Func().Build()).Build()
	valueState := product.WithPresence(reg, typevalue.WithWitness(reg, typevalue.FromType(reg, valueType), valueType), presence.Maybe())
	errState := product.WithPresence(reg, typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String), presence.Maybe())
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(value), valueState).
		WriteValue(reg, key.SymbolValue(err), errState)

	falseOut := edgeTransfer(transfer.EdgeContext{
		Graph:    graph,
		Registry: reg,
		Edge:     cfg.Edge{From: branch, To: elsePoint, Cond: false},
		HasCond:  true,
	}, in)

	if state.Domain(reg).Equal(falseOut, state.Domain(reg).Bottom()) {
		t.Fatal("false edge was killed, but error-absent result arm is reachable")
	}
	gotValue := falseOut.ReadValue(reg, key.SymbolValue(valuePath.Symbol))
	if gotPresence := product.PresenceOf(gotValue); !presence.Equal(gotPresence, presence.Present()) {
		t.Fatalf("value presence = %s (%s), want present", gotPresence, formatValue(reg, gotValue))
	}
	gotErr := falseOut.ReadValue(reg, key.SymbolValue(err))
	if gotPresence := product.PresenceOf(gotErr); !presence.Equal(gotPresence, presence.Absent()) {
		t.Fatalf("err presence = %s (%s), want absent", gotPresence, formatValue(reg, gotErr))
	}
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

func TestFactsEdgeTransferCallOutcomeAppliesReturnConditionSlotRefinement(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	assignReady := graph.AddNode(cfg.NodeAssign)
	assignErr := graph.AddNode(cfg.NodeAssign)
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, assignReady, false)
	graph.AddEdge(assignReady, assignErr, false)
	graph.AddEdge(assignErr, branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)

	ready := symbol.ID(645)
	err := symbol.ID(646)
	readyPath := pathdom.NewPath(ready, "ready")
	errPath := pathdom.NewPath(err, "err")
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			call: factflow.NewCallSite(factflow.CallSiteConfig{
				Context: factflow.CallSiteContextAssignmentSource,
				ResultTargets: []factflow.CallResultTarget{
					factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, 0, 0, ready, readyPath),
					factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, 1, 1, err, errPath),
				},
			}),
		},
		RootAssignments: map[cfg.Point]factflow.RootAssignment{
			assignReady: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, ready, readyPath, factflow.ValueSource{
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
		BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
			branch: factflow.NewBranchRefinementSet(
				factflow.NewBranchRefinement(
					readyPath,
					factflow.NewValueConstraint(typeValue(reg, typ.False)), true,
					factflow.NewValueConstraint(typeValue(reg, typ.True)), true,
				),
			),
		},
	})
	edgeTransfer := NewFactsEdgeTransfer(FactsEdgeTransferConfig{
		Facts: facts,
		CallOutcome: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
			return callpayload.CallOutcome{
				ReturnConditionSlots: []callpayload.CallReturnConditionSlotRefinement{
					{ReturnIndex: 0, ReturnValue: false, TargetIndex: 1, Value: typeValue(reg, typ.String)},
				},
			}
		},
	})
	optionalString := typeValue(reg, typeexpr.Optional(typ.String))
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(ready), typeValue(reg, typ.Boolean)).
		WriteValue(reg, key.SymbolValue(err), optionalString)

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

	assertValue(t, reg, trueOut, key.SymbolValue(err), typeValue(reg, typ.String))
	assertValue(t, reg, falseOut, key.SymbolValue(err), optionalString)
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
	facts := factflow.NewFacts(factflow.FactsInput{
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
	options ...bool,
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
	if len(options) != 0 && options[0] {
		valuePath.Version = 1
		errPath.Version = 1
	}
	branchRefinement := factflow.NewBranchRefinement(
		branchErrPath,
		factflow.NewValueConstraint(product.NewWithPresence(reg, product.ShapeTop, presence.Present())), true,
		factflow.NewValueConstraint(product.NewWithPresence(reg, product.ShapeTop, presence.Absent())), true,
	)
	if len(options) > 1 && options[1] {
		branchRefinement = factflow.NewBranchRefinement(
			branchErrPath,
			factflow.NewValueConstraint(product.NewWithPresence(reg, product.ShapeTop, presence.Present())), true,
			factflow.NewFalsyAbsentConstraint(product.NewWithPresence(reg, product.ShapeTop, presence.Absent())), true,
		)
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
			branch: factflow.NewBranchRefinementSet(branchRefinement),
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
