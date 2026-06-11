package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func (l *lowerer) callProducer(fact semantics.CallFact) (factflow.CallProducer, bool) {
	context, ok := callProducerContext(fact.Context)
	if !ok {
		return factflow.CallProducer{}, false
	}
	exprRef, hasExpr := l.exprRef(fact.Call)
	calleeSymbol := symbol.ID(0)
	if fact.HasCalleeSymbol {
		calleeSymbol = fact.CalleeSymbol
	}
	calleePath := path.Path{}
	if fact.HasCalleePath {
		calleePath = fact.CalleePath
	}
	return factflow.NewCallProducer(factflow.CallProducerConfig{
		Context:       context,
		CalleeSymbol:  calleeSymbol,
		CalleePath:    calleePath,
		ExprRef:       exprRef,
		HasExpr:       hasExpr,
		ExprIndex:     fact.ExprIndex,
		ResultTargets: l.callProducerResultTargets(fact.ResultTargets),
		Final:         fact.Final,
		Expanded:      fact.Expanded,
		Adjusted:      fact.Adjusted,
		OpenTail:      fact.OpenTail,
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
		ArgumentSources: l.argumentValueSources(fact.Args),
		TypeArgs:        l.typeRefs(fact.TypeArgs),
		ResultTargets:   l.callSiteResultTargets(fact.ResultTargets),
		Final:           fact.Final,
		Expanded:        fact.Expanded,
		Adjusted:        fact.Adjusted,
		OpenTail:        fact.OpenTail,
	})
}

func callProducerContext(kind semantics.CallContextKind) (factflow.CallProducerContext, bool) {
	switch kind {
	case semantics.CallContextAssignmentSource:
		return factflow.CallProducerContextAssignment, true
	case semantics.CallContextReturnSource:
		return factflow.CallProducerContextReturn, true
	default:
		return factflow.CallProducerContextUnknown, false
	}
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
