package transferfacts

import (
	"strconv"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/type/ambient"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func (l *lowerer) channelSelects(point cfg.Point, result *semantics.Result) []factflow.ChannelSelect {
	if events := l.channelSelectsFromWIR(point); len(events) != 0 {
		return events
	}
	if fact, ok := result.ChannelSelect(point); ok && fact.ResultTarget.HasPath && !fact.ResultTarget.Path.IsEmpty() {
		return l.channelSelectEvents(point, fact)
	}
	return nil
}

func (l *lowerer) channelSelectsFromWIR(point cfg.Point) []factflow.ChannelSelect {
	if l == nil || l.wir == nil {
		return nil
	}
	for _, inst := range l.wir.PointInstructions(point) {
		if inst.Op != wir.OpSelect {
			continue
		}
		if target, ok := l.wir.CallResultTarget(point, 0); ok && !target.Path.IsEmpty() {
			return l.channelSelectEventsFromWIR(point, inst, target.Path)
		}
	}
	return nil
}

func (l *lowerer) channelSelectEventsFromWIR(point cfg.Point, inst wir.Instruction, resultPath pathdom.Path) []factflow.ChannelSelect {
	if resultPath.IsEmpty() {
		return nil
	}
	selectID := factflow.ChannelSelectID("lua.channel_select@" + strconv.Itoa(int(point)))
	events := []factflow.ChannelSelect{
		factflow.NewChannelSelect(factflow.ChannelSelectConfig{
			SelectID:      selectID,
			Kind:          factflow.ChannelSelectSelect,
			ResultPath:    resultPath,
			HasResultPath: true,
			Index:         0,
			HasDefault:    inst.SelectDefault,
		}),
	}
	for i, op := range l.wir.Operands(inst.List) {
		if op.Kind != wir.OperandPath {
			continue
		}
		casePath := l.wir.Path(wir.PathRef(op.Ref))
		if casePath.IsEmpty() {
			continue
		}
		payloadValue, hasPayloadValue := l.channelSelectPayloadValue(casePath)
		events = append(events,
			factflow.NewChannelSelect(factflow.ChannelSelectConfig{
				SelectID:    selectID,
				Kind:        factflow.ChannelSelectCase,
				CasePath:    casePath,
				HasCasePath: true,
				Index:       i,
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
				Index:           i,
			}),
		)
	}
	return events
}

func (l *lowerer) channelSelectEvents(point cfg.Point, fact semantics.ChannelSelectFact) []factflow.ChannelSelect {
	if !fact.ResultTarget.HasPath || fact.ResultTarget.Path.IsEmpty() {
		return nil
	}
	selectID := factflow.ChannelSelectID("lua.channel_select@" + strconv.Itoa(int(point)))
	events := []factflow.ChannelSelect{
		factflow.NewChannelSelect(factflow.ChannelSelectConfig{
			SelectID:      selectID,
			Kind:          factflow.ChannelSelectSelect,
			ResultPath:    fact.ResultTarget.Path,
			HasResultPath: true,
			Index:         fact.ResultTarget.ResultIndex,
			HasDefault:    fact.HasDefault,
		}),
	}
	for i, c := range fact.Cases {
		if !c.HasChannelPath || c.ChannelPath.IsEmpty() {
			continue
		}
		payloadValue, hasPayloadValue := l.channelSelectPayloadValue(c.ChannelPath)
		events = append(events,
			factflow.NewChannelSelect(factflow.ChannelSelectConfig{
				SelectID:    selectID,
				Kind:        factflow.ChannelSelectCase,
				CasePath:    c.ChannelPath,
				HasCasePath: true,
				Index:       i,
			}),
			factflow.NewChannelSelect(factflow.ChannelSelectConfig{
				SelectID:        selectID,
				Kind:            factflow.ChannelSelectReceive,
				ResultPath:      fact.ResultTarget.Path,
				HasResultPath:   true,
				CasePath:        c.ChannelPath,
				HasCasePath:     true,
				PayloadValue:    payloadValue,
				HasPayloadValue: hasPayloadValue,
				Index:           i,
			}),
		)
	}
	return events
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
