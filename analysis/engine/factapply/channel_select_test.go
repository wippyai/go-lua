package factapply

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/ssa"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/access"
	"github.com/wippyai/go-lua/analysis/type/ambient"
	"github.com/wippyai/go-lua/analysis/type/channelselect"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestChannelSelectResultPathFromChannel(t *testing.T) {
	result := symbol.ID(719)
	p := pathdom.NewPath(result, "result").Field(channelselect.ResultChannelField)

	got, ok := channelSelectResultPathFromChannel(p)
	if !ok {
		t.Fatal("channelSelectResultPathFromChannel returned false")
	}
	if got.Root != "result" || got.Symbol != result || len(got.Segments) != 0 {
		t.Fatalf("result path = %#v, want root result with no suffix", got)
	}
}

func TestFactsNodeTransferMaterializesChannelSelectResultCases(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(720)
	result := symbol.ID(721)
	events := symbol.ID(722)
	stop := symbol.ID(723)
	resultPath := pathdom.NewPath(result, "result")
	eventsPath := pathdom.NewPath(events, "events_ch")
	stopPath := pathdom.NewPath(stop, "stop_ch")
	selectID := factflow.ChannelSelectID("select-1")
	eventPayload := typetable.NewRecord().Field("kind", typ.String).Build()
	stopPayload := typetable.NewRecord().Field("reason", typ.String).Build()

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			ChannelSelects: map[cfg.Point]factflow.ChannelSelectSet{
				point: channelSelectEvents(reg, selectID, resultPath, eventsPath, stopPath, eventPayload, stopPayload),
			},
		}),
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	resultValue := got.ReadReturnSlot(reg, 0)
	assertChannelSelectCasePayload(t, reg, resultValue, channelselectfact.ID(selectID), 0, eventPayload)
	assertChannelSelectCasePayload(t, reg, resultValue, channelselectfact.ID(selectID), 1, stopPayload)
}

func TestFactsNodeTransferMaterializesChannelSelectDefaultResultCase(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(721)
	result := symbol.ID(724)
	resultPath := pathdom.NewPath(result, "result")
	selectID := factflow.ChannelSelectID("select-default")

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			ChannelSelects: map[cfg.Point]factflow.ChannelSelectSet{
				point: factflow.NewChannelSelectSet(
					factflow.NewChannelSelect(factflow.ChannelSelectConfig{
						SelectID:      selectID,
						Kind:          factflow.ChannelSelectSelect,
						ResultPath:    resultPath,
						HasResultPath: true,
						HasDefault:    true,
						Index:         0,
					}),
				),
			},
		}),
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	resultValue := got.ReadReturnSlot(reg, 0)
	assertChannelSelectCasePayload(t, reg, resultValue, channelselectfact.ID(selectID), channelselect.DefaultCaseIndex, typ.Nil)
}

func TestFactsNodeTransferMaterializesChannelSelectResultFromVisibleCasePath(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(730)
	result := symbol.ID(731)
	events := symbol.ID(732)
	resultPath := pathdom.NewPath(result, "result")
	eventsPath := pathdom.NewPath(events, "events_ch")
	selectID := factflow.ChannelSelectID("select-visible-case")
	eventPayload := typetable.NewRecord().Field("kind", typ.String).Build()

	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, result, "result")
	visibilityBuilder.Define(point, events, "events_ch")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	initial := state.State{}.
		WriteValue(reg, key.SymbolValue(events), typeValue(reg, typ.Instantiate(ambient.ChannelGeneric(), eventPayload)))

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			ChannelSelects: map[cfg.Point]factflow.ChannelSelectSet{
				point: factflow.NewChannelSelectSet(
					factflow.NewChannelSelect(factflow.ChannelSelectConfig{
						SelectID:      selectID,
						Kind:          factflow.ChannelSelectSelect,
						ResultPath:    resultPath,
						HasResultPath: true,
						Index:         0,
					}),
					factflow.NewChannelSelect(factflow.ChannelSelectConfig{
						SelectID:    selectID,
						Kind:        factflow.ChannelSelectReceive,
						ResultPath:  resultPath,
						CasePath:    eventsPath,
						HasCasePath: true,
						Index:       0,
					}),
				),
			},
		}),
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, initial)

	resultValue := got.ReadReturnSlot(reg, 0)
	assertChannelSelectCasePayload(t, reg, resultValue, channelselectfact.ID(selectID), 0, eventPayload)
}

func TestFactsNodeTransferMaterializesChannelSelectResultFromStructuralNestedCasePath(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(731)
	result := symbol.ID(733)
	route := symbol.ID(734)
	resultPath := pathdom.NewPath(result, "result")
	routePath := pathdom.NewPath(route, "route")
	casePath := routePath.Field("ch")
	selectID := factflow.ChannelSelectID("select-nested-case")
	eventPayload := typetable.NewRecord().Field("kind", typ.LiteralString("event")).Build()
	routeType := typetable.NewRecord().
		Field("kind", typ.LiteralString("route")).
		Field("ch", typ.Instantiate(ambient.ChannelGeneric(), eventPayload)).
		Build()

	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, result, "result")
	visibilityBuilder.Define(point, route, "route")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	initial := state.State{}.
		WriteValue(reg, key.SymbolValue(route), typeValue(reg, routeType))

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			ChannelSelects: map[cfg.Point]factflow.ChannelSelectSet{
				point: factflow.NewChannelSelectSet(
					factflow.NewChannelSelect(factflow.ChannelSelectConfig{
						SelectID:      selectID,
						Kind:          factflow.ChannelSelectSelect,
						ResultPath:    resultPath,
						HasResultPath: true,
						Index:         0,
					}),
					factflow.NewChannelSelect(factflow.ChannelSelectConfig{
						SelectID:    selectID,
						Kind:        factflow.ChannelSelectReceive,
						ResultPath:  resultPath,
						CasePath:    casePath,
						HasCasePath: true,
						Index:       0,
					}),
				),
			},
		}),
		Visibility:  resolver,
		ProjectPath: testLuaPathTypeProjector,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, initial)

	resultValue := got.ReadReturnSlot(reg, 0)
	assertChannelSelectCasePayload(t, reg, resultValue, channelselectfact.ID(selectID), 0, eventPayload)
}

func TestFactsEdgeTransferChannelSelectEqualityNarrowsPayload(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)

	result := symbol.ID(724)
	events := symbol.ID(725)
	stop := symbol.ID(726)
	resultPath := pathdom.NewPath(result, "result")
	eventsPath := pathdom.NewPath(events, "events_ch")
	stopPath := pathdom.NewPath(stop, "stop_ch")
	selectID := factflow.ChannelSelectID("select-branch")
	eventPayload := typetable.NewRecord().Field("kind", typ.String).Build()
	stopPayload := typetable.NewRecord().Field("reason", typ.String).Build()
	resultValue, ok := testChannelSelectResultValue(reg, selectID, channelSelectEvents(reg, selectID, resultPath, eventsPath, stopPath, eventPayload, stopPayload).Events())
	if !ok {
		t.Fatal("failed to build channel select result value")
	}

	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(branch, result, "result")
	visibilityBuilder.Define(branch, events, "events_ch")
	visibilityBuilder.Define(branch, stop, "stop_ch")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	initial := state.State{}.
		WriteValue(reg, key.SymbolValue(result), resultValue).
		WritePathKey(reg, pathdom.PathKey("sym724@1.value"), typeValue(reg, stopPayload)).
		AddChannelSelectFact(channelselectfact.Fact{
			Select: channelselectfact.ID(selectID),
			Kind:   channelselectfact.FactReceive,
			Result: pathdom.PathKey("sym724@1"),
			Case:   pathdom.PathKey("sym725@1"),
			Index:  0,
		}).
		AddChannelSelectFact(channelselectfact.Fact{
			Select: channelselectfact.ID(selectID),
			Kind:   channelselectfact.FactReceive,
			Result: pathdom.PathKey("sym724@1"),
			Case:   pathdom.PathKey("sym726@1"),
			Index:  1,
		})

	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchPathRelations: map[cfg.Point]factflow.BranchPathRelationSet{
					branch: factflow.NewBranchPathRelationSet(
						factflow.NewBranchPathEquality(resultPath.Field("channel"), eventsPath, true, false),
					),
				},
			}),
			Visibility: resolver,
		}),
	})

	thenValue := got[thenPoint].ReadValue(reg, key.SymbolValue(result))
	assertChannelSelectCasePayload(t, reg, thenValue, channelselectfact.ID(selectID), 0, eventPayload)
	if got := got[thenPoint].ReadPathKey(reg, pathdom.PathKey("sym724@1.value")); !product.Equal(reg, got, product.Bottom(reg)) {
		t.Fatalf("stale result.value path = %s, want bottom", formatValue(reg, got))
	}
}

func TestFactsEdgeTransferChannelSelectEqualityKeepsDuplicateCasePathIndexes(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)

	result := symbol.ID(744)
	events := symbol.ID(745)
	stop := symbol.ID(746)
	resultPath := pathdom.NewPath(result, "result")
	eventsPath := pathdom.NewPath(events, "events_ch")
	stopPath := pathdom.NewPath(stop, "stop_ch")
	selectID := factflow.ChannelSelectID("select-duplicate-equality")
	eventPayload := typetable.NewRecord().Field("kind", typ.LiteralString("event")).Build()
	retryPayload := typetable.NewRecord().Field("kind", typ.LiteralString("retry")).Build()
	stopPayload := typetable.NewRecord().Field("reason", typ.String).Build()
	eventsSet := channelSelectDuplicateEvents(reg, selectID, resultPath, eventsPath, stopPath, eventPayload, retryPayload, stopPayload)
	resultValue, ok := testChannelSelectResultValue(reg, selectID, eventsSet.Events())
	if !ok {
		t.Fatal("failed to build channel select result value")
	}

	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(branch, result, "result")
	visibilityBuilder.Define(branch, events, "events_ch")
	visibilityBuilder.Define(branch, stop, "stop_ch")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	initial := state.State{}.
		WriteValue(reg, key.SymbolValue(result), resultValue).
		AddChannelSelectFact(channelselectfact.Fact{
			Select: channelselectfact.ID(selectID),
			Kind:   channelselectfact.FactReceive,
			Result: pathdom.PathKey("sym744@1"),
			Case:   pathdom.PathKey("sym745@1"),
			Index:  0,
		}).
		AddChannelSelectFact(channelselectfact.Fact{
			Select: channelselectfact.ID(selectID),
			Kind:   channelselectfact.FactReceive,
			Result: pathdom.PathKey("sym744@1"),
			Case:   pathdom.PathKey("sym745@1"),
			Index:  1,
		}).
		AddChannelSelectFact(channelselectfact.Fact{
			Select: channelselectfact.ID(selectID),
			Kind:   channelselectfact.FactReceive,
			Result: pathdom.PathKey("sym744@1"),
			Case:   pathdom.PathKey("sym746@1"),
			Index:  2,
		})

	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchPathRelations: map[cfg.Point]factflow.BranchPathRelationSet{
					branch: factflow.NewBranchPathRelationSet(
						factflow.NewBranchPathEquality(resultPath.Field("channel"), eventsPath, true, false),
					),
				},
			}),
			Visibility: resolver,
		}),
	})

	thenValue := got[thenPoint].ReadValue(reg, key.SymbolValue(result))
	assertChannelSelectCasePayload(t, reg, thenValue, channelselectfact.ID(selectID), 0, eventPayload)
	assertChannelSelectCasePayload(t, reg, thenValue, channelselectfact.ID(selectID), 1, retryPayload)
	assertNoChannelSelectCasePayload(t, reg, thenValue, channelselectfact.ID(selectID), 2)
}

func TestFactsEdgeTransferChannelSelectInequalityRemovesCase(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)

	result := symbol.ID(824)
	events := symbol.ID(825)
	stop := symbol.ID(826)
	resultPath := pathdom.NewPath(result, "result")
	eventsPath := pathdom.NewPath(events, "events_ch")
	stopPath := pathdom.NewPath(stop, "stop_ch")
	selectID := factflow.ChannelSelectID("select-branch")
	eventPayload := typetable.NewRecord().Field("kind", typ.String).Build()
	stopPayload := typetable.NewRecord().Field("reason", typ.String).Build()
	resultValue, ok := testChannelSelectResultValue(reg, selectID, channelSelectEvents(reg, selectID, resultPath, eventsPath, stopPath, eventPayload, stopPayload).Events())
	if !ok {
		t.Fatal("failed to build channel select result value")
	}

	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(branch, result, "result")
	visibilityBuilder.Define(branch, events, "events_ch")
	visibilityBuilder.Define(branch, stop, "stop_ch")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	initial := state.State{}.
		WriteValue(reg, key.SymbolValue(result), resultValue).
		WritePathKey(reg, pathdom.PathKey("sym824@1.value"), typeValue(reg, eventPayload)).
		AddChannelSelectFact(channelselectfact.Fact{
			Select: channelselectfact.ID(selectID),
			Kind:   channelselectfact.FactReceive,
			Result: pathdom.PathKey("sym824@1"),
			Case:   pathdom.PathKey("sym825@1"),
			Index:  0,
		}).
		AddChannelSelectFact(channelselectfact.Fact{
			Select: channelselectfact.ID(selectID),
			Kind:   channelselectfact.FactReceive,
			Result: pathdom.PathKey("sym824@1"),
			Case:   pathdom.PathKey("sym826@1"),
			Index:  1,
		})

	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchPathRelations: map[cfg.Point]factflow.BranchPathRelationSet{
					branch: factflow.NewBranchPathRelationSet(
						factflow.NewBranchPathInequality(resultPath.Field("channel"), eventsPath, false, true),
					),
				},
			}),
			Visibility: resolver,
		}),
	})

	elseValue := got[elsePoint].ReadValue(reg, key.SymbolValue(result))
	assertNoChannelSelectCasePayload(t, reg, elseValue, channelselectfact.ID(selectID), 0)
	assertChannelSelectCasePayload(t, reg, elseValue, channelselectfact.ID(selectID), 1, stopPayload)
	if got := got[elsePoint].ReadPathKey(reg, pathdom.PathKey("sym824@1.value")); !product.Equal(reg, got, product.Bottom(reg)) {
		t.Fatalf("stale result.value path = %s, want bottom", formatValue(reg, got))
	}
}

func TestFactsEdgeTransferChannelSelectInequalityRemovesDuplicateCasePathIndexes(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)

	result := symbol.ID(844)
	events := symbol.ID(845)
	stop := symbol.ID(846)
	resultPath := pathdom.NewPath(result, "result")
	eventsPath := pathdom.NewPath(events, "events_ch")
	stopPath := pathdom.NewPath(stop, "stop_ch")
	selectID := factflow.ChannelSelectID("select-duplicate-inequality")
	eventPayload := typetable.NewRecord().Field("kind", typ.LiteralString("event")).Build()
	retryPayload := typetable.NewRecord().Field("kind", typ.LiteralString("retry")).Build()
	stopPayload := typetable.NewRecord().Field("reason", typ.String).Build()
	eventsSet := channelSelectDuplicateEvents(reg, selectID, resultPath, eventsPath, stopPath, eventPayload, retryPayload, stopPayload)
	resultValue, ok := testChannelSelectResultValue(reg, selectID, eventsSet.Events())
	if !ok {
		t.Fatal("failed to build channel select result value")
	}

	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(branch, result, "result")
	visibilityBuilder.Define(branch, events, "events_ch")
	visibilityBuilder.Define(branch, stop, "stop_ch")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	initial := state.State{}.
		WriteValue(reg, key.SymbolValue(result), resultValue).
		WritePathKey(reg, pathdom.PathKey("sym844@1.value"), typeValue(reg, eventPayload)).
		AddChannelSelectFact(channelselectfact.Fact{
			Select: channelselectfact.ID(selectID),
			Kind:   channelselectfact.FactReceive,
			Result: pathdom.PathKey("sym844@1"),
			Case:   pathdom.PathKey("sym845@1"),
			Index:  0,
		}).
		AddChannelSelectFact(channelselectfact.Fact{
			Select: channelselectfact.ID(selectID),
			Kind:   channelselectfact.FactReceive,
			Result: pathdom.PathKey("sym844@1"),
			Case:   pathdom.PathKey("sym845@1"),
			Index:  1,
		}).
		AddChannelSelectFact(channelselectfact.Fact{
			Select: channelselectfact.ID(selectID),
			Kind:   channelselectfact.FactReceive,
			Result: pathdom.PathKey("sym844@1"),
			Case:   pathdom.PathKey("sym846@1"),
			Index:  2,
		})

	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchPathRelations: map[cfg.Point]factflow.BranchPathRelationSet{
					branch: factflow.NewBranchPathRelationSet(
						factflow.NewBranchPathInequality(resultPath.Field("channel"), eventsPath, false, true),
					),
				},
			}),
			Visibility: resolver,
		}),
	})

	elseValue := got[elsePoint].ReadValue(reg, key.SymbolValue(result))
	assertNoChannelSelectCasePayload(t, reg, elseValue, channelselectfact.ID(selectID), 0)
	assertNoChannelSelectCasePayload(t, reg, elseValue, channelselectfact.ID(selectID), 1)
	assertChannelSelectCasePayload(t, reg, elseValue, channelselectfact.ID(selectID), 2, stopPayload)
	if got := got[elsePoint].ReadPathKey(reg, pathdom.PathKey("sym844@1.value")); !product.Equal(reg, got, product.Bottom(reg)) {
		t.Fatalf("stale result.value path = %s, want bottom", formatValue(reg, got))
	}
}

func TestFactsEdgeTransferChannelSelectInequalityPreservesDefaultCase(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)

	result := symbol.ID(834)
	events := symbol.ID(835)
	resultPath := pathdom.NewPath(result, "result")
	eventsPath := pathdom.NewPath(events, "events_ch")
	selectID := factflow.ChannelSelectID("select-default-branch")
	eventPayload := typetable.NewRecord().Field("kind", typ.String).Build()
	eventsSet := factflow.NewChannelSelectSet(
		factflow.NewChannelSelect(factflow.ChannelSelectConfig{
			SelectID:      selectID,
			Kind:          factflow.ChannelSelectSelect,
			ResultPath:    resultPath,
			HasResultPath: true,
			HasDefault:    true,
			Index:         0,
		}),
		channelSelectReceive(reg, selectID, resultPath, eventsPath, 0, eventPayload),
	)
	resultValue, ok := testChannelSelectResultValueWithDefault(reg, selectID, eventsSet.Events(), true)
	if !ok {
		t.Fatal("failed to build channel select result value")
	}

	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(branch, result, "result")
	visibilityBuilder.Define(branch, events, "events_ch")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	initial := state.State{}.
		WriteValue(reg, key.SymbolValue(result), resultValue).
		WritePathKey(reg, pathdom.PathKey("sym834@1.value"), typeValue(reg, eventPayload)).
		AddChannelSelectFact(channelselectfact.Fact{
			Select:     channelselectfact.ID(selectID),
			Kind:       channelselectfact.FactSelect,
			Result:     pathdom.PathKey("sym834@1"),
			Index:      0,
			HasDefault: true,
		}).
		AddChannelSelectFact(channelselectfact.Fact{
			Select: channelselectfact.ID(selectID),
			Kind:   channelselectfact.FactReceive,
			Result: pathdom.PathKey("sym834@1"),
			Case:   pathdom.PathKey("sym835@1"),
			Index:  0,
		})

	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchPathRelations: map[cfg.Point]factflow.BranchPathRelationSet{
					branch: factflow.NewBranchPathRelationSet(
						factflow.NewBranchPathInequality(resultPath.Field("channel"), eventsPath, false, true),
					),
				},
			}),
			Visibility: resolver,
		}),
	})

	elseValue := got[elsePoint].ReadValue(reg, key.SymbolValue(result))
	assertNoChannelSelectCasePayload(t, reg, elseValue, channelselectfact.ID(selectID), 0)
	assertChannelSelectCasePayload(t, reg, elseValue, channelselectfact.ID(selectID), channelselect.DefaultCaseIndex, typ.Nil)
	if got := got[elsePoint].ReadPathKey(reg, pathdom.PathKey("sym834@1.value")); !product.Equal(reg, got, product.Bottom(reg)) {
		t.Fatalf("stale result.value path = %s, want bottom", formatValue(reg, got))
	}
}

func TestFactsEdgeTransferChannelSelectEqualityMatchesDriftingVersions(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)

	result := symbol.ID(724)
	events := symbol.ID(725)
	stop := symbol.ID(726)
	resultPath := pathdom.NewPath(result, "result")
	eventsPath := pathdom.NewPath(events, "events_ch")
	stopPath := pathdom.NewPath(stop, "stop_ch")
	selectID := factflow.ChannelSelectID("select-1")
	eventPayload := typetable.NewRecord().Field("kind", typ.String).Build()
	stopPayload := typetable.NewRecord().Field("reason", typ.String).Build()
	resultValue, ok := testChannelSelectResultValue(reg, selectID, channelSelectEvents(reg, selectID, resultPath, eventsPath, stopPath, eventPayload, stopPayload).Events())
	if !ok {
		t.Fatal("failed to build channel select result value")
	}

	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(branch, result, "result")
	visibilityBuilder.Define(branch, events, "events_ch")
	visibilityBuilder.Define(branch, stop, "stop_ch")
	visibilityBuilder.SetVisible(branch, result, ssa.Version{Root: "result", Symbol: result, ID: 2})
	visibilityBuilder.SetVisible(branch, events, ssa.Version{Root: "events_ch", Symbol: events, ID: 2})
	visibilityBuilder.SetVisible(branch, stop, ssa.Version{Root: "stop_ch", Symbol: stop, ID: 2})
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	got := transfer.Run(transfer.Config{
		Graph:    graph,
		Registry: reg,
		EntryState: state.State{}.
			WriteValue(reg, key.SymbolValue(result), resultValue).
			WritePathKey(reg, pathdom.PathKey("sym724@2.value"), typeValue(reg, stopPayload)).
			AddChannelSelectFact(channelselectfact.Fact{
				Select: channelselectfact.ID(selectID),
				Kind:   channelselectfact.FactReceive,
				Result: pathdom.PathKey("sym724@1"),
				Case:   pathdom.PathKey("sym725@1"),
				Index:  0,
			}).
			AddChannelSelectFact(channelselectfact.Fact{
				Select: channelselectfact.ID(selectID),
				Kind:   channelselectfact.FactReceive,
				Result: pathdom.PathKey("sym724@1"),
				Case:   pathdom.PathKey("sym726@1"),
				Index:  1,
			}),
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchPathRelations: map[cfg.Point]factflow.BranchPathRelationSet{
					branch: factflow.NewBranchPathRelationSet(
						factflow.NewBranchPathEquality(resultPath.Field("channel"), eventsPath, true, false),
					),
				},
			}),
			Visibility: resolver,
		}),
	})

	thenValue := got[thenPoint].ReadValue(reg, key.SymbolValue(result))
	assertChannelSelectCasePayload(t, reg, thenValue, channelselectfact.ID(selectID), 0, eventPayload)
	if got := got[thenPoint].ReadPathKey(reg, pathdom.PathKey("sym724@2.value")); !product.Equal(reg, got, product.Bottom(reg)) {
		t.Fatalf("stale result.value path = %s, want bottom", formatValue(reg, got))
	}
}

func TestFactsEdgeTransferChannelSelectEqualityMatchesNestedCasePath(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)

	result := symbol.ID(924)
	source := symbol.ID(925)
	resultPath := pathdom.NewPath(result, "result")
	primaryPath := pathdom.NewPath(source, "source").Field("primary")
	timerPath := pathdom.NewPath(source, "source").Field("timers")
	selectID := factflow.ChannelSelectID("select-nested")
	eventPayload := typetable.NewRecord().Field("id", typ.String).Build()
	timerPayload := typetable.NewRecord().Field("elapsed", typ.Number).Build()
	resultValue, ok := testChannelSelectResultValue(reg, selectID, channelSelectEvents(reg, selectID, resultPath, primaryPath, timerPath, eventPayload, timerPayload).Events())
	if !ok {
		t.Fatal("failed to build channel select result value")
	}

	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(branch, result, "result")
	visibilityBuilder.Define(branch, source, "source")
	visibilityBuilder.SetVisible(branch, result, ssa.Version{Root: "result", Symbol: result, ID: 2})
	visibilityBuilder.SetVisible(branch, source, ssa.Version{Root: "source", Symbol: source, ID: 2})
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	got := transfer.Run(transfer.Config{
		Graph:    graph,
		Registry: reg,
		EntryState: state.State{}.
			WriteValue(reg, key.SymbolValue(result), resultValue).
			AddChannelSelectFact(channelselectfact.Fact{
				Select: channelselectfact.ID(selectID),
				Kind:   channelselectfact.FactReceive,
				Result: pathdom.PathKey("sym924@1"),
				Case:   pathdom.PathKey("sym925@1.primary"),
				Index:  0,
			}).
			AddChannelSelectFact(channelselectfact.Fact{
				Select: channelselectfact.ID(selectID),
				Kind:   channelselectfact.FactReceive,
				Result: pathdom.PathKey("sym924@1"),
				Case:   pathdom.PathKey("sym925@1.timers"),
				Index:  1,
			}),
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchPathRelations: map[cfg.Point]factflow.BranchPathRelationSet{
					branch: factflow.NewBranchPathRelationSet(
						factflow.NewBranchPathEquality(resultPath.Field("channel"), primaryPath, true, false),
					),
				},
			}),
			Visibility: resolver,
		}),
	})

	thenValue := got[thenPoint].ReadValue(reg, key.SymbolValue(result))
	assertChannelSelectCasePayload(t, reg, thenValue, channelselectfact.ID(selectID), 0, eventPayload)
}

func channelSelectEvents(
	reg *axis.Registry,
	selectID factflow.ChannelSelectID,
	resultPath pathdom.Path,
	firstPath pathdom.Path,
	secondPath pathdom.Path,
	firstPayload typ.Type,
	secondPayload typ.Type,
) factflow.ChannelSelectSet {
	return factflow.NewChannelSelectSet(
		factflow.NewChannelSelect(factflow.ChannelSelectConfig{
			SelectID:      selectID,
			Kind:          factflow.ChannelSelectSelect,
			ResultPath:    resultPath,
			HasResultPath: true,
			Index:         0,
		}),
		channelSelectReceive(reg, selectID, resultPath, firstPath, 0, firstPayload),
		channelSelectReceive(reg, selectID, resultPath, secondPath, 1, secondPayload),
	)
}

func channelSelectDuplicateEvents(
	reg *axis.Registry,
	selectID factflow.ChannelSelectID,
	resultPath pathdom.Path,
	duplicatePath pathdom.Path,
	otherPath pathdom.Path,
	firstPayload typ.Type,
	secondPayload typ.Type,
	otherPayload typ.Type,
) factflow.ChannelSelectSet {
	return factflow.NewChannelSelectSet(
		factflow.NewChannelSelect(factflow.ChannelSelectConfig{
			SelectID:      selectID,
			Kind:          factflow.ChannelSelectSelect,
			ResultPath:    resultPath,
			HasResultPath: true,
			Index:         0,
		}),
		channelSelectReceive(reg, selectID, resultPath, duplicatePath, 0, firstPayload),
		channelSelectReceive(reg, selectID, resultPath, duplicatePath, 1, secondPayload),
		channelSelectReceive(reg, selectID, resultPath, otherPath, 2, otherPayload),
	)
}

func channelSelectReceive(
	reg *axis.Registry,
	selectID factflow.ChannelSelectID,
	resultPath pathdom.Path,
	casePath pathdom.Path,
	index int,
	payload typ.Type,
) factflow.ChannelSelect {
	return factflow.NewChannelSelect(factflow.ChannelSelectConfig{
		SelectID:        selectID,
		Kind:            factflow.ChannelSelectReceive,
		ResultPath:      resultPath,
		HasResultPath:   true,
		CasePath:        casePath,
		HasCasePath:     true,
		PayloadValue:    typeValue(reg, payload),
		HasPayloadValue: true,
		Index:           index,
	})
}

func testChannelSelectResultValue(
	reg *axis.Registry,
	selectID factflow.ChannelSelectID,
	events []factflow.ChannelSelect,
) (product.Value, bool) {
	return testChannelSelectResultValueWithDefault(reg, selectID, events, false)
}

func testChannelSelectResultValueWithDefault(
	reg *axis.Registry,
	selectID factflow.ChannelSelectID,
	events []factflow.ChannelSelect,
	hasDefault bool,
) (product.Value, bool) {
	return channelSelectResultValue(transfer.NodeContext{Registry: reg}, nil, nil, nil, state.State{}, selectID, events, hasDefault)
}

func assertChannelSelectCasePayload(
	t *testing.T,
	reg *axis.Registry,
	value product.Value,
	selectID channelselectfact.ID,
	index int,
	want typ.Type,
) {
	t.Helper()
	resultType, ok := valueWitnessType(reg, value)
	if !ok {
		t.Fatalf("missing channel select witness in %s", formatValue(reg, value))
	}
	caseType, ok := channelselect.ResultCaseTypeFromValue(resultType, string(selectID), index)
	if !ok {
		t.Fatalf("missing channel select case %d in %s", index, formatValue(reg, value))
	}
	payloadType, ok := access.Field(caseType, channelselect.ResultValueField)
	if !ok || !typ.TypeEquals(payloadType, want) {
		t.Fatalf("case %d payload type = %v/%v, want %v", index, payloadType, ok, want)
	}
}

func assertNoChannelSelectCasePayload(
	t *testing.T,
	reg *axis.Registry,
	value product.Value,
	selectID channelselectfact.ID,
	index int,
) {
	t.Helper()
	resultType, ok := valueWitnessType(reg, value)
	if !ok {
		t.Fatalf("missing channel select witness in %s", formatValue(reg, value))
	}
	if _, ok := channelselect.ResultCaseTypeFromValue(resultType, string(selectID), index); ok {
		t.Fatalf("unexpected channel select case %d in %s", index, formatValue(reg, value))
	}
}

func typeValue(reg *axis.Registry, t typ.Type) product.Value {
	return typevalue.WithWitness(reg, typevalue.FromType(reg, t), t)
}
