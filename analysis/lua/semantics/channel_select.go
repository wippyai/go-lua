package semantics

import (
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/compiler/ast"
)

func channelSelectFact(fact CallFact, bindings *bind.Result) (ChannelSelectFact, bool) {
	if fact.Call == nil || !isChannelSelectCall(fact) {
		return ChannelSelectFact{}, false
	}
	target, ok := channelSelectResultTarget(fact.ResultTargets)
	if !ok {
		return ChannelSelectFact{}, false
	}
	table, ok := channelSelectCaseTable(fact.Call)
	if !ok {
		return ChannelSelectFact{}, false
	}
	cases, ok := channelSelectCases(table, bindings)
	if !ok {
		return ChannelSelectFact{}, false
	}
	return ChannelSelectFact{
		Call:         fact.Call,
		ResultTarget: target,
		Cases:        cases,
	}, true
}

func isChannelSelectCall(fact CallFact) bool {
	if !fact.HasCalleePath {
		return false
	}
	return fact.CalleePath.FieldName() == "select"
}

func channelSelectResultTarget(targets []CallResultTarget) (CallResultTarget, bool) {
	if len(targets) == 0 {
		return CallResultTarget{}, false
	}
	target := targets[0]
	switch target.Kind {
	case CallResultTargetLocalAssignment, CallResultTargetOrdinaryAssignment:
	default:
		return CallResultTarget{}, false
	}
	if !target.HasPath || target.Path.IsEmpty() {
		return CallResultTarget{}, false
	}
	return copyResultTarget(target), true
}

func channelSelectCaseTable(call *ast.FuncCallExpr) (*ast.TableExpr, bool) {
	if call == nil || len(call.Args) == 0 {
		return nil, false
	}
	table, ok := call.Args[0].(*ast.TableExpr)
	return table, ok
}

func channelSelectCases(table *ast.TableExpr, bindings *bind.Result) ([]ChannelSelectCaseFact, bool) {
	if table == nil {
		return nil, false
	}
	cases := make([]ChannelSelectCaseFact, 0, len(table.Fields))
	for _, field := range table.Fields {
		if field == nil || isChannelSelectDefaultEntry(field) {
			continue
		}
		caseFact, ok := channelSelectCase(field.Value, bindings)
		if !ok {
			return nil, false
		}
		cases = append(cases, caseFact)
	}
	if len(cases) == 0 {
		return nil, false
	}
	return cases, true
}

func isChannelSelectDefaultEntry(field *ast.Field) bool {
	if field == nil || field.Key == nil {
		return false
	}
	key, ok := field.Key.(*ast.StringExpr)
	return ok && key.Value == "default"
}

func channelSelectCase(expr ast.Expr, bindings *bind.Result) (ChannelSelectCaseFact, bool) {
	call, ok := expr.(*ast.FuncCallExpr)
	if !ok || call.Receiver == nil || call.Method != "case_receive" || len(call.Args) != 0 || len(call.TypeArgs) != 0 {
		return ChannelSelectCaseFact{}, false
	}
	channelPath, ok := pathexpr.Resolve(call.Receiver, bindings)
	if !ok || channelPath.IsEmpty() {
		return ChannelSelectCaseFact{}, false
	}
	return ChannelSelectCaseFact{
		CaseCall:       call,
		ChannelPath:    copyPath(channelPath),
		HasChannelPath: true,
	}, true
}
