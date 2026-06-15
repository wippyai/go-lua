package transferfacts

import (
	"strconv"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/channelruntime"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (l *lowerer) channelSelects(point cfg.Point, result *semantics.Result) []factflow.ChannelSelect {
	if fact, ok := result.ChannelSelect(point); ok && fact.ResultTarget.HasPath && !fact.ResultTarget.Path.IsEmpty() {
		return l.channelSelectEvents(point, fact)
	}
	call, ok := result.Call(point)
	if !ok {
		return nil
	}
	fact, ok := l.channelSelectFactFromCall(call)
	if !ok {
		return nil
	}
	return l.channelSelectEvents(point, fact)
}

func (l *lowerer) channelSelectFactFromCall(fact semantics.CallFact) (semantics.ChannelSelectFact, bool) {
	if l == nil || fact.Call == nil || l.bindings == nil || !channelruntime.IsSelectCall(fact.Call, l.bindings) {
		return semantics.ChannelSelectFact{}, false
	}
	target, ok := lowerChannelSelectResultTarget(fact.ResultTargets)
	if !ok {
		return semantics.ChannelSelectFact{}, false
	}
	table, ok := lowerChannelSelectCaseTable(fact.Call)
	if !ok {
		return semantics.ChannelSelectFact{}, false
	}
	cases, hasDefault, ok := l.lowerChannelSelectCases(table)
	if !ok {
		return semantics.ChannelSelectFact{}, false
	}
	return semantics.ChannelSelectFact{
		Call:         fact.Call,
		ResultTarget: target,
		Cases:        cases,
		HasDefault:   hasDefault,
	}, true
}

func lowerChannelSelectResultTarget(targets []semantics.CallResultTarget) (semantics.CallResultTarget, bool) {
	if len(targets) == 0 {
		return semantics.CallResultTarget{}, false
	}
	target := targets[0]
	switch target.Kind {
	case semantics.CallResultTargetLocalAssignment, semantics.CallResultTargetOrdinaryAssignment:
	default:
		return semantics.CallResultTarget{}, false
	}
	if !target.HasPath || target.Path.IsEmpty() {
		return semantics.CallResultTarget{}, false
	}
	return target, true
}

func lowerChannelSelectCaseTable(call *ast.FuncCallExpr) (*ast.TableExpr, bool) {
	if call == nil || len(call.Args) == 0 {
		return nil, false
	}
	table, ok := call.Args[0].(*ast.TableExpr)
	return table, ok
}

func (l *lowerer) lowerChannelSelectCases(table *ast.TableExpr) ([]semantics.ChannelSelectCaseFact, bool, bool) {
	if table == nil {
		return nil, false, false
	}
	hasDefault := false
	cases := make([]semantics.ChannelSelectCaseFact, 0, len(table.Fields))
	for _, field := range table.Fields {
		if field == nil {
			continue
		}
		if field.Key != nil && ast.KeyName(field.Key) == "default" {
			hasDefault = true
			continue
		}
		caseFact, ok := l.lowerChannelSelectCase(field.Value)
		if !ok {
			return nil, false, false
		}
		cases = append(cases, caseFact)
	}
	if len(cases) == 0 {
		return nil, false, false
	}
	return cases, hasDefault, true
}

func (l *lowerer) lowerChannelSelectCase(expr ast.Expr) (semantics.ChannelSelectCaseFact, bool) {
	call, ok := expr.(*ast.FuncCallExpr)
	if !ok || !channelruntime.IsReceiveCaseSyntax(call, l.bindings) {
		return semantics.ChannelSelectCaseFact{}, false
	}
	channelPath, ok := pathexpr.Resolve(call.Receiver, l.bindings)
	if !ok {
		return semantics.ChannelSelectCaseFact{}, false
	}
	return semantics.ChannelSelectCaseFact{
		CaseCall:       call,
		ChannelPath:    channelPath,
		HasChannelPath: true,
	}, true
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
	return typevalue.WithWitness(l.registry, typevalue.FromType(l.registry, payloadType), payloadType), true
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
	return channelruntime.ChannelPayloadType(channelType)
}
