package transferfacts

import (
	"strconv"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/type/ambient"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func (l *lowerer) channelSelects(point cfg.Point) []factflow.ChannelSelect {
	return l.channelSelectsFromWIR(point)
}

func (l *lowerer) channelSelectsFromWIR(point cfg.Point) []factflow.ChannelSelect {
	if l == nil || l.wir == nil {
		return nil
	}
	for _, inst := range l.wir.PointInstructions(point) {
		if inst.Op != wir.OpSelect {
			continue
		}
		if target, ok := l.channelSelectResultTargetFromWIR(point); ok {
			return l.channelSelectEventsFromWIR(point, inst, target)
		}
	}
	return nil
}

func (l *lowerer) channelSelectResultTargetFromWIR(point cfg.Point) (wir.CallResultTarget, bool) {
	if l == nil || l.wir == nil {
		return wir.CallResultTarget{}, false
	}
	for _, target := range l.wir.CallResultTargets(point) {
		if target.Path.IsEmpty() {
			continue
		}
		return target, true
	}
	return wir.CallResultTarget{}, false
}

func (l *lowerer) channelSelectEventsFromWIR(point cfg.Point, inst wir.Instruction, target wir.CallResultTarget) []factflow.ChannelSelect {
	if target.Path.IsEmpty() {
		return nil
	}
	selectID := factflow.ChannelSelectID("lua.channel_select@" + strconv.Itoa(int(point)))
	events := channelSelectStart(selectID, target.Path, target.ResultIndex, inst.SelectDefault)
	for i, op := range l.wir.Operands(inst.List) {
		if op.Kind != wir.OperandPath {
			continue
		}
		casePath := l.wir.Path(wir.PathRef(op.Ref))
		if casePath.IsEmpty() {
			continue
		}
		payloadValue, hasPayloadValue := l.channelSelectPayloadValue(casePath)
		events = appendChannelSelectCaseEvents(events, selectID, target.Path, i, casePath, payloadValue, hasPayloadValue)
	}
	return events
}

func channelSelectStart(
	selectID factflow.ChannelSelectID,
	resultPath pathdom.Path,
	resultIndex int,
	hasDefault bool,
) []factflow.ChannelSelect {
	if resultPath.IsEmpty() {
		return nil
	}
	events := []factflow.ChannelSelect{
		factflow.NewChannelSelect(factflow.ChannelSelectConfig{
			SelectID:      selectID,
			Kind:          factflow.ChannelSelectSelect,
			ResultPath:    resultPath,
			HasResultPath: true,
			Index:         resultIndex,
			HasDefault:    hasDefault,
		}),
	}
	return events
}

func appendChannelSelectCaseEvents(
	events []factflow.ChannelSelect,
	selectID factflow.ChannelSelectID,
	resultPath pathdom.Path,
	index int,
	casePath pathdom.Path,
	payloadValue product.Value,
	hasPayloadValue bool,
) []factflow.ChannelSelect {
	if casePath.IsEmpty() {
		return events
	}
	return append(events,
		factflow.NewChannelSelect(factflow.ChannelSelectConfig{
			SelectID:    selectID,
			Kind:        factflow.ChannelSelectCase,
			CasePath:    casePath,
			HasCasePath: true,
			Index:       index,
		}),
		factflow.NewChannelSelect(factflow.ChannelSelectConfig{
			SelectID:        selectID,
			Kind:            factflow.ChannelSelectReceive,
			ResultPath:      resultPath,
			HasResultPath:   true,
			CasePath:        casePath,
			HasCasePath:     true,
			PayloadValue:    payloadValue,
			HasPayloadValue: hasPayloadValue,
			Index:           index,
		}),
	)
}

func (l *lowerer) channelSelectPayloadValue(channelPath pathdom.Path) (product.Value, bool) {
	if l == nil || l.registry == nil {
		return product.Value{}, false
	}
	channelType, ok := l.channelSelectPathType(channelPath)
	if !ok {
		return product.Value{}, false
	}
	payloadType, ok := channelPayloadType(channelType)
	if !ok {
		return product.Value{}, false
	}
	return l.valueFromTypeWithWitness(payloadType), true
}

func (l *lowerer) channelSelectPathType(p pathdom.Path) (typ.Type, bool) {
	if l == nil || p.Symbol == 0 {
		return nil, false
	}
	current, ok := l.symbolTypes[p.Symbol]
	if !ok {
		return nil, false
	}
	return luatypeprojection.ApplySegments(current, p.Segments)
}

func channelPayloadType(channelType typ.Type) (typ.Type, bool) {
	return ambient.ChannelPayloadType(channelType)
}
