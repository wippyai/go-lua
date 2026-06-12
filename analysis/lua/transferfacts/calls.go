package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func (l *lowerer) callProducer(fact semantics.CallFact) (factflow.CallProducer, bool) {
	switch fact.Context {
	case semantics.CallContextAssignmentSource, semantics.CallContextReturnSource:
	default:
		return factflow.CallProducer{}, false
	}
	calleeSymbol := symbol.ID(0)
	if fact.HasCalleeSymbol {
		calleeSymbol = fact.CalleeSymbol
	}
	calleePath := path.Path{}
	if fact.HasCalleePath {
		calleePath = fact.CalleePath
	}
	return factflow.NewCallProducer(factflow.CallProducerConfig{
		CalleeSymbol:  calleeSymbol,
		CalleePath:    calleePath,
		ResultTargets: l.strictProducerResultTargets(fact.ResultTargets),
	}), true
}

func (l *lowerer) callSite(fact semantics.CallFact) factflow.CallSite {
	exprRef, hasExpr := l.exprRef(fact.Call)
	calleeSymbol := symbol.ID(0)
	if fact.HasCalleeSymbol {
		calleeSymbol = fact.CalleeSymbol
	}
	calleePath := path.Path{}
	if fact.HasCalleePath {
		calleePath = fact.CalleePath
	}
	receiverPath := path.Path{}
	if fact.HasReceiverPath {
		receiverPath = fact.ReceiverPath
	}
	methodPath := path.Path{}
	if fact.HasMethodPath {
		methodPath = fact.MethodPath
	}
	return factflow.NewCallSite(factflow.CallSiteConfig{
		Context:         callSiteContext(fact.Context),
		CalleeSymbol:    calleeSymbol,
		CalleePath:      calleePath,
		ReceiverPath:    receiverPath,
		HasReceiverPath: fact.HasReceiverPath,
		MethodPath:      methodPath,
		HasMethodPath:   fact.HasMethodPath,
		MethodName:      fact.Method,
		ExprRef:         exprRef,
		HasExpr:         hasExpr,
		ExprIndex:       fact.ExprIndex,
		ArgumentSources: l.valueSources(fact.ArgumentSources),
		TypeArgs:        l.typeRefs(fact.TypeArgs),
		ResultTargets:   l.evidenceCallSiteResultTargets(fact.ResultTargets),
		Final:           fact.Final,
		Expanded:        fact.Expanded,
		Adjusted:        fact.Adjusted,
		OpenTail:        fact.OpenTail,
	})
}

func callSiteContext(kind semantics.CallContextKind) factflow.CallSiteContext {
	switch kind {
	case semantics.CallContextStatement:
		return factflow.CallSiteContextStatement
	case semantics.CallContextAssignmentSource:
		return factflow.CallSiteContextAssignmentSource
	case semantics.CallContextReturnSource:
		return factflow.CallSiteContextReturnSource
	case semantics.CallContextIteratorSource:
		return factflow.CallSiteContextIteratorSource
	case semantics.CallContextCondition:
		return factflow.CallSiteContextCondition
	default:
		return factflow.CallSiteContextUnknown
	}
}
