package factapply

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/channelselect"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/typeaccess"
	"github.com/wippyai/go-lua/analysis/symbol"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

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
	assertChannelSelectCasePayload(t, reg, resultValue, state.ChannelSelectID(selectID), 0, eventPayload)
	assertChannelSelectCasePayload(t, reg, resultValue, state.ChannelSelectID(selectID), 1, stopPayload)
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
	resultValue, ok := channelSelectResultValue(reg, selectID, channelSelectEvents(reg, selectID, resultPath, eventsPath, stopPath, eventPayload, stopPayload).Events())
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
		AddChannelSelectFact(state.ChannelSelectFact{
			Select: state.ChannelSelectID(selectID),
			Kind:   state.ChannelSelectFactReceive,
			Result: pathdom.PathKey("sym724@1"),
			Case:   pathdom.PathKey("sym725@1"),
			Index:  0,
		}).
		AddChannelSelectFact(state.ChannelSelectFact{
			Select: state.ChannelSelectID(selectID),
			Kind:   state.ChannelSelectFactReceive,
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
	assertChannelSelectCasePayload(t, reg, thenValue, state.ChannelSelectID(selectID), 0, eventPayload)
	if got := got[thenPoint].ReadPathKey(reg, pathdom.PathKey("sym724@1.value")); !product.Equal(reg, got, product.Bottom(reg)) {
		t.Fatalf("stale result.value path = %s, want bottom", formatValue(reg, got))
	}
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

func assertChannelSelectCasePayload(
	t *testing.T,
	reg *axis.Registry,
	value product.Value,
	selectID state.ChannelSelectID,
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
	payloadType, ok := typeaccess.Field(caseType, channelselect.ResultValueField)
	if !ok || !typ.TypeEquals(payloadType, want) {
		t.Fatalf("case %d payload type = %v/%v, want %v", index, payloadType, ok, want)
	}
}

func typeValue(reg *axis.Registry, t typ.Type) product.Value {
	return typevalue.WithWitness(reg, typevalue.FromType(reg, t), t)
}
