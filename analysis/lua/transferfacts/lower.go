// Package transferfacts lowers Lua semantic sidecars into generic transfer facts.
package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// Lower converts Lua semantic facts into the generic transfer fact DTOs consumed
// by the engine. It intentionally lowers only syntax facts already represented
// by transfer.Facts; higher semantic layers add branch, iterator, table,
// interproc, and diagnostic facts separately.
func Lower(result *semantics.Result, graph cfg.Graph) transfer.Facts {
	if result == nil || graph == nil {
		return transfer.NewFacts(transfer.FactsInput{})
	}
	l := lowerer{
		exprs: make(map[any]transfer.ExprRef),
	}
	input := transfer.FactsInput{
		LocalAssignments:    make(map[cfg.Point]transfer.LocalAssignment),
		OrdinaryAssignments: make(map[cfg.Point]transfer.OrdinaryAssignment),
		Returns:             make(map[cfg.Point]transfer.Return),
		Calls:               make(map[cfg.Point]transfer.CallProducer),
	}
	for _, point := range graph.RPO() {
		if fact, ok := result.LocalAssignment(point); ok {
			if lowered, ok := l.localAssignment(fact); ok {
				input.LocalAssignments[point] = lowered
			}
		}
		if fact, ok := result.OrdinaryAssignment(point); ok {
			if lowered, ok := l.ordinaryAssignment(fact); ok {
				input.OrdinaryAssignments[point] = lowered
			}
		}
		if fact, ok := result.Return(point); ok {
			input.Returns[point] = transfer.NewReturn(l.valueSources(fact.Sources))
		}
		if fact, ok := result.Call(point); ok {
			if lowered, ok := l.callProducer(fact); ok {
				input.Calls[point] = lowered
			}
		}
	}
	return transfer.NewFacts(input)
}

type lowerer struct {
	exprs map[any]transfer.ExprRef
}

func (l *lowerer) localAssignment(fact semantics.LocalAssignmentFact) (transfer.LocalAssignment, bool) {
	if !fact.HasSymbol || fact.Symbol == 0 {
		return transfer.LocalAssignment{}, false
	}
	target := path.NewPath(fact.Symbol, fact.Name)
	return transfer.NewLocalAssignment(fact.Symbol, target, l.valueSource(fact.Source)), true
}

func (l *lowerer) ordinaryAssignment(fact semantics.OrdinaryAssignmentFact) (transfer.OrdinaryAssignment, bool) {
	// Ordinary assignment facts expose a symbol only for root identifier writes.
	// Member and subtree writes stay out of the first transfer DTO slice.
	if !fact.HasSymbol || fact.Symbol == 0 {
		return transfer.OrdinaryAssignment{}, false
	}
	target := path.NewPath(fact.Symbol, "")
	return transfer.NewOrdinaryAssignment(fact.Symbol, target, l.valueSource(fact.Source)), true
}

func (l *lowerer) callProducer(fact semantics.CallFact) (transfer.CallProducer, bool) {
	context, ok := callProducerContext(fact.Context)
	if !ok {
		return transfer.CallProducer{}, false
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
	return transfer.NewCallProducer(transfer.CallProducerConfig{
		Context:       context,
		CalleeSymbol:  calleeSymbol,
		CalleePath:    calleePath,
		ExprRef:       exprRef,
		HasExpr:       hasExpr,
		ExprIndex:     fact.ExprIndex,
		ResultTargets: l.callResultTargets(fact.ResultTargets),
		Final:         fact.Final,
		Expanded:      fact.Expanded,
		Adjusted:      fact.Adjusted,
		OpenTail:      fact.OpenTail,
	}), true
}

func (l *lowerer) valueSources(sources []semantics.ValueSource) []transfer.ValueSource {
	if len(sources) == 0 {
		return nil
	}
	out := make([]transfer.ValueSource, len(sources))
	for i := range sources {
		out[i] = l.valueSource(sources[i])
	}
	return out
}

func (l *lowerer) valueSource(source semantics.ValueSource) transfer.ValueSource {
	exprRef, hasExpr := l.exprRef(source.Expr)
	return transfer.ValueSource{
		Kind:         valueSourceKind(source.Kind),
		ExprRef:      exprRef,
		HasExpr:      hasExpr,
		ExprIndex:    source.ExprIndex,
		TargetIndex:  source.TargetIndex,
		ResultIndex:  source.ResultIndex,
		CallPoint:    source.CallPoint,
		HasCallPoint: source.HasCallPoint,
		Final:        source.Final,
		Expanded:     source.Expanded,
		Adjusted:     source.Adjusted,
		OpenTail:     source.OpenTail,
	}
}

func (l *lowerer) callResultTargets(targets []semantics.CallResultTarget) []transfer.CallResultTarget {
	if len(targets) == 0 {
		return nil
	}
	out := make([]transfer.CallResultTarget, 0, len(targets))
	for _, target := range targets {
		if lowered, ok := l.callResultTarget(target); ok {
			out = append(out, lowered)
		}
	}
	return out
}

func (l *lowerer) callResultTarget(target semantics.CallResultTarget) (transfer.CallResultTarget, bool) {
	switch target.Kind {
	case semantics.CallResultTargetLocalAssignment:
		if !target.HasSymbol || target.Symbol == 0 {
			return transfer.CallResultTarget{}, false
		}
		targetPath := target.Path
		if !target.HasPath {
			targetPath = path.NewPath(target.Symbol, target.Name)
		}
		return transfer.NewCallResultTarget(transfer.CallResultTargetLocalAssignment, target.Index, target.Symbol, targetPath), true
	case semantics.CallResultTargetOrdinaryAssignment:
		if !target.HasSymbol || target.Symbol == 0 {
			return transfer.CallResultTarget{}, false
		}
		if target.HasPath && len(target.Path.Segments) != 0 {
			return transfer.CallResultTarget{}, false
		}
		targetPath := target.Path
		if !target.HasPath {
			targetPath = path.NewPath(target.Symbol, "")
		}
		return transfer.NewCallResultTarget(transfer.CallResultTargetOrdinaryAssignment, target.Index, target.Symbol, targetPath), true
	case semantics.CallResultTargetReturn:
		return transfer.NewCallResultTarget(transfer.CallResultTargetReturn, target.Index, 0, path.Path{}), true
	default:
		return transfer.CallResultTarget{}, false
	}
}

func (l *lowerer) exprRef(expr any) (transfer.ExprRef, bool) {
	if expr == nil {
		return 0, false
	}
	if ref, ok := l.exprs[expr]; ok {
		return ref, true
	}
	ref := transfer.ExprRef(len(l.exprs) + 1)
	l.exprs[expr] = ref
	return ref, true
}

func callProducerContext(kind semantics.CallContextKind) (transfer.CallProducerContext, bool) {
	switch kind {
	case semantics.CallContextAssignmentSource:
		return transfer.CallProducerContextAssignment, true
	case semantics.CallContextReturnSource:
		return transfer.CallProducerContextReturn, true
	default:
		return transfer.CallProducerContextUnknown, false
	}
}

func valueSourceKind(kind semantics.ValueSourceKind) transfer.ValueSourceKind {
	switch kind {
	case semantics.ValueSourceExpression:
		return transfer.ValueSourceExpression
	case semantics.ValueSourceCall:
		return transfer.ValueSourceCall
	case semantics.ValueSourceVararg:
		return transfer.ValueSourceVararg
	case semantics.ValueSourceNil:
		return transfer.ValueSourceNil
	default:
		return transfer.ValueSourceUnknown
	}
}
